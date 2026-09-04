package store

import (
	"path"
	"strings"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// mimeToExt maps distinguishing fragments of a MIME type (or a bare extension)
// to a canonical extension key. Workflow templates declare output slots with
// loose short forms such as "docx" / "markdown" while agents upload full MIME
// types such as
// "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
// normalising both sides to the same key is what makes them match.
var mimeToExt = []struct {
	frag string
	ext  string
}{
	{"wordprocessingml.document", "docx"},
	{"spreadsheetml.sheet", "xlsx"},
	{"presentationml.presentation", "pptx"},
	{"opendocument.text", "odt"},
	{"opendocument.spreadsheet", "ods"},
	{"text/markdown", "md"},
	{"markdown", "md"},
	{"text/csv", "csv"},
	{"text/plain", "txt"},
	{"application/pdf", "pdf"},
	{"application/json", "json"},
	{"application/zip", "zip"},
	{"image/jpeg", "jpg"},
	{"image/png", "png"},
	{"video/mp4", "mp4"},
	{"x-tar", "tar"},
	{"gzip", "gz"},
}

// extToMime maps file extensions to canonical MIME types, used to backfill
// an upload whose transfer message omitted mimeType (the ClawSynapse CLI
// currently sends mimeType:"" for every file). Keeping the same canonical
// families as mimeToExt lets the inferred value compare equal to workflow
// slot declarations such as "docx" or "markdown".
var extToMime = map[string]string{
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx":     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".csv":      "text/csv",
	".txt":      "text/plain",
	".pdf":      "application/pdf",
	".json":     "application/json",
	".zip":      "application/zip",
	".jpg":      "image/jpeg",
	".jpeg":     "image/jpeg",
	".png":      "image/png",
	".mp4":      "video/mp4",
}

// InferMimeFromName returns a MIME type inferred from the file name
// extension, or "" when the extension is unknown.
func InferMimeFromName(fileName string) string {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(fileName)))
	if m, ok := extToMime[ext]; ok {
		return m
	}
	return ""
}

// normalizeMimeKey reduces a MIME type or a bare extension to a canonical
// short key so that loose declarations and full MIME types compare equal.
// Returns "" for empty input.
func normalizeMimeKey(mime string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	if m == "" {
		return ""
	}
	for _, e := range mimeToExt {
		if strings.Contains(m, e.frag) {
			return e.ext
		}
	}
	if !strings.Contains(m, "/") {
		return strings.TrimPrefix(m, ".")
	}
	// Fall back to the MIME subtype, stripping parameters, structured-syntax
	// suffixes (+json) and the legacy x- prefix.
	sub := m
	if i := strings.LastIndex(sub, "/"); i >= 0 {
		sub = sub[i+1:]
	}
	if i := strings.Index(sub, ";"); i >= 0 {
		sub = sub[:i]
	}
	if i := strings.Index(sub, "+"); i >= 0 {
		sub = sub[:i]
	}
	return strings.TrimPrefix(sub, "x-")
}

// mimeMatchesDeclared reports whether an uploaded file's MIME type satisfies a
// declared output slot. An empty declaration is a wildcard and matches
// everything. An unknown declaration value falls through as a match so a
// missing mapping never blocks a legitimate binding, but an upload with an
// unknown MIME never matches: unknown must degrade to "unbound", not silently
// claim a slot.
func mimeMatchesDeclared(declared, actual string) bool {
	if strings.TrimSpace(declared) == "" {
		return true
	}
	d, a := normalizeMimeKey(declared), normalizeMimeKey(actual)
	if d == "" {
		return true
	}
	// An unknown actual type must NOT auto-match: an upload whose MIME we
	// cannot determine is a draft/unknown file, and silently filing it into
	// the step's single slot is exactly the misclassification this package
	// must avoid (2026-09-04: .md screenplay drafts filed as the docx
	// deliverable because the agent sends mimeType:"" and empty matched all).
	if a == "" {
		return false
	}
	return d == a
}

// ArtifactFilingResult describes how an uploaded file was classified.
type ArtifactFilingResult struct {
	// OutputName is the slot the file was finally filed under; empty means the
	// file is a process artifact.
	OutputName string
	// Bound is true when the artifact was attached to a workflow output slot.
	Bound bool
	// BoundBy is "declared" when the agent supplied outputName explicitly,
	// "inferred" when the backend matched it automatically, "" otherwise.
	BoundBy string
	// UnboundWarn is true when the step declares output slots but this upload
	// could not be matched to one — the caller should surface a warning.
	UnboundWarn bool
	// DeclaredNames lists the slot names the step declares (empty when the step
	// declares none, i.e. the file is legitimately a process artifact).
	DeclaredNames []string
}

