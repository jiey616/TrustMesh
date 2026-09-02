package clawsynapse

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
)

// newTransferTestFixture builds a user + developer agent + project + task with
// a single todo assigned to that agent, mirroring the 2026-09-02 screenwriter
// task shape (single active todo owned by the uploading node).
func newTransferTestFixture(t *testing.T) (*store.Store, string, string, string) {
	t.Helper()
	s := store.New()
	user, appErr := s.CreateUser("transfer@example.com", "Transfer User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	dev, appErr := s.CreateAgent(user.ID, "node-transfer-dev", "编剧智能体", "developer", "writer", nil)
	if appErr != nil {
		t.Fatalf("create agent: %v", appErr)
	}
	pm, appErr := s.CreateAgent(user.ID, "node-transfer-pm", "PM", "pm", "pm", nil)
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(user.ID, "军旅影视制作", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	task, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "", store.TaskCreateInput{
		ProjectID:   proj.ID,
		Title:       "军旅题材微电影制作",
		Description: "transfer visibility",
		Todos: []store.TaskCreateTodoInput{
			{ID: "TD_01", Order: 1, Title: "剧本创作", Description: "draft", AssigneeNodeID: dev.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	return s, user.ID, task.ID, dev.NodeID
}

// TestTransferReceivedWithoutTaskIdInfersOwner covers the 2026-09-02 incident:
// the screenwriter node sent files with a bare
// `transfer send --target … --file …` (no --metadata), so the webhook carried
// no taskId and the upload was silently 422'd — the files sat on the transfer
// volume while the agent reported success. The platform must now infer the
// owner from the sending node's current assignment and file the artifact.
func TestTransferReceivedWithoutTaskIdInfersOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(s, nil, nil)

	body, err := json.Marshal(map[string]any{
		"transferId": "tid-no-metadata-00001",
		"fileName":   "03-direction.md",
		"fileSize":   16780,
		"localPath":  "/var/lib/trustmesh-transfers/tid-no-metadata-00001-03-direction.md",
		"mimeType":   "text/markdown",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   devNode,
		Type:     "transfer.received",
		From:     devNode,
		Message:  string(body),
		Metadata: map[string]any{}, // no taskId at all — the incident shape
	})

	if w.Code != 200 {
		t.Fatalf("expected 200 after owner inference, got %d body=%s", w.Code, w.Body.String())
	}
	artifacts := s.GetArtifactsByTaskID(taskID)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact on the inferred task, got %d", len(artifacts))
	}
	if artifacts[0].TaskID != taskID || artifacts[0].TodoID != "TD_01" {
		t.Fatalf("artifact filed under task=%q todo=%q, want task=%q todo=%q",
			artifacts[0].TaskID, artifacts[0].TodoID, taskID, "TD_01")
	}
	if artifacts[0].Kind != "process" {
		t.Fatalf("kind = %q, want process (no outputName declared)", artifacts[0].Kind)
	}
}

// TestTransferReceivedRejectionAppendsSystemComment guards the visibility half
// of the fix: any upload that still cannot be filed must leave a ⚠️ comment on
// the owning task instead of vanishing without a trace.
func TestTransferReceivedRejectionAppendsSystemComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(s, nil, nil)

	body, err := json.Marshal(map[string]any{
		"transferId": "tid-bad-todo-0000001",
		"fileName":   "04-story-structure.md",
		"fileSize":   45752,
		"localPath":  "/var/lib/trustmesh-transfers/tid-bad-todo-0000001-04-story-structure.md",
		"mimeType":   "text/markdown",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID: devNode,
		Type:   "transfer.received",
		From:   devNode,
		Message: string(body),
		// taskId is present but points at a todo that does not exist →
		// SaveArtifact rejects and the rejection must become visible.
		Metadata: map[string]any{"taskId": taskID, "todoId": "TD_404"},
	})

	if w.Code == 200 {
		t.Fatalf("expected rejection for unknown todo, got 200")
	}
	comments, appErr := s.ListTaskComments(userID, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	found := false
	for _, cm := range comments {
		if strings.Contains(cm.Content, "文件上传未入库") && strings.Contains(cm.Content, "04-story-structure.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected transfer-rejection system comment, got %d comment(s)", len(comments))
	}
	if len(s.GetArtifactsByTaskID(taskID)) != 0 {
		t.Fatalf("rejected upload must not create an artifact")
	}
}

// TestTransferReceivedBadPayloadAppendsSystemComment covers the case where the
// transfer notification body itself is unparseable (no transferId): the file
// is already on disk and unrecoverable through the API, so the timeline is the
// only place the loss can be seen.
func TestTransferReceivedBadPayloadAppendsSystemComment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(s, nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   devNode,
		Type:     "transfer.received",
		From:     devNode,
		Message:  "@/workspace/shanyu-work/drafts/full-draft.md",
		Metadata: map[string]any{"taskId": taskID},
	})

	if w.Code != 400 {
		t.Fatalf("expected 400 BAD_PAYLOAD, got %d", w.Code)
	}
	comments, appErr := s.ListTaskComments(userID, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	found := false
	for _, cm := range comments {
		if strings.Contains(cm.Content, "文件上传未入库") && strings.Contains(cm.Content, "BAD_PAYLOAD") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected bad-payload transfer system comment, got %d comment(s)", len(comments))
	}
}
