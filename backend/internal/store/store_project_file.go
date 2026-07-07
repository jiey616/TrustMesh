package store

import (
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// SaveProjectFile creates a new ProjectFile record for a user upload.
// The caller must supply a full ProjectFile with at least ProjectID, FileName,
// FileSize, MimeType, and Source set. ID and timestamps are generated here.
// ParentID is supported for uploading into a user-created folder.
func (s *Store) SaveProjectFile(userID, projectID string, pf *model.ProjectFile) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate project ownership.
	project, ok := s.projects[projectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if project.UserID != userID {
		return nil, transport.Forbidden("access denied to project")
	}

	// If task_id is provided, validate it belongs to this project.
	if pf.TaskID != "" {
		task, taskOk := s.tasks[pf.TaskID]
		if !taskOk || task.ProjectID != projectID {
			return nil, transport.Validation("task not found in project", map[string]any{"task_id": pf.TaskID})
		}
	}

	// If parent_id is provided, validate it's a folder in this project.
	if pf.ParentID != "" {
		parent, parentOk := s.projectFiles[pf.ParentID]
		if !parentOk || parent.ProjectID != projectID || !parent.IsFolder {
			return nil, transport.Validation("parent folder not found", map[string]any{"parent_id": pf.ParentID})
		}
	}

	pf.ID = "pf_" + newID()
	pf.ProjectID = projectID
	pf.UploadedBy = userID
	pf.CreatedAt = time.Now().UTC()

	s.projectFiles[pf.ID] = pf
	s.projectFileIndex[projectID] = append(s.projectFileIndex[projectID], pf.ID)
	if pf.TransferID != "" {
		s.transferFileIndex[pf.TransferID] = pf.ID
	}

	if err := s.persistProjectFileUnsafe(pf); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist project file", zap.String("id", pf.ID), zap.Error(err))
		}
	}

	return pf, nil
}

// CreateFolder creates a new folder record in the project file space.
func (s *Store) CreateFolder(userID, projectID, name, parentID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[projectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if project.UserID != userID {
		return nil, transport.Forbidden("access denied to project")
	}

	// If parent_id is provided, validate the parent folder.
	if parentID != "" {
		parent, parentOk := s.projectFiles[parentID]
		if !parentOk || parent.ProjectID != projectID || !parent.IsFolder {
			return nil, transport.Validation("parent folder not found", map[string]any{"parent_id": parentID})
		}
	}

	// Check for duplicate folder name at the same level.
	for _, fid := range s.projectFileIndex[projectID] {
		if existing, ok := s.projectFiles[fid]; ok {
			if existing.IsFolder && existing.ParentID == parentID && existing.FileName == name {
				return nil, transport.Conflict("FOLDER_EXISTS", "a folder with this name already exists")
			}
		}
	}

	folder := &model.ProjectFile{
		ID:         "pf_" + newID(),
		ProjectID:  projectID,
		ParentID:   parentID,
		FileName:   name,
		FileSize:   0,
		MimeType:   "",
		Source:     "user_upload",
		IsFolder:   true,
		UploadedBy: userID,
		CreatedAt:  time.Now().UTC(),
	}

	s.projectFiles[folder.ID] = folder
	s.projectFileIndex[projectID] = append(s.projectFileIndex[projectID], folder.ID)

	if err := s.persistProjectFileUnsafe(folder); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist project folder", zap.String("id", folder.ID), zap.Error(err))
		}
	}

	return folder, nil
}

// RenameProjectFile updates the name of a file or folder.
func (s *Store) RenameProjectFile(userID, projectID, fileID, name string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}
	if pf.ProjectID != projectID {
		return nil, transport.NotFound("file not found in project")
	}

	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}

	// Check duplicate name at the same level (same parent).
	for _, fid := range s.projectFileIndex[projectID] {
		if fid == fileID {
			continue // skip self
		}
		if existing, exists := s.projectFiles[fid]; exists {
			if existing.ParentID == pf.ParentID && existing.FileName == name {
				return nil, transport.Conflict("NAME_EXISTS", "a file or folder with this name already exists")
			}
		}
	}

	pf.FileName = name
	if err := s.persistProjectFileUnsafe(pf); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist renamed project file", zap.String("id", fileID), zap.Error(err))
		}
	}
	return pf, nil
}