// inferOutputBinding decides which workflow output slot an upload claims.
//
// Policy (deliberately conservative):
//   - an explicit outputName always wins;
//   - a step declaring exactly one slot whose mime matches is bound
//     automatically;
//   - a step declaring several slots is never auto-bound — which slot a file
//     belongs to is genuinely ambiguous there (the 军旅 workflow declares two
//     xlsx slots on one step), and misfiling a draft as the final deliverable
//     is harder to notice than leaving it unbound;
//   - a single slot whose mime does not match is left unbound with a warning;
//     an upload with unknown mime (empty after normalisation) also fails to
//     match — call sites should backfill mime via InferMimeFromName first.
func inferOutputBinding(artifact model.TaskArtifact, declared []model.StepOutput) (outputName, boundBy string, unboundWarn bool) {
	if artifact.OutputName != "" {
		return artifact.OutputName, "declared", false
	}
	if len(declared) == 0 {
		return "", "", false
	}
	if len(declared) > 1 {
		return "", "", true
	}
	if !mimeMatchesDeclared(declared[0].MimeType, artifact.MimeType) {
		return "", "", true
	}
	return declared[0].Name, "inferred", false
}

// taskArtifactsClosed reports whether a task no longer accepts new artifacts.
// Only terminal tasks close the deliverable channel; in_progress and
// awaiting_review stay open so late final deliverables still land.
func taskArtifactsClosed(taskStatus string) bool {
	switch taskStatus {
	case "done", "failed", "canceled":
		return true
	default:
		return false
	}
}

// SaveArtifact stores a new artifact and persists it to MongoDB. It is a
// convenience wrapper around SaveArtifactWithFiling for callers that do not
// know (or care about) the step's declared output slots.
func (s *Store) SaveArtifact(artifact model.TaskArtifact) *transport.AppError {
	_, appErr := s.SaveArtifactWithFiling(artifact, nil)
	return appErr
}

