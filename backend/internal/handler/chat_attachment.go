package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/auth"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/transport"
)

// chatStorageBucket is the storage scope passed to FileStorage.Save. Files land
// under {basePath}/chat/uploads/{fileID}_{safeName}, so the on-disk location can
// be reconstructed deterministically from {fileID}_<name> without persisting it.
const chatStorageBucket = "chat"

// downloadTokenType matches the token type used by agentfile so chat attachment
// download links are signed by the same HS256 JWT manager.
const downloadTokenType = "agent_download"

// ChatAttachmentHandler serves attachment upload/download for chat messages
// (数字员工对话 / 任务对话). Uploads are stored as raw bytes via FileStorage and
// referenced on messages by ID; no separate DB collection is introduced.
type ChatAttachmentHandler struct {
	storage     project.FileStorage
	basePath    string
	jwtMgr      *auth.JWTManager
	jwtSecret   []byte
	downloadTTL time.Duration
	externalURL string
	log         *zap.Logger
}

// NewChatAttachmentHandler creates a new chat attachment handler.
func NewChatAttachmentHandler(storage project.FileStorage, basePath string, jwtMgr *auth.JWTManager, jwtSecret []byte, downloadTTL time.Duration, externalURL string, log *zap.Logger) *ChatAttachmentHandler {
	return &ChatAttachmentHandler{
		storage:     storage,
		basePath:    basePath,
		jwtMgr:      jwtMgr,
		jwtSecret:   jwtSecret,
		downloadTTL: downloadTTL,
		externalURL: externalURL,
		log:         log,
	}
}

// Upload handles POST /api/v1/chats/attachments (multipart field "file").
// Returns the ChatAttachment metadata (ID/name/size/mime) the client echoes back
// when sending a message with attachments.
func (h *ChatAttachmentHandler) Upload(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}
	_ = sc

	applyUploadLimit(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file is required"))
		return
	}
	defer file.Close()

	if !validateFileExtension(header.Filename) {
		transport.WriteError(c, transport.BadRequest("UNSUPPORTED_FILE_TYPE",
			"不支持的文件类型。允许的格式：文档、图片、压缩包、代码文件等。"))
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	// Sanitize: only the base name is persisted, so a browser/desktop-supplied
	// path can never escape the bucket.
	safeName := filepath.Base(header.Filename)
	id := uuid.NewString()

	if _, err := h.storage.Save(c.Request.Context(), chatStorageBucket, id, safeName, file); err != nil {
		if h.log != nil {
			h.log.Warn("failed to save chat attachment", zap.String("id", id), zap.Error(err))
		}
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "UPLOAD_FAILED", "failed to store file"))
		return
	}

	att := model.ChatAttachment{
		ID:       id,
		FileName: safeName,
		FileSize: header.Size,
		MimeType: mimeType,
	}
	transport.WriteData(c, http.StatusCreated, att)
}

// Download handles GET /api/v1/chats/attachments/:fileId/token/:token
// (and legacy ?token= form). Auth is a short-lived HS256 download token.
func (h *ChatAttachmentHandler) Download(c *gin.Context) {
	fileID := c.Param("fileId")
	if fileID == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file id is required"))
		return
	}

	token := c.Param("token")
	if token == "" {
		token = c.Query("token")
	}
	if token == "" {
		transport.WriteError(c, transport.Unauthorized("missing download token"))
		return
	}

	claims, err := h.jwtMgr.ParseToken(token)
	if err != nil {
		transport.WriteError(c, transport.Unauthorized("invalid or expired download token"))
		return
	}
	if claims.TokenType != downloadTokenType {
		transport.WriteError(c, transport.Unauthorized("invalid token type"))
		return
	}
	if claims.UserID != fileID {
		transport.WriteError(c, transport.Forbidden("token does not match requested file"))
		return
	}

	// Locate the on-disk file deterministically: {basePath}/chat/uploads/{fileID}_<name>.
	pattern := filepath.Join(h.basePath, chatStorageBucket, "uploads", fileID+"_*")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		transport.WriteError(c, transport.NotFound("file not found"))
		return
	}

	localPath := matches[0]
	file, err := os.Open(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			transport.WriteError(c, transport.NotFound("file not found on disk"))
			return
		}
		if h.log != nil {
			h.log.Warn("chat attachment open failed", zap.String("file_id", fileID), zap.Error(err))
		}
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to read file"))
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to stat file"))
		return
	}

	fileName := filepath.Base(localPath)
	if idx := len(fileID) + 1; idx < len(fileName) {
		fileName = fileName[idx:]
	}
	if fileName == "" {
		fileName = filepath.Base(localPath)
	}

	c.Header("Content-Disposition", contentDisposition("inline", fileName))
	http.ServeContent(c.Writer, c.Request, fileName, info.ModTime(), file)
}

// buildChatDownloadURL builds a signed download URL for a chat attachment.
// The path must match the handler's own route (/api/v1/chats/attachments/...),
// NOT the agent-file route used by project files.
func buildChatDownloadURL(externalURL, fileID, token string) string {
	return fmt.Sprintf("%s/api/v1/chats/attachments/%s/token/%s", strings.TrimRight(externalURL, "/"), fileID, token)
}

// buildDownloadURL builds a signed download URL for a chat attachment.
func (h *ChatAttachmentHandler) buildDownloadURL(fileID string) (string, error) {
	token, err := agentfile.GenerateDownloadToken(h.jwtSecret, fileID, h.downloadTTL)
	if err != nil {
		return "", err
	}
	return buildChatDownloadURL(h.externalURL, fileID, token), nil
}

// enrichChatAttachmentURLs fills the URL field of a slice of chat attachments
// using fresh signed download tokens. Shared by both chat send handlers so
// delivered/returned messages carry reachable download links.
func enrichChatAttachmentURLs(atts []model.ChatAttachment, externalURL string, jwtSecret []byte, ttl time.Duration) {
	for i := range atts {
		if atts[i].URL != "" {
			continue
		}
		token, err := agentfile.GenerateDownloadToken(jwtSecret, atts[i].ID, ttl)
		if err != nil {
			continue
		}
		atts[i].URL = buildChatDownloadURL(externalURL, atts[i].ID, token)
	}
}

// EnrichAgentChatDetailURLs signs attachment URLs on every user message of an
// agent chat detail (must be an owned copy, not a store-internal live object).
func EnrichAgentChatDetailURLs(detail *model.AgentChatDetail, externalURL string, jwtSecret []byte, ttl time.Duration) {
	if detail == nil {
		return
	}
	for i := range detail.Messages {
		if len(detail.Messages[i].Attachments) == 0 {
			continue
		}
		enrichChatAttachmentURLs(detail.Messages[i].Attachments, externalURL, jwtSecret, ttl)
	}
}


// EnrichAttachmentURLs fills the URL field of any attachment on the given
// messages' attachments using fresh signed tokens. The input messages must be
// owned copies (not store-internal live objects).
func (h *ChatAttachmentHandler) EnrichAttachmentURLs(attachments []model.ChatAttachment) []model.ChatAttachment {
	for i := range attachments {
		if attachments[i].URL != "" {
			continue
		}
		url, err := h.buildDownloadURL(attachments[i].ID)
		if err != nil {
			if h.log != nil {
				h.log.Warn("failed to sign chat attachment url", zap.String("id", attachments[i].ID), zap.Error(err))
			}
			continue
		}
		attachments[i].URL = url
	}
	return attachments
}
