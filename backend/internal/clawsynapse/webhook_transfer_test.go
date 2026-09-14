package clawsynapse

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/model"
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
	dev, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-transfer-dev", "编剧智能体", "developer", "writer", nil)
	if appErr != nil {
		t.Fatalf("create agent: %v", appErr)
	}
	pm, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-transfer-pm", "PM", "pm", "pm", nil)
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(store.Scope{UserID: user.ID}, "军旅影视制作", "demo", pm.ID)
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
	h := NewWebhookHandler(WebhookDeps{Store: s})

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
	h := NewWebhookHandler(WebhookDeps{Store: s})

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
		NodeID:  devNode,
		Type:    "transfer.received",
		From:    devNode,
		Message: string(body),
		// taskId is present but points at a todo that does not exist →
		// SaveArtifact rejects and the rejection must become visible.
		Metadata: map[string]any{"taskId": taskID, "todoId": "TD_404"},
	})

	if w.Code == 200 {
		t.Fatalf("expected rejection for unknown todo, got 200")
	}
	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
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
	h := NewWebhookHandler(WebhookDeps{Store: s})

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
	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
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

// postTransferReceived drives one transfer.received delivery through the
// handler and returns the recorder, so tests can replay the same delivery the
// way the platform-side forwarder does.
func postTransferReceived(h *WebhookHandler, from, message string, metadata map[string]any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   from,
		Type:     "transfer.received",
		From:     from,
		Message:  message,
		Metadata: metadata,
	})
	return w
}