// SaveArtifactWithFiling stores an artifact and classifies it against the
// output slots declared by the owning workflow step. `declared` carries those
// slots (nil when unknown); when the upload omits outputName the backend
// infers the binding instead of silently degrading the file to a process
// artifact. See inferOutputBinding for the matching policy.
func (s *Store) SaveArtifactWithFiling(artifact model.TaskArtifact, declared []model.StepOutput) (ArtifactFilingResult, *transport.AppError) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve agent from node ID.
	agent, agentErr := s.agentByNodeUnsafe(artifact.FromNodeID)
	if agentErr != nil {
		return ArtifactFilingResult{}, agentErr
	}
	artifact.FromAgentID = agent.ID
	artifact.FromAgentName = agent.Name

	// Verify the task exists.
	task, ok := s.tasks[artifact.TaskID]
	if !ok || task.UserID != agent.UserID {
		return ArtifactFilingResult{}, transport.NotFound("task not found")
	}

	// Verify todo belongs to the task if specified.
	// 多租户阶段 1：交付物归属跟随任务。
	if artifact.OrgID == "" {
		artifact.OrgID = s.personalOrgOfUnsafe(task.UserID)
	}

	var ownerTodo *model.Todo
	if artifact.TodoID != "" {
		found := false
		for i := range task.Todos {
			if task.Todos[i].ID == artifact.TodoID {
				found = true
				ownerTodo = &task.Todos[i]
				// Reject artifacts delivered to a todo of an already-closed task.
				// An agent that keeps producing after todo.complete (a common LLM
				// behaviour) would otherwise pile up duplicate deliverables.
				// Rework resets the todo to pending, so re-done todos can
				// upload again normally.
				//
				// The channel stays open while the task itself is not terminal:
				// agents routinely report todo.complete first and then attach the
				// finished deliverable (final script, rendered docx, ...). Closing
				// the door right at todo.done silently drops the real deliverable —
				// see 2026-09-02, where the finished screenplay was rejected twice
				// with TODO_ALREADY_DONE and never reached the platform.
				// Duplicates are still collapsed by (todo, file name) below.
				if task.Todos[i].Status == "done" && taskArtifactsClosed(task.Status) {
					return ArtifactFilingResult{}, transport.Conflict("TODO_ALREADY_DONE", "todo already done; deliverables are closed")
				}
				break
			}
		}
		if !found {
			return ArtifactFilingResult{}, transport.NotFound("todo not found in task")
		}
	}

	// Classify the file nature. An upload carrying an outputName is a declared
	// final deliverable bound to a workflow step output; when it is missing the
	// backend infers the binding from the step's declared slots instead of
	// blindly degrading the file to a process artifact — agents frequently omit
	// outputName because they were never told the slot name, and the resulting
	// misfile is completely silent (2026-09-02 军旅终稿 DOCX). Only deliverables
	// participate in downstream step-input resolution — see resolveStepInput.
	// Legacy artifacts persisted before this field default to process.
	outputName, boundBy, unboundWarn := inferOutputBinding(artifact, declared)
	artifact.OutputName = outputName
	if artifact.OutputName != "" {
		artifact.Kind = "deliverable"
	} else {
		artifact.Kind = "process"
	}

	// Deduplicate by transfer ID or by (todo, file name) — an agent re-uploading
	// the same deliverable under a new transfer id (a common LLM behaviour)
	// overwrites the previous artifact instead of piling up duplicates.
	existing := s.taskArtifacts[artifact.TaskID]
	replaced := false
	for i, a := range existing {
		if a.TransferID == artifact.TransferID ||
			(a.TodoID != "" && a.TodoID == artifact.TodoID && a.FileName == artifact.FileName) {
			existing[i] = artifact
			replaced = true
			break
		}
	}
	if !replaced {
		s.taskArtifacts[artifact.TaskID] = append(existing, artifact)
	}

	if err := s.persistArtifactUnsafe(&artifact); err != nil {
		if s.log != nil {
			s.log.Warn("failed to persist artifact", zap.String("transfer_id", artifact.TransferID), zap.Error(err))
		}
	}

	// Create a timeline event only for new artifacts (not overwrites).
	if !replaced {
		now := time.Now().UTC()
		content := artifact.FileName
		s.addEventUnsafe(task.UserID, task.ProjectID, artifact.TaskID, artifact.TodoID,
			"agent", agent.ID, agent.Name, "artifact_received", &content,
			map[string]any{
				"transfer_id": artifact.TransferID,
				"file_name":   artifact.FileName,
				"file_size":   artifact.FileSize,
				"mime_type":   artifact.MimeType,
				"task_title":  task.Title,
				"kind":        artifact.Kind,
				"output_name": artifact.OutputName,
			}, now)

		if err := s.persistTaskEventsUnsafe(artifact.TaskID); err != nil {
			if s.log != nil {
				s.log.Warn("failed to persist artifact event", zap.String("task_id", artifact.TaskID), zap.Error(err))
			}
		}
	}

	s.publishTaskUnsafe(artifact.TaskID)

	// Auto-create ProjectFile record for the artifact (best-effort).
	// NOTE: Call Unsafe variant because we already hold s.mu.Lock.
	var pf *model.ProjectFile
	if created, err := s.saveProjectFileFromArtifactUnsafe(artifact); err != nil {
		if s.log != nil {
			s.log.Warn("failed to auto-create project file from artifact",
				zap.String("transfer_id", artifact.TransferID),
				zap.Error(err))
		}
	} else {
		pf = created
	}

	// Link the produced project file onto the artifact so the pipeline
	// progress panel can resolve it for preview/download even when the agent
	// did not declare an output name. Done after the ProjectFile is created so
	// the real file id is captured.
	if pf != nil {
		artifact.ProjectFileID = pf.ID
	}

	// Bind the artifact to the owning todo's declared output slot when the
	// upload carried an outputName (matches a workflow StepOutput.Name). This
	// is what lets downstream steps resolve "上一个流程的输出文件". Done after
	// the ProjectFile is created so FileRef can reference its real ID.
	if ownerTodo != nil && artifact.OutputName != "" {
		bound := model.TodoOutput{
			OutputName: artifact.OutputName,
			ArtifactID: artifact.TransferID,
		}
		if pf != nil {
			bound.FileRef = pf.ID
		}
		replacedOut := false
		for i := range ownerTodo.Outputs {
			if ownerTodo.Outputs[i].OutputName == artifact.OutputName {
				ownerTodo.Outputs[i] = bound
				replacedOut = true
				break
			}
		}
		if !replacedOut {
			ownerTodo.Outputs = append(ownerTodo.Outputs, bound)
		}
		if err := s.persistTaskUnsafe(task); err != nil && s.log != nil {
			s.log.Warn("failed to persist todo output binding",
				zap.String("task_id", artifact.TaskID),
				zap.String("output_name", artifact.OutputName),
				zap.Error(err))
		}
	}

	// Persist the artifact-with-project-file link (covers the common case
	// where no output name was declared but the file must still be reachable
	// from the pipeline progress panel).
	if err := s.persistArtifactUnsafe(&artifact); err != nil && s.log != nil {
		s.log.Warn("failed to persist artifact project file link",
			zap.String("transfer_id", artifact.TransferID), zap.Error(err))
	}

	return ArtifactFilingResult{
		OutputName:    artifact.OutputName,
		Bound:         artifact.OutputName != "",
		BoundBy:       boundBy,
		UnboundWarn:   unboundWarn,
		DeclaredNames: declaredSlotNames(declared),
	}, nil
}

