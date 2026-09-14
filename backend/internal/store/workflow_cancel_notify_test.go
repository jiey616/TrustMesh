package store

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

// TestCancelTaskNotifiesAssigneeNodes 取消任务时，所有处于 pending/in_progress
// 且 Assignee.NodeID 非空的 todo 必须生成取消通知（adapter lifecycle 取消链路，
// dev-spec-adapter-lifecycle §6）；done/canceled 的 todo 和空 NodeID 的 todo 不生成。
func TestCancelTaskNotifiesAssigneeNodes(t *testing.T) {
	s := New()
	now := time.Now().UTC()

	agent := &model.Agent{ID: "agent-1", Name: "执行者", UserID: "user-1", NodeID: "node-a"}
	s.agents[agent.ID] = agent
	s.projects["project-1"] = &model.Project{ID: "project-1", UserID: "user-1", Name: "测试项目"}
	s.tasks["task-cancel-1"] = &model.TaskDetail{
		ID:        "task-cancel-1",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "取消通知测试",
		Status:    "in_progress",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
		Todos: []model.Todo{
			{ID: "TD_01", Title: "执行中", Status: "in_progress", Assignee: model.TodoAssignee{AgentID: "agent-1", NodeID: "node-a"}},
			{ID: "TD_02", Title: "待执行有节点", Status: "pending", Assignee: model.TodoAssignee{AgentID: "agent-2", NodeID: "node-b"}},
			{ID: "TD_03", Title: "待执行无节点", Status: "pending", Assignee: model.TodoAssignee{AgentID: "agent-3"}},
			{ID: "TD_04", Title: "已完成", Status: "done", Assignee: model.TodoAssignee{AgentID: "agent-4", NodeID: "node-d"}},
		},
	}

	type cancelCall struct {
		taskID  string
		version int
		notices []model.TodoCancelNotice
	}
	var calls []cancelCall
	s.SetCancelNotifyHook(func(taskID string, taskVersion int, notices []model.TodoCancelNotice) {
		calls = append(calls, cancelCall{taskID: taskID, version: taskVersion, notices: notices})
		// 锁外语义由 CancelTask 结构保证（hook 在 s.mu.Unlock() 之后触发）；
		// 不用 RWMutex.TryLock 断言——刚 Unlock 的锁上 TryLock 可能伪失败。
	})

	res, appErr := s.CancelTask(Scope{UserID: "user-1"}, TaskCancelInput{TaskID: "task-cancel-1", Reason: "用户终止"})
	if appErr != nil {
		t.Fatalf("CancelTask failed: %v", appErr)
	}
	if res.Status != "canceled" {
		t.Fatalf("task status = %q, want canceled", res.Status)
	}

	if len(calls) != 1 {
		t.Fatalf("hook called %d times, want 1", len(calls))
	}
	call := calls[0]
	if call.taskID != "task-cancel-1" {
		t.Fatalf("hook taskID = %q, want task-cancel-1", call.taskID)
	}
	// T2.2：版本号由提交原语统一推进。测试任务初始 Version=0（未经过任何持久化），
	// 取消时归一化为 1，再由版本化提交推进到 2，hook 收到的是提交成功后的权威版本 2。
	if call.version != 2 {
		t.Fatalf("hook version = %d, want 2 (task.Version advances 0->normalize 1->persist 2 on cancel)", call.version)
	}
	if len(call.notices) != 2 {
		t.Fatalf("notices = %+v, want exactly TD_01+TD_02", call.notices)
	}
	if call.notices[0].TodoID != "TD_01" || call.notices[0].NodeID != "node-a" {
		t.Fatalf("notices[0] = %+v, want TD_01@node-a", call.notices[0])
	}
	if call.notices[1].TodoID != "TD_02" || call.notices[1].NodeID != "node-b" {
		t.Fatalf("notices[1] = %+v, want TD_02@node-b", call.notices[1])
	}
	if call.notices[0].Reason != "用户终止" {
		t.Fatalf("notice reason = %q, want 用户终止", call.notices[0].Reason)
	}

	// 终态后重复取消：Conflict 且不再触发 hook。
	if _, appErr := s.CancelTask(Scope{UserID: "user-1"}, TaskCancelInput{TaskID: "task-cancel-1"}); appErr == nil {
		t.Fatal("second cancel should conflict")
	}
	if len(calls) != 1 {
		t.Fatalf("hook called %d times after second cancel, want still 1", len(calls))
	}
}

// TestCancelTaskWithoutHookNoPanic 未注册 hook 时取消任务必须正常完成（hook 可选）。
func TestCancelTaskWithoutHookNoPanic(t *testing.T) {
	s := New()
	now := time.Now().UTC()
	s.projects["project-1"] = &model.Project{ID: "project-1", UserID: "user-1", Name: "测试项目"}
	s.tasks["task-cancel-2"] = &model.TaskDetail{
		ID:        "task-cancel-2",
		UserID:    "user-1",
		ProjectID: "project-1",
		Title:     "无 hook 取消",
		Status:    "in_progress",
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
		Todos: []model.Todo{
			{ID: "TD_01", Title: "执行中", Status: "in_progress", Assignee: model.TodoAssignee{AgentID: "agent-1", NodeID: "node-a"}},
		},
	}

	res, appErr := s.CancelTask(Scope{UserID: "user-1"}, TaskCancelInput{TaskID: "task-cancel-2", Reason: "x"})
	if appErr != nil {
		t.Fatalf("CancelTask failed: %v", appErr)
	}
	if res.Status != "canceled" {
		t.Fatalf("task status = %q, want canceled", res.Status)
	}
	if s.tasks["task-cancel-2"].Todos[0].Status != "canceled" {
		t.Fatalf("todo status = %q, want canceled", s.tasks["task-cancel-2"].Todos[0].Status)
	}
}