// MoveProjectFile moves a file or folder to a different parent folder.
func (s *Store) MoveProjectFile(userID, projectID, fileID, targetParentID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}
	if pf.ProjectID != projectID {
		return nil, transport.NotFound("file not found in project")
	}

	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}

	// If target is not root, validate it's a folder in the same project.
	if targetParentID != "" {
		target, targetOk := s.projectFiles[targetParentID]
		if !targetOk || target.ProjectID != projectID || !target.IsFolder {
			return nil, transport.Validation("target parent is not a folder", map[string]any{"parent_id": targetParentID})
		}
		// Prevent moving into self or a descendant (cycle detection).
		if pf.IsFolder && (targetParentID == fileID || s.isDescendantUnsafe(targetParentID, fileID)) {
			return nil, transport.Validation("cannot move a folder into itself or its children", nil)
		}
	}

	// No-op: already at the target location.
	if pf.ParentID == targetParentID {
		return pf, nil
	}

	// Check duplicate name at target location.
	for _, fid := range s.projectFileIndex[projectID] {
		if fid == fileID {
			continue
		}
		if existing, exists := s.projectFiles[fid]; exists {
			if existing.ParentID == targetParentID && existing.FileName == pf.FileName {
				return nil, transport.Conflict("NAME_EXISTS", "a file or folder with this name already exists in the target folder")
			}
		}
	}

	pf.ParentID = targetParentID
	if err := s.persistProjectFileUnsafe(pf); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist moved project file", zap.String("id", fileID), zap.Error(err))
		}
	}
	return pf, nil
}

// isDescendantUnsafe checks whether candidateID is a descendant of ancestorID.
// Must be called with s.mu held.
func (s *Store) isDescendantUnsafe(ancestorID, candidateID string) bool {
	for {
		current, ok := s.projectFiles[candidateID]
		if !ok || current.ParentID == "" {
			return false
		}
		if current.ParentID == ancestorID {
			return true
		}
		candidateID = current.ParentID
	}
}

// BatchDeleteProjectFiles deletes multiple files/folders and cascades to children.
// Returns a result summarizing deleted count and any IDs that failed.
func (s *Store) BatchDeleteProjectFiles(userID, projectID string, ids []string) model.BatchDeleteResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return model.BatchDeleteResult{Deleted: 0, Failed: ids}
	}

	// First pass: collect all IDs to delete (including descendants of folders).
	var toDelete []string
	for _, id := range ids {
		pf, exists := s.projectFiles[id]
		if !exists || pf.ProjectID != projectID {
			continue // skip invalid IDs
		}
		toDelete = append(toDelete, id)
		if pf.IsFolder {
			descendants := s.collectDescendantIDsUnsafe(projectID, id)
			toDelete = append(toDelete, descendants...)
		}
	}

	// Deduplicate.
	seen := make(map[string]bool)
	var unique []string
	for _, id := range toDelete {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	// Delete them all.
	deleted := 0
	var failed []string
	for _, id := range unique {
		pf, exists := s.projectFiles[id]
		if !exists {
			failed = append(failed, id)
			continue
		}

		delete(s.projectFiles, id)
		_ = s.deleteProjectFileUnsafe(id)

		// Remove from project index.
		indexIDs := s.projectFileIndex[projectID]
		for i, iid := range indexIDs {
			if iid == id {
				s.projectFileIndex[projectID] = append(indexIDs[:i], indexIDs[i+1:]...)
				break
			}
		}
		// Remove from transfer index.
		if pf.TransferID != "" {
			delete(s.transferFileIndex, pf.TransferID)
		}
		deleted++
	}

	return model.BatchDeleteResult{Deleted: deleted, Failed: failed}
}

// collectDescendantIDsUnsafe recursively collects all descendant IDs of a folder.
// Must be called with s.mu held.
func (s *Store) collectDescendantIDsUnsafe(projectID, folderID string) []string {
	var result []string
	for _, fid := range s.projectFileIndex[projectID] {
		if child, ok := s.projectFiles[fid]; ok && child.ParentID == folderID {
			result = append(result, fid)
			if child.IsFolder {
				result = append(result, s.collectDescendantIDsUnsafe(projectID, fid)...)
			}
		}
	}
	return result
}

