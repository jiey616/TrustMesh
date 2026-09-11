package store

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"trustmesh/backend/internal/model"
)

// T0.0 测试前置：防 P0①「任务卡在第 1 步不进第 2 步」。
//
// 关键事实（读代码确认，非假设）：
//   - store 层**没有** dispatchNextTodo/自动推进；CompleteTodoByNode
//     （workflow.go:750）只把当前 todo 置 done，不会把下一个 todo 置
//     in_progress。
//   - 步骤推进只有两条路径：
//     a) agent 主动上报 todo.progress → UpdateTodoProgressByNode
//        （workflow.go:714-715）把 pending 翻成 in_progress；
//     b) 后台对账 reconcilePendingDispatches（dispatch_reconciler.go:52）
//        在 pending 停留超过 dispatchReconcileGrace(90s) 后经 dispatchHook 补派。
//
// 这正是「卡第 1 步」的根因：一旦 agent 因 skill pruning 不回报进度，
// 后续步骤会永远停在 pending。下面三个测试分别锁定推进契约、补派自愈、
// 以及 6 步流水线可连续跑完。

// seedTwoStepTask 建一个两步任务（step-1 / step-2 均派给 developer），
// 并把 step-1 推进到 in_progress，返回任务与两个 todoID。
func seedTwoStepTask(t *testing.T) (*Store, stringAgent, string, string) {
	t.Helper()
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "跨步骤推进",
		Description: "验证 step-1 完成后 step-2 可推进",
		Todos: []TaskCreateTodoInput{
			{Title: "step-1", Description: "d", AssigneeNodeID: developer.NodeID},
			{Title: "step-2", Description: "d", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	if len(task.Todos) != 2 {
		t.Fatalf("task.Todos len = %d, want 2", len(task.Todos))
	}

	if _, appErr := s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID: task.ID, TodoID: task.Todos[0].ID, Message: "开始执行 step-1",
	}); appErr != nil {
		t.Fatalf("start step-1: %v", appErr)
	}
	return s, developer, task.ID, task.Todos[1].ID
}

// TestStepAdvancesAfterPredecessorCompletes 锁定跨步骤推进契约：
// step-1 完成后 step-2 不再被前序阻塞，agent 一回报即进入 in_progress。
func TestStepAdvancesAfterPredecessorCompletes(t *testing.T) {
	s, developer, taskID, step2ID := seedTwoStepTask(t)
	step1ID := s.tasks[taskID].Todos[0].ID

	// 完成 step-1
	if _, _, appErr := s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: taskID, TodoID: step1ID,
		Result: model.TodoResult{Summary: "step-1 完成", Output: "产出 ok"},
	}); appErr != nil {
		t.Fatalf("complete step-1: %v", appErr)
	}
	if got := s.tasks[taskID].Todos[0].Status; got != "done" {
		t.Fatalf("step-1 status = %s, want done", got)
	}

	// step-1 完成的瞬间 step-2 仍是 pending（store 不自动推进，见文件头注释）。
	if got := s.tasks[taskID].Todos[1].Status; got != "pending" {
		t.Fatalf("step-2 status right after step-1 completion = %s, want pending", got)
	}

	// step-2 不再被前序阻塞：agent 回报进展即可推进
	if _, appErr := s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID: taskID, TodoID: step2ID, Message: "开始执行 step-2",
	}); appErr != nil {
		t.Fatalf("start step-2: %v (step-2 被前序阻塞 = 卡第 1 步症状)", appErr)
	}
	td := &s.tasks[taskID].Todos[1]
	if td.Status != "in_progress" {
		t.Fatalf("step-2 status = %s, want in_progress", td.Status)
	}
	if td.AssignedAt == nil {
		t.Fatal("step-2 AssignedAt must be set once it starts")
	}
	if td.StartedAt == nil {
		t.Fatal("step-2 StartedAt must be set once it starts")
	}
}