// declaredSlotNames extracts the raw names of a step's declared output slots.
func declaredSlotNames(declared []model.StepOutput) []string {
	if len(declared) == 0 {
		return nil
	}
	names := make([]string, 0, len(declared))
	for _, d := range declared {
		names = append(names, d.Name)
	}
	return names
}

// TransferOwner is the best-effort owner lookup result used when an agent
// uploads a file without declaring metadata.taskId.
type TransferOwner struct {
	TaskID string
	TodoID string
}

// ResolveTransferOwner finds the task+todo an agent node is currently working
// on, so an upload that forgot to declare metadata.taskId can still be filed
// instead of being dropped with a silent 422.
//
// This is the platform-side safety net for the 2026-09-02 incident where the
// screenwriter node's SOUL.md taught a bare
// `clawsynapse transfer send --target ... --file ...` with no metadata: seven
// process files landed on the transfer volume and none of them ever reached
// the artifacts collection, while the agent kept reporting success.
//
// Selection rules (deterministic, never a coin flip):
//   - only tasks owned by the same user as the agent, and only non-terminal
//     tasks (the artifact channel is closed for done/failed/canceled);
//   - only todos assigned to that agent (by agent id or node id) whose status
//     is still open (in_progress > waiting_user > awaiting_review > pending);
//   - ties are broken by the task's UpdatedAt (freshest wins).
//
// An empty result means the owner is genuinely ambiguous or absent — callers
// must then reject the upload rather than file it under a guess.
func (s *Store) ResolveTransferOwner(nodeID string) (*TransferOwner, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agent, appErr := s.agentByNodeUnsafe(nodeID)
	if appErr != nil {
		return nil, appErr
	}
	if agent == nil {
		return nil, transport.NotFound("agent not found for node")
	}

	type candidate struct {
		taskID  string
		todoID  string
		rank    int
		updated time.Time
	}
	var candidates []candidate

	for _, task := range s.tasks {
		if task.UserID != agent.UserID || taskArtifactsClosed(task.Status) {
			continue
		}
		for i := range task.Todos {
			todo := &task.Todos[i]
			owned := todo.Assignee.AgentID == agent.ID ||
				(todo.Assignee.NodeID != "" && todo.Assignee.NodeID == nodeID)
			if !owned {
				continue
			}
			rank, ok := transferTodoRank(todo.Status)
			if !ok {
				continue
			}
			candidates = append(candidates, candidate{
				taskID:  task.ID,
				todoID:  todo.ID,
				rank:    rank,
				updated: task.UpdatedAt,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, transport.NotFound("no active task for node")
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.rank < best.rank || (c.rank == best.rank && c.updated.After(best.updated)) {
			best = c
		}
	}
	if s.log != nil && len(candidates) > 1 {
		s.log.Warn("transfer owner resolved among multiple candidates",
			zap.String("node_id", nodeID),
			zap.String("task_id", best.taskID),
			zap.String("todo_id", best.todoID),
			zap.Int("candidates", len(candidates)))
	}
	return &TransferOwner{TaskID: best.taskID, TodoID: best.todoID}, nil
}

// transferTodoRank orders open todo statuses by how likely the assignee is to
// be uploading a file for them right now. Lower rank wins.
func transferTodoRank(status string) (int, bool) {
	switch status {
	case "in_progress":
		return 0, true
	case "waiting_user":
		return 1, true
	case "awaiting_review":
		return 2, true
	case "pending":
		return 3, true
	default:
		return 0, false
	}
}

// GetArtifactsByTaskID returns all artifacts for a given task.
func (s *Store) GetArtifactsByTaskID(taskID string) []model.TaskArtifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getArtifactsByTaskIDUnsafe(taskID)
}

func (s *Store) getArtifactsByTaskIDUnsafe(taskID string) []model.TaskArtifact {
	artifacts := s.taskArtifacts[taskID]
	if len(artifacts) == 0 {
		return []model.TaskArtifact{}
	}
	out := make([]model.TaskArtifact, len(artifacts))
	copy(out, artifacts)
	return out
}

// HasArtifactNamed reports whether an artifact with this exact file name is
// already filed on the task. The rejection path needs it to tell two very
// different situations apart: a file that genuinely never made it in, versus a
// re-upload of something the platform already holds. Both surface as the same
// 409 to the caller, but only the first one deserves a ⚠️ on the timeline —
// see 2026-09-03 资产提取, where four deliverables filed at 03:30 were re-sent
// at 03:49 and produced eight alarming comments for files that were sitting
// right there in the file list.
func (s *Store) HasArtifactNamed(taskID, fileName string) bool {
	taskID = strings.TrimSpace(taskID)
	fileName = strings.TrimSpace(fileName)
	if taskID == "" || fileName == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.taskArtifacts[taskID] {
		if a.FileName == fileName {
			return true
		}
	}
	return false
}

// GetArtifact returns a single artifact by task ID and transfer ID.
func (s *Store) GetArtifact(taskID, transferID string) (*model.TaskArtifact, *transport.AppError) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.taskArtifacts[taskID] {
		if a.TransferID == transferID {
			clone := a
			return &clone, nil
		}
	}
	return nil, transport.NotFound("artifact not found")
}

// fillTaskArtifactsUnsafe populates task.Artifacts from the artifact store.
// Caller must hold at least s.mu.RLock.
func (s *Store) fillTaskArtifactsUnsafe(task *model.TaskDetail) {
	if task == nil {
		return
	}
	task.Artifacts = s.getArtifactsByTaskIDUnsafe(task.ID)
}

// copyTaskWithArtifactsUnsafe returns a deep copy of the task with artifacts filled.
// Caller must hold at least s.mu.RLock.
func (s *Store) copyTaskWithArtifactsUnsafe(task *model.TaskDetail) *model.TaskDetail {
	clone := copyTask(task)
	s.fillTaskArtifactsUnsafe(clone)
	return clone
}

// BindArtifactOutput promotes an already-filed artifact to a declared
// deliverable by attaching it to one of its step's workflow output slots.
//
// This is the manual escape hatch for uploads that arrived without
// `--metadata outputName` and could not be auto-matched (ambiguous multi-slot
// steps, unexpected mime types). Without it the only remedy was re-running the
// transfer webhook by hand.
func (s *Store) BindArtifactOutput(userID, taskID, todoID, transferID, outputName string) (*model.TaskArtifact, *transport.AppError) {
	taskID = strings.TrimSpace(taskID)
	todoID = strings.TrimSpace(todoID)
	transferID = strings.TrimSpace(transferID)
	outputName = strings.TrimSpace(outputName)
	if taskID == "" || todoID == "" || transferID == "" || outputName == "" {
		return nil, transport.Validation("invalid bind payload", map[string]any{
			"task_id": "required", "todo_id": "required",
			"artifact_id": "required", "output_name": "required",
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return nil, transport.NotFound("task not found")
	}
	todoIdx := -1
	for i := range task.Todos {
		if task.Todos[i].ID == todoID {
			todoIdx = i
			break
		}
	}
	if todoIdx < 0 {
		return nil, transport.NotFound("todo not found in task")
	}
	arts := s.taskArtifacts[taskID]
	artIdx := -1
	for i := range arts {
		if arts[i].TransferID == transferID {
			artIdx = i
			break
		}
	}
	if artIdx < 0 {
		return nil, transport.NotFound("artifact not found in task")
	}

	arts[artIdx].OutputName = outputName
	arts[artIdx].Kind = model.ArtifactKindDeliverable

	ownerTodo := &task.Todos[todoIdx]
	bound := model.TodoOutput{
		OutputName: outputName,
		ArtifactID: transferID,
		FileRef:    arts[artIdx].ProjectFileID,
	}
	replaced := false
	for i := range ownerTodo.Outputs {
		if ownerTodo.Outputs[i].OutputName == outputName {
			ownerTodo.Outputs[i] = bound
			replaced = true
			break
		}
	}
	if !replaced {
		ownerTodo.Outputs = append(ownerTodo.Outputs, bound)
	}

	if err := s.persistArtifactUnsafe(&arts[artIdx]); err != nil && s.log != nil {
		s.log.Warn("failed to persist bound artifact",
			zap.String("transfer_id", transferID), zap.Error(err))
	}
	if err := s.persistTaskUnsafe(task); err != nil && s.log != nil {
		s.log.Warn("failed to persist todo output binding",
			zap.String("task_id", taskID), zap.String("output_name", outputName), zap.Error(err))
	}
	s.publishTaskUnsafe(taskID)

	clone := arts[artIdx]
	return &clone, nil
}
