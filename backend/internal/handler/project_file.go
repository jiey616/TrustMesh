package handler

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/project"
	"trustmesh/backend/internal/store"
	"trustmesh/backend/internal/transport"
)

// ProjectFileHandler handles project file management endpoints.
type ProjectFileHandler struct {
	store   *store.Store
	storage project.FileStorage
	log     *zap.Logger
}

// NewProjectFileHandler creates a new ProjectFileHandler.
func NewProjectFileHandler(s *store.Store, storage project.FileStorage, log *zap.Logger) *ProjectFileHandler {
	return &ProjectFileHandler{
		store:   s,
		storage: storage,
		log:     log,
	}
}

// Upload handles POST /api/v1/projects/:projectId/files
func (h *ProjectFileHandler) Upload(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")

	// Apply upload limits: max file size 100MB + file type whitelist
	applyUploadLimit(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "file is required"))
		return
	}
	defer file.Close()

	// Validate file extension against whitelist
	if !validateFileExtension(header.Filename) {
		transport.WriteError(c, transport.BadRequest("UNSUPPORTED_FILE_TYPE",
			"不支持的文件类型。允许的格式：文档、图片、压缩包、代码文件等。"))
		return
	}

	taskID := strings.TrimSpace(c.PostForm("task_id"))
	parentID := strings.TrimSpace(c.PostForm("parent_id"))

	pf := &model.ProjectFile{
		FileName: header.Filename,
		FileSize: header.Size,
		MimeType: header.Header.Get("Content-Type"),
		Source:   "user_upload",
		TaskID:   taskID,
		ParentID: parentID,
	}
	if pf.MimeType == "" {
		pf.MimeType = "application/octet-stream"
	}

	// Create record in store (validates project ownership & optional task).
	pf, appErr := h.store.SaveProjectFile(userID, projectID, pf)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Save file to disk. We need to re-read the file since it's a stream.
	// Reset the file reader by re-parsing.
	file2, _, err := c.Request.FormFile("file")
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "UPLOAD_FAILED", "failed to read file for storage"))
		return
	}
	defer file2.Close()

	path, err := h.storage.Save(c.Request.Context(), projectID, pf.ID, header.Filename, file2)
	if err != nil {
		if h.log != nil {
			h.log.Warn("failed to save project file", zap.String("id", pf.ID), zap.Error(err))
		}
		transport.WriteError(c, transport.NewError(http.StatusInternalServerError, "UPLOAD_FAILED", "failed to store file"))
		return
	}

	// Update LocalPath.
	_ = h.store.SetProjectFileLocalPath(pf.ID, path)

	// Re-read to get updated record.
	pf, appErr = h.store.GetProjectFile(userID, pf.ID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusCreated, pf)
}

// CreateFolder handles POST /api/v1/projects/:projectId/folders
func (h *ProjectFileHandler) CreateFolder(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")

	var req model.CreateProjectFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.Validation("invalid request", map[string]any{"detail": err.Error()}))
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "folder name is required"))
		return
	}

	req.ParentID = strings.TrimSpace(req.ParentID)

	folder, appErr := h.store.CreateFolder(userID, projectID, req.Name, req.ParentID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusCreated, folder)
}

// List handles GET /api/v1/projects/:projectId/files
func (h *ProjectFileHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")
	source := c.Query("source")
	taskID := c.Query("task_id")
	agentID := c.Query("agent_id")

	files := h.store.ListProjectFiles(userID, projectID, source, taskID, agentID)
	if files == nil {
		files = []model.ProjectFile{}
	}

	// If ListProjectFiles returned empty but we have no access, check explicitly.
	if len(files) == 0 && source == "" && taskID == "" && agentID == "" {
		// Verify the project exists and user has access.
		if appErr := h.store.ValidateProjectOwnership(userID, projectID); appErr != nil {
			transport.WriteError(c, appErr)
			return
		}
	}

	transport.WriteList(c, files, len(files))
}

// GetTree handles GET /api/v1/projects/:projectId/files/tree
func (h *ProjectFileHandler) GetTree(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")

	tree := h.store.GetProjectFileTree(userID, projectID)
	transport.WriteData(c, http.StatusOK, tree)
}

// GetContent handles GET /api/v1/projects/:projectId/files/:fileId/content
func (h *ProjectFileHandler) GetContent(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	fileID := c.Param("fileId")

	pf, appErr := h.store.GetProjectFile(userID, fileID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	if pf.LocalPath == "" {
		transport.WriteError(c, transport.NotFound("file path unavailable"))
		return
	}

	file, err := os.Open(pf.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Lazy recovery: meeting minutes content is persisted on the meeting
			// record, so a missing on-disk file can be rebuilt on demand. This
			// keeps minutes downloadable/previewable even after a storage reset.
			if h.tryRecoverMeetingMinutesFile(userID, pf) {
				file, err = os.Open(pf.LocalPath)
			}
			if err != nil {
				transport.WriteError(c, transport.NotFound("file not found"))
				return
			}
		} else {
			transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to read file"))
			return
		}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to stat file"))
		return
	}

	mimeType := pf.MimeType
	if mimeType == "" {
		mimeType = detectContentType(file)
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			transport.WriteError(c, transport.NewError(http.StatusBadGateway, "FILE_READ_FAILED", "failed to rewind file"))
			return
		}
	}
	mimeType = normalizeTextContentType(mimeType)
	if mimeType != "" {
		c.Header("Content-Type", mimeType)
	}

	fileName := pf.FileName
	if fileName == "" {
		fileName = filepath.Base(pf.LocalPath)
	}
	c.Header("Content-Disposition", contentDisposition("inline", fileName))
	http.ServeContent(c.Writer, c.Request, fileName, info.ModTime(), file)
}

