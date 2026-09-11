package store

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

func TestRecordSequentialDispatchFailurePersists(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	task := &model.TaskDetail{
		ID: "t-d1", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending",
			Assignee: model.TodoAssignee{AgentID: "a1", Name: "编剧", NodeID: "node-1"},
		}},
	}
	s.tasks[task.ID] = task

	if appErr := s.RecordSequentialDispatchFailure(task.ID, "td-1", "publish request failed: connection refused"); appErr != nil {
		t.Fatalf("record dispatch failure: %v", appErr)
	}

	td := &s.tasks[task.ID].Todos[0]
	if td.DispatchAttempts != 1 {
		t.Fatalf("DispatchAttempts = %d, want 1", td.DispatchAttempts)
	}
	if td.LastDispatchAt == nil {
		t.Fatalf("LastDispatchAt must be set")
	}
	if td.LastDispatchErr == nil || *td.LastDispatchErr == "" {
		t.Fatalf("LastDispatchErr must be set")
	}

	var found bool
	for _, ev := range s.taskEvents[task.ID] {
		if ev.EventType == "todo_dispatch_failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected todo_dispatch_failed event to be persisted")
	}
}

func TestRecordSequentialTodoDispatchClearsFailureTrace(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "P-01 dispatch trace",
		Description: "验证成功派发会清空失败痕迹",
		Todos: []TaskCreateTodoInput{
			{Title: "阶段一", Description: "d", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	todoID := task.Todos[0].ID
	msg := "boom"
	td := &s.tasks[task.ID].Todos[0]
	td.DispatchAttempts = 2
	td.LastDispatchErr = &msg

	if _, appErr := s.RecordSequentialTodoDispatch(task.ID, todoID); appErr != nil {
		t.Fatalf("record dispatch: %v", appErr)
	}

	td = &s.tasks[task.ID].Todos[0]
	if td.Status != "in_progress" {
		t.Fatalf("status = %s, want in_progress", td.Status)
	}
	if td.LastDispatchErr != nil {
		t.Fatalf("successful dispatch must clear LastDispatchErr")
	}
	if td.DispatchAttempts != 3 {
		t.Fatalf("DispatchAttempts = %d, want 3", td.DispatchAttempts)
	}
	if td.LastDispatchAt == nil {
		t.Fatalf("LastDispatchAt must be set on success")
	}
}

func TestDispatchReconcilerRepublishesStalledPendingTodo(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-10 * time.Minute)
	task := &model.TaskDetail{
		ID: "t-r1", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending", CreatedAt: old,
			Assignee: model.TodoAssignee{AgentID: "a1", Name: "编剧", NodeID: "node-1"},
		}},
	}
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.reconcilePendingDispatches()

	if len(calls) != 1 || calls[0] != "t-r1/td-1" {
		t.Fatalf("expected one reconcile dispatch for t-r1/td-1, got %v", calls)
	}
}

func TestDispatchReconcilerSkipsFreshAndBlockedTodos(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	now := time.Now().UTC()

	// 刚创建的 pending todo：位于宽限期内，不应补派（避免与正常派发抢跑）
	fresh := &model.TaskDetail{
		ID: "t-r2", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending", CreatedAt: now,
			Assignee: model.TodoAssignee{AgentID: "a1", NodeID: "node-1"},
		}},
	}
	s.tasks[fresh.ID] = fresh

	// 已完成的任务：不参与对账
	done := &model.TaskDetail{
		ID: "t-r3", UserID: "u1", ProjectID: "p1", Status: "done",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "pending",
			CreatedAt: now.Add(-1 * time.Hour),
			Assignee:  model.TodoAssignee{AgentID: "a1", NodeID: "node-1"},
		}},
	}
	s.tasks[done.ID] = done

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.reconcilePendingDispatches()

	if len(calls) != 0 {
		t.Fatalf("reconciler must not dispatch fresh or finished tasks, got %v", calls)
	}
}

func TestDispatchReconcilerSkipsInProgressTodo(t *testing.T) {
	s := New()
	s.log = zap.NewNop()

	old := time.Now().UTC().Add(-1 * time.Hour)
	task := &model.TaskDetail{
		ID: "t-r4", UserID: "u1", ProjectID: "p1", Status: "in_progress",
		Todos: []model.Todo{{
			ID: "td-1", Title: "剧本创作", Status: "in_progress", CreatedAt: old,
			Assignee: model.TodoAssignee{AgentID: "a1", NodeID: "node-1"},
		}},
	}
	s.tasks[task.ID] = task

	var calls []string
	s.SetDispatchHook(func(ctx context.Context, taskID, todoID string) {
		calls = append(calls, taskID+"/"+todoID)
	})

	s.reconcilePendingDispatches()

	if len(calls) != 0 {
		t.Fatalf("reconciler must not re-dispatch an already in_progress todo, got %v", calls)
	}
}
