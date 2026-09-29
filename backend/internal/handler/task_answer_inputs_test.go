package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/protocol"
	"trustmesh/backend/internal/store"
)

// answerInputTask mirrors the 2026-09-29 incident shape: the downstream todo (t2)
// declares an input fed by the preceding step of the SAME task, so the refs can
// be re-resolved with no store-side cross-task lookup. bound controls whether the
// predecessor todo (t1) actually carries the declared output.
func answerInputTask(bound bool) *model.TaskDetail {
	wf := &model.Workflow{
		Name: "wf",
		Steps: []model.WorkflowStep{
			{Name: "s1", Role: "编剧", Outputs: []model.StepOutput{{Name: "剧本正文"}}},
			{Name: "s2", Role: "编剧", Inputs: []model.StepInput{
				{Name: "上一段剧本", Source: model.StepIOLink{Step: "prev", Output: "剧本正文"}},
			}},
		},
	}
	task := &model.TaskDetail{
		ID:        "task-1",
		ProjectID: "proj-1",
		Title:     "Test Task",
		Workflow:  wf,
		Todos: []model.Todo{
			{
				ID: "t1", Order: 1, Status: "done",
				Assignee: model.TodoAssignee{AgentID: "agent-A", Name: "编剧", NodeID: "node-A"},
			},
			{
				ID: "t2", Order: 2, Status: "waiting_user",
				Assignee: model.TodoAssignee{AgentID: "agent-A", Name: "编剧", NodeID: "node-A"},
			},
		},
	}
	if bound {
		task.Todos[0].Outputs = []model.TodoOutput{
			{OutputName: "剧本正文", ArtifactID: "tr-1", FileRef: "pf-1"},
		}
		task.Artifacts = []model.TaskArtifact{
			{TransferID: "tr-1", TodoID: "t1", FileName: "scene1.md", FileSize: 1234, MimeType: "text/markdown"},
		}
	}
	return task
}

// publishCapture records the todo.answer envelope a fake node received. The
// server handler runs on its own goroutine, so every field is read/written
// through the mutex — the race detector runs over these tests.
type publishCapture struct {
	mu      sync.Mutex
	msgType string
	payload protocol.TodoAnswerPayload
}

func (c *publishCapture) record(msgType, message string, decodeErr *error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgType = msgType
	if err := json.Unmarshal([]byte(message), &c.payload); err != nil {
		*decodeErr = err
	}
}

func (c *publishCapture) snapshot() (string, protocol.TodoAnswerPayload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.msgType, c.payload
}

// capturePublish stands up a fake node that records the single todo.answer
// envelope the handler publishes, so the test can assert on what the agent would
// actually receive (the daemon feeds the raw payload JSON to the session).
func capturePublish(t *testing.T) (*clawsynapse.Client, *publishCapture) {
	t.Helper()
	got := &publishCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode publish request: %v", err)
		}
		var decodeErr error
		got.record(req.Type, req.Message, &decodeErr)
		if decodeErr != nil {
			t.Errorf("decode todo.answer payload: %v", decodeErr)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"targetNode":"node-A","messageId":"m1"}}`))
	}))
	t.Cleanup(srv.Close)
	return clawsynapse.NewClient(srv.URL, 5*time.Second), got
}

func publishAnswer(t *testing.T, task *model.TaskDetail) (string, protocol.TodoAnswerPayload) {
	t.Helper()
	client, got := capturePublish(t)
	h := NewTaskHandler(
		store.New(),
		client,
		clawsynapse.NewWebhookHandler(clawsynapse.WebhookDeps{}),
		"", nil, 0, nil,
	)
	h.PublishTodoAnswer(context.Background(), task, "t2", &model.TodoQuestion{
		ID:       "q1",
		Question: "本步输入位未解析，请拍板",
		Answer:   "文件已在项目文件里",
	})
	msgType, payload := got.snapshot()
	return msgType, payload
}

// TestPublishTodoAnswerCarriesResolvedInputs pins the 2026-09-29 fix: answering a
// todo.ask is the ONLY channel that reaches a parked session without
// re-dispatching, so the payload must carry freshly re-resolved step inputs.
// Without them the user can answer any number of times and the file still never
// arrives (the incident: TD_02 answered twice, agent still had no download URL).
func TestPublishTodoAnswerCarriesResolvedInputs(t *testing.T) {
	msgType, payload := publishAnswer(t, answerInputTask(true))

	if msgType != "todo.answer" {
		t.Fatalf("message type = %q, want todo.answer", msgType)
	}
	if len(payload.Inputs) != 1 {
		t.Fatalf("payload must carry the re-resolved inputs, got %+v", payload.Inputs)
	}
	in := payload.Inputs[0]
	if !in.Resolved {
		t.Fatalf("expected the input to resolve now that the source step is present: %+v", in)
	}
	if in.Name != "上一段剧本" || in.OutputName != "剧本正文" || in.SourceTodoID != "t1" {
		t.Fatalf("unexpected input ref: %+v", in)
	}
}

// TestPublishTodoAnswerCarriesUnresolvedInputs is the mirror case: when the
// source is still missing, the payload must say so explicitly (Resolved=false)
// rather than silently omitting the input — the agent needs to know WHICH input
// is missing to ask a useful question.
func TestPublishTodoAnswerCarriesUnresolvedInputs(t *testing.T) {
	msgType, payload := publishAnswer(t, answerInputTask(false))

	if msgType != "todo.answer" {
		t.Fatalf("message type = %q, want todo.answer", msgType)
	}
	if len(payload.Inputs) != 1 {
		t.Fatalf("payload must still surface the declared-but-unresolved input, got %+v", payload.Inputs)
	}
	if payload.Inputs[0].Resolved {
		t.Fatalf("expected Resolved=false, got %+v", payload.Inputs[0])
	}
}