// TestTransferReceivedDuplicateStaysQuiet covers 2026-09-03 资产提取: the agent
// re-sent four deliverables that had already been filed 19 minutes earlier.
// Every re-send was rejected and each rejection wrote a ⚠️ "文件上传未入库"
// comment, so the user saw the names of deliverables that were sitting right
// there in the file list and concluded they had been lost. A rejection of a
// file the task already holds is not a loss — it must stay quiet instead of
// crying wolf.
func TestTransferReceivedDuplicateStaysQuiet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	first, err := json.Marshal(map[string]any{
		"transferId": "tid-dup-first-000001",
		"fileName":   "生死靶心_角色清单_v01.xlsx",
		"fileSize":   16737,
		"localPath":  "/var/lib/trustmesh-transfers/tid-dup-first-000001-生死靶心_角色清单_v01.xlsx",
		"mimeType":   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if w := postTransferReceived(h, devNode, string(first),
		map[string]any{"taskId": taskID, "todoId": "TD_01"}); w.Code != 200 {
		t.Fatalf("first upload should be filed, got %d body=%s", w.Code, w.Body.String())
	}
	if got := len(s.GetArtifactsByTaskID(taskID)); got != 1 {
		t.Fatalf("expected 1 filed artifact, got %d", got)
	}

	// Same file name, new transfer id, rejected on an unknown todo. The task
	// already holds a file by this name, so nothing is actually missing.
	second, err := json.Marshal(map[string]any{
		"transferId": "tid-dup-second-00001",
		"fileName":   "生死靶心_角色清单_v01.xlsx",
		"fileSize":   16737,
		"localPath":  "/var/lib/trustmesh-transfers/tid-dup-second-00001-生死靶心_角色清单_v01.xlsx",
		"mimeType":   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if w := postTransferReceived(h, devNode, string(second),
		map[string]any{"taskId": taskID, "todoId": "TD_404"}); w.Code == 200 {
		t.Fatalf("expected the duplicate to be rejected, got 200")
	}

	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	for _, cm := range comments {
		if strings.Contains(cm.Content, "文件上传未入库") {
			t.Fatalf("duplicate upload must not raise a 未入库 warning, got: %s", cm.Content)
		}
	}
}

// TestTransferRejectionWarnedOnce covers the forwarder's duplicate delivery:
// it fires transfer.received twice per transfer (measured 2026-09-03: 114
// sends for 57 distinct transfer ids in 24h), so an unguarded warning path
// wrote every rejection notice in duplicate.
func TestTransferRejectionWarnedOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferTestFixture(t)
	h := NewWebhookHandler(WebhookDeps{Store: s})

	body, err := json.Marshal(map[string]any{
		"transferId": "tid-twice-0000000001",
		"fileName":   "05-audiovisual-blueprint.md",
		"fileSize":   24254,
		"localPath":  "/var/lib/trustmesh-transfers/tid-twice-0000000001-05-audiovisual-blueprint.md",
		"mimeType":   "text/markdown",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Byte-identical replay: same transfer id, same everything.
	for i := 0; i < 2; i++ {
		w := postTransferReceived(h, devNode, string(body),
			map[string]any{"taskId": taskID, "todoId": "TD_404"})
		if w.Code == 200 {
			t.Fatalf("delivery %d: expected rejection, got 200", i)
		}
	}

	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	count := 0
	for _, cm := range comments {
		if strings.Contains(cm.Content, "文件上传未入库") &&
			strings.Contains(cm.Content, "05-audiovisual-blueprint.md") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 rejection warning for a doubly-delivered transfer, got %d", count)
	}
}

// newTransferWorkflowFixture extends the base fixture with a workflow whose
// single step is bound to the developer agent (matching the todo's assignee)
// and declares the given output slots.
func newTransferWorkflowFixture(t *testing.T, outputs []model.StepOutput) (*store.Store, string, string, string) {
	t.Helper()
	s, userID, taskID, devNode := newTransferTestFixture(t)
	var devID string
	for _, a := range s.ListAgents(store.Scope{UserID: userID}) {
		if a.NodeID == devNode {
			devID = a.ID
			break
		}
	}
	if devID == "" {
		t.Fatalf("developer agent not found for node %s", devNode)
	}
	if appErr := s.SetTaskWorkflow(taskID, &model.Workflow{
		Name: "军旅微电影流水线",
		Steps: []model.WorkflowStep{{
			Name:    "剧本创作",
			Role:    "developer",
			AgentID: devID,
			Outputs: outputs,
		}},
	}); appErr != nil {
		t.Fatalf("attach workflow: %v", appErr)
	}
	return s, userID, taskID, devNode
}

// postTransfer drives handleTransferReceived the way the platform node does.
func postTransfer(t *testing.T, h *WebhookHandler, fromNode, taskID, todoID, transferID, fileName, mime string, metadata map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"transferId": transferID,
		"fileName":   fileName,
		"fileSize":   4096,
		"localPath":  "/var/lib/trustmesh-transfers/" + transferID + "-" + fileName,
		"mimeType":   mime,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	if taskID != "" {
		metadata["taskId"] = taskID
	}
	if todoID != "" {
		metadata["todoId"] = todoID
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/clawsynapse", nil)
	h.handleTransferReceived(c, protocol.WebhookPayload{
		NodeID:   fromNode,
		Type:     "transfer.received",
		From:     fromNode,
		Message:  string(body),
		Metadata: metadata,
	})
	return w
}

const docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// TestTransferAutoBindsSingleDeclaredOutput covers the gap found on
// 2026-09-02: agents are never told the slot name (todo.assigned carried no
// outputs at all), so the final deliverable arrives without outputName and
// used to be filed as a process artifact — invisible on the workflow diagram
// and unreachable as a downstream step input. A step declaring exactly one
// matching slot must now be bound automatically.
func TestTransferAutoBindsSingleDeclaredOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_剧本类型_版本_时间", MimeType: "docx"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postTransfer(t, h, devNode, taskID, "TD_01",
		"tid-autobind-0000001", "生死靶心_微电影_剧本_v1.docx", docxMime, nil)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"bound_by":"inferred"`) {
		t.Fatalf("expected inferred binding in response, got %s", w.Body.String())
	}
	artifacts := s.GetArtifactsByTaskID(taskID)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != "deliverable" {
		t.Fatalf("kind = %q, want deliverable", artifacts[0].Kind)
	}
	if artifacts[0].OutputName != "剧名_剧本类型_版本_时间" {
		t.Fatalf("output_name = %q, want the declared slot name", artifacts[0].OutputName)
	}
	task := s.GetTaskInternal(taskID)
	if len(task.Todos[0].Outputs) != 1 || task.Todos[0].Outputs[0].OutputName != "剧名_剧本类型_版本_时间" {
		t.Fatalf("todo outputs = %+v, want the slot bound", task.Todos[0].Outputs)
	}
}

// TestTransferMultiOutputWarnsOnly pins the conservative policy: a step
// declaring several slots is never auto-bound (the 军旅 workflow declares two
// xlsx slots on one step, so guessing is genuinely unsafe). The upload stays a
// process artifact and one ⚠️ comment tells the user how to fix it.
func TestTransferMultiOutputWarnsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_导演视觉体系方案", MimeType: "markdown"},
		{Name: "剧名_分镜头脚本", MimeType: "xlsx"},
		{Name: "剧名_逐镜视频生成提示词", MimeType: "xlsx"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postTransfer(t, h, devNode, taskID, "TD_01",
		"tid-multiout-0000001", "direction-notes.md", "text/markdown", nil)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	artifacts := s.GetArtifactsByTaskID(taskID)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != "process" || artifacts[0].OutputName != "" {
		t.Fatalf("kind=%q output_name=%q, want process with no slot",
			artifacts[0].Kind, artifacts[0].OutputName)
	}
	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	found := false
	for _, cm := range comments {
		if strings.Contains(cm.Content, "疑似最终交付物未绑定") &&
			strings.Contains(cm.Content, "direction-notes.md") &&
			strings.Contains(cm.Content, "剧名_分镜头脚本") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an unbound-deliverable warning naming the declared slots, got %d comment(s)", len(comments))
	}
}

// TestUnboundWarningThrottled keeps the warning from becoming noise: an agent
// that uploads several intermediate drafts must produce exactly one warning per
// todo, not one per file.
func TestUnboundWarningThrottled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_分镜头脚本", MimeType: "xlsx"},
		{Name: "剧名_逐镜视频生成提示词", MimeType: "xlsx"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	for i, name := range []string{"draft-a.md", "draft-b.md", "draft-c.md"} {
		w := postTransfer(t, h, devNode, taskID, "TD_01",
			"tid-throttle-000000"+string(rune('1'+i)), name, "text/markdown", nil)
		if w.Code != 200 {
			t.Fatalf("upload %s: expected 200, got %d", name, w.Code)
		}
	}
	comments, appErr := s.ListTaskComments(store.Scope{UserID: userID}, taskID)
	if appErr != nil {
		t.Fatalf("list comments: %v", appErr)
	}
	warns := 0
	for _, cm := range comments {
		if strings.Contains(cm.Content, "疑似最终交付物未绑定") {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("expected exactly 1 throttled warning, got %d", warns)
	}
	if got := len(s.GetArtifactsByTaskID(taskID)); got != 3 {
		t.Fatalf("expected all 3 uploads filed, got %d", got)
	}
}

// TestMimeNormalizationMatchesLooseDeclarations guards the lookup table that
// lets loose template declarations ("docx", "markdown") match the full MIME
// types agents actually upload.
func TestMimeNormalizationMatchesLooseDeclarations(t *testing.T) {
	cases := []struct {
		name      string
		declared  string
		uploaded  string
		wantBound bool
	}{
		{
			name:      "docx declaration vs full OOXML mime",
			declared:  "docx",
			uploaded:  docxMime,
			wantBound: true,
		},
		{
			name:      "markdown declaration vs text/markdown",
			declared:  "markdown",
			uploaded:  "text/markdown",
			wantBound: true,
		},
		{
			name:      "xlsx declaration vs full spreadsheet mime",
			declared:  "xlsx",
			uploaded:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			wantBound: true,
		},
		{
			name:      "empty declaration matches anything",
			declared:  "",
			uploaded:  "application/pdf",
			wantBound: true,
		},
		{
			name:      "docx declaration vs markdown upload",
			declared:  "docx",
			uploaded:  "text/markdown",
			wantBound: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			s, _, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
				{Name: "slot", MimeType: tc.declared},
			})
			h := NewWebhookHandler(WebhookDeps{Store: s})
			w := postTransfer(t, h, devNode, taskID, "TD_01",
				"tid-mime-"+strings.ReplaceAll(tc.name, " ", "-"), "file.bin", tc.uploaded, nil)
			if w.Code != 200 {
				t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
			}
			artifacts := s.GetArtifactsByTaskID(taskID)
			if len(artifacts) != 1 {
				t.Fatalf("expected 1 artifact, got %d", len(artifacts))
			}
			gotBound := artifacts[0].Kind == "deliverable"
			if gotBound != tc.wantBound {
				t.Fatalf("bound = %v (kind=%q output_name=%q), want %v",
					gotBound, artifacts[0].Kind, artifacts[0].OutputName, tc.wantBound)
			}
		})
	}
}

// TestBuildTodoOutputsCarriesDeclaredSlots is the other half of the fix: the
// slot names must actually reach the agent on dispatch, otherwise it has
// nothing to copy into --metadata outputName.
func TestBuildTodoOutputsCarriesDeclaredSlots(t *testing.T) {
	s, _, taskID, _ := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_剧本类型_版本_时间", MimeType: "docx", Description: "最终剧本定稿"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	task := s.GetTaskInternal(taskID)
	refs := h.BuildTodoOutputs(task, &task.Todos[0])
	if len(refs) != 1 {
		t.Fatalf("expected 1 declared output ref, got %d", len(refs))
	}
	if refs[0].Name != "剧名_剧本类型_版本_时间" || refs[0].MimeType != "docx" {
		t.Fatalf("output ref = %+v, want the declared slot verbatim", refs[0])
	}

	// A task with no workflow yields nothing — the agent legitimately has no
	// slot to claim and every upload is a process file.
	s2, _, taskID2, _ := newTransferTestFixture(t)
	h2 := NewWebhookHandler(WebhookDeps{Store: s2})
	task2 := s2.GetTaskInternal(taskID2)
	if got := h2.BuildTodoOutputs(task2, &task2.Todos[0]); got != nil {
		t.Fatalf("expected nil outputs for a workflow-less task, got %+v", got)
	}
}

// TestBindArtifactOutputPromotesProcessFile covers the manual escape hatch for
// ambiguous multi-slot steps: the user picks the slot in the UI and the
// already-filed process artifact becomes a deliverable.
func TestBindArtifactOutputPromotesProcessFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID, devNode := newTransferWorkflowFixture(t, []model.StepOutput{
		{Name: "剧名_分镜头脚本", MimeType: "xlsx"},
		{Name: "剧名_逐镜视频生成提示词", MimeType: "xlsx"},
	})
	h := NewWebhookHandler(WebhookDeps{Store: s})

	w := postTransfer(t, h, devNode, taskID, "TD_01",
		"tid-manualbind-00001", "shotlist.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if before := s.GetArtifactsByTaskID(taskID); before[0].Kind != "process" {
		t.Fatalf("precondition: expected a process artifact, got %q", before[0].Kind)
	}

	bound, appErr := s.BindArtifactOutput(store.Scope{UserID: userID}, taskID, "TD_01", "tid-manualbind-00001", "剧名_分镜头脚本")
	if appErr != nil {
		t.Fatalf("bind: %v", appErr)
	}
	if bound.Kind != "deliverable" || bound.OutputName != "剧名_分镜头脚本" {
		t.Fatalf("bound artifact = kind %q output %q", bound.Kind, bound.OutputName)
	}
	task := s.GetTaskInternal(taskID)
	if len(task.Todos[0].Outputs) != 1 || task.Todos[0].Outputs[0].OutputName != "剧名_分镜头脚本" {
		t.Fatalf("todo outputs = %+v, want the slot bound", task.Todos[0].Outputs)
	}

	// Unknown artifact / slot-less payloads are rejected explicitly.
	if _, err := s.BindArtifactOutput(store.Scope{UserID: userID}, taskID, "TD_01", "tid-does-not-exist", "slot"); err == nil {
		t.Fatalf("expected an error for an unknown artifact")
	}
	if _, err := s.BindArtifactOutput(store.Scope{UserID: userID}, taskID, "TD_01", "tid-manualbind-00001", ""); err == nil {
		t.Fatalf("expected an error when output_name is missing")
	}
}