// SaveProjectFileFromArtifact creates a ProjectFile from an agent artifact.
// Returns nil, nil if the task has no projectID (graceful skip).
// Public API — acquires s.mu.Lock.
func (s *Store) SaveProjectFileFromArtifact(artifact model.TaskArtifact) (*model.ProjectFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveProjectFileFromArtifactUnsafe(artifact)
}

// saveProjectFileFromArtifactUnsafe is the internal implementation.
// Caller MUST hold s.mu (at least Write lock).
func (s *Store) saveProjectFileFromArtifactUnsafe(artifact model.TaskArtifact) (*model.ProjectFile, error) {
	// Look up task to find projectID.
	task, ok := s.tasks[artifact.TaskID]
	if !ok || task.ProjectID == "" {
		return nil, nil // graceful skip
	}

	// Deduplicate by TransferID.
	if artifact.TransferID != "" {
		if existingID, exists := s.transferFileIndex[artifact.TransferID]; exists {
			if existing, ok := s.projectFiles[existingID]; ok {
				return existing, nil
			}
		}
	}

	pf := &model.ProjectFile{
		ID:         "pf_" + newID(),
		ProjectID:  task.ProjectID,
		TaskID:     artifact.TaskID,
		AgentID:    artifact.FromAgentID,
		AgentName:  artifact.FromAgentName,
		FileName:   artifact.FileName,
		FileSize:   artifact.FileSize,
		MimeType:   artifact.MimeType,
		LocalPath:  artifact.LocalPath, // temp path from transfer volume
		Source:     "agent_artifact",
		TransferID: artifact.TransferID,
		UploadedBy: task.UserID,
		CreatedAt:  time.Now().UTC(),
	}

	s.projectFiles[pf.ID] = pf
	s.projectFileIndex[task.ProjectID] = append(s.projectFileIndex[task.ProjectID], pf.ID)
	if pf.TransferID != "" {
		s.transferFileIndex[pf.TransferID] = pf.ID
	}

	if err := s.persistProjectFileUnsafe(pf); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist project file from artifact", zap.String("id", pf.ID), zap.Error(err))
		}
	}

	return pf, nil
}

// ListProjectFiles returns files for a project with optional filters.
func (s *Store) ListProjectFiles(userID, projectID, source, taskID, agentID string) []model.ProjectFile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Validate ownership.
	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return []model.ProjectFile{}
	}

	fileIDs := s.projectFileIndex[projectID]
	if len(fileIDs) == 0 {
		return []model.ProjectFile{}
	}

	result := make([]model.ProjectFile, 0, len(fileIDs))
	for _, id := range fileIDs {
		pf, ok := s.projectFiles[id]
		if !ok {
			continue
		}
		// Exclude folders from flat file list.
		if pf.IsFolder {
			continue
		}
		if source != "" && pf.Source != source {
			continue
		}
		if taskID != "" && pf.TaskID != taskID {
			continue
		}
		if agentID != "" && pf.AgentID != agentID {
			continue
		}
		result = append(result, *pf)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

// GetProjectFile returns a single file by ID with ownership validation.
func (s *Store) GetProjectFile(userID, fileID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}

	project, ok := s.projects[pf.ProjectID]
	if !ok || project.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}

	return pf, nil
}

// GetProjectFileByID returns a file by its ID without ownership validation.
// Used by agent file download endpoint where agents authenticate via download token.
func (s *Store) GetProjectFileByID(fileID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}
	return pf, nil
}

// GetProjectFileByTransferID finds a file by its transfer ID.
func (s *Store) GetProjectFileByTransferID(transferID string) (*model.ProjectFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fileID, ok := s.transferFileIndex[transferID]
	if !ok {
		return nil, nil
	}
	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, nil
	}
	return pf, nil
}

