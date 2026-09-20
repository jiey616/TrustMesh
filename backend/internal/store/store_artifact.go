package store

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

// declaredSlotStrict gates whether an explicit outputName is validated against
// the step's declared output slots. It exists because the two binding routes
// had drifted apart: the manual BindStepOutput path has always refused
// undeclared names (OUTPUT_SLOT_NOT_DECLARED) and demoted displaced
// deliverables, while the agent upload path trusted whatever name the agent
// sent. 2026-09-19 画宗《双羊尊》 is the cost of that drift — the screenwriter's
// 24 process .md drafts all carried the single declared slot name and were
// filed as deliverables.
//
// Default ON. Set TRUSTMESH_DECLARED_SLOT_STRICT=false to restore the legacy
// trust-the-agent behaviour (e.g. if a workflow template declares a wrong mime
// and legitimate final deliverables start being demoted to process files).
var declaredSlotStrict = os.Getenv("TRUSTMESH_DECLARED_SLOT_STRICT") != "false"

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
	// BoundBy records how the classification was reached:
	//   "declared"              the agent supplied outputName and it passed
	//                           validation against the step's declared slots;
	//   "inferred"              the backend matched a single declared slot;
	//   "declared_slot_unknown" the agent supplied a name this step does not
	//                           declare — rejected, filed as process;
	//   "declared_mime_mismatch" the name is declared but its mime contract is
	//                           not satisfied — rejected, filed as process;
	//   ""                      not bound (no slots declared, ambiguous
	//                           multi-slot step, or no mime match).
	// The two rejection values are what lets callers explain *why* a file that
	// looked like a deliverable was not filed as one.
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
//   - an explicit outputName wins **only after it passes the same contract the
//     manual BindStepOutput path enforces**: the name must be one this step
//     declares, and the upload's mime must satisfy that slot. A name the step
//     does not declare can never be resolved by a downstream step, so trusting
//     it files an unusable deliverable; a mime mismatch means the upload is not
//     the artifact the slot describes (2026-09-04: an .md screenplay filed as
//     the docx deliverable; 2026-09-19: 24 .md drafts filed as one slot).
//     Rejections are handed back as boundBy so the caller can explain them.
//     A step declaring no slots keeps the legacy free-form behaviour — there is
//     nothing to validate against.
//   - a step declaring exactly one slot whose mime matches is bound
//     automatically;
//   - a step declaring several slots is never auto-bound — which slot a file
//     belongs to is genuinely ambiguous there (the 军旅 workflow declares two
//     xlsx slots on one step), and misfiling a draft as the final deliverable
//     is harder to notice than leaving it unbound;
//   - a single slot whose mime does not match is left unbound with a warning;
//     an upload with unknown mime (empty after normalisation) also fails to
//     match — call sites should backfill mime via InferMimeFromName first.
//
// See declaredSlotStrict for the kill switch.
func inferOutputBinding(artifact model.TaskArtifact, declared []model.StepOutput) (outputName, boundBy string, unboundWarn bool) {
	if artifact.OutputName != "" {
		if len(declared) == 0 || !declaredSlotStrict {
			return artifact.OutputName, "declared", false
		}
		slot, ok := declaredSlotByName(declared, artifact.OutputName)
		if !ok {
			return "", "declared_slot_unknown", true
		}
		if !mimeMatchesDeclared(slot.MimeType, artifact.MimeType) {
			return "", "declared_mime_mismatch", true
		}
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

// declaredSlotByName finds the declared output slot carrying the given name.
// Names are compared after trimming so a template that pads its slot name does
// not silently reject a valid binding.
func declaredSlotByName(declared []model.StepOutput, name string) (model.StepOutput, bool) {
	want := strings.TrimSpace(name)
	for _, d := range declared {
		if strings.TrimSpace(d.Name) == want {
			return d, true
		}
	}
	return model.StepOutput{}, false
}

// stepForTodoUnsafe resolves which workflow step a todo belongs to, so the
// store can read that step's declared output slots without going through the
// clawSynapse webhook. The alignment is the same ordered role/agent match used
// by applyWorkflowReviewFlags (workflow.go) and by
// clawsynapse.stepIndexForTodo — deliberately mirrored rather than
// re-invented, because a todo that aligns differently here than on upload
// would validate an upload against the wrong step's slots.
//
// Returns ok=false when the task carries no workflow snapshot, the todo has no
// match, or there is nothing to match on (todo without assignee and step
// without explicit AgentID) — callers must treat that as "no declared slots"
// and fall back to free-form naming.
//
// Caller must hold at least s.mu.RLock.
func (s *Store) stepForTodoUnsafe(task *model.TaskDetail, todo *model.Todo) (model.WorkflowStep, bool) {
	if task == nil || todo == nil || task.Workflow == nil || len(task.Workflow.Steps) == 0 {
		return model.WorkflowStep{}, false
	}
	sorted := make([]model.Todo, len(task.Todos))
	copy(sorted, task.Todos)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	cur := 0
	for ti := 0; ti < len(sorted) && cur < len(task.Workflow.Steps); ti++ {
		step := task.Workflow.Steps[cur]
		if !stepMatchesTodo(step, sorted[ti], s.assigneeRoleUnsafe(sorted[ti])) {
			continue
		}
		if sorted[ti].ID == todo.ID {
			return step, true
		}
		cur++
	}
	return model.WorkflowStep{}, false
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
	if !ok || !s.agentCanWriteTaskUnsafe(task, agent) {
		return ArtifactFilingResult{}, transport.NotFound("task not found")
	}

	// Verify todo belongs to the task if specified.
	// 多租户阶段 1：交付物归属跟随任务。
	if artifact.OrgID == "" {
		artifact.OrgID = s.personalOrgOfUnsafe(task.UserID)
	}

	var ownerTodo *model.Todo
	// orphan marks a deliverable that landed after its todo had already
	// reached a terminal state (see model.TaskArtifact.Orphan).
	orphan := false
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
				//
				// Terminal todos no longer reject: an agent that keeps working
				// after its todo was failed still produces the REAL deliverable
				// (2026-09-10 TD_06: failed 23:08, 4/6 videos delivered 00:08).
				// Dropping it left a failed todo carrying a done deliverable
				// with nothing able to reconcile the two, so accept it and mark
				// it as a late arrival instead.
				switch task.Todos[i].Status {
				case "done", "failed", "canceled":
					orphan = true
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
		// 绑定成功 = deliverable_unbound 规则不再满足 → 进入观察期，
		// 由扫描器在 OpsResolveObserve 期满后自动关闭工单。
		s.markOpsIncidentClearedUnsafe(model.RuleDeliverableUnbound, artifact.TaskID, artifact.TodoID, time.Now().UTC())
	} else {
		artifact.Kind = "process"
	}
	// Must be set before persistArtifactUnsafe so the late-arrival flag
	// reaches MongoDB, not just the in-memory copy.
	artifact.Orphan = orphan

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

		if orphan {
			late := fmt.Sprintf("交付物迟到：%s 的 todo 已处于终态，该产物被标记为 orphan（可用 reopen 回补）", artifact.FileName)
			s.addEventUnsafe(task.UserID, task.ProjectID, artifact.TaskID, artifact.TodoID,
				"system", "artifact_gate", "平台", "artifact_bound_after_terminal", &late,
				map[string]any{
					"transfer_id": artifact.TransferID,
					"file_name":   artifact.FileName,
					"output_name": artifact.OutputName,
					"task_title":  task.Title,
					"orphan":      true,
				}, now)
		}

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
		// An output slot holds exactly ONE deliverable. Remember what it was
		// bound to before, so the superseded revision can be demoted below —
		// the manual BindStepOutput path has always done this, and without it
		// every revision an agent uploads keeps kind=deliverable, so the
		// pipeline fallback branch lists them all (2026-09-19 画宗《双羊尊》:
		// 24 revisions of a single slot all surfaced as deliverables).
		displaced := ""
		replacedOut := false
		for i := range ownerTodo.Outputs {
			if ownerTodo.Outputs[i].OutputName == artifact.OutputName {
				displaced = ownerTodo.Outputs[i].ArtifactID
				ownerTodo.Outputs[i] = bound
				replacedOut = true
				break
			}
		}
		if !replacedOut {
			ownerTodo.Outputs = append(ownerTodo.Outputs, bound)
		}
		if displaced != "" && displaced != artifact.TransferID {
			s.demoteDisplacedDeliverableUnsafe(task, ownerTodo.ID, artifact.TransferID, displaced)
		}
		// P-03: receiving a declared deliverable is PRODUCTIVE progress — it must
		// refresh the hard-deadline gate even if the agent never sent
		// todo.progress (an agent that uploads files but reports nothing would
		// otherwise be failed as "no progress").
		progressAt := time.Now().UTC()
		ownerTodo.LastActivityAt = &progressAt
		ownerTodo.LastProgressAt = &progressAt
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

// demoteDisplacedDeliverableUnsafe demotes the deliverable a slot was bound to
// before being rebound, plus any redundant duplicate artifact pointing at the
// same physical file, once no output slot references them any more.
//
// It mirrors the manual BindStepOutput path (workflow.go) so both binding
// routes converge on one invariant: an output slot carries exactly one
// deliverable. Without it, re-uploading a slot leaves every superseded
// revision marked kind=deliverable, and stepOutputsUnsafe's fallback branch
// (which collects deliverables by TodoID) lists them all on the pipeline.
//
// keepTransferID must be the artifact that just claimed the slot — it is never
// demoted even when it happens to share a file with the displaced one.
// Caller must hold s.mu.
func (s *Store) demoteDisplacedDeliverableUnsafe(task *model.TaskDetail, todoID, keepTransferID, displacedTransferID string) {
	if task == nil || displacedTransferID == "" {
		return
	}
	arts := s.taskArtifacts[task.ID]
	displacedFile := ""
	for i := range arts {
		if arts[i].TransferID == displacedTransferID {
			displacedFile = arts[i].ProjectFileID
			break
		}
	}
	for i := range arts {
		if arts[i].TransferID == keepTransferID {
			continue
		}
		// 降级资格按「正向交付物」口径：kind=deliverable，或 kind 为空但带
		// output_name 的存量数据（kind 字段晚于绑定出现，不能反向排除）。
		if !artifactDeliverableLike(&arts[i]) || arts[i].TodoID != todoID {
			continue
		}
		sameFile := displacedFile != "" && arts[i].ProjectFileID == displacedFile
		if !sameFile && arts[i].TransferID != displacedTransferID {
			continue
		}
		// An artifact still bound by another slot must not be touched — it is
		// a live deliverable elsewhere on the task.
		if outputArtifactReferenced(task, arts[i].TransferID) {
			continue
		}
		arts[i].Kind = model.ArtifactKindProcess
		arts[i].OutputName = ""
		if err := s.persistArtifactUnsafe(&arts[i]); err != nil && s.log != nil {
			s.log.Warn("failed to demote displaced artifact",
				zap.String("transfer_id", arts[i].TransferID),
				zap.String("displaced_by", keepTransferID),
				zap.Error(err))
		}
	}
	// §4.1（2026-09-20）：artifact 降级必须镜像到项目文件树 —— 被顶掉的文件若还挂着
	// 「交付」标签，文件树与任务结果页会各说各话，标签要旧到下次重传才收敛。
	s.demoteLinkedProjectFileUnsafe(task, displacedFile)
}

// artifactDeliverableLike 是前端 isDeliverable 的后端镜像（正向判定）：
// kind=deliverable，或 kind 为空但 output_name 非空的存量记录。写成
// `kind != deliverable ⇒ 过程文件` 会把 kind 字段出现之前的绑定记录误降级。
func artifactDeliverableLike(a *model.TaskArtifact) bool {
	if a == nil {
		return false
	}
	if a.Kind == model.ArtifactKindDeliverable {
		return true
	}
	return a.Kind == "" && strings.TrimSpace(a.OutputName) != ""
}

// demoteLinkedProjectFileUnsafe 把 artifact 降级镜像到它指向的 ProjectFile，
// 让文件树的「交付/过程」标签与任务结果页保持一致（§4.1，2026-09-20）。
//
// 只有当该文件在**两条引用链**上都不再有活绑定时才清标签：
//  1. artifact.project_file_id（同物理文件的 keeper 冗余副本仍交付 ⇒ 保留）
//  2. todo.outputs[].file_ref（输出位仍引用该文件 ⇒ 保留；流水线进度面板的
//     steps outputs 引用即由此派生，保住这条链即保住全部下游取数）
//
// 2026-09-20 生产回填实测：只查 artifact 链会把 keeper 的 file_ref 文件误降。
func (s *Store) demoteLinkedProjectFileUnsafe(task *model.TaskDetail, projectFileID string) {
	if task == nil || projectFileID == "" {
		return
	}
	for i := range s.taskArtifacts[task.ID] {
		a := &s.taskArtifacts[task.ID][i]
		if a.ProjectFileID == projectFileID && artifactDeliverableLike(a) {
			return
		}
	}
	for i := range task.Todos {
		for _, o := range task.Todos[i].Outputs {
			if o.FileRef == projectFileID {
				return
			}
		}
	}
	pf, ok := s.projectFiles[projectFileID]
	if !ok || (pf.Kind != model.ArtifactKindDeliverable && pf.OutputName == "") {
		return
	}
	pf.Kind = model.ArtifactKindProcess
	pf.OutputName = ""
	// 与 BindStepOutput 的项目文件标签写点同款 warn-only 取舍（T2.3b）：只是清一个
	// 归类标签，不涉及记录存在性；写失败短期分叉，下次成功写自动收敛。
	if appErr := s.persistProjectFileUnsafe(pf); appErr != nil && s.log != nil {
		s.log.Warn("failed to demote displaced project file",
			zap.String("project_file_id", pf.ID),
			zap.String("task_id", task.ID), zap.Error(appErr))
	}
}

// promoteLinkedProjectFileUnsafe 是 demoteLinkedProjectFileUnsafe 的镜像：手工绑定
// 成功后把 artifact 指向的 ProjectFile 标成交付物。没有这一步，一份历史上被建成
// 「过程文件」的 artifact 在手工绑定后文件树仍显示「过程」（2026-09-20 生产回填的
// 4 条反向失配皆源于此）。
func (s *Store) promoteLinkedProjectFileUnsafe(artifact *model.TaskArtifact, outputName string) {
	if artifact == nil || artifact.ProjectFileID == "" {
		return
	}
	pf, ok := s.projectFiles[artifact.ProjectFileID]
	if !ok || (pf.Kind == model.ArtifactKindDeliverable && pf.OutputName == outputName) {
		return
	}
	pf.Kind = model.ArtifactKindDeliverable
	pf.OutputName = outputName
	// 同上：标签补写走 warn-only（T2.3b 对 BindStepOutput 标签写点的既定取舍）。
	if appErr := s.persistProjectFileUnsafe(pf); appErr != nil && s.log != nil {
		s.log.Warn("failed to persist project file kind",
			zap.String("file_id", pf.ID),
			zap.String("transfer_id", artifact.TransferID), zap.Error(appErr))
	}
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
		if !s.agentCanWriteTaskUnsafe(task, agent) || taskArtifactsClosed(task.Status) {
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
//
// 三条收敛口径（2026-09-19，与另外两个绑定入口对齐）：
//  1. **名字必须在步骤声明的输出位里**，否则 400 OUTPUT_SLOT_NOT_DECLARED —— 与
//     手工 BindStepOutput（workflow.go）逐字同款。理由不是"更严"，而是下游解析：
//     下游步骤按 (步骤, output_name) 取上一步产物，拼出来的名字取不到就是一条
//     断链，而且要到下游真正执行时才暴露。步骤未声明输出位时保持自由命名
//     （无从校验），与 inferOutputBinding 的无声明分支一致。
//  2. **mime 与声明位不符只告警、不拒绝**：本入口的存在意义之一就是"agent 上传
//     mime 意外"的救场，兄弟实现 BindStepOutput 也不比对 mime；但它是"人明确
//     点了一个位"的动作，偏离契约必须留痕（s.log.Warn）。
//  3. **一个输出位只挂一个交付物**：覆盖旧绑定时把被顶掉的旧交付物（及其同一
//     物理文件的冗余副本）降级为过程文件，否则 stepOutputsUnsafe 的兜底分支
//     会继续把旧的列出来，pipeline 同一位置同时出现新旧两份。
//
// 用 declaredSlotStrict 开关吗？**不**。该开关是为 agent 上传路径准备的 ——
// 那条路违规是"静默降级"，需要一个一键回退把行为还原成信任 agent。手工路径
// 违规是可读的 400，且前端 TaskResultView 的下拉只喂声明位名字（不会产生非法
// 名字），另一条手工路径 BindStepOutput 也从不带开关。两条手工路径保持同一
// 口径比"多一个开关"更不容易出错。
func (s *Store) BindArtifactOutput(sc Scope, taskID, todoID, transferID, outputName string) (*model.TaskArtifact, *transport.AppError) {
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
	if !ok || !visibleToScope(sc, task.OrgID, task.UserID) {
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

	ownerTodo := &task.Todos[todoIdx]

	// 口径 1：名字必须在步骤声明的输出位里。
	// 步骤解析不到（任务无 workflow 快照 / todo 对不上步骤）⇒ 视为"未声明输出位"，
	// 沿用自由命名，避免把存量数据与手工救场一并挡死。
	var slot model.StepOutput
	hasDeclaredSlots := false
	if step, ok := s.stepForTodoUnsafe(task, ownerTodo); ok && len(step.Outputs) > 0 {
		hasDeclaredSlots = true
		found, ok := declaredSlotByName(step.Outputs, outputName)
		if !ok {
			names := make([]string, 0, len(step.Outputs))
			for _, o := range step.Outputs {
				names = append(names, o.Name)
			}
			return nil, transport.BadRequest("OUTPUT_SLOT_NOT_DECLARED",
				fmt.Sprintf("步骤「%s」声明的输出位为 %s，不接受「%s」", step.Name, strings.Join(names, "、"), outputName))
		}
		slot = found
	}

	arts[artIdx].OutputName = outputName
	arts[artIdx].Kind = model.ArtifactKindDeliverable
	// 手工绑定是显式意图，绝不算「终态后迟到」（与 BindStepOutput 同款）。
	arts[artIdx].Orphan = false

	// 口径 2：mime 偏离声明位只留痕，不拦截（救场场景需要它）。
	if hasDeclaredSlots {
		effectiveMime := strings.TrimSpace(arts[artIdx].MimeType)
		if effectiveMime == "" {
			effectiveMime = InferMimeFromName(arts[artIdx].FileName)
		}
		if !mimeMatchesDeclared(slot.MimeType, effectiveMime) && s.log != nil {
			s.log.Warn("manual bind: artifact mime does not match the declared slot",
				zap.String("task_id", taskID),
				zap.String("todo_id", ownerTodo.ID),
				zap.String("transfer_id", transferID),
				zap.String("output_name", outputName),
				zap.String("slot_mime", slot.MimeType),
				zap.String("artifact_mime", effectiveMime),
				zap.String("file_name", arts[artIdx].FileName))
		}
	}

	bound := model.TodoOutput{
		OutputName: outputName,
		ArtifactID: transferID,
		FileRef:    arts[artIdx].ProjectFileID,
	}
	// 口径 3：覆盖旧绑定时记住被顶掉的交付物，稍后降级。
	displaced := ""
	replaced := false
	for i := range ownerTodo.Outputs {
		if ownerTodo.Outputs[i].OutputName == outputName {
			displaced = ownerTodo.Outputs[i].ArtifactID
			ownerTodo.Outputs[i] = bound
			replaced = true
			break
		}
	}
	if !replaced {
		ownerTodo.Outputs = append(ownerTodo.Outputs, bound)
	}
	if displaced != "" && displaced != arts[artIdx].TransferID {
		s.demoteDisplacedDeliverableUnsafe(task, ownerTodo.ID, arts[artIdx].TransferID, displaced)
	}
	// 文件树同步：手工绑定成功 ⇒ 该 artifact 指向的 ProjectFile 标成交付物
	//（镜像 BindStepOutput 的标签写点；否则历史过程文件绑定后文件树仍显示「过程」）。
	s.promoteLinkedProjectFileUnsafe(&arts[artIdx], outputName)
	// P-03: a successful manual binding is productive progress too.
	progressAt := time.Now().UTC()
	ownerTodo.LastActivityAt = &progressAt
	ownerTodo.LastProgressAt = &progressAt

	if err := s.persistArtifactUnsafe(&arts[artIdx]); err != nil && s.log != nil {
		s.log.Warn("failed to persist bound artifact",
			zap.String("transfer_id", transferID), zap.Error(err))
	}
	if err := s.persistTaskUnsafe(task); err != nil && s.log != nil {
		s.log.Warn("failed to persist todo output binding",
			zap.String("task_id", taskID), zap.String("output_name", outputName), zap.Error(err))
	}
	s.publishTaskUnsafe(taskID)

	// 手工绑定交付物成功 = deliverable_unbound 规则不再满足 → 进入观察期
	//（与自动绑定路径 SaveArtifactWithFiling 同款处理，第 2 批遗留缺口）。
	s.markOpsIncidentClearedUnsafe(model.RuleDeliverableUnbound, taskID, todoID, time.Now().UTC())

	clone := arts[artIdx]
	return &clone, nil
}
