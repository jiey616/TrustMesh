package handler

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/transport"
)

// Upload limits.
const (
	maxUploadSize     = 100 << 20 // 100MB
	maxFilesPerProject = 1000
)

// allowedFileExtensions defines the whitelist of allowed file extensions.
var allowedFileExtensions = map[string]bool{
	// Documents
	".pdf": true, ".doc": true, ".docx": true, ".txt": true, ".md": true,
	".rtf": true, ".odt": true,
	// Spreadsheets
	".xls": true, ".xlsx": true, ".csv": true,
	// Presentations
	".ppt": true, ".pptx": true,
	// Data
	".json": true, ".xml": true, ".yaml": true, ".yml": true,
	// Images
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".svg": true, ".webp": true, ".bmp": true, ".ico": true,
	// Archives
	".zip": true, ".tar": true, ".gz": true, ".7z": true, ".rar": true,
	// Code
	".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true,
	".tsx": true, ".java": true, ".c": true, ".cpp": true, ".h": true,
	".rs": true, ".rb": true, ".php": true, ".sh": true, ".sql": true,
	".html": true, ".css": true, ".vue": true,
	// Config
	".env": true, ".ini": true, ".conf": true, ".toml": true,
	// Audio/Video
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true,
}

// applyUploadLimit wraps the request body with a MaxBytesReader to prevent
// oversized uploads. Must be called before c.Request.FormFile().
func applyUploadLimit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize)
}

// validateFileExtension checks if the given filename has an allowed extension.
func validateFileExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return true // allow files without extension (e.g., Makefile)
	}
	return allowedFileExtensions[ext]
}

// checkUploadLimits applies size limit and validates file extension.
// Returns true if the upload is allowed, false if it was rejected (error already written).
func checkUploadLimits(c *gin.Context, filename string) bool {
	// Apply size limit
	applyUploadLimit(c)

	// Validate file extension
	if !validateFileExtension(filename) {
		transport.WriteError(c, transport.BadRequest("UNSUPPORTED_FILE_TYPE",
			"不支持的文件类型。允许的格式：文档、图片、压缩包、代码文件等。"))
		return false
	}

	return true
}