// DeleteProjectFile removes a file record and returns it.
// If the entry is a folder, all child files and sub-folders are also deleted.
func (s *Store) DeleteProjectFile(userID, fileID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}

	project, ok := s.projects[pf.ProjectID]
	if !ok || project.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}

	// If it's a folder, recursively delete all children.
	if pf.IsFolder {
		s.deleteFolderChildrenUnsafe(pf.ProjectID, fileID)
	}

	delete(s.projectFiles, fileID)

	// Remove from project index.
	fileIDs := s.projectFileIndex[pf.ProjectID]
	for i, id := range fileIDs {
		if id == fileID {
			s.projectFileIndex[pf.ProjectID] = append(fileIDs[:i], fileIDs[i+1:]...)
			break
		}
	}

	// Remove from transfer index.
	if pf.TransferID != "" {
		delete(s.transferFileIndex, pf.TransferID)
	}

	_ = s.deleteProjectFileUnsafe(fileID)
	return pf, nil
}

// deleteFolderChildrenUnsafe recursively deletes all children of a folder.
// Must be called with s.mu held.
func (s *Store) deleteFolderChildrenUnsafe(projectID, folderID string) {
	// Collect child IDs first to avoid modifying the index slice during iteration.
	var childIDs []string
	for _, fid := range s.projectFileIndex[projectID] {
		if child, ok := s.projectFiles[fid]; ok && child.ParentID == folderID {
			childIDs = append(childIDs, fid)
		}
	}

	for _, fid := range childIDs {
		child, ok := s.projectFiles[fid]
		if !ok {
			continue
		}
		if child.IsFolder {
			s.deleteFolderChildrenUnsafe(projectID, child.ID)
		}
		delete(s.projectFiles, fid)
		_ = s.deleteProjectFileUnsafe(fid)

		// Remove from project index.
		ids := s.projectFileIndex[projectID]
		for i, id := range ids {
			if id == fid {
				s.projectFileIndex[projectID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}
}

// GetProjectFileTree builds a hierarchical view of project files.
// User uploads are organized by the ParentID folder hierarchy.
// Agent artifacts remain grouped by task → agent.
func (s *Store) GetProjectFileTree(userID, projectID string) *model.ProjectFileTree {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return &model.ProjectFileTree{
			Uploads: []model.ProjectFileTreeNode{},
			Tasks:   []model.ProjectFileTreeNode{},
		}
	}

	tree := &model.ProjectFileTree{
		Uploads: []model.ProjectFileTreeNode{},
		Tasks:   []model.ProjectFileTreeNode{},
	}

	var uploadEntries []model.ProjectFile    // files + folders with source "user_upload"
	taskFilesMap := make(map[string][]model.ProjectFile) // agent_artifact

	fileIDs := s.projectFileIndex[projectID]
	for _, id := range fileIDs {
		pf, ok := s.projectFiles[id]
		if !ok {
			continue
		}
		switch pf.Source {
		case "user_upload":
			uploadEntries = append(uploadEntries, *pf)
		case "agent_artifact":
			if pf.TaskID != "" {
				taskFilesMap[pf.TaskID] = append(taskFilesMap[pf.TaskID], *pf)
			}
		}
	}

	// --- Build uploads tree using ParentID hierarchy ---
	if len(uploadEntries) > 0 {
		tree.Uploads = buildFileTree(uploadEntries)
	}

	// --- Build tasks nodes (agent artifacts) and merge into uploads tree ---
	var taskNodes []model.ProjectFileTreeNode
	for taskID, files := range taskFilesMap {
		task, taskOk := s.tasks[taskID]
		taskName := taskID
		if taskOk {
			taskName = task.Title
		}

		agentMap := make(map[string][]model.ProjectFile)
		var noAgentFiles []model.ProjectFile
		for _, f := range files {
			if f.AgentID != "" {
				agentMap[f.AgentID] = append(agentMap[f.AgentID], f)
			} else {
				noAgentFiles = append(noAgentFiles, f)
			}
		}

		var children []model.ProjectFileTreeNode
		for agentID, agentFiles := range agentMap {
			agentName := agentID
			if len(agentFiles) > 0 && agentFiles[0].AgentName != "" {
				agentName = agentFiles[0].AgentName
			}
			sort.Slice(agentFiles, func(i, j int) bool {
				return agentFiles[i].CreatedAt.Before(agentFiles[j].CreatedAt)
			})
			children = append(children, model.ProjectFileTreeNode{
				ID:    "__agent__" + taskID + "__" + agentID,
				Name:  agentName,
				Type:  "directory",
				Files: agentFiles,
			})
		}

		sort.Slice(children, func(i, j int) bool {
			return children[i].Name < children[j].Name
		})

		if len(noAgentFiles) > 0 {
			sort.Slice(noAgentFiles, func(i, j int) bool {
				return noAgentFiles[i].CreatedAt.Before(noAgentFiles[j].CreatedAt)
			})
			children = append(children, model.ProjectFileTreeNode{
				ID:    "__agent__" + taskID + "__" + "__unknown__",
				Name:  "其他",
				Type:  "directory",
				Files: noAgentFiles,
			})
		}

		taskNodes = append(taskNodes, model.ProjectFileTreeNode{
			ID:       "__task__" + taskID,
			Name:     taskName,
			Type:     "directory",
			Children: children,
		})
	}

	sort.Slice(taskNodes, func(i, j int) bool {
		return taskNodes[i].Name < taskNodes[j].Name
	})

	// Merge artifact tree into uploads tree under "智能体产物" virtual root.
	if len(taskNodes) > 0 {
		tree.Uploads = append(tree.Uploads, model.ProjectFileTreeNode{
			ID:       "__artifacts__",
			Name:     "智能体产物",
			Type:     "directory",
			Children: taskNodes,
		})
	}

	return tree
}

// buildFileTree converts a flat list of user_upload entries (files + folders)
// into a recursive tree using ParentID. Root-level entries have ParentID == "".
func buildFileTree(entries []model.ProjectFile) []model.ProjectFileTreeNode {
	// Separate folders and files.
	folderMap := make(map[string]*model.ProjectFile)
	var rootFolders []model.ProjectFile
	var rootFiles []model.ProjectFile
	childrenMap := make(map[string][]model.ProjectFile) // parentID → children

	for i := range entries {
		e := entries[i]
		if e.IsFolder {
			folderMap[e.ID] = &e
			if e.ParentID == "" {
				rootFolders = append(rootFolders, e)
			} else {
				childrenMap[e.ParentID] = append(childrenMap[e.ParentID], e)
			}
		} else {
			if e.ParentID == "" {
				rootFiles = append(rootFiles, e)
			} else {
				childrenMap[e.ParentID] = append(childrenMap[e.ParentID], e)
			}
		}
	}

	// Sort: folders first, then files.
	sort.Slice(rootFolders, func(i, j int) bool { return rootFolders[i].FileName < rootFolders[j].FileName })
	sort.Slice(rootFiles, func(i, j int) bool { return rootFiles[i].CreatedAt.Before(rootFiles[j].CreatedAt) })

	var nodes []model.ProjectFileTreeNode

	// Root folders.
	for _, f := range rootFolders {
		nodes = append(nodes, buildFolderNode(f.ID, f.FileName, folderMap, childrenMap))
	}
	// Root files.
	if len(rootFiles) > 0 {
		nodes = append(nodes, model.ProjectFileTreeNode{
			ID:    "",
			Name:  "用户上传",
			Type:  "directory",
			Files: rootFiles,
		})
	}

	// If no folders, wrap root files in "用户上传" node (legacy behavior).
	if len(rootFolders) == 0 && len(rootFiles) == 0 {
		// Collect any files that are children of folders — shouldn't happen but safe.
		var stray []model.ProjectFile
		for _, children := range childrenMap {
			for _, c := range children {
				if !c.IsFolder {
					stray = append(stray, c)
				}
			}
		}
		if len(stray) > 0 {
			sort.Slice(stray, func(i, j int) bool { return stray[i].CreatedAt.Before(stray[j].CreatedAt) })
			nodes = append(nodes, model.ProjectFileTreeNode{
				ID:    "",
				Name:  "用户上传",
				Type:  "directory",
				Files: stray,
			})
		}
	}

	return nodes
}

// buildFolderNode recursively builds a tree node for a folder.
func buildFolderNode(folderID, folderName string, folderMap map[string]*model.ProjectFile, childrenMap map[string][]model.ProjectFile) model.ProjectFileTreeNode {
	children := childrenMap[folderID]
	sort.Slice(children, func(i, j int) bool {
		// Folders first, then files; alphabetical for folders, chronological for files.
		if children[i].IsFolder != children[j].IsFolder {
			return children[i].IsFolder
		}
		if children[i].IsFolder {
			return children[i].FileName < children[j].FileName
		}
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})

	var childNodes []model.ProjectFileTreeNode
	var childFiles []model.ProjectFile

	for _, c := range children {
		if c.IsFolder {
			childNodes = append(childNodes, buildFolderNode(c.ID, c.FileName, folderMap, childrenMap))
		} else {
			childFiles = append(childFiles, c)
		}
	}

	node := model.ProjectFileTreeNode{
		ID:    folderID,
		Name:  folderName,
		Type:  "directory",
		Files: childFiles,
	}
	if len(childNodes) > 0 {
		node.Children = childNodes
	}
	return node
}

// BrowseProjectFiles returns the flat contents of a folder (folders + files) plus breadcrumbs.
// parentID empty means the project root.
func (s *Store) BrowseProjectFiles(userID, projectID, parentID string) (*model.BrowseFilesResult, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}

	// ---- Virtual artifact folder routing ----
	if parentID == "__artifacts__" {
		return s.browseArtifactTasks(projectID)
	}
	if strings.HasPrefix(parentID, "__task__") {
		taskID := strings.TrimPrefix(parentID, "__task__")
		return s.browseArtifactAgents(projectID, taskID)
	}
	if strings.HasPrefix(parentID, "__agent__") {
		parts := strings.SplitN(strings.TrimPrefix(parentID, "__agent__"), "__", 2)
		if len(parts) == 2 {
			return s.browseArtifactFiles(projectID, parts[0], parts[1])
		}
	}

	// ---- Normal user-upload folder browsing ----
	var folders, files []model.ProjectFile
	hasArtifacts := false
	fileIDs := s.projectFileIndex[projectID]
	for _, fid := range fileIDs {
		pf, ok := s.projectFiles[fid]
		if !ok {
			continue
		}
		if pf.Source == "agent_artifact" && !pf.IsFolder {
			hasArtifacts = true
			continue
		}
		if pf.Source != "user_upload" {
			continue
		}
		if pf.ParentID != parentID {
			continue
		}
		if pf.IsFolder {
			folders = append(folders, *pf)
		} else {
			files = append(files, *pf)
		}
	}

	sort.Slice(folders, func(i, j int) bool { return folders[i].FileName < folders[j].FileName })
	sort.Slice(files, func(i, j int) bool { return files[i].CreatedAt.Before(files[j].CreatedAt) })

	if folders == nil {
		folders = []model.ProjectFile{}
	}
	if files == nil {
		files = []model.ProjectFile{}
	}

	var virtualFolders []model.VirtualFolder
	if parentID == "" && hasArtifacts {
		count := s.countArtifactsUnsafe(projectID)
		virtualFolders = append(virtualFolders, model.VirtualFolder{
			ID: "__artifacts__", Name: "智能体产物", ItemCount: count,
		})
	}

	breadcrumbs := buildBreadcrumbs(s.projectFiles, parentID)

	return &model.BrowseFilesResult{
		ParentID:      parentID,
		VirtualFolders: virtualFolders,
		Folders:       folders,
		Files:         files,
		Breadcrumbs:   breadcrumbs,
	}, nil
}

// ---- Virtual folder helpers ----

func (s *Store) countArtifactsUnsafe(projectID string) int {
	count := 0
	for _, fid := range s.projectFileIndex[projectID] {
		if pf, ok := s.projectFiles[fid]; ok && pf.Source == "agent_artifact" && !pf.IsFolder {
			count++
		}
	}
	return count
}

func (s *Store) browseArtifactTasks(projectID string) (*model.BrowseFilesResult, *transport.AppError) {
	taskMap := make(map[string]int)
	taskNameMap := make(map[string]string)
	for _, fid := range s.projectFileIndex[projectID] {
		pf, ok := s.projectFiles[fid]
		if !ok || pf.Source != "agent_artifact" || pf.IsFolder || pf.TaskID == "" {
			continue
		}
		taskMap[pf.TaskID]++
		if _, exists := taskNameMap[pf.TaskID]; !exists {
			name := pf.TaskID
			if t, ok := s.tasks[pf.TaskID]; ok {
				name = t.Title
			}
			taskNameMap[pf.TaskID] = name
		}
	}

	var vf []model.VirtualFolder
	for tid, count := range taskMap {
		vf = append(vf, model.VirtualFolder{ID: "__task__" + tid, Name: taskNameMap[tid], ItemCount: count})
	}
	sort.Slice(vf, func(i, j int) bool { return vf[i].Name < vf[j].Name })
	if vf == nil {
		vf = []model.VirtualFolder{}
	}

	return &model.BrowseFilesResult{
		ParentID:      "__artifacts__",
		VirtualFolders: vf,
		Folders:       []model.ProjectFile{},
		Files:         []model.ProjectFile{},
		Breadcrumbs:   []model.BreadcrumbNode{{ID: "__artifacts__", Name: "智能体产物"}},
	}, nil
}

func (s *Store) browseArtifactAgents(projectID, taskID string) (*model.BrowseFilesResult, *transport.AppError) {
	taskName := taskID
	if t, ok := s.tasks[taskID]; ok {
		taskName = t.Title
	}

	agentMap := make(map[string]int)  // agentID → count
	agentNameMap := make(map[string]string)
	for _, fid := range s.projectFileIndex[projectID] {
		pf, ok := s.projectFiles[fid]
		if !ok || pf.Source != "agent_artifact" || pf.IsFolder || pf.TaskID != taskID {
			continue
		}
		key := pf.AgentID
		if key == "" {
			key = "__unknown__"
		}
		agentMap[key]++
		if _, exists := agentNameMap[key]; !exists {
			name := pf.AgentName
			if name == "" {
				name = pf.AgentID
			}
			if name == "" {
				name = "其他"
			}
			agentNameMap[key] = name
		}
	}

	var vf []model.VirtualFolder
	for aid, count := range agentMap {
		vf = append(vf, model.VirtualFolder{ID: "__agent__" + taskID + "__" + aid, Name: agentNameMap[aid], ItemCount: count})
	}
	sort.Slice(vf, func(i, j int) bool { return vf[i].Name < vf[j].Name })
	if vf == nil {
		vf = []model.VirtualFolder{}
	}

	return &model.BrowseFilesResult{
		ParentID:      "__task__" + taskID,
		VirtualFolders: vf,
		Folders:       []model.ProjectFile{},
		Files:         []model.ProjectFile{},
		Breadcrumbs: []model.BreadcrumbNode{
			{ID: "__artifacts__", Name: "智能体产物"},
			{ID: "__task__" + taskID, Name: taskName},
		},
	}, nil
}

func (s *Store) browseArtifactFiles(projectID, taskID, agentID string) (*model.BrowseFilesResult, *transport.AppError) {
	taskName := taskID
	if t, ok := s.tasks[taskID]; ok {
		taskName = t.Title
	}
	agentName := agentID
	if agentID == "__unknown__" {
		agentName = "其他"
	}

	var files []model.ProjectFile
	for _, fid := range s.projectFileIndex[projectID] {
		pf, ok := s.projectFiles[fid]
		if !ok || pf.Source != "agent_artifact" || pf.IsFolder || pf.TaskID != taskID {
			continue
		}
		pfAgentID := pf.AgentID
		if pfAgentID == "" {
			pfAgentID = "__unknown__"
		}
		if pfAgentID != agentID {
			continue
		}
		if agentName == agentID && pf.AgentName != "" {
			agentName = pf.AgentName
		}
		files = append(files, *pf)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].CreatedAt.Before(files[j].CreatedAt) })
	if files == nil {
		files = []model.ProjectFile{}
	}

	return &model.BrowseFilesResult{
		ParentID: "__agent__" + taskID + "__" + agentID,
		Folders:  []model.ProjectFile{},
		Files:    files,
		Breadcrumbs: []model.BreadcrumbNode{
			{ID: "__artifacts__", Name: "智能体产物"},
			{ID: "__task__" + taskID, Name: taskName},
			{ID: "__agent__" + taskID + "__" + agentID, Name: agentName},
		},
	}, nil
}

