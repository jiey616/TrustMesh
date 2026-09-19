package store

import (
	"testing"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// Two distinct slots, so a test can show one being rebound while the other
// still holds the same physical file.
const (
	fillingSlotA = filingTestSlotName
	fillingSlotB = "剧名_分镜头脚本"
)

// ───────────────── 一个输出位只挂一个交付物（写路径收敛） ─────────────────
//
// The manual BindStepOutput path has always demoted a displaced deliverable and
// every redundant duplicate copy of it (2026-09-11 实测 TD_06). The agent
// upload path did not, so re-uploading a slot left every superseded revision
// marked kind=deliverable and stepOutputsUnsafe's TodoID fallback listed them
// all. These tests pin the convergence.

// seedFilingTask builds a task with one open todo and a developer agent allowed
// to write to it, plus a ProjectID so each upload also materialises a
// ProjectFile (which the duplicate-copy demotion path keys off).
func seedFilingTask(t *testing.T) (*Store, string, stringAgent, string) {
	t.Helper()
	s, userID, _, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	const taskID = "t-filing-1"
	s.tasks[taskID] = &model.TaskDetail{
		ID: taskID, UserID: userID, ProjectID: project.ID,
		Status: "in_progress", Title: "交付物归档",
		Todos: []model.Todo{{ID: "td-1", Title: "剧本创作", Status: "in_progress"}},
	}
	return s, userID, developer, taskID
}

// TestSaveArtifactRebindingSlotDemotesSupersededRevision is the 画宗《双羊尊》
// reducer: 24 process drafts were uploaded against one slot, and each one
// became its own deliverable. After the fix the slot holds exactly one
// deliverable — the newest revision — and the superseded ones fall back to
// process files.
func TestSaveArtifactRebindingSlotDemotesSupersededRevision(t *testing.T) {
	s, _, developer, taskID := seedFilingTask(t)
	slot := []model.StepOutput{{Name: filingTestSlotName, MimeType: "markdown"}}

	first, appErr := s.SaveArtifactWithFiling(model.TaskArtifact{
		TransferID: "tr-draft-01", TaskID: taskID, TodoID: "td-1",
		FileName: "01-intake.md", FileSize: 1024, MimeType: "text/markdown",
		FromNodeID: developer.NodeID, OutputName: filingTestSlotName,
	}, slot)
	if appErr != nil {
		t.Fatalf("first upload: %v", appErr)
	}
	if !first.Bound || first.BoundBy != "declared" {
		t.Fatalf("first upload = bound:%v by:%q, want a declared binding", first.Bound, first.BoundBy)
	}

	second, appErr := s.SaveArtifactWithFiling(model.TaskArtifact{
		TransferID: "tr-draft-03", TaskID: taskID, TodoID: "td-1",
		FileName: "07-draft-v03.md", FileSize: 2048, MimeType: "text/markdown",
		FromNodeID: developer.NodeID, OutputName: filingTestSlotName,
	}, slot)
	if appErr != nil {
		t.Fatalf("second upload: %v", appErr)
	}
	if !second.Bound {
		t.Fatalf("second upload = %+v, want it bound to the slot", second)
	}

	// The slot carries exactly one deliverable, and it is the newest revision.
	todo := &s.tasks[taskID].Todos[0]
	if len(todo.Outputs) != 1 || todo.Outputs[0].ArtifactID != "tr-draft-03" {
		t.Fatalf("todo outputs = %+v, want exactly the newest revision", todo.Outputs)
	}

	arts := s.taskArtifacts[taskID]
	if len(arts) != 2 {
		t.Fatalf("expected 2 artifacts (both revisions kept as files), got %d", len(arts))
	}
	for _, a := range arts {
		switch a.TransferID {
		case "tr-draft-01":
			if a.Kind != model.ArtifactKindProcess || a.OutputName != "" {
				t.Fatalf("superseded revision = kind %q output %q, want process with no slot",
					a.Kind, a.OutputName)
			}
		case "tr-draft-03":
			if a.Kind != model.ArtifactKindDeliverable || a.OutputName != filingTestSlotName {
				t.Fatalf("current revision = kind %q output %q, want the deliverable binding",
					a.Kind, a.OutputName)
			}
		default:
			t.Fatalf("unexpected artifact %s", a.TransferID)
		}
	}
}

// TestDemoteDisplacedDeliverableAlsoDemotesDuplicateCopies: an agent that
// re-uploads the same file under a new transfer id leaves several artifacts
// pointing at one ProjectFile. Demoting only the recorded one would leave the
// copies listed by the TodoID fallback, so both must go.
func TestDemoteDisplacedDeliverableAlsoDemotesDuplicateCopies(t *testing.T) {
	s, _, _, taskID := seedFilingTask(t)
	s.taskArtifacts[taskID] = []model.TaskArtifact{
		{TransferID: "tr-old", TaskID: taskID, TodoID: "td-1", ProjectFileID: "pf-1",
			Kind: model.ArtifactKindDeliverable, OutputName: filingTestSlotName},
		{TransferID: "tr-old-dup", TaskID: taskID, TodoID: "td-1", ProjectFileID: "pf-1",
			Kind: model.ArtifactKindDeliverable, OutputName: filingTestSlotName},
		{TransferID: "tr-new", TaskID: taskID, TodoID: "td-1", ProjectFileID: "pf-2",
			Kind: model.ArtifactKindDeliverable, OutputName: filingTestSlotName},
	}
	task := s.tasks[taskID]
	task.Todos[0].Outputs = []model.TodoOutput{
		{OutputName: filingTestSlotName, ArtifactID: "tr-new", FileRef: "pf-2"},
	}

	s.mu.Lock()
	s.demoteDisplacedDeliverableUnsafe(task, "td-1", "tr-new", "tr-old")
	s.mu.Unlock()

	for _, a := range s.taskArtifacts[taskID] {
		switch a.TransferID {
		case "tr-old", "tr-old-dup":
			if a.Kind != model.ArtifactKindProcess || a.OutputName != "" {
				t.Fatalf("%s = kind %q output %q, want process with no slot", a.TransferID, a.Kind, a.OutputName)
			}
		case "tr-new":
			if a.Kind != model.ArtifactKindDeliverable {
				t.Fatalf("the artifact that claimed the slot must keep its deliverable kind, got %q", a.Kind)
			}
		}
	}
}

// TestDemoteDisplacedDeliverableLeavesStillReferencedArtifactAlone: the two
// artifacts point at the SAME physical file (a re-upload under a new transfer
// id) but claim different slots. Demotion may not blank a binding another slot
// still relies on, even when the file it shares is being superseded.
func TestDemoteDisplacedDeliverableLeavesStillReferencedArtifactAlone(t *testing.T) {
	s, _, _, taskID := seedFilingTask(t)
	task := s.tasks[taskID]
	s.taskArtifacts[taskID] = []model.TaskArtifact{
		{TransferID: "tr-displaced", TaskID: taskID, TodoID: "td-1", ProjectFileID: "pf-1",
			Kind: model.ArtifactKindDeliverable, OutputName: fillingSlotA},
		{TransferID: "tr-shared", TaskID: taskID, TodoID: "td-1", ProjectFileID: "pf-1",
			Kind: model.ArtifactKindDeliverable, OutputName: fillingSlotB},
	}
	// Slot B still relies on tr-shared, so that binding must survive.
	task.Todos[0].Outputs = []model.TodoOutput{
		{OutputName: fillingSlotB, ArtifactID: "tr-shared", FileRef: "pf-1"},
	}

	s.mu.Lock()
	s.demoteDisplacedDeliverableUnsafe(task, "td-1", "tr-new", "tr-displaced")
	s.mu.Unlock()

	for _, a := range s.taskArtifacts[taskID] {
		switch a.TransferID {
		case "tr-displaced":
			if a.Kind != model.ArtifactKindProcess || a.OutputName != "" {
				t.Fatalf("displaced artifact = kind %q output %q, want it demoted", a.Kind, a.OutputName)
			}
		case "tr-shared":
			if a.Kind != model.ArtifactKindDeliverable || a.OutputName != fillingSlotB {
				t.Fatalf("still-referenced artifact = kind %q output %q, want it left untouched",
					a.Kind, a.OutputName)
			}
		}
	}
}

// TestSaveArtifactRejectsUndeclaredSlotName drives the rejection through the
// real entry point: the artifact lands as a process file, nothing claims the
// slot, and the caller gets a machine-readable reason.
func TestSaveArtifactRejectsUndeclaredSlotName(t *testing.T) {
	s, _, developer, taskID := seedFilingTask(t)
	slot := []model.StepOutput{{Name: filingTestSlotName, MimeType: "docx"}}

	res, appErr := s.SaveArtifactWithFiling(model.TaskArtifact{
		TransferID: "tr-bogus-slot", TaskID: taskID, TodoID: "td-1",
		FileName: "《西游记后传：金身血裂》_剧本解析_v03.md", FileSize: 4096, MimeType: "text/markdown",
		FromNodeID: developer.NodeID, OutputName: "剧本解析",
	}, slot)
	if appErr != nil {
		t.Fatalf("upload must be accepted (as a process file), got: %v", appErr)
	}
	if res.Bound || res.OutputName != "" {
		t.Fatalf("res = %+v, want an unbound classification", res)
	}
	if res.BoundBy != "declared_slot_unknown" || !res.UnboundWarn {
		t.Fatalf("res = %+v, want declared_slot_unknown with a warning", res)
	}
	if len(res.DeclaredNames) != 1 || res.DeclaredNames[0] != filingTestSlotName {
		t.Fatalf("declared names = %v, want the step's slot listed for the warning", res.DeclaredNames)
	}

	arts := s.taskArtifacts[taskID]
	if len(arts) != 1 || arts[0].Kind != model.ArtifactKindProcess || arts[0].OutputName != "" {
		t.Fatalf("artifacts = %+v, want a single process artifact with no slot", arts)
	}
	if got := len(s.tasks[taskID].Todos[0].Outputs); got != 0 {
		t.Fatalf("todo.Outputs = %d entries, want none — an unvalidated name must not bind", got)
	}
}

// TestSaveArtifactRejectsDeclaredSlotMimeMismatch is the other rejection arm:
// right name, wrong artifact. Nothing may feed a downstream step a file the
// slot's contract does not describe.
func TestSaveArtifactRejectsDeclaredSlotMimeMismatch(t *testing.T) {
	s, _, developer, taskID := seedFilingTask(t)
	slot := []model.StepOutput{{Name: filingTestSlotName, MimeType: "docx"}}

	res, appErr := s.SaveArtifactWithFiling(model.TaskArtifact{
		TransferID: "tr-md-as-docx", TaskID: taskID, TodoID: "td-1",
		FileName: "西游记后传之劫火重燃_剧本_v1.md", FileSize: 8192, MimeType: "text/markdown",
		FromNodeID: developer.NodeID, OutputName: filingTestSlotName,
	}, slot)
	if appErr != nil {
		t.Fatalf("upload must be accepted (as a process file), got: %v", appErr)
	}
	if res.Bound || res.BoundBy != "declared_mime_mismatch" || !res.UnboundWarn {
		t.Fatalf("res = %+v, want declared_mime_mismatch with a warning", res)
	}

	arts := s.taskArtifacts[taskID]
	if len(arts) != 1 || arts[0].Kind != model.ArtifactKindProcess || arts[0].OutputName != "" {
		t.Fatalf("artifacts = %+v, want a single process artifact with no slot", arts)
	}
	if got := len(s.tasks[taskID].Todos[0].Outputs); got != 0 {
		t.Fatalf("todo.Outputs = %d entries, want none", got)
	}
}
