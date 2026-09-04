package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"trustmesh/backend/internal/config"
	"trustmesh/backend/internal/logger"
	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/store"
)

func clawsynapsePeersResponse(t *testing.T, nodeIDs ...string) string {
	t.Helper()

	items := make([]map[string]any, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		items = append(items, map[string]any{
			"nodeId":     nodeID,
			"lastSeenMs": time.Now().UnixMilli(),
		})
	}

	body, err := json.Marshal(map[string]any{
		"ok":      true,
		"code":    "OK",
		"message": "ok",
		"data": map[string]any{
			"items": items,
		},
		"ts": 1,
	})
	if err != nil {
		t.Fatalf("marshal peers response: %v", err)
	}
	return string(body)
}

func clawsynapseHealthResponse(t *testing.T, nodeID string) string {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"ok":      true,
		"code":    "health.ok",
		"message": "ok",
		"data": map[string]any{
			"self": map[string]any{
				"nodeId": nodeID,
			},
			"peersCount": 0,
			"adapter": map[string]any{
				"name":    "webhook",
				"healthy": true,
			},
			"nats": map[string]any{
				"name":      "nats",
				"serverUrl": "nats://127.0.0.1:4222",
				"connected": true,
				"status":    "CONNECTED",
			},
		},
		"ts": 1,
	})
	if err != nil {
		t.Fatalf("marshal health response: %v", err)
	}
	return string(body)
}

func TestHappyPathAuthToPlanningTask(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/peers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(clawsynapsePeersResponse(t, "node-pm-001")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "user@example.com",
		"name":     "User",
		"password": "StrongPass123!",
	})
	if registerResp.StatusCode != http.StatusCreated {
		t.Fatalf("register status=%d", registerResp.StatusCode)
	}
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	if token == "" {
		t.Fatal("empty register token")
	}

	agentResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/agents", token, map[string]any{
		"node_id":      "node-pm-001",
		"name":         "PM Agent",
		"role":         "pm",
		"description":  "PM",
		"capabilities": []string{"plan"},
	})
	if agentResp.StatusCode != http.StatusCreated {
		t.Fatalf("create agent status=%d", agentResp.StatusCode)
	}
	agentData := decodeBody(t, agentResp)
	agentID := nestedString(agentData, "data", "id")
	if agentID == "" {
		t.Fatal("empty agent id")
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: "node-pm-001", LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	projectResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/projects", token, map[string]any{
		"name":        "TrustMesh MVP",
		"description": "demo",
		"pm_agent_id": agentID,
	})
	if projectResp.StatusCode != http.StatusCreated {
		t.Fatalf("create project status=%d", projectResp.StatusCode)
	}
	projectData := decodeBody(t, projectResp)
	projectID := nestedString(projectData, "data", "id")
	if projectID == "" {
		t.Fatal("empty project id")
	}
	if nestedString(projectData, "data", "task_summary", "work_status") != "empty" {
		t.Fatalf("unexpected initial project work status: %s", nestedString(projectData, "data", "task_summary", "work_status"))
	}
	if nestedFloat(projectData, "data", "task_summary", "task_total") != 0 {
		t.Fatalf("unexpected initial project task count: %v", nestedFloat(projectData, "data", "task_summary", "task_total"))
	}

	planningResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/projects/"+projectID+"/tasks/planning", token, map[string]any{
		"content": "我需要一个登录功能",
	})
	if planningResp.StatusCode != http.StatusCreated {
		t.Fatalf("create planning task status=%d", planningResp.StatusCode)
	}
	planningData := decodeBody(t, planningResp)
	if nestedString(planningData, "data", "status") != "planning" {
		t.Fatalf("unexpected planning task status: %s", nestedString(planningData, "data", "status"))
	}

	taskResp := doJSON(t, testServer.Client(), "GET", testServer.URL+"/api/v1/projects/"+projectID+"/tasks", token, nil)
	if taskResp.StatusCode != http.StatusOK {
		t.Fatalf("list tasks status=%d", taskResp.StatusCode)
	}
	taskData := decodeBody(t, taskResp)
	if nestedFloat(taskData, "meta", "count") != 1 {
		t.Fatalf("unexpected task count: %v", nestedFloat(taskData, "meta", "count"))
	}
}

