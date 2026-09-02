package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"trustmesh/backend/internal/clawsynapse"
	"trustmesh/backend/internal/store"
)

// helperCapabilityAgent 创建一个 Agent（走 JoinRequest 审批，可指定产品标识），返回 handler 与 agent。
func helperCapabilityAgent(t *testing.T, product string) (*AgentHandler, *store.Store, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	st := store.New()
	user, appErr := st.CreateUser("cap@example.com", "Cap", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	jr, appErr := st.CreateJoinRequest(store.CreateJoinRequestInput{
		TrustRequestID: "trust-req-cap-001",
		UserID:         user.ID,
		NodeID:         "node-cap-001",
		Name:           "Cap Agent",
		Description:    "capability test",
		Role:           "developer",
		Capabilities:   []string{"conversation"},
		AgentProduct:   product,
	})
	if appErr != nil {
		t.Fatalf("create join request: %v", appErr)
	}
	agent, appErr := st.ApproveJoinRequest(user.ID, jr.ID, store.JoinRequestOverrides{})
	if appErr != nil {
		t.Fatalf("approve join request: %v", appErr)
	}

	// 模拟旁挂 daemon 的能力查询/写回端点
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/peers/node-cap-001/capabilities" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"product":"hermes","available":true,"skills":[{"name":"tm-task-plan","description":"plan","category":"task"}],"models":[{"id":"p1","provider":"openai","model":"gpt-4o","isDefault":true}],"jobs":[{"id":"j1","name":"daily","schedule":"0 9 * * *","enabled":true,"prompt":"report"}],"reason":""},"ts":1}`))
			return
		}
		if r.URL.Path == "/v1/peers/node-cap-001/capabilities" && r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"ok":true,"target":"cron","action":"create","jobId":"j9","restartStatus":"none","error":""},"ts":1}`))
			return
		}
		if r.URL.Path == "/v1/peers/node-cap-001/cron/executions" && r.Method == http.MethodGet {
			jobID := r.URL.Query().Get("jobId")
			limit := r.URL.Query().Get("limit")
			execJSON := `[]`
			if jobID == "j1" {
				execJSON = `[{"executionId":"e1","jobId":"j1","status":"completed","startedAtMs":1785828520991,"finishedAtMs":1785828524286,"durationMs":3295,"error":"","outputFile":"/root/.hermes/cron/output/j1/2026-08-04.md","outputPreview":"# Report\\n..."}]`
			}
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"executions":` + execJSON + `,"error":""},"ts":1}`))
			_ = limit
			return
		}
		_, _ = w.Write([]byte(`{"ok":false,"code":"NOT_FOUND","message":"not found","data":{},"ts":1}`))
	}))
	t.Cleanup(server.Close)

	h := NewAgentHandler(st, clawsynapse.NewClient(server.URL, 0))
	return h, st, user.ID, agent.ID
}

