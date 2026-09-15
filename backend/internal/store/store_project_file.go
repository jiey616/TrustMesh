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
func (s *Store) SaveProjectFile(sc Scope, projectID string, pf *model.ProjectFile) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate project ownership.
	project, ok := s.projects[projectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if !s.projectVisible(sc, project) {
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
	pf.UploadedBy = sc.UserID
	pf.OrgID = s.resolveOwnerOrgUnsafe(sc)
	pf.CreatedAt = time.Now().UTC()

	s.projectFiles[pf.ID] = pf
	s.projectFileIndex[projectID] = append(s.projectFileIndex[projectID], pf.ID)
	// 快照将被覆盖的 transfer 映射旧值（QA 路由修复）：transfer_id 冲突时旧映射指向
	// 其他记录，persist 失败的回滚必须还原给那条记录，而不是删键。
	prevTransferFileID, hadPrevTransferFile := s.transferFileIndex[pf.TransferID]
	if pf.TransferID != "" {
		s.transferFileIndex[pf.TransferID] = pf.ID
	}

	// T2.3b：Mongo 权威 —— persist 失败必须把三张内存索引全部回滚并返错，
	// 否则会留下「内存有、库里没有」的幽灵记录（看得到、重启即丢）。
	if appErr := s.persistProjectFileUnsafe(pf); appErr != nil {
		s.rollbackProjectFileAddUnsafe(pf, prevTransferFileID, hadPrevTransferFile)
		return nil, appErr
	}

	return pf, nil
}

// CreateMeetingMinutesFile creates a ProjectFile record for meeting minutes
// (Source = "meeting_minutes"). It resolves the project owner internally so the
// caller (the webhook handler, acting on behalf of the host agent) does not need
// to know the owner user ID. The caller is responsible for writing the file
// bytes to storage and then calling SetProjectFileLocalPath.
func (s *Store) CreateMeetingMinutesFile(meetingID, fileName, content string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.meetings[meetingID]
	if !ok {
		return nil, transport.NotFound("meeting not found")
	}
	project, ok := s.projects[m.ProjectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	ownerID := project.UserID
	if ownerID == "" {
		ownerID = m.CreatorID
	}

	pf := &model.ProjectFile{
		ID:         "pf_" + newID(),
		ProjectID:  m.ProjectID,
		OrgID:      s.personalOrgOfUnsafe(ownerID),
		FileName:   fileName,
		FileSize:   int64(len([]byte(content))),
		MimeType:   "text/markdown",
		Source:     "meeting_minutes",
		MeetingID:  meetingID,
		UploadedBy: ownerID,
		CreatedAt:  time.Now().UTC(),
	}

	s.projectFiles[pf.ID] = pf
	s.projectFileIndex[m.ProjectID] = append(s.projectFileIndex[m.ProjectID], pf.ID)
	// 会议纪要记录不含 TransferID，快照恒为「无旧映射」；统一传参保持回滚helper契约一致。
	prevTransferFileID, hadPrevTransferFile := s.transferFileIndex[pf.TransferID]

	// T2.3b：Mongo 权威 —— 失败回滚两张内存索引并返错（Mongo 抖动时 webhook 会拿到错误，
	// 由上游重投；这是本批明确接受的用户可见失败面，见规格 §6）。
	if appErr := s.persistProjectFileUnsafe(pf); appErr != nil {
		s.rollbackProjectFileAddUnsafe(pf, prevTransferFileID, hadPrevTransferFile)
		return nil, appErr
	}

	return pf, nil
}

// CreateFolder creates a new folder record in the project file space.
func (s *Store) CreateFolder(sc Scope, projectID, name, parentID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[projectID]
	if !ok {
		return nil, transport.NotFound("project not found")
	}
	if !s.projectVisible(sc, project) {
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
		OrgID:      s.resolveOwnerOrgUnsafe(sc),
		ParentID:   parentID,
		FileName:   name,
		FileSize:   0,
		MimeType:   "",
		Source:     "user_upload",
		IsFolder:   true,
		UploadedBy: sc.UserID,
		CreatedAt:  time.Now().UTC(),
	}

	s.projectFiles[folder.ID] = folder
	s.projectFileIndex[projectID] = append(s.projectFileIndex[projectID], folder.ID)
	// 文件夹不含 TransferID，快照恒为「无旧映射」；统一传参保持回滚 helper 契约一致。
	prevTransferFileID, hadPrevTransferFile := s.transferFileIndex[folder.TransferID]

	// T2.3b：Mongo 权威 —— 失败回滚两张内存索引并返错，不留幽灵文件夹。
	if appErr := s.persistProjectFileUnsafe(folder); appErr != nil {
		s.rollbackProjectFileAddUnsafe(folder, prevTransferFileID, hadPrevTransferFile)
		return nil, appErr
	}

	return folder, nil
}

// rollbackProjectFileAddUnsafe 回滚一次「新增项目文件 / 文件夹」的内存写入（T2.3b）。
//
// 新增路径（SaveProjectFile / CreateMeetingMinutesFile / CreateFolder / 工件自动入库的新建分支）
// 先写内存再 persist；persist 失败必须把三张索引（projectFiles / projectFileIndex /
// transferFileIndex）**逐字节**还原到调用前形态，否则会留下「内存有、库里没有」的幽灵记录
// （ListProjectFiles / Browse 能列出它，重启即消失）。还原要求：索引切片为空时删除键本身
// （而非留一个空切片），否则与调用前「键不存在」的形态不等价。
//
// prevTransferFileID / hadPrevTransferFile 是新增写入**覆盖 transferFileIndex 映射之前**的
// 旧值快照（QA 路由修复）：transfer_id 冲突场景下，新增路径会无条件把映射改写成自己，
// 而 persist 失败后回滚时「映射指向本记录」这一恒等守卫必然通过 —— 若直接删键，就会把
// 无辜既有记录的映射一并抹掉。因此：
//   - 新增前键不存在 → 删键（无冲突的常规路径，与原行为一致）；
//   - 新增前键已存在（指向其他记录）→ 把映射**还原给那条记录**，而不是删键。
//
// 必须持锁调用。
func (s *Store) rollbackProjectFileAddUnsafe(pf *model.ProjectFile, prevTransferFileID string, hadPrevTransferFile bool) {
	if pf == nil {
		return
	}
	delete(s.projectFiles, pf.ID)

	if ids := s.projectFileIndex[pf.ProjectID]; len(ids) > 0 {
		remaining := make([]string, 0, len(ids))
		for _, id := range ids {
			if id != pf.ID {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) == 0 {
			delete(s.projectFileIndex, pf.ProjectID)
		} else {
			s.projectFileIndex[pf.ProjectID] = remaining
		}
	}

	// transfer 映射回滚：按「覆盖前的旧值快照」还原，见函数头注释。
	if pf.TransferID == "" {
		return
	}
	if hadPrevTransferFile {
		s.transferFileIndex[pf.TransferID] = prevTransferFileID
		return
	}
	// 无旧映射：仅当映射仍指向本记录时才删键（防御性恒等守卫，正常路径恒真）。
	if s.transferFileIndex[pf.TransferID] == pf.ID {
		delete(s.transferFileIndex, pf.TransferID)
	}
}

// RenameProjectFile updates the name of a file or folder.
func (s *Store) RenameProjectFile(sc Scope, projectID, fileID, name string) (*model.ProjectFile, *transport.AppError) {
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
	if !ok || !s.projectVisible(sc, project) {
		return nil, transport.NotFound("project not found") // 不可见 = 不存在，防存在性泄漏
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

	// T2.3b：就地改名走零副作用提交包装器 —— persist 失败回滚内存、冲突返回 409。
	if appErr := s.mutateProjectFileUnsafe(fileID, func(pf *model.ProjectFile) *transport.AppError {
		pf.FileName = name
		return nil
	}); appErr != nil {
		return nil, appErr
	}
	return pf, nil
}

// MoveProjectFile moves a file or folder to a different parent folder.
func (s *Store) MoveProjectFile(sc Scope, projectID, fileID, targetParentID string) (*model.ProjectFile, *transport.AppError) {
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
	if !ok || !s.projectVisible(sc, project) {
		return nil, transport.NotFound("project not found") // 不可见 = 不存在，防存在性泄漏
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

	// T2.3b：就地移动走零副作用提交包装器 —— persist 失败回滚内存、冲突返回 409。
	if appErr := s.mutateProjectFileUnsafe(fileID, func(pf *model.ProjectFile) *transport.AppError {
		pf.ParentID = targetParentID
		return nil
	}); appErr != nil {
		return nil, appErr
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
func (s *Store) BatchDeleteProjectFiles(sc Scope, projectID string, ids []string) model.BatchDeleteResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, project) {
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
	//
	// T2.3b：先删 Mongo、成功后再改内存。任一条 Mongo 删除失败则该条计入 Failed[] 且
	// **内存保持不动**（不删、不回滚），并继续处理其余条目 —— 保持「尽量删完、不因一条
	// 失败中断整批」的语义。这样绝不会出现「内存已删、库里还在」→ 重启复活的形态。
	deleted := 0
	var failed []string
	for _, id := range unique {
		pf, exists := s.projectFiles[id]
		if !exists {
			failed = append(failed, id)
			continue
		}

		if err := s.deleteProjectFileUnsafe(id); err != nil {
			if s.log != nil {
				s.log.Warn("failed to delete project file from mongo", zap.String("id", id), zap.Error(err))
			}
			failed = append(failed, id)
			continue
		}

		// Mongo 删除成功后才落实内存。
		delete(s.projectFiles, id)

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
			if s.transferFileIndex[pf.TransferID] == pf.ID {
				delete(s.transferFileIndex, pf.TransferID)
			}
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
				// Refresh the file-nature classification if it changed (e.g. the
				// artifact was re-registered with an outputName binding after the
				// project file was first created as a process file).
				if existing.Kind != artifact.Kind || existing.OutputName != artifact.OutputName {
					// T2.3b：本分支也**致命化**（而非沿用原来的刻意 warn-only）。分析：
					// 它是 dedupe 命中（同一 transfer_id 的记录已存在且可见）后，仅在
					// file-nature 分类（Kind: deliverable/process 与 OutputName 绑定）
					// 变化时补一次写入。静默失败会让 agent 工件在项目文件树里的
					// 「交付/过程」归类与 artifacts 侧不一致（用户可见的归类缺口）。
					// 上游重投有 transfer_id 唯一部分索引兜底（mongo_state.go:231），
					// 重试相对安全，故返错比吞错更诚实。失败零副作用回滚，避免内存分叉。
					snapshot := copyProjectFile(existing)
					existing.Kind = artifact.Kind
					existing.OutputName = artifact.OutputName
					s.projectFiles[existing.ID] = existing
					if appErr := s.persistProjectFileUnsafe(existing); appErr != nil {
						s.projectFiles[existing.ID] = snapshot
						return nil, appErr
					}
				}
				return existing, nil
			}
		}
	}

	// Deduplicate by (task, agent, file name): an agent re-uploading the same
	// deliverable under a new transfer id overwrites the previous project file
	// instead of piling up duplicates in the project file list.
	if artifact.FromAgentID != "" {
		for _, id := range s.projectFileIndex[task.ProjectID] {
			existing, ok := s.projectFiles[id]
			if !ok || existing.TaskID != artifact.TaskID || existing.AgentID != artifact.FromAgentID {
				continue
			}
			if existing.FileName == artifact.FileName {
				// Update in place: keep the original id/created_at, refresh the
				// payload so the latest transfer content wins.
				//
				// T2.3b：致命化 + 零副作用回滚。旧实现 warn-only：持久化失败会让内存
				// 里的 FileSize/MimeType/LocalPath/TransferID 领先 Mongo，重启后被旧内容
				// 覆盖。现在任一步失败即还原记录与 transfer 索引并返错。
				snapshot := copyProjectFile(existing)
				existing.FileSize = artifact.FileSize
				existing.MimeType = artifact.MimeType
				existing.LocalPath = artifact.LocalPath
				existing.TransferID = artifact.TransferID
				existing.Kind = artifact.Kind
				existing.OutputName = artifact.OutputName
				s.projectFiles[existing.ID] = existing
				if existing.TransferID != "" {
					s.transferFileIndex[existing.TransferID] = existing.ID
				}
				if appErr := s.persistProjectFileUnsafe(existing); appErr != nil {
					s.projectFiles[existing.ID] = snapshot
					if existing.TransferID != "" && s.transferFileIndex[existing.TransferID] == existing.ID {
						delete(s.transferFileIndex, existing.TransferID)
					}
					if snapshot.TransferID != "" {
						s.transferFileIndex[snapshot.TransferID] = snapshot.ID
					}
					return nil, appErr
				}
				return existing, nil
			}
		}
	}

	pf := &model.ProjectFile{
		ID:         "pf_" + newID(),
		ProjectID:  task.ProjectID,
		OrgID:      s.personalOrgOfUnsafe(task.UserID),
		TaskID:     artifact.TaskID,
		AgentID:    artifact.FromAgentID,
		AgentName:  artifact.FromAgentName,
		FileName:   artifact.FileName,
		FileSize:   artifact.FileSize,
		MimeType:   artifact.MimeType,
		LocalPath:  artifact.LocalPath, // temp path from transfer volume
		Source:     "agent_artifact",
		TransferID: artifact.TransferID,
		Kind:       artifact.Kind,
		OutputName: artifact.OutputName,
		UploadedBy: task.UserID,
		CreatedAt:  time.Now().UTC(),
	}

	s.projectFiles[pf.ID] = pf
	s.projectFileIndex[task.ProjectID] = append(s.projectFileIndex[task.ProjectID], pf.ID)
	// 快照将被覆盖的 transfer 映射旧值（QA 路由修复，同 SaveProjectFile）。
	prevTransferFileID, hadPrevTransferFile := s.transferFileIndex[pf.TransferID]
	if pf.TransferID != "" {
		s.transferFileIndex[pf.TransferID] = pf.ID
	}

	// T2.3b：Mongo 权威 —— 失败回滚三张索引并返错；静默失败会让 agent 工件在项目
	// 文件树里不可见（用户可见缺口），故致命化。
	if appErr := s.persistProjectFileUnsafe(pf); appErr != nil {
		s.rollbackProjectFileAddUnsafe(pf, prevTransferFileID, hadPrevTransferFile)
		return nil, appErr
	}

	return pf, nil
}

// ListProjectFiles returns files for a project with optional filters.
func (s *Store) ListProjectFiles(sc Scope, projectID, source, taskID, agentID string) []model.ProjectFile {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Validate ownership.
	project, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, project) {
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
func (s *Store) GetProjectFile(sc Scope, fileID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}

	project, ok := s.projects[pf.ProjectID]
	if !ok || !s.projectVisible(sc, project) {
		return nil, transport.NotFound("project not found") // 不可见 = 不存在，防存在性泄漏
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
func (s *Store) DeleteProjectFile(sc Scope, fileID string) (*model.ProjectFile, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pf, ok := s.projectFiles[fileID]
	if !ok {
		return nil, transport.NotFound("file not found")
	}

	project, ok := s.projects[pf.ProjectID]
	if !ok || !s.projectVisible(sc, project) {
		return nil, transport.NotFound("project not found") // 不可见 = 不存在，防存在性泄漏
	}

	// T2.3b：先删 Mongo（文件夹含子树递归），全部成功后再改内存。
	// 任一 Mongo 删除失败即原样返错、**内存保持不动** —— 杜绝「内存已删、库里还在」
	// 的重启复活形态（旧实现 `_ = s.deleteProjectFileUnsafe(fileID)` 完全不检查错误）。
	if pf.IsFolder {
		if err := s.deleteFolderChildrenUnsafe(pf.ProjectID, fileID); err != nil {
			return nil, mongoWriteError(err)
		}
	}
	if err := s.deleteProjectFileUnsafe(fileID); err != nil {
		return nil, mongoWriteError(err)
	}

	// Mongo 删除成功后才落实内存。
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
	if pf.TransferID != "" && s.transferFileIndex[pf.TransferID] == pf.ID {
		delete(s.transferFileIndex, pf.TransferID)
	}

	return pf, nil
}

// deleteFolderChildrenUnsafe recursively deletes all children of a folder.
// Must be called with s.mu held.
//
// T2.3b：改为「先删 Mongo、后改内存」，并在 Mongo 删除失败时返回错误（调用方据此保持
// 内存不动）。先一次性收集整棵子树再统一删 Mongo，保证失败时**尚未**产生任何内存副作用。
func (s *Store) deleteFolderChildrenUnsafe(projectID, folderID string) error {
	descendants := s.collectDescendantIDsUnsafe(projectID, folderID)
	if len(descendants) == 0 {
		return nil
	}

	// Mongo 先行：任一删除失败即返回错误（内存未改）。
	for _, fid := range descendants {
		if err := s.deleteProjectFileUnsafe(fid); err != nil {
			return err
		}
	}

	// Mongo 全部成功后落实内存（含 transfer 索引清理）。
	for _, fid := range descendants {
		child, ok := s.projectFiles[fid]
		if !ok {
			continue
		}
		delete(s.projectFiles, fid)
		if child.TransferID != "" && s.transferFileIndex[child.TransferID] == child.ID {
			delete(s.transferFileIndex, child.TransferID)
		}

		// Remove from project index.
		ids := s.projectFileIndex[projectID]
		for i, id := range ids {
			if id == fid {
				s.projectFileIndex[projectID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
	}
	return nil
}

// GetProjectFileTree builds a hierarchical view of project files.
// User uploads are organized by the ParentID folder hierarchy.
// Agent artifacts remain grouped by task → agent.
func (s *Store) GetProjectFileTree(sc Scope, projectID string) *model.ProjectFileTree {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, project) {
		return &model.ProjectFileTree{
			Uploads: []model.ProjectFileTreeNode{},
			Tasks:   []model.ProjectFileTreeNode{},
		}
	}

	tree := &model.ProjectFileTree{
		Uploads: []model.ProjectFileTreeNode{},
		Tasks:   []model.ProjectFileTreeNode{},
	}

	var uploadEntries []model.ProjectFile                // files + folders with source "user_upload"
	taskFilesMap := make(map[string][]model.ProjectFile) // agent_artifact
	var minutesEntries []model.ProjectFile               // meeting_minutes

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
		case "meeting_minutes":
			minutesEntries = append(minutesEntries, *pf)
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

	// Merge artifact tree into uploads tree under "数字员工产物" virtual root.
	if len(taskNodes) > 0 {
		tree.Uploads = append(tree.Uploads, model.ProjectFileTreeNode{
			ID:       "__artifacts__",
			Name:     "数字员工产物",
			Type:     "directory",
			Children: taskNodes,
		})
	}

	// Merge meeting minutes under "会议纪要" virtual root.
	if len(minutesEntries) > 0 {
		sort.Slice(minutesEntries, func(i, j int) bool {
			return minutesEntries[i].CreatedAt.After(minutesEntries[j].CreatedAt)
		})
		tree.Uploads = append(tree.Uploads, model.ProjectFileTreeNode{
			ID:    "__minutes__",
			Name:  "会议纪要",
			Type:  "directory",
			Files: minutesEntries,
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
	// Root files — show directly in root, no "用户上传" wrapper.
	for _, f := range rootFiles {
		nodes = append(nodes, model.ProjectFileTreeNode{
			ID:    f.ID,
			Name:  f.FileName,
			Type:  "file",
			Files: []model.ProjectFile{f},
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
func (s *Store) BrowseProjectFiles(sc Scope, projectID, parentID string) (*model.BrowseFilesResult, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, project) {
		return nil, transport.NotFound("project not found") // 不可见 = 不存在，防存在性泄漏
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
	if parentID == "__minutes__" {
		return s.browseMeetingMinutes(projectID)
	}

	// ---- Normal user-upload folder browsing ----
	var folders, files []model.ProjectFile
	hasArtifacts := false
	hasMinutes := false
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
		if pf.Source == "meeting_minutes" && !pf.IsFolder {
			hasMinutes = true
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
			ID: "__artifacts__", Name: "数字员工产物", ItemCount: count,
		})
	}
	if parentID == "" && hasMinutes {
		count := s.countMinutesUnsafe(projectID)
		virtualFolders = append(virtualFolders, model.VirtualFolder{
			ID: "__minutes__", Name: "会议纪要", ItemCount: count,
		})
	}

	breadcrumbs := buildBreadcrumbs(s.projectFiles, parentID)

	return &model.BrowseFilesResult{
		ParentID:       parentID,
		VirtualFolders: virtualFolders,
		Folders:        folders,
		Files:          files,
		Breadcrumbs:    breadcrumbs,
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

func (s *Store) countMinutesUnsafe(projectID string) int {
	count := 0
	for _, fid := range s.projectFileIndex[projectID] {
		if pf, ok := s.projectFiles[fid]; ok && pf.Source == "meeting_minutes" && !pf.IsFolder {
			count++
		}
	}
	return count
}

func (s *Store) browseMeetingMinutes(projectID string) (*model.BrowseFilesResult, *transport.AppError) {
	var files []model.ProjectFile
	for _, fid := range s.projectFileIndex[projectID] {
		pf, ok := s.projectFiles[fid]
		if !ok || pf.Source != "meeting_minutes" || pf.IsFolder {
			continue
		}
		files = append(files, *pf)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].CreatedAt.After(files[j].CreatedAt)
	})
	if files == nil {
		files = []model.ProjectFile{}
	}

	return &model.BrowseFilesResult{
		ParentID:    "__minutes__",
		Folders:     []model.ProjectFile{},
		Files:       files,
		Breadcrumbs: []model.BreadcrumbNode{{ID: "__minutes__", Name: "会议纪要"}},
	}, nil
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
		ParentID:       "__artifacts__",
		VirtualFolders: vf,
		Folders:        []model.ProjectFile{},
		Files:          []model.ProjectFile{},
		Breadcrumbs:    []model.BreadcrumbNode{{ID: "__artifacts__", Name: "数字员工产物"}},
	}, nil
}

func (s *Store) browseArtifactAgents(projectID, taskID string) (*model.BrowseFilesResult, *transport.AppError) {
	taskName := taskID
	if t, ok := s.tasks[taskID]; ok {
		taskName = t.Title
	}

	agentMap := make(map[string]int) // agentID → count
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
		ParentID:       "__task__" + taskID,
		VirtualFolders: vf,
		Folders:        []model.ProjectFile{},
		Files:          []model.ProjectFile{},
		Breadcrumbs: []model.BreadcrumbNode{
			{ID: "__artifacts__", Name: "数字员工产物"},
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
			{ID: "__artifacts__", Name: "数字员工产物"},
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
func (s *Store) ListArtifactGroups(sc Scope, projectID string) ([]model.ArtifactTaskGroup, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, ok := s.projects[projectID]
	if !ok || !s.projectVisible(sc, project) {
		return nil, transport.NotFound("project not found") // 不可见 = 不存在，防存在性泄漏
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
//
// T2.3b：改为走版本化原语 + 致命（返错）。规格 §4.4 定案：本函数在字节已写入存储**之后**被
// 调用；若失败只 warn，元数据会静默缺失 → 重启后「字节在、索引里没有本地路径」。返错让
// 调用方重试更诚实（字节重复写入由 T2.6 幂等键收敛）。失败时零副作用回滚内存。
func (s *Store) SetProjectFileLocalPath(fileID, localPath string) *transport.AppError {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.projectFiles[fileID]; !ok {
		return transport.NotFound("file not found")
	}
	return s.mutateProjectFileUnsafe(fileID, func(pf *model.ProjectFile) *transport.AppError {
		pf.LocalPath = localPath
		return nil
	})
}
