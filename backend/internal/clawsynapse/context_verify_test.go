package clawsynapse

import (
	"strings"
	"testing"
	"time"

	"trustmesh/backend/internal/agentfile"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/protocol"
)

// TestSeparation_UserUploadOnlyInAttachedFiles verifies the core contract:
// user-uploaded files travel ONLY through AttachedFiles (source=user_upload),
// while predecessor todo products travel ONLY through PriorResults, and an
// agent_artifact NEVER leaks into AttachedFiles.
func TestSeparation_UserUploadOnlyInAttachedFiles(t *testing.T) {
	task := makeTask([]model.Todo{
		makeTodo("t1", 1, "done", "node-A", "AgentA", model.TodoResult{Summary: "Done A"}),
		makeTodo("t2", 2, "pending", "node-B", "AgentB", model.TodoResult{}),
	})
	// A user-uploaded file at the task level.
	task.AttachedFiles = []model.TaskAttachedFile{
		{ID: "pf_user1", FileName: "需求文档.pdf", FileSize: 204800, MimeType: "application/pdf", Source: "user_upload"},
	}
	// No task.Artifacts so buildPriorResult's store loop is skipped (nil store ok).

	h := &WebhookHandler{}
	payload := h.buildTodoAssignedPayload(task, &task.Todos[1]) // t2, first-time agent

	// 1) AttachedFiles must contain exactly the user upload, source=user_upload.
	if len(payload.AttachedFiles) != 1 {
		t.Fatalf("AttachedFiles count = %d, want 1", len(payload.AttachedFiles))
	}
	if payload.AttachedFiles[0].Source != "user_upload" {
		t.Errorf("AttachedFiles[0].Source = %q, want %q", payload.AttachedFiles[0].Source, "user_upload")
	}
	if payload.AttachedFiles[0].FileName != "需求文档.pdf" {
		t.Errorf("AttachedFiles[0].FileName = %q", payload.AttachedFiles[0].FileName)
	}
	// 2) GUARD: no agent_artifact may ever appear in AttachedFiles.
	for _, f := range payload.AttachedFiles {
		if f.Source == "agent_artifact" {
			t.Errorf("agent_artifact leaked into AttachedFiles: %+v", f)
		}
	}
	// 3) Predecessor todo must be delivered via PriorResults, not AttachedFiles.
	if len(payload.PriorResults) != 1 {
		t.Fatalf("PriorResults count = %d, want 1", len(payload.PriorResults))
	}
	if payload.PriorResults[0].TodoID != "t1" {
		t.Errorf("PriorResults[0].TodoID = %q, want %q", payload.PriorResults[0].TodoID, "t1")
	}
}

// TestArtifactDownloadURLGeneration verifies that predecessor-product files
// (source=agent_artifact) get a valid signed download URL through the exact
// same machinery buildPriorResult uses (agentfile.EnrichWithDownloadURLs).
func TestArtifactDownloadURLGeneration(t *testing.T) {
	arts := []model.TaskAttachedFile{
		{ID: "pf_art1", FileName: "设计稿.png", FileSize: 10240, MimeType: "image/png", Source: "agent_artifact"},
	}
	refs := agentfile.EnrichWithDownloadURLs(arts, "http://example:8080", []byte("test-secret"), 10*time.Minute)
	if len(refs) != 1 {
		t.Fatalf("refs count = %d, want 1", len(refs))
	}
	dl := refs[0].DownloadUrl
	if dl == "" {
		t.Fatal("artifact download_url is empty")
	}
	if !strings.Contains(dl, "pf_art1") {
		t.Errorf("download_url missing file id: %q", dl)
	}
	if !strings.Contains(dl, "/token/") {
		t.Errorf("download_url missing token path segment: %q", dl)
	}
	if refs[0].Source != "agent_artifact" {
		t.Errorf("ref.Source = %q, want %q", refs[0].Source, "agent_artifact")
	}
}

// compile-time assertion that the payload types carry the fields we rely on.
var _ = protocol.TaskAttachedFileRef{}
var _ = protocol.TodoArtifactRef{}