func TestCreateAgentRejectsOfflineNode(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/peers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(clawsynapsePeersResponse(t, "node-online-001")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "offline-node@example.com",
		"name":     "Offline Node User",
		"password": "StrongPass123!",
	})
	token := nestedString(decodeBody(t, registerResp), "data", "access_token")

	agentResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/agents", token, map[string]any{
		"node_id":      "node-offline-001",
		"name":         "Offline Agent",
		"role":         "developer",
		"description":  "Developer",
		"capabilities": []string{"backend"},
	})
	if agentResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("create offline agent status=%d", agentResp.StatusCode)
	}

	body := decodeBody(t, agentResp)
	if nestedString(body, "error", "code") != "VALIDATION_ERROR" {
		t.Fatalf("unexpected error code: %#v", body)
	}
	if nestedString(body, "error", "details", "node_id") != "offline_or_not_found" {
		t.Fatalf("unexpected error details: %#v", body)
	}
}

func TestInvitePromptUsesNodeIDFromClawSynapseHealth(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	const localNodeID = "n1-2f4c6e8a0b1d3f557799aabbccddeeff"

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(clawsynapseHealthResponse(t, localNodeID)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "invite@example.com",
		"name":     "Invite User",
		"password": "StrongPass123!",
	})
	token := nestedString(decodeBody(t, registerResp), "data", "access_token")

	resp := doJSON(t, testServer.Client(), "GET", testServer.URL+"/api/v1/agents/invite-prompt", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("invite prompt status=%d", resp.StatusCode)
	}

	body := decodeBody(t, resp)
	if nestedString(body, "data", "node_id") != localNodeID {
		t.Fatalf("unexpected node_id: %#v", body)
	}

	prompt := nestedString(body, "data", "prompt")
	if !strings.Contains(prompt, "--target "+localNodeID) {
		t.Fatalf("invite prompt does not use local node id from health: %s", prompt)
	}
}

