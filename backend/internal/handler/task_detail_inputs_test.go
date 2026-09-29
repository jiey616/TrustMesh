package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

// detailInputFixture builds a user-owned task whose second todo carries a
// platform-observed input snapshot, recorded through the same store method the
// dispatch path uses (so the test cannot pass against a shape the writer would
// never produce).
func detailInputFixture(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	s := store.New()
	user, appErr := s.CreateUser("detail-inputs@example.com", "Detail Inputs", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	dev, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-detail-dev", "编剧", "developer", "writer", nil)
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	pm, appErr := s.CreateAgent(store.Scope{UserID: user.ID}, "node-detail-pm", "PM", "pm", "pm", nil)
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	s.SyncAgentPresence([]store.AgentPresence{
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	proj, appErr := s.CreateProject(store.Scope{UserID: user.ID}, "任务详情输入位", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	task, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "", store.TaskCreateInput{
		ProjectID:   proj.ID,
		Title:       "任务详情输入位",
		Description: "task detail inputs",
		Todos: []store.TaskCreateTodoInput{
			{ID: "TD_01", Order: 1, Title: "资产制作", Description: "d1", AssigneeNodeID: dev.NodeID},
			{ID: "TD_02", Order: 2, Title: "分镜视频生成", Description: "d2", AssigneeNodeID: dev.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	checkedAt := time.Now().UTC()
	if appErr := s.RecordTodoInputRefs(task.ID, "TD_02", []model.TodoInput{{
		Name:       "分组视频生成提示词",
		SourceStep: "分镜分组",
		OutputName: "分组视频生成提示词",
		FileName:   "prompt.md",
		FileSize:   51382,
		State:      model.InputStateResolved,
		CheckedAt:  &checkedAt,
	}}); appErr != nil {
		t.Fatalf("record input refs: %v", appErr)
	}
	return s, user.ID, task.ID
}

// getTaskDetail drives the real handler through a real gin context, so the
// assertion runs against the bytes the browser actually receives rather than
// against a struct in a test binary.
func getTaskDetail(t *testing.T, s *store.Store, userID, taskID string) (int, string) {
	t.Helper()
	h := NewTaskHandler(s, nil, nil, "", nil, 0, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", userID)
	c.Params = gin.Params{{Key: "id", Value: taskID}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID, nil)

	h.Get(c)
	return w.Code, w.Body.String()
}

// todoInputItem digs the inputs array of one todo out of an already-decoded task
// detail payload, returning the raw maps so the test can judge the actual keys.
func todoInputItem(t *testing.T, body, todoID string) []map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, body)
	}
	data, ok := raw["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %s", body)
	}
	todos, ok := data["todos"].([]any)
	if !ok {
		t.Fatalf("response has no todos array: %s", body)
	}
	for _, tv := range todos {
		tm, _ := tv.(map[string]any)
		if tm["id"] != todoID {
			continue
		}
		items, _ := tm["inputs"].([]any)
		out := make([]map[string]any, 0, len(items))
		for _, iv := range items {
			im, _ := iv.(map[string]any)
			out = append(out, im)
		}
		return out
	}
	t.Fatalf("todo %s not found in response: %s", todoID, body)
	return nil
}

// TestTaskDetailExposesInputsWithoutCredentials is the endpoint-level pair for
// the model-level JSON guard. Both halves matter:
//
//   - Positive: before this change the API never returned `inputs` at all, so a
//     broken upstream link was invisible to the user — the entire point of the
//     change is that the task page can now show it.
//   - Reverse: the payload must still never carry the signed download URL or
//     the internal file handle. Asserted key-by-key on the decoded response,
//     not by grepping the whole body, because other parts of the payload
//     legitimately contain URL-ish fields.
func TestTaskDetailExposesInputsWithoutCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID := detailInputFixture(t)

	status, body := getTaskDetail(t, s, userID, taskID)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}

	items := todoInputItem(t, body, "TD_02")
	if len(items) != 1 {
		t.Fatalf("TD_02 inputs = %d, want 1 (body=%s)", len(items), body)
	}
	item := items[0]
	if item["state"] != model.InputStateResolved {
		t.Errorf("state = %v, want %q", item["state"], model.InputStateResolved)
	}
	if item["name"] != "分组视频生成提示词" {
		t.Errorf("name = %v", item["name"])
	}
	if got, ok := item["file_size"].(float64); !ok || int64(got) != 51382 {
		t.Errorf("file_size = %v, want 51382", item["file_size"])
	}

	keys := make([]string, 0, len(item))
	for k := range item {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"checked_at", "file_name", "file_size", "name", "output_name", "source_step", "state"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("inputs[] keys = %v, want %v\n"+
			"This shape is serialised straight from model.TodoInput to the browser.\n"+
			"download_url (signed JWT) and file_ref must never appear here.",
			keys, want)
	}
}

// TestTaskDetailOmitsInputsForLegacyTodos is the compatibility half: todos that
// have never been dispatched keep exactly the shape they had before, so no
// client has to defend against a new key that is always empty.
func TestTaskDetailOmitsInputsForLegacyTodos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, userID, taskID := detailInputFixture(t)

	status, body := getTaskDetail(t, s, userID, taskID)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, body)
	}
	if items := todoInputItem(t, body, "TD_01"); len(items) != 0 {
		t.Fatalf("a never-dispatched todo must not carry inputs, got %+v", items)
	}
}