// tryRecoverMeetingMinutesFile rebuilds a missing meeting-minutes file from the
// persisted meeting.Minutes content. It returns true when the file was
// successfully re-created and pf.LocalPath updated in memory. Used as a lazy
// recovery path inside GetContent so minutes remain downloadable even if the
// underlying storage (the project-files volume) was reset and lost the bytes.
func (h *ProjectFileHandler) tryRecoverMeetingMinutesFile(userID string, pf *model.ProjectFile) bool {
	if pf.Source != "meeting_minutes" || pf.MeetingID == "" || h.storage == nil {
		return false
	}
	meeting, appErr := h.store.GetMeeting("", pf.MeetingID)
	if appErr != nil || meeting == nil || strings.TrimSpace(meeting.Minutes) == "" {
		return false
	}
	localPath, err := h.storage.Save(context.Background(), pf.ProjectID, pf.ID, pf.FileName, strings.NewReader(meeting.Minutes))
	if err != nil {
		if h.log != nil {
			h.log.Warn("failed to recover missing meeting minutes file", zap.String("file_id", pf.ID), zap.Error(err))
		}
		return false
	}
	if appErr := h.store.SetProjectFileLocalPath(pf.ID, localPath); appErr != nil {
		if h.log != nil {
			h.log.Warn("failed to persist recovered minutes local path", zap.String("file_id", pf.ID), zap.Error(appErr))
		}
	}
	if h.log != nil {
		h.log.Info("recovered missing meeting minutes file from meeting record",
			zap.String("file_id", pf.ID), zap.String("meeting_id", pf.MeetingID))
	}
	return true
}

// Delete handles DELETE /api/v1/projects/:projectId/files/:fileId
func (h *ProjectFileHandler) Delete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	fileID := c.Param("fileId")

	// Get file record first to obtain the LocalPath.
	pf, appErr := h.store.GetProjectFile(userID, fileID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	// Delete from storage.
	if pf.LocalPath != "" {
		if err := h.storage.Delete(c.Request.Context(), pf.LocalPath); err != nil {
			if h.log != nil {
				h.log.Warn("failed to delete project file from storage", zap.String("id", fileID), zap.Error(err))
			}
		}
	}

	// Delete the store record.
	deleted, appErr := h.store.DeleteProjectFile(userID, fileID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, deleted)
}

// Rename handles PATCH /api/v1/projects/:projectId/files/:fileId/rename
func (h *ProjectFileHandler) Rename(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")
	fileID := c.Param("fileId")

	var req model.RenameProjectFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.Validation("invalid request", map[string]any{"detail": err.Error()}))
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		transport.WriteError(c, transport.BadRequest("BAD_REQUEST", "name is required"))
		return
	}

	pf, appErr := h.store.RenameProjectFile(userID, projectID, fileID, req.Name)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, pf)
}

// Move handles PATCH /api/v1/projects/:projectId/files/:fileId/move
func (h *ProjectFileHandler) Move(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")
	fileID := c.Param("fileId")

	var req model.MoveProjectFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.Validation("invalid request", map[string]any{"detail": err.Error()}))
		return
	}

	pf, appErr := h.store.MoveProjectFile(userID, projectID, fileID, req.ParentID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, pf)
}

// BatchDelete handles POST /api/v1/projects/:projectId/files/batch-delete
func (h *ProjectFileHandler) BatchDelete(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")

	var req model.BatchDeleteProjectFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.WriteError(c, transport.Validation("invalid request", map[string]any{"detail": err.Error()}))
		return
	}

	result := h.store.BatchDeleteProjectFiles(userID, projectID, req.IDs)
	transport.WriteData(c, http.StatusOK, result)
}

// Browse handles GET /api/v1/projects/:projectId/files/browse
func (h *ProjectFileHandler) Browse(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")
	parentID := c.Query("parent_id")

	result, appErr := h.store.BrowseProjectFiles(userID, projectID, parentID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, result)
}

// ListArtifacts handles GET /api/v1/projects/:projectId/files/artifacts
func (h *ProjectFileHandler) ListArtifacts(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	projectID := c.Param("projectId")

	groups, appErr := h.store.ListArtifactGroups(userID, projectID)
	if appErr != nil {
		transport.WriteError(c, appErr)
		return
	}

	transport.WriteData(c, http.StatusOK, groups)
}
