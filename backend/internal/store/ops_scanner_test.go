package store

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

// 第 2 批验收：造异常能开工单、去重生效、复发可再开、跨租户零泄漏。

func newOpsFixture(t *testing.T) (*Store, string, string, string) {
	t.Helper()
	s := New()
	ua, err := s.CreateUser("a@example.com", "ua", "hash")
	if err != nil {
		t.Fatalf("create user a: %v", err)
	}
	orgA, err := s.EnsurePersonalOrg(ua.ID, "ua")
	if err != nil {
		t.Fatalf("org a: %v", err)
	}
	ub, err := s.CreateUser("b@example.com", "ub", "hash")
	if err != nil {
		t.Fatalf("create user b: %v", err)
	}
	orgB, err := s.EnsurePersonalOrg(ub.ID, "ub")
	if err != nil {
		t.Fatalf("org b: %v", err)
	}
	return s, orgA.ID, orgB.ID, ua.ID
}

// 造一个执行态任务 + 一个停滞 todo（LastActivityAt 在 threshold 之前）。
func seedStalledTask(s *Store, orgID, userID string) (taskID, todoID string) {
	stale := time.Now().UTC().Add(-2 * time.Hour)
	started := stale
	taskID = "t-ops-" + orgID
	todoID = "td-ops-1"
	s.mu.Lock()
	s.tasks[taskID] = &model.TaskDetail{
		ID: taskID, UserID: userID, OrgID: orgID, ProjectID: "p-x",
		Title: "卡住的编剧任务", Status: "in_progress",
		CreatedAt: stale, UpdatedAt: stale,
		Todos: []model.Todo{{
			ID: todoID, Title: "写第一场", Status: "in_progress",
			Assignee:       model.TodoAssignee{NodeID: "node-stalled"},
			StartedAt:      &started,
			LastActivityAt: &stale,
			CreatedAt:      stale,
		}},
	}
	s.mu.Unlock()
	return taskID, todoID
}

func TestOpsScannerCreatesStalledIncidentWithOrg(t *testing.T) {
	s, orgA, orgB, ua := newOpsFixture(t)
	seedStalledTask(s, orgA, ua)

	rt := opsRuntime{scanInterval: time.Minute, silentThreshold: 30 * time.Minute, resolveObserve: 10 * time.Minute}
	s.runOpsScanOnce(rt)

	scA := Scope{UserID: ua, OrgID: orgA}
	list := s.ListOpsIncidents(scA)
	if len(list) != 2 {
		t.Fatalf("orgA incidents = %d, want 2 (todo_stalled + task_silent)", len(list))
	}
	for _, inc := range list {
		if inc.OrgID != orgA {
			t.Fatalf("incident org = %q, want %q (org must follow resource)", inc.OrgID, orgA)
		}
		if !inc.Active || inc.Status != model.OpsStatusOpen {
			t.Fatalf("new incident must be active/open, got %s", inc.Status)
		}
	}

	// 跨租户：orgB 视角绝不能看到 orgA 的工单（宁可漏、不可泄）。
	scB := Scope{UserID: "user-b-x", OrgID: orgB}
	if got := len(s.ListOpsIncidents(scB)); got != 0 {
		t.Fatalf("orgB must see 0 incidents, got %d", got)
	}
	// user 维度（无租户上下文）：退回 user 维度，同样看不到别人的。
	if got := len(s.ListOpsIncidents(Scope{UserID: "user-b-x"})); got != 0 {
		t.Fatalf("user-b must see 0 incidents, got %d", got)
	}
}