// buildBreadcrumbs walks up the ParentID chain to produce a breadcrumb path.
func buildBreadcrumbs(projectFiles map[string]*model.ProjectFile, folderID string) []model.BreadcrumbNode {
	if folderID == "" {
		return []model.BreadcrumbNode{}
	}

	var reversed []model.BreadcrumbNode
	currentID := folderID
	for currentID != "" {
		pf, ok := projectFiles[currentID]
		if !ok {
			break
		}
		reversed = append(reversed, model.BreadcrumbNode{ID: pf.ID, Name: pf.FileName})
		currentID = pf.ParentID
	}

	// Reverse so root is first.
	result := make([]model.BreadcrumbNode, len(reversed))
	for i, node := range reversed {
		result[len(reversed)-1-i] = node
	}
	return result
}

// ListArtifactGroups returns agent artifacts grouped by task → agent.
func (s *Store) ListArtifactGroups(userID, projectID string) ([]model.ArtifactTaskGroup, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok || project.UserID != userID {
		return nil, transport.Forbidden("access denied")
	}

	// Collect artifacts and group by task.
	taskMap := make(map[string][]model.ProjectFile)
	fileIDs := s.projectFileIndex[projectID]
	for _, fid := range fileIDs {
		pf, ok := s.projectFiles[fid]
		if !ok || pf.Source != "agent_artifact" || pf.IsFolder {
			continue
		}
		if pf.TaskID == "" {
			continue
		}
		taskMap[pf.TaskID] = append(taskMap[pf.TaskID], *pf)
	}

	var groups []model.ArtifactTaskGroup
	for taskID, artifacts := range taskMap {
		// Resolve task name.
		taskName := taskID
		if task, ok := s.tasks[taskID]; ok {
			taskName = task.Title
		}

		// Group by agent within task.
		agentMap := make(map[string]*model.ArtifactAgentGroup)
		var noAgentFiles []model.ProjectFile
		for _, a := range artifacts {
			if a.AgentID != "" {
				key := a.AgentID
				g, exists := agentMap[key]
				if !exists {
					agentName := a.AgentName
					if agentName == "" {
						agentName = a.AgentID
					}
					g = &model.ArtifactAgentGroup{AgentID: a.AgentID, AgentName: agentName}
					agentMap[key] = g
				}
				g.Files = append(g.Files, a)
			} else {
				noAgentFiles = append(noAgentFiles, a)
			}
		}

		var agents []model.ArtifactAgentGroup
		for _, ag := range agentMap {
			sort.Slice(ag.Files, func(i, j int) bool { return ag.Files[i].CreatedAt.Before(ag.Files[j].CreatedAt) })
			agents = append(agents, *ag)
		}
		sort.Slice(agents, func(i, j int) bool { return agents[i].AgentName < agents[j].AgentName })

		if len(noAgentFiles) > 0 {
			sort.Slice(noAgentFiles, func(i, j int) bool { return noAgentFiles[i].CreatedAt.Before(noAgentFiles[j].CreatedAt) })
			agents = append(agents, model.ArtifactAgentGroup{AgentID: "", AgentName: "其他", Files: noAgentFiles})
		}

		groups = append(groups, model.ArtifactTaskGroup{
			TaskID:   taskID,
			TaskName: taskName,
			Agents:   agents,
		})
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].TaskName < groups[j].TaskName })

	return groups, nil
}

// SetProjectFileLocalPath updates the LocalPath of a ProjectFile in memory and persists it.
func (s *Store) SetProjectFileLocalPath(fileID, localPath string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return transport.NotFound("file not found")
	}

	pf.LocalPath = localPath

	if err := s.persistProjectFileUnsafe(pf); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist project file local path", zap.String("id", fileID), zap.Error(err))
		}
	}

	return nil
}
