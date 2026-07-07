package agentfile

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"trustmesh/backend/internal/auth"
	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

const (
	tokenTypeAgentDownload = "agent_download"
	defaultDownloadTTL     = 10 * time.Minute
)

// Handler serves agent file download requests authenticated by short-lived tokens.
type Handler struct {
	store   *store.Store
	storage project.FileStorage
	jwtMgr  *auth.JWTManager
	log     *zap.Logger
}

// NewHandler creates a new agent file handler.
func NewHandler(s *store.Store, storage project.FileStorage, jwtMgr *auth.JWTManager, log *zap.Logger) *Handler {
	return &Handler{
		store:   s,
		storage: storage,
		jwtMgr:  jwtMgr,
		log:     log,
	}
}

// GenerateDownloadToken creates a short-lived JWT that authorizes downloading
// the specific file. The token embeds the file ID and expires after ttl.
func GenerateDownloadToken(jwtSecret []byte, fileID string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = defaultDownloadTTL
	}
	now := time.Now().UTC()
	claims := auth.Claims{
		UserID:    fileID,
		TokenType: tokenTypeAgentDownload,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// DebugGenToken handles GET /api/v1/debug/gen-token/:fileId - temporary debug endpoint.
func (h *Handler) DebugGenToken(c *gin.Context) {
	fileID := c.Param("fileId")
	if fileID == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file id is required"))
		return
	}

	// Use the same config as the webhook handler would
	externalURL := os.Getenv("TRUSTMESH_EXTERNAL_URL")
	jwtSecret := []byte(os.Getenv("JWT_SECRET"))
	ttlStr := os.Getenv("DOWNLOAD_TOKEN_TTL")
	ttl, _ := time.ParseDuration(ttlStr)
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	token, err := GenerateDownloadToken(jwtSecret, fileID, ttl)
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "TOKEN_FAIL", err.Error()))
		return
	}

	// Self-verify
	claims, verifyErr := h.jwtMgr.ParseToken(token)

	downloadURL := BuildDownloadURL(externalURL, fileID, token)

	c.JSON(http.StatusOK, gin.H{
		"file_id":        fileID,
		"token":          token,
		"download_url":   downloadURL,
		"external_url":   externalURL,
		"secret_len":     len(jwtSecret),
		"ttl":            ttl.String(),
		"token_len":      len(token),
		"self_verify":    verifyErr == nil,
		"self_verify_err": func() string { if verifyErr != nil { return verifyErr.Error() } ; return "" }(),
		"claims_user_id": func() string { if claims != nil { return claims.UserID } ; return "" }(),
	})
}

// BuildDownloadURL constructs a full download URL from the external base URL,
// file ID, and token. Token is placed in the URL path (not query parameters)
// to prevent it from being hidden/filtered by middleware or message transports.
func BuildDownloadURL(externalURL, fileID, token string) string {
	return fmt.Sprintf("%s/api/v1/files/agent/%s/token/%s", externalURL, fileID, token)
}

// EnrichWithDownloadURLs converts model-level attached files to protocol refs
// and injects download URLs using fresh tokens.
func EnrichWithDownloadURLs(files []model.TaskAttachedFile, externalURL string, jwtSecret []byte, ttl time.Duration) []protocol.TaskAttachedFileRef {
	if len(files) == 0 {
		return nil
	}
	if ttl <= 0 {
		ttl = defaultDownloadTTL
	}
	out := make([]protocol.TaskAttachedFileRef, len(files))
	for i, f := range files {
		token, err := GenerateDownloadToken(jwtSecret, f.ID, ttl)
		if err != nil {
			// Non-fatal: agent gets metadata without a valid download URL.
			fmt.Fprintf(os.Stderr, "[DEBUG] GenerateDownloadToken error for file %s: %v\n", f.ID, err)
			token = ""
		}
		ref := protocol.TaskAttachedFileRef{
			ID:       f.ID,
			FileName: f.FileName,
			FileSize: f.FileSize,
			MimeType: f.MimeType,
			Source:   f.Source,
		}
		if token != "" {
			ref.DownloadUrl = BuildDownloadURL(externalURL, f.ID, token)
			fmt.Fprintf(os.Stderr, "[DEBUG] EnrichWithDownloadURLs: fileID=%s externalURL=%s jwtSecretLen=%d ttl=%s downloadUrl=%s\n", f.ID, externalURL, len(jwtSecret), ttl.String(), ref.DownloadUrl)
		}
		out[i] = ref
	}
	return out
}

// Download handles GET /api/v1/files/agent/:fileId/token/:token (new format)
// and GET /api/v1/files/agent/:fileId?token=xxx (old format, kept for compatibility).
// It validates the download token and serves the file content without JWT auth.
func (h *Handler) Download(c *gin.Context) {
	fileID := c.Param("fileId")
	if fileID == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file id is required"))
		return
	}

	// Try path parameter first (new format: /token/:token).
	// Fall back to query parameter (old format: ?token=xxx).
	token := c.Param("token")
	if token == "" {
		token = c.Query("token")
	}
	if token == "" {
		transport.WriteError(c, transport.Unauthorized("missing download token"))
		return
	}

	// Parse and validate the token.
	fmt.Fprintf(os.Stderr, "[DEBUG] Download: fileID=%s tokenLen=%d jwtMgr_secretLen=%d\n", fileID, len(token), len(h.jwtMgr.GetSecret()))
	claims, err := h.jwtMgr.ParseToken(token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] Download: ParseToken FAILED for fileID=%s: %v\n", fileID, err)
		transport.WriteError(c, transport.Unauthorized("invalid or expired download token"))
		return
	}
	if claims.TokenType != tokenTypeAgentDownload {
		transport.WriteError(c, transport.Unauthorized("invalid token type"))
		return
	}
	if claims.UserID != fileID {
		transport.WriteError(c, transport.Forbidden("token does not match requested file"))
		return
	}

	// Look up file metadata without user ownership check (agent doesn't have a user context).
	pf, appErr := h.store.GetProjectFileByID(fileID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}
	if pf.IsFolder {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "cannot download a folder"))
		return
	}
	if pf.LocalPath == "" {
		transport.WriteError(c, transport.NotFound("file path unavailable"))
		return
	}

	file, err := os.Open(pf.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			transport.WriteError(c, transport.NotFound("file not found on disk"))
			return
		}
		h.log.Warn("agent file open failed", zap.String("file_id", fileID), zap.String("path", pf.LocalPath), zap.Error(err))
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to read file"))
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to stat file"))
		return
	}

	mimeType := pf.MimeType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	c.Header("Content-Type", mimeType)

	fileName := pf.FileName
	if fileName == "" {
		fileName = filepath.Base(pf.LocalPath)
	}
	c.Header("Content-Disposition", contentDisposition("inline", fileName))
	http.ServeContent(c.Writer, c.Request, fileName, info.ModTime(), file)
}

// NewHandlerFromConfig creates a Handler using config values.
func NewHandlerFromConfig(s *store.Store, storage project.FileStorage, cfg config.Config, log *zap.Logger) *Handler {
	jwtMgr := auth.NewJWTManager(cfg.JWTSecret, cfg.DownloadTokenTTL, cfg.DownloadTokenTTL)
	return NewHandler(s, storage, jwtMgr, log)
}

func contentDisposition(disposition, filename string) string {
	return fmt.Sprintf(`%s; filename="%s"`, disposition, filename)
}