func TestOpsIncidentDedupeAndReopen(t *testing.T) {
	s, orgA, _, ua := newOpsFixture(t)
	taskID, todoID := seedStalledTask(s, orgA, ua)

	rt := opsRuntime{scanInterval: time.Minute, silentThreshold: 30 * time.Minute, resolveObserve: 10 * time.Minute}
	s.runOpsScanOnce(rt)
	s.runOpsScanOnce(rt) // 同一问题第二轮仍命中

	s.mu.RLock()
	key := opsDedupeKey(model.RuleTodoStalled, taskID, todoID)
	id := s.opsByDedupeKey[key]
	inc := s.opsIncidents[id]
	actionCount := len(inc.Actions)
	s.mu.RUnlock()

	if id == "" {
		t.Fatal("dedupe key must resolve to an active incident")
	}
	if actionCount != 1 {
		t.Fatalf("actions = %d, want 1 (re-hit must NOT append actions)", actionCount)
	}

	// 反向验证：todo 恢复上报 → 规则不再命中 → 观察期满自动关闭。
	now := time.Now().UTC()
	fresh := now.Add(-time.Minute)
	s.mu.Lock()
	s.tasks[taskID].Todos[0].LastActivityAt = &fresh
	s.mu.Unlock()

	s.runOpsScanOnce(rt)
	s.mu.RLock()
	inc = s.opsIncidents[id]
	statusAfterFirstClear := inc.Status
	s.mu.RUnlock()
	if statusAfterFirstClear != model.OpsStatusOpen {
		t.Fatalf("within observation window status = %s, want open", statusAfterFirstClear)
	}

	// 观察期满（模拟时间推进：直接把观察起点拨回）。
	s.mu.Lock()
	s.opsClearSince[id] = now.Add(-11 * time.Minute)
	s.mu.Unlock()
	s.runOpsScanOnce(rt)
	s.mu.RLock()
	inc = s.opsIncidents[id]
	closed := inc.Status
	stillActive := inc.Active
	dedupeFreed := true
	if _, ok := s.opsByDedupeKey[key]; ok {
		dedupeFreed = false
	}
	s.mu.RUnlock()

	if closed != model.OpsStatusResolved || stillActive {
		t.Fatalf("after observation status = %s active=%v, want resolved/inactive", closed, stillActive)
	}
	if !dedupeFreed {
		t.Fatal("dedupe key must be freed after resolution (same problem may reopen)")
	}

	// 复发：todo 再次停滞 → 应能开出**新**工单（部分唯一索引语义）。
	s.mu.Lock()
	stale := time.Now().UTC().Add(-2 * time.Hour)
	s.tasks[taskID].Todos[0].LastActivityAt = &stale
	s.mu.Unlock()
	s.runOpsScanOnce(rt)
	s.mu.RLock()
	newID := s.opsByDedupeKey[key]
	s.mu.RUnlock()
	if newID == "" || newID == id {
		t.Fatalf("recurrence must open a NEW incident, got %q (old %q)", newID, id)
	}
}

func TestDeliverableUnboundResolveFlow(t *testing.T) {
	s, orgA, _, ua := newOpsFixture(t)
	taskID, todoID := seedStalledTask(s, orgA, ua)

	// 事件驱动上报（webhook 信号路径同款调用）。
	id := s.ReportOpsFinding(OpsFinding{
		RuleID:   model.RuleDeliverableUnbound,
		Severity: model.OpsSeverityCritical,
		Title:    "交付物未绑定输出位",
		Summary:  "终稿.docx 未携带 outputName",
		TaskID:   taskID,
		TodoID:   todoID,
		NodeID:   "node-x",
	})
	if id == "" {
		t.Fatal("ReportOpsFinding must create an incident")
	}

	// 绑定成功 → 观察信号。
	now := time.Now().UTC()
	s.mu.Lock()
	s.markOpsIncidentClearedUnsafe(model.RuleDeliverableUnbound, taskID, todoID, now.Add(-11*time.Minute))
	s.mu.Unlock()

	rt := opsRuntime{scanInterval: time.Minute, silentThreshold: 30 * time.Minute, resolveObserve: 10 * time.Minute}
	s.runOpsScanOnce(rt)

	s.mu.RLock()
	inc := s.opsIncidents[id]
	status := inc.Status
	s.mu.RUnlock()
	if status != model.OpsStatusResolved {
		t.Fatalf("after bind success + observation, status = %s, want resolved", status)
	}
}