// TestStalledStepIsRedispatchedByReconciler 是自愈守卫：
// 若 agent 始终不回报（skill pruning 症状），step-2 停在 pending 超过
// dispatchReconcileGrace(90s) 后，后台对账必须经 dispatchHook 补派它。
//
// 这也是阶段0 smoke「注入 stop 90s → 3min 内自动补派」的确定性替代方案：
// 不依赖真实节点 stop/start，秒级完成且零 flaky。
func TestStalledStepIsRedispatchedByReconciler(t *testing.T) {
	s, developer, taskID, step2ID := seedTwoStepTask(t)
	step1ID := s.tasks[taskID].Todos[0].ID

	if _, _, appErr := s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: taskID, TodoID: step1ID,
		Result: model.TodoResult{Summary: "step-1 完成", Output: "产出 ok"},
	}); appErr != nil {
		t.Fatalf("complete step-1: %v", appErr)
	}

	// 制造「step-2 停在 pending 且已超过补派宽限窗口」
	s.mu.Lock()
	stale := time.Now().UTC().Add(-(dispatchReconcileGrace + time.Minute))
	s.tasks[taskID].Todos[1].LastDispatchAt = &stale
	taskStatus := s.tasks[taskID].Status
	s.mu.Unlock()

	// 对账器只扫描 in_progress 的任务
	if taskStatus != "in_progress" {
		t.Fatalf("task status = %s, want in_progress (否则对账器不会扫描)", taskStatus)
	}

	var dispatched []string
	s.SetDispatchHook(func(ctx context.Context, gotTaskID, gotTodoID string) {
		dispatched = append(dispatched, gotTodoID)
	})

	s.reconcilePendingDispatches()

	if len(dispatched) != 1 {
		t.Fatalf("reconciler dispatch count = %d, want 1 (dispatched=%v)", len(dispatched), dispatched)
	}
	if dispatched[0] != step2ID {
		t.Fatalf("reconciler dispatched todo %q, want stalled step-2 %q", dispatched[0], step2ID)
	}

	// 未超宽限的 todo 不应被重复补派（避免刷屏式重派）
	dispatched = nil
	s.mu.Lock()
	now := time.Now().UTC()
	s.tasks[taskID].Todos[1].LastDispatchAt = &now
	s.mu.Unlock()
	s.reconcilePendingDispatches()
	if len(dispatched) != 0 {
		t.Fatalf("todo inside grace window must NOT be re-dispatched, got %v", dispatched)
	}
}

// TestSixStepPipelineRunsToCompletion 覆盖 6 步流水线连续推进不中断，
// 对应阶段1 smoke 的「6 步全 manual=false」在 store 层的单测版本。
func TestSixStepPipelineRunsToCompletion(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)
	s.log = zap.NewNop()

	const stepCount = 6
	todos := make([]TaskCreateTodoInput, 0, stepCount)
	for i := 1; i <= stepCount; i++ {
		todos = append(todos, TaskCreateTodoInput{
			Title:          "step-" + string(rune('0'+i)),
			Description:    "d",
			AssigneeNodeID: developer.NodeID,
		})
	}

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "6 步流水线",
		Description: "验证连续推进不中断",
		Todos:       todos,
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	if len(task.Todos) != stepCount {
		t.Fatalf("task.Todos len = %d, want %d", len(task.Todos), stepCount)
	}

	for i := 0; i < stepCount; i++ {
		todoID := task.Todos[i].ID
		if _, appErr := s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
			TaskID: task.ID, TodoID: todoID, Message: "开始执行",
		}); appErr != nil {
			t.Fatalf("step %d start: %v", i+1, appErr)
		}
		if _, _, appErr := s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
			TaskID: task.ID, TodoID: todoID,
			Result: model.TodoResult{Summary: "完成", Output: "ok"},
		}); appErr != nil {
			t.Fatalf("step %d complete: %v", i+1, appErr)
		}
		if got := s.tasks[task.ID].Todos[i].Status; got != "done" {
			t.Fatalf("step %d status = %s, want done", i+1, got)
		}
	}

	if got := s.tasks[task.ID].Status; got != "done" {
		t.Fatalf("task status after all steps = %s, want done", got)
	}
}