func TestApproveJoinRequestParsesAgentIDAndReturnsPersistedAgent(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/peers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(clawsynapsePeersResponse(t)))
		case "/v1/trust/pending":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"items":[]},"ts":1}`))
		case "/v1/auth/challenge", "/v1/trust/approve":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","ts":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "approve@example.com",
		"name":     "Approve User",
		"password": "StrongPass123!",
	})
	token := nestedString(decodeBody(t, registerResp), "data", "access_token")

	jr, appErr := application.Store.CreateJoinRequest(store.CreateJoinRequestInput{
		TrustRequestID: "trust-req-001",
		NodeID:         "node-remote-001",
		Name:           "Remote Agent",
		Description:    "remote",
		Role:           "developer",
		Capabilities:   []string{"task"},
		AgentProduct:   "openclaw",
	})
	if appErr != nil {
		t.Fatalf("create join request: %v", appErr)
	}

	approveResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/agents/join-requests/"+jr.ID+"/approve", token, map[string]any{})
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve join request status=%d", approveResp.StatusCode)
	}

	listResp := doJSON(t, testServer.Client(), "GET", testServer.URL+"/api/v1/agents/join-requests", token, nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list join requests status=%d", listResp.StatusCode)
	}
	listBody := decodeBody(t, listResp)
	items, ok := nestedMap(listBody, "data")["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one join request, got %#v", listBody)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected join request item: %#v", items[0])
	}
	if item["approved_trustmesh_agent_id"] == "" {
		t.Fatalf("expected approved_trustmesh_agent_id to be set, got %#v", item)
	}
}

func TestCreatePlanningTaskPublishesInitialPMBrief(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	var (
		mu              sync.Mutex
		publishRequests []map[string]any
	)

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/peers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(clawsynapsePeersResponse(t, "node-pm-001", "node-dev-001", "node-review-001")))
		case "/v1/publish":
			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode publish payload: %v", err)
			}
			mu.Lock()
			publishRequests = append(publishRequests, payload)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"targetNode":"node-pm-001","messageId":"msg-1"},"ts":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "brief@example.com",
		"name":     "Brief User",
		"password": "StrongPass123!",
	})
	token := nestedString(decodeBody(t, registerResp), "data", "access_token")

	pmResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/agents", token, map[string]any{
		"node_id":      "node-pm-001",
		"name":         "PM Agent",
		"role":         "pm",
		"description":  "PM",
		"capabilities": []string{"plan"},
	})
	pmID := nestedString(decodeBody(t, pmResp), "data", "id")

	devResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/agents", token, map[string]any{
		"node_id":      "node-dev-001",
		"name":         "Backend Agent",
		"role":         "developer",
		"description":  "backend",
		"capabilities": []string{"backend", "auth"},
	})
	_ = nestedString(decodeBody(t, devResp), "data", "id")

	reviewerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/agents", token, map[string]any{
		"node_id":      "node-review-001",
		"name":         "Reviewer Agent",
		"role":         "reviewer",
		"description":  "review",
		"capabilities": []string{"review", "qa"},
	})
	_ = nestedString(decodeBody(t, reviewerResp), "data", "id")

	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: "node-pm-001", LastSeenAt: time.Now().UTC()},
		{NodeID: "node-dev-001", LastSeenAt: time.Now().UTC()},
		{NodeID: "node-review-001", LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	projectResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/projects", token, map[string]any{
		"name":        "TrustMesh MVP",
		"description": "multi-agent task orchestration",
		"pm_agent_id": pmID,
	})
	projectID := nestedString(decodeBody(t, projectResp), "data", "id")

	conversationResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/projects/"+projectID+"/tasks/planning", token, map[string]any{
		"content": "实现一个带邮箱密码登录和退出能力的认证功能",
	})
	if conversationResp.StatusCode != http.StatusCreated {
		t.Fatalf("create planning task status=%d", conversationResp.StatusCode)
	}

	mu.Lock()
	if len(publishRequests) != 1 {
		t.Fatalf("expected 1 publish request, got %d", len(publishRequests))
	}
	publishReq := publishRequests[0]
	mu.Unlock()

	if publishReq["targetNode"] != "node-pm-001" {
		t.Fatalf("unexpected targetNode: %v", publishReq["targetNode"])
	}
	if publishReq["type"] != "task.message" {
		t.Fatalf("unexpected publish type: %v", publishReq["type"])
	}

	rawMessage, ok := publishReq["message"].(string)
	if !ok || rawMessage == "" {
		t.Fatalf("publish message missing: %#v", publishReq["message"])
	}

	var message map[string]any
	if err := json.Unmarshal([]byte(rawMessage), &message); err != nil {
		t.Fatalf("unmarshal publish message: %v", err)
	}

	if message["is_initial_message"] != true {
		t.Fatalf("expected is_initial_message=true, got %#v", message["is_initial_message"])
	}
	if message["user_content"] != "实现一个带邮箱密码登录和退出能力的认证功能" {
		t.Fatalf("unexpected user_content: %#v", message["user_content"])
	}

	content, ok := message["content"].(string)
	if !ok {
		t.Fatalf("content is not string: %#v", message["content"])
	}
	if content == message["user_content"] {
		t.Fatalf("content should differ from user_content for initial message, got: %s", content)
	}
	if !strings.Contains(content, "tm-task-plan") {
		t.Fatalf("content should reference tm-task-plan skill, got: %s", content)
	}

	candidateAgents, ok := message["candidate_agents"].([]any)
	if !ok || len(candidateAgents) != 2 {
		t.Fatalf("unexpected candidate_agents: %#v", message["candidate_agents"])
	}

	if _, exists := message["pm_brief"]; exists {
		t.Fatalf("pm_brief should be removed, got: %#v", message["pm_brief"])
	}
	if message["schema_version"] != "1.0" {
		t.Fatalf("schema_version expected 1.0, got: %#v", message["schema_version"])
	}
}

func TestCancelTaskEndpointCancelsTaskAndRejectsLateUpdates(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/peers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(clawsynapsePeersResponse(t, "node-pm-001", "node-dev-001")))
		case "/v1/publish":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"messageId":"msg-cancel"},"ts":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "cancel@example.com",
		"name":     "Cancel User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "PM", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := application.Store.CreateAgent(userID, "node-dev-001", "Developer", "developer", "Dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Cancel Project", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need login")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}
	task, appErr := application.Store.FinalizePlanByPMNode(pm.NodeID, "msg-cancel-finalize", store.TaskPlanReadyInput{
		TaskID:      planTask.ID,
		Title:       "Implement login",
		Description: "Support email/password login",
		Todos: []store.TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: dev.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("finalize plan: %v", appErr)
	}

	cancelResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/tasks/"+task.ID+"/cancel", token, map[string]any{
		"reason": "manual stop",
	})
	if cancelResp.StatusCode != http.StatusOK {
		t.Fatalf("cancel task status=%d", cancelResp.StatusCode)
	}
	cancelData := decodeBody(t, cancelResp)
	if nestedString(cancelData, "data", "status") != "canceled" {
		t.Fatalf("unexpected cancel response: %#v", cancelData)
	}
	if nestedString(cancelData, "data", "cancel_reason") != "manual stop" {
		t.Fatalf("unexpected cancel reason: %#v", cancelData)
	}

	_, appErr = application.Store.UpdateTodoProgressByNode(dev.NodeID, store.TodoProgressInput{
		TaskID:  task.ID,
		TodoID:  "todo-1",
		Message: "late update",
	})
	if appErr == nil || appErr.Code != "TASK_CANCELED" {
		t.Fatalf("expected TASK_CANCELED after endpoint cancel, got %#v", appErr)
	}
}

func TestDispatchTodoPublishesAssignmentToAssignee(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	var (
		mu              sync.Mutex
		publishRequests []map[string]any
	)

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/publish":
			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode publish payload: %v", err)
			}
			mu.Lock()
			publishRequests = append(publishRequests, payload)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"targetNode":"node-dev-001","messageId":"msg-1"},"ts":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "dispatch@example.com",
		"name":     "Dispatch User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "PM", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	_, appErr = application.Store.CreateAgent(userID, "node-dev-001", "Developer", "developer", "Dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create developer: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: "node-pm-001", LastSeenAt: time.Now().UTC()},
		{NodeID: "node-dev-001", LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Dispatch Project", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need login")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}
	task, appErr := application.Store.FinalizePlanByPMNode(pm.NodeID, "msg-dispatch-finalize", store.TaskPlanReadyInput{
		TaskID:      planTask.ID,
		Title:       "Implement login",
		Description: "Support email/password login",
		Todos: []store.TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: "node-dev-001",
			},
		},
	})
	if appErr != nil {
		t.Fatalf("finalize plan: %v", appErr)
	}

	dispatchResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/tasks/"+task.ID+"/todos/todo-1/dispatch", token, nil)
	if dispatchResp.StatusCode != http.StatusOK {
		t.Fatalf("dispatch todo status=%d", dispatchResp.StatusCode)
	}
	dispatchData := decodeBody(t, dispatchResp)
	if nestedString(dispatchData, "data", "status") != "in_progress" {
		t.Fatalf("unexpected dispatched task status: %#v", dispatchData)
	}
	todos, ok := dispatchData["data"].(map[string]any)["todos"].([]any)
	if !ok || len(todos) != 1 {
		t.Fatalf("unexpected dispatched todos payload: %#v", dispatchData)
	}
	dispatchedTodo, ok := todos[0].(map[string]any)
	if !ok || dispatchedTodo["status"] != "in_progress" {
		t.Fatalf("unexpected dispatched todo payload: %#v", dispatchData)
	}
	if nestedString(dispatchData, "data", "result", "summary") == "" {
		t.Fatalf("expected in-progress task result summary, got %#v", dispatchData)
	}

	mu.Lock()
	if len(publishRequests) != 1 {
		t.Fatalf("expected 1 publish request, got %d", len(publishRequests))
	}
	publishReq := publishRequests[0]
	mu.Unlock()

	if publishReq["targetNode"] != "node-dev-001" {
		t.Fatalf("unexpected targetNode: %#v", publishReq["targetNode"])
	}
	if publishReq["type"] != "todo.assigned" {
		t.Fatalf("unexpected publish type: %#v", publishReq["type"])
	}
	if publishReq["sessionKey"] != task.ID {
		t.Fatalf("unexpected session key: %#v", publishReq["sessionKey"])
	}

	rawMessage, ok := publishReq["message"].(string)
	if !ok || rawMessage == "" {
		t.Fatalf("publish message missing: %#v", publishReq["message"])
	}
	var message map[string]any
	if err := json.Unmarshal([]byte(rawMessage), &message); err != nil {
		t.Fatalf("unmarshal publish message: %v", err)
	}
	if message["task_id"] != task.ID || message["todo_id"] != "todo-1" {
		t.Fatalf("unexpected dispatch payload: %#v", message)
	}

	eventsResp := doJSON(t, testServer.Client(), "GET", testServer.URL+"/api/v1/tasks/"+task.ID+"/events", token, nil)
	if eventsResp.StatusCode != http.StatusOK {
		t.Fatalf("list events status=%d", eventsResp.StatusCode)
	}
	eventsData := decodeBody(t, eventsResp)
	items, ok := eventsData["data"].(map[string]any)["items"].([]any)
	if !ok {
		t.Fatalf("unexpected events payload: %#v", eventsData)
	}
	assignedCount := 0
	for _, item := range items {
		event, ok := item.(map[string]any)
		if ok && event["event_type"] == "todo_assigned" {
			assignedCount++
		}
	}
	if assignedCount != 1 {
		t.Fatalf("expected 1 todo_assigned event, got %d", assignedCount)
	}
}

func TestAddTaskCommentPublishesMentionsToTaskParticipants(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	var (
		mu              sync.Mutex
		publishRequests []map[string]any
	)

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/publish":
			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode publish payload: %v", err)
			}
			mu.Lock()
			publishRequests = append(publishRequests, payload)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"messageId":"msg-comment"},"ts":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "task-comment@example.com",
		"name":     "Task Comment User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "PM", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := application.Store.CreateAgent(userID, "node-dev-001", "Developer", "developer", "Dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	if _, appErr = application.Store.CreateAgent(userID, "node-other-001", "Other Agent", "developer", "Other", []string{"misc"}); appErr != nil {
		t.Fatalf("create other agent: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Mention Project", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need auth work")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}
	task, appErr := application.Store.FinalizePlanByPMNode(pm.NodeID, "msg-mention-finalize", store.TaskPlanReadyInput{
		TaskID:      planTask.ID,
		Title:       "Implement login",
		Description: "Support email/password login",
		Todos: []store.TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: dev.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("finalize plan: %v", appErr)
	}

	commentResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/tasks/"+task.ID+"/comments", token, map[string]any{
		"content": "请 @PM Agent 和 @Developer 一起看看",
		"mentions": []map[string]any{
			{"agent_id": pm.ID},
			{"agent_id": dev.ID},
		},
	})
	if commentResp.StatusCode != http.StatusCreated {
		t.Fatalf("add comment status=%d", commentResp.StatusCode)
	}

	body := decodeBody(t, commentResp)
	if nestedString(body, "data", "comment", "content") != "请 @PM Agent 和 @Developer 一起看看" {
		t.Fatalf("unexpected comment response: %#v", body)
	}

	mentionDeliveries, ok := body["data"].(map[string]any)["mention_deliveries"].([]any)
	if !ok || len(mentionDeliveries) != 2 {
		t.Fatalf("unexpected mention deliveries: %#v", body)
	}

	mu.Lock()
	if len(publishRequests) != 2 {
		t.Fatalf("expected 2 publish requests, got %d", len(publishRequests))
	}
	firstReq := publishRequests[0]
	secondReq := publishRequests[1]
	mu.Unlock()

	if firstReq["type"] != "task.mention" || secondReq["type"] != "task.mention" {
		t.Fatalf("unexpected publish types: %#v / %#v", firstReq["type"], secondReq["type"])
	}

	targetNodes := map[string]bool{
		firstReq["targetNode"].(string):  true,
		secondReq["targetNode"].(string): true,
	}
	if !targetNodes["node-pm-001"] || !targetNodes["node-dev-001"] {
		t.Fatalf("unexpected target nodes: %#v", targetNodes)
	}

	rawMessage, ok := firstReq["message"].(string)
	if !ok || rawMessage == "" {
		t.Fatalf("first publish message missing: %#v", firstReq)
	}

	var message map[string]any
	if err := json.Unmarshal([]byte(rawMessage), &message); err != nil {
		t.Fatalf("unmarshal publish message: %v", err)
	}
	if message["task_id"] != task.ID {
		t.Fatalf("unexpected task mention payload: %#v", message)
	}
	if message["comment_id"] == "" {
		t.Fatalf("comment_id missing from task mention payload: %#v", message)
	}
}

func TestDispatchTodoDoesNotPublishForArchivedProject(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	var (
		mu              sync.Mutex
		publishRequests []map[string]any
	)

	clawServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/publish":
			defer r.Body.Close()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode publish payload: %v", err)
			}
			mu.Lock()
			publishRequests = append(publishRequests, payload)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"code":"OK","message":"ok","data":{"targetNode":"node-dev-001","messageId":"msg-1"},"ts":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer clawServer.Close()

	application, err := New(config.Config{
		Port:               "0",
		JWTSecret:          "test-secret",
		AccessTokenTTL:     time.Hour,
		RefreshTokenTTL:    168 * time.Hour,
		LogLevel:           "error",
		AllowAllCORS:       true,
		ReadTimeout:        3 * time.Second,
		ShutdownGrace:      3 * time.Second,
		ClawSynapseAPIURL:  clawServer.URL,
		ClawSynapseTimeout: time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "archived-dispatch@example.com",
		"name":     "Archived Dispatch User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "PM", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	_, appErr = application.Store.CreateAgent(userID, "node-dev-001", "Developer", "developer", "Dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create developer: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: "node-pm-001", LastSeenAt: time.Now().UTC()},
		{NodeID: "node-dev-001", LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Archived Dispatch Project", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need login")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}
	task, appErr := application.Store.FinalizePlanByPMNode(pm.NodeID, "msg-archived-finalize", store.TaskPlanReadyInput{
		TaskID:      planTask.ID,
		Title:       "Implement login",
		Description: "Support email/password login",
		Todos: []store.TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: "node-dev-001",
			},
		},
	})
	if appErr != nil {
		t.Fatalf("finalize plan: %v", appErr)
	}
	if _, appErr := application.Store.ArchiveProject(store.Scope{UserID: userID}, project.ID); appErr != nil {
		t.Fatalf("archive project: %v", appErr)
	}

	dispatchResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/tasks/"+task.ID+"/todos/todo-1/dispatch", token, nil)
	if dispatchResp.StatusCode != http.StatusConflict {
		t.Fatalf("dispatch todo status=%d", dispatchResp.StatusCode)
	}
	dispatchData := decodeBody(t, dispatchResp)
	if nestedString(dispatchData, "error", "code") != "PROJECT_ARCHIVED" {
		t.Fatalf("unexpected dispatch error payload: %#v", dispatchData)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(publishRequests) != 0 {
		t.Fatalf("expected no publish request for archived project, got %d", len(publishRequests))
	}
}

func TestUserRealtimeStreamPushesDomainEvents(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	application, err := New(config.Config{
		Port:            "0",
		JWTSecret:       "test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 168 * time.Hour,
		LogLevel:        "error",
		AllowAllCORS:    true,
		ReadTimeout:     3 * time.Second,
		ShutdownGrace:   3 * time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "user-stream@example.com",
		"name":     "Realtime User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "PM", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := application.Store.CreateAgent(userID, "node-dev-001", "Developer", "developer", "Dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Realtime Project", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need login flow")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}

	streamResp := openSSE(t, testServer.Client(), testServer.URL+"/api/v1/events/stream", token)
	defer streamResp.Body.Close()
	reader := bufio.NewReader(streamResp.Body)

	if _, appErr := application.Store.AppendPMTaskReply(pm.NodeID, planTask.ID, "先确认一下登录方式", nil); appErr != nil {
		t.Fatalf("append pm task reply: %v", appErr)
	}

	notificationCreated := readUserStreamEventOfType(t, reader, "notification.created")
	if nestedString(notificationCreated, "payload", "notification", "category") != "task" {
		t.Fatalf("unexpected notification.created payload: %#v", notificationCreated)
	}

	taskUpdated := readUserStreamEventMatching(t, reader, func(payload map[string]any) bool {
		return nestedString(payload, "type") == "task.updated" &&
			nestedString(payload, "payload", "task", "id") == planTask.ID &&
			nestedString(payload, "payload", "task", "status") == "planning"
	})
	messages, ok := taskUpdated["payload"].(map[string]any)["task"].(map[string]any)["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected planning task messages payload: %#v", taskUpdated)
	}

	task, appErr := application.Store.FinalizePlanByPMNode(pm.NodeID, "msg-realtime-finalize", store.TaskPlanReadyInput{
		TaskID:      planTask.ID,
		Title:       "Implement login",
		Description: "Support email/password login",
		Todos: []store.TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: dev.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("finalize plan: %v", appErr)
	}

	if _, appErr := application.Store.UpdateTodoProgressByNode(dev.NodeID, store.TodoProgressInput{
		TaskID:  task.ID,
		TodoID:  "todo-1",
		Message: "Started implementing auth endpoints",
	}); appErr != nil {
		t.Fatalf("update todo progress: %v", appErr)
	}

	taskEventCreated := readUserStreamEventOfType(t, reader, "task.event.created")
	if nestedString(taskEventCreated, "payload", "task_id") != task.ID {
		t.Fatalf("unexpected task.event.created payload: %#v", taskEventCreated)
	}

	taskInProgress := readUserStreamEventMatching(t, reader, func(payload map[string]any) bool {
		return nestedString(payload, "type") == "task.updated" &&
			nestedString(payload, "payload", "task", "id") == task.ID &&
			nestedString(payload, "payload", "task", "status") == "in_progress"
	})
	if nestedString(taskInProgress, "payload", "task", "status") != "in_progress" {
		t.Fatalf("unexpected task.updated payload: %#v", taskInProgress)
	}

	if _, appErr := application.Store.AddTaskComment(userID, task.ID, store.TaskCommentInput{
		TaskID:  task.ID,
		Content: "Please add refresh token support as well",
	}); appErr != nil {
		t.Fatalf("add task comment: %v", appErr)
	}

	taskCommentCreated := readUserStreamEventOfType(t, reader, "task.comment.created")
	if nestedString(taskCommentCreated, "payload", "comment", "content") != "Please add refresh token support as well" {
		t.Fatalf("unexpected task.comment.created payload: %#v", taskCommentCreated)
	}

	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	agentStatusChanged := readUserStreamEventOfType(t, reader, "agent.status.changed")
	if nestedString(agentStatusChanged, "payload", "agent", "id") != dev.ID {
		t.Fatalf("unexpected agent.status.changed payload: %#v", agentStatusChanged)
	}
	if nestedString(agentStatusChanged, "payload", "agent", "status") != "offline" {
		t.Fatalf("unexpected agent status payload: %#v", agentStatusChanged)
	}
}

func TestUserRealtimeStreamPushesNotificationReadLifecycle(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	application, err := New(config.Config{
		Port:            "0",
		JWTSecret:       "test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 168 * time.Hour,
		LogLevel:        "error",
		AllowAllCORS:    true,
		ReadTimeout:     3 * time.Second,
		ShutdownGrace:   3 * time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "notification-stream@example.com",
		"name":     "Notification User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "PM", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Notification Project", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need notification coverage")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}

	streamResp := openSSE(t, testServer.Client(), testServer.URL+"/api/v1/events/stream", token)
	defer streamResp.Body.Close()
	reader := bufio.NewReader(streamResp.Body)

	if _, appErr := application.Store.AppendPMTaskReply(pm.NodeID, planTask.ID, "收到，我来分析", nil); appErr != nil {
		t.Fatalf("append pm task reply: %v", appErr)
	}
	created := readUserStreamEventOfType(t, reader, "notification.created")
	notificationID := nestedString(created, "payload", "notification", "id")
	if notificationID == "" {
		t.Fatalf("missing notification id in created payload: %#v", created)
	}

	readResp := doJSON(t, testServer.Client(), http.MethodPatch, testServer.URL+"/api/v1/notifications/"+notificationID+"/read", token, nil)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("mark read status=%d", readResp.StatusCode)
	}
	readEvent := readUserStreamEventOfType(t, reader, "notification.read")
	if nestedString(readEvent, "payload", "notification_id") != notificationID {
		t.Fatalf("unexpected notification.read payload: %#v", readEvent)
	}

	if _, appErr := application.Store.AppendPMTaskReply(pm.NodeID, planTask.ID, "补充一个细节", nil); appErr != nil {
		t.Fatalf("append second pm task reply: %v", appErr)
	}
	secondCreated := readUserStreamEventOfType(t, reader, "notification.created")
	secondID := nestedString(secondCreated, "payload", "notification", "id")
	if secondID == "" {
		t.Fatalf("missing second notification id: %#v", secondCreated)
	}

	allReadResp := doJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/v1/notifications/mark-all-read", token, nil)
	if allReadResp.StatusCode != http.StatusOK {
		t.Fatalf("mark all read status=%d", allReadResp.StatusCode)
	}
	allReadEvent := readUserStreamEventOfType(t, reader, "notifications.all_read")
	ids, ok := allReadEvent["payload"].(map[string]any)["notification_ids"].([]any)
	if !ok || len(ids) == 0 {
		t.Fatalf("unexpected notifications.all_read payload: %#v", allReadEvent)
	}
	found := false
	for _, id := range ids {
		if id == secondID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected notifications.all_read payload to include %s: %#v", secondID, allReadEvent)
	}
}

func TestGetTaskArtifactContent(t *testing.T) {
	log, err := logger.New("error")
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer func() { _ = log.Sync() }()

	application, err := New(config.Config{
		Port:            "0",
		JWTSecret:       "test-secret",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 168 * time.Hour,
		LogLevel:        "error",
		AllowAllCORS:    true,
		ReadTimeout:     3 * time.Second,
		ShutdownGrace:   3 * time.Second,
	}, log)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() {
		if closeErr := application.Close(); closeErr != nil {
			t.Fatalf("close app: %v", closeErr)
		}
	}()

	testServer := httptest.NewServer(application.Engine)
	defer testServer.Close()

	registerResp := doJSON(t, testServer.Client(), "POST", testServer.URL+"/api/v1/auth/register", "", map[string]any{
		"email":    "artifact-content@example.com",
		"name":     "Artifact Content User",
		"password": "StrongPass123!",
	})
	registerData := decodeBody(t, registerResp)
	token := nestedString(registerData, "data", "access_token")
	userID := nestedString(registerData, "data", "user", "id")

	pm, appErr := application.Store.CreateAgent(userID, "node-pm-001", "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	dev, appErr := application.Store.CreateAgent(userID, "node-dev-001", "Developer", "developer", "dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create dev: %v", appErr)
	}
	application.Store.SyncAgentPresence([]store.AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: dev.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())

	project, appErr := application.Store.CreateProject(store.Scope{UserID: userID}, "Artifacts", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	planTask, appErr := application.Store.CreateTaskPlanning(store.Scope{UserID: userID}, project.ID, "Need content")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}
	task, appErr := application.Store.FinalizePlanByPMNode(pm.NodeID, "msg-artifact-finalize", store.TaskPlanReadyInput{
		TaskID:      planTask.ID,
		Title:       "Deliver document",
		Description: "Upload markdown guide",
		Todos: []store.TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Write guide",
				Description:    "Create markdown file",
				AssigneeNodeID: dev.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("finalize plan: %v", appErr)
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "guide-*.md")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	content := "# Git 指南\n\n你好，世界"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	// Save artifact via the new transfer.received path.
	appErr = application.Store.SaveArtifact(model.TaskArtifact{
		TransferID: "tf_markdown_1",
		TaskID:     task.ID,
		TodoID:     "todo-1",
		FileName:   "git-guide.md",
		FileSize:   int64(len(content)),
		LocalPath:  tmpFile.Name(),
		MimeType:   "text/markdown",
		FromNodeID: dev.NodeID,
		CreatedAt:  time.Now(),
	})
	if appErr != nil {
		t.Fatalf("save artifact: %v", appErr)
	}

	resp := doJSON(t, testServer.Client(), "GET", testServer.URL+"/api/v1/tasks/"+task.ID+"/artifacts/tf_markdown_1/content", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get content status=%d", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read content body: %v", err)
	}
	if string(body) != content {
		t.Fatalf("unexpected content body: %s", string(body))
	}
	if got := resp.Header.Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", got)
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), `filename="git-guide.md"`) {
		t.Fatalf("unexpected content disposition: %s", resp.Header.Get("Content-Disposition"))
	}
}

func doJSON(t *testing.T, client *http.Client, method, url, token string, payload any) *http.Response {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatalf("encode payload: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func openSSE(t *testing.T, client *http.Client, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new sse request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open sse request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open sse status=%d", resp.StatusCode)
	}
	return resp
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) (string, map[string]any) {
	t.Helper()

	type result struct {
		event string
		data  map[string]any
		err   error
	}

	done := make(chan result, 1)
	go func() {
		eventName := "message"
		dataLines := make([]string, 0, 4)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- result{err: err}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &payload); err != nil {
			done <- result{err: err}
			return
		}
		done <- result{event: eventName, data: payload}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("read sse event: %v", res.err)
		}
		return res.event, res.data
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sse event")
		return "", nil
	}
}

func readUserStreamEventOfType(t *testing.T, reader *bufio.Reader, eventType string) map[string]any {
	t.Helper()

	return readUserStreamEventMatching(t, reader, func(payload map[string]any) bool {
		return nestedString(payload, "type") == eventType
	})
}

func readUserStreamEventMatching(t *testing.T, reader *bufio.Reader, match func(payload map[string]any) bool) map[string]any {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, payload := readSSEEvent(t, reader)
		if match(payload) {
			return payload
		}
	}

	t.Fatal("timed out waiting for matching user stream event")
	return nil
}

func decodeBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return out
}

func nestedString(m map[string]any, keys ...string) string {
	cur := any(m)
	for _, key := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = obj[key]
		if !ok {
			return ""
		}
	}
	v, ok := cur.(string)
	if !ok {
		return ""
	}
	return v
}

func nestedFloat(m map[string]any, keys ...string) float64 {
	cur := any(m)
	for _, key := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return -1
		}
		cur, ok = obj[key]
		if !ok {
			return -1
		}
	}
	v, ok := cur.(float64)
	if !ok {
		return -1
	}
	return v
}

func nestedMap(m map[string]any, keys ...string) map[string]any {
	cur := any(m)
	for _, key := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = obj[key]
		if !ok {
			return nil
		}
	}
	v, _ := cur.(map[string]any)
	return v
}