// 非 hermes 节点：返回 available:false，不调用 daemon。
func TestGetCapabilitiesNonHermesReturnsUnavailable(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "openclaw")

	req := httptest.NewRequest(http.MethodGet, "/agents/"+agentID+"/capabilities", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.GetCapabilities(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	out := resp.Data
	if out.Available {
		t.Fatalf("expected available=false for non-hermes node, got %#v", out)
	}
	if out.Reason != "not a hermes node" {
		t.Fatalf("expected reason 'not a hermes node', got %q", out.Reason)
	}
}

// hermes 节点：返回 daemon 的能力清单（技能/模型/cron）。
func TestGetCapabilitiesHermesReturnsSkillsModelsJobs(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "hermes")

	req := httptest.NewRequest(http.MethodGet, "/agents/"+agentID+"/capabilities", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.GetCapabilities(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Available bool                          `json:"available"`
			Skills    []clawsynapse.CapabilitySkill `json:"skills"`
			Models    []clawsynapse.CapabilityModel `json:"models"`
			Jobs      []clawsynapse.CapabilityJob   `json:"jobs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	out := resp.Data
	if !out.Available {
		t.Fatalf("expected available=true, got %#v", out)
	}
	if len(out.Skills) != 1 || out.Skills[0].Name != "tm-task-plan" {
		t.Fatalf("unexpected skills: %#v", out.Skills)
	}
	if len(out.Models) != 1 || !out.Models[0].IsDefault {
		t.Fatalf("unexpected models: %#v", out.Models)
	}
	if len(out.Jobs) != 1 || out.Jobs[0].Name != "daily" {
		t.Fatalf("unexpected jobs: %#v", out.Jobs)
	}
}

// clawsynapse client 禁用：hermes 节点返回 available:false + client disabled。
func TestGetCapabilitiesClientDisabledDegrades(t *testing.T) {
	gin.SetMode(gin.TestMode)
	st := store.New()
	user, appErr := st.CreateUser("cap2@example.com", "Cap2", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	jr, appErr := st.CreateJoinRequest(store.CreateJoinRequestInput{
		TrustRequestID: "trust-req-cap-002",
		UserID:         user.ID,
		NodeID:         "node-cap-002",
		Name:           "Cap Agent 2",
		Description:    "capability test",
		Role:           "developer",
		Capabilities:   []string{"conversation"},
		AgentProduct:   "hermes",
	})
	if appErr != nil {
		t.Fatalf("create join request: %v", appErr)
	}
	agent, appErr := st.ApproveJoinRequest(user.ID, jr.ID, store.JoinRequestOverrides{})
	if appErr != nil {
		t.Fatalf("approve join request: %v", appErr)
	}

	h := NewAgentHandler(st, nil)

	req := httptest.NewRequest(http.MethodGet, "/agents/"+agent.ID+"/capabilities", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agent.ID}}
	c.Set("user_id", user.ID)

	h.GetCapabilities(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			Available bool   `json:"available"`
			Reason    string `json:"reason"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	out := resp.Data
	if out.Available {
		t.Fatalf("expected available=false, got %#v", out)
	}
	if out.Reason != "clawsynapse client disabled" {
		t.Fatalf("expected reason 'clawsynapse client disabled', got %q", out.Reason)
	}
}

// 非 hermes 节点：写回被拒绝（ok=false + not a hermes node）。
func TestSetCapabilitiesNonHermesRejected(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "openclaw")

	body := bytes.NewBufferString(`{"target":"cron","action":"create"}`)
	req := httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/capabilities", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.SetCapabilities(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			OK     bool   `json:"ok"`
			Error  string `json:"error"`
			Target string `json:"target"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.OK {
		t.Fatalf("expected ok=false, got %#v", resp.Data)
	}
	if resp.Data.Error != "not a hermes node" {
		t.Fatalf("expected error 'not a hermes node', got %q", resp.Data.Error)
	}
}

// hermes 节点：写回透传到 daemon，返回 cron create 结果。
func TestSetCapabilitiesHermesProxiesToDaemon(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "hermes")

	body := bytes.NewBufferString(`{"target":"cron","action":"create","job":{"name":"daily","schedule":"0 9 * * *","prompt":"report"}}`)
	req := httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/capabilities", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.SetCapabilities(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			OK            bool   `json:"ok"`
			Target        string `json:"target"`
			Action        string `json:"action"`
			JobID         string `json:"jobId"`
			RestartStatus string `json:"restartStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Data.OK {
		t.Fatalf("expected ok=true, got %#v", resp.Data)
	}
	if resp.Data.Target != "cron" || resp.Data.Action != "create" || resp.Data.JobID != "j9" {
		t.Fatalf("unexpected writeback result: %#v", resp.Data)
	}
	if resp.Data.RestartStatus != "none" {
		t.Fatalf("expected restartStatus none, got %q", resp.Data.RestartStatus)
	}
}

// 缺 target/action：返回 400 校验错误。
func TestSetCapabilitiesMissingTargetRejected(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "hermes")

	body := bytes.NewBufferString(`{"action":"create"}`)
	req := httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/capabilities", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.SetCapabilities(c)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

// hermes 节点：cron 执行历史查询返回 executions 列表（透传 daemon）。
func TestListCronExecutionsHermesReturnsExecutions(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "hermes")

	req := httptest.NewRequest(http.MethodGet, "/agents/"+agentID+"/cron/executions?jobId=j1&limit=5", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.ListCronExecutions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Executions []struct {
				ExecutionID   string `json:"executionId"`
				JobID         string `json:"jobId"`
				Status        string `json:"status"`
				DurationMs    int64  `json:"durationMs"`
				OutputPreview string `json:"outputPreview"`
			} `json:"executions"`
			Error string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(resp.Data.Executions))
	}
	ex := resp.Data.Executions[0]
	if ex.ExecutionID != "e1" || ex.JobID != "j1" || ex.Status != "completed" || ex.DurationMs != 3295 {
		t.Fatalf("unexpected execution: %#v", ex)
	}
	if resp.Data.Error != "" {
		t.Fatalf("expected no error, got %q", resp.Data.Error)
	}
}

// 非 hermes 节点：cron 执行历史返回空列表 + not a hermes node。
func TestListCronExecutionsNonHermesDegrades(t *testing.T) {
	h, _, userID, agentID := helperCapabilityAgent(t, "openclaw")

	req := httptest.NewRequest(http.MethodGet, "/agents/"+agentID+"/cron/executions", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: agentID}}
	c.Set("user_id", userID)

	h.ListCronExecutions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			Executions []any  `json:"executions"`
			Error      string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Executions) != 0 {
		t.Fatalf("expected empty executions, got %#v", resp.Data.Executions)
	}
	if resp.Data.Error != "not a hermes node" {
		t.Fatalf("expected error 'not a hermes node', got %q", resp.Data.Error)
	}
}
