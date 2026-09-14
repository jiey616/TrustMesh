package store

import (
	"testing"

	"trustmesh/backend/internal/model"
)

func TestApplyWorkflowReviewFlags(t *testing.T) {
	mkTodo := func(agentID, name string) model.Todo {
		return model.Todo{
			Title: name,
			Assignee: model.TodoAssignee{
				AgentID: agentID,
				Name:    name,
				NodeID:  "node-" + agentID,
			},
		}
	}

	cases := []struct {
		name      string
		steps     []model.WorkflowStep
		todos     []model.Todo
		roles     []string
		wantFlags []bool
	}{
		{
			name: "review on second step by agent id",
			steps: []model.WorkflowStep{
				{Name: "产出", Role: "developer", AgentID: "ag-a"},
				{Name: "审核", Role: "developer", AgentID: "ag-b", NeedReview: true},
			},
			todos:     []model.Todo{mkTodo("ag-a", "编剧A"), mkTodo("ag-b", "导演B")},
			roles:     []string{"developer", "developer"},
			wantFlags: []bool{false, true},
		},
		{
			name: "review by role match",
			steps: []model.WorkflowStep{
				{Name: "产出", Role: "编剧"},
				{Name: "审核", Role: "导演", NeedReview: true},
			},
			todos:     []model.Todo{mkTodo("ag-a", "编剧小A"), mkTodo("ag-b", "导演小B")},
			roles:     []string{"编剧", "导演"},
			wantFlags: []bool{false, true},
		},
		{
			name: "same-role consecutive steps with review on last",
			steps: []model.WorkflowStep{
				{Name: "圆桌", Role: "developer"},
				{Name: "修改", Role: "developer"},
				{Name: "审核", Role: "developer", NeedReview: true},
			},
			todos:     []model.Todo{mkTodo("ag-a", "导演A"), mkTodo("ag-a", "导演A"), mkTodo("ag-b", "导演B")},
			roles:     []string{"developer", "developer", "developer"},
			wantFlags: []bool{false, false, true},
		},
		{
			name:      "no workflow steps",
			steps:     nil,
			todos:     []model.Todo{mkTodo("ag-a", "A")},
			roles:     []string{"developer"},
			wantFlags: []bool{false},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			todos := tc.todos
			applyWorkflowReviewFlags(tc.steps, &todos, tc.roles)
			for i, want := range tc.wantFlags {
				if todos[i].NeedReview != want {
					t.Fatalf("todo[%d] NeedReview = %v, want %v", i, todos[i].NeedReview, want)
				}
			}
		})
	}
}

// Human review (nodeID empty) audits the todo itself: a reject must reset and
// re-dispatch that very todo, even when it is the only todo in the task —
// never let the task flip to done.
func TestReviewTodoHumanRejectReworksSelf(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "单步人工审核任务",
		Description: "验证人工退回重做",
		Todos: []TaskCreateTodoInput{
			{ID: "todo-1", Title: "写剧本", Description: "完成初稿", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	task, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID:     task.ID,
		TodoID:     "todo-1",
		NeedReview: true,
		Result:     model.TodoResult{Summary: "初稿完成"},
	})
	if appErr != nil {
		t.Fatalf("complete todo: %v", appErr)
	}
	if task.Status != model.TaskStatusAwaitingReview {
		t.Fatalf("expected awaiting_review, got %s", task.Status)
	}

	task, reworked, appErr := s.ReviewTodo(Scope{UserID: userID}, "", task.ID, "todo-1", "reject", "节奏太拖，重写")
	if appErr != nil {
		t.Fatalf("human reject: %v", appErr)
	}
	if reworked == nil || reworked.ID != "todo-1" {
		t.Fatalf("expected reworked todo-1, got %+v", reworked)
	}
	if task.Status != "in_progress" {
		t.Fatalf("expected in_progress after human reject, got %s", task.Status)
	}
	td := task.Todos[0]
	if td.Status != "in_progress" || td.ReviewStatus != "" || td.ReworkCount != 1 {
		t.Fatalf("unexpected todo state after rework: status=%s review=%q rework=%d", td.Status, td.ReviewStatus, td.ReworkCount)
	}
}

// Agent review (nodeID set) audits the predecessor: a reject resets and
// re-dispatches the previous todo, not the reviewer itself.
func TestReviewTodoAgentRejectReworksPredecessor(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "两步智能体审核任务",
		Description: "验证智能体退回前身",
		Todos: []TaskCreateTodoInput{
			{ID: "todo-1", Title: "写剧本", Description: "完成初稿", AssigneeNodeID: developer.NodeID},
			{ID: "todo-2", Title: "审剧本", Description: "审核初稿", AssigneeNodeID: developer.NodeID},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	if _, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: model.TodoResult{Summary: "初稿完成"},
	}); appErr != nil {
		t.Fatalf("complete todo-1: %v", appErr)
	}
	if _, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID:     task.ID,
		TodoID:     "todo-2",
		NeedReview: true,
		Result:     model.TodoResult{Summary: "审核意见"},
	}); appErr != nil {
		t.Fatalf("complete todo-2: %v", appErr)
	}

	task, reworked, appErr := s.ReviewTodo(Scope{UserID: userID}, developer.NodeID, task.ID, "todo-2", "reject", "前稿不合格")
	if appErr != nil {
		t.Fatalf("agent reject: %v", appErr)
	}
	if reworked == nil || reworked.ID != "todo-1" {
		t.Fatalf("expected reworked todo-1 (predecessor), got %+v", reworked)
	}
	if task.Todos[1].Status != "pending" {
		t.Fatalf("reviewer todo should be reset to pending, got %s", task.Todos[1].Status)
	}
	if task.Todos[1].ReworkCount != 0 {
		t.Fatalf("rework counter belongs to the audited predecessor, got %d on reviewer", task.Todos[1].ReworkCount)
	}
	if task.Status != "in_progress" {
		t.Fatalf("expected in_progress after agent reject, got %s", task.Status)
	}
}

// Defense: a done todo with review_status=rejected must never aggregate the
// task to done.
func TestAggregateTaskStatusRejectedNotDone(t *testing.T) {
	task := model.TaskDetail{
		Todos: []model.Todo{{
			Status:       "done",
			ReviewStatus: model.ReviewRejected,
		}},
	}
	if got := aggregateTaskStatus(task); got == "done" {
		t.Fatalf("rejected todo must not aggregate to done, got %s", got)
	}
}
