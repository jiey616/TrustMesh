package handler

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

type TransferHandler struct {
	store *store.Store
}

func NewTransferHandler(s *store.Store) *TransferHandler {
	return &TransferHandler{store: s}
}

func (h *TransferHandler) GetTaskArtifactContent(c *gin.Context) {
	sc, ok := currentScope(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	artifactID := c.Param("artifactId")

	// Verify user owns the task.
	if _, appErr := h.store.GetTask(sc, taskID); appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	artifact, appErr := h.store.GetArtifact(taskID, artifactID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if artifact.LocalPath == "" {
		transport.WriteError(c, transport.NotFound("artifact file path unavailable"))
		return
	}

	file, err := os.Open(artifact.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			transport.WriteError(c, transport.NotFound("artifact file not found"))
			return
		}
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "ARTIFACT_READ_FAILED", "failed to read artifact file"))
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "ARTIFACT_READ_FAILED", "failed to stat artifact file"))
		return
	}

	mimeType := artifact.MimeType
	if mimeType == "" {
		mimeType = detectContentType(file)
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			transport.WriteError(c, transport.NewError(http.StatusBadGateway, "ARTIFACT_READ_FAILED", "failed to rewind artifact file"))
			return
		}
	}
	mimeType = normalizeTextContentType(mimeType)
	if mimeType != "" {
		c.Header("Content-Type", mimeType)
	}

	fileName := artifact.FileName
	if fileName == "" {
		fileName = filepath.Base(artifact.LocalPath)
	}
	c.Header("Content-Disposition", contentDisposition("inline", fileName))
	http.ServeContent(c.Writer, c.Request, fileName, fileModTime(info), file)
}

func detectContentType(file *os.File) string {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return ""
	}
	if n == 0 {
		return ""
	}
	return http.DetectContentType(buf[:n])
}

func normalizeTextContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ""
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		if strings.HasPrefix(strings.ToLower(contentType), "text/") && !strings.Contains(strings.ToLower(contentType), "charset=") {
			return contentType + "; charset=utf-8"
		}
		return contentType
	}
	if !strings.HasPrefix(strings.ToLower(mediaType), "text/") {
		return contentType
	}
	if _, ok := params["charset"]; ok {
		return contentType
	}
	params["charset"] = "utf-8"
	return mime.FormatMediaType(mediaType, params)
}

func fileModTime(info os.FileInfo) time.Time {
	if info == nil {
		return time.Time{}
	}
	return info.ModTime()
}

func contentDisposition(mode, fileName string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "inline"
	}
	if fileName == "" {
		return mode
	}
	return mode + `; filename="` + strings.ReplaceAll(fileName, `"`, "") + `"`
}
