package store

import (
	"testing"

	"trustmesh/backend/internal/model"
	"trustmesh/backend/internal/transport"
)

func TestProjectTaskSummaryReflectsTaskState(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	projectState, appErr := s.GetProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("get empty project: %v", appErr)
	}
	if projectState.TaskSummary.WorkStatus != "empty" {
		t.Fatalf("expected empty work status, got %s", projectState.TaskSummary.WorkStatus)
	}
	if projectState.TaskSummary.TaskTotal != 0 {
		t.Fatalf("expected no tasks, got %d", projectState.TaskSummary.TaskTotal)
	}

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	projectState, appErr = s.GetProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("get queued project: %v", appErr)
	}
	if projectState.TaskSummary.WorkStatus != "queued" {
		t.Fatalf("expected queued work status, got %s", projectState.TaskSummary.WorkStatus)
	}
	if projectState.TaskSummary.PendingCount != 1 || projectState.TaskSummary.TaskTotal != 1 {
		t.Fatalf("unexpected queued summary: %#v", projectState.TaskSummary)
	}
	if projectState.TaskSummary.LatestTaskAt == nil {
		t.Fatal("expected latest task timestamp to be set")
	}

	_, appErr = s.RecordSequentialTodoDispatch(task.ID, "todo-1")
	if appErr != nil {
		t.Fatalf("dispatch todo: %v", appErr)
	}

	projectState, appErr = s.GetProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("get running project: %v", appErr)
	}
	if projectState.TaskSummary.WorkStatus != "running" {
		t.Fatalf("expected running work status, got %s", projectState.TaskSummary.WorkStatus)
	}
	if projectState.TaskSummary.InProgressCount != 1 {
		t.Fatalf("unexpected running summary: %#v", projectState.TaskSummary)
	}

	_, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
	})
	if appErr != nil {
		t.Fatalf("complete todo: %v", appErr)
	}

	projectState, appErr = s.GetProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("get idle project: %v", appErr)
	}
	if projectState.TaskSummary.WorkStatus != "idle" {
		t.Fatalf("expected idle work status, got %s", projectState.TaskSummary.WorkStatus)
	}
	if projectState.TaskSummary.DoneCount != 1 {
		t.Fatalf("unexpected idle summary: %#v", projectState.TaskSummary)
	}
}

func TestProjectTaskSummaryPrioritizesAttentionAndArchive(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Investigate bug",
		Description: "Reproduce flaky test",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Inspect logs",
				Description:    "Collect the latest failure logs",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	_, appErr = s.FailTodoByNode(developer.NodeID, TodoFailInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Error:  "tests are still flaky",
	})
	if appErr != nil {
		t.Fatalf("fail todo: %v", appErr)
	}

	projectState, appErr := s.GetProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("get attention project: %v", appErr)
	}
	if projectState.TaskSummary.WorkStatus != "attention" {
		t.Fatalf("expected attention work status, got %s", projectState.TaskSummary.WorkStatus)
	}
	if projectState.TaskSummary.FailedCount != 1 {
		t.Fatalf("unexpected attention summary: %#v", projectState.TaskSummary)
	}

	projectState, appErr = s.ArchiveProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("archive project: %v", appErr)
	}
	if projectState.TaskSummary.WorkStatus != "archived" {
		t.Fatalf("expected archived work status, got %s", projectState.TaskSummary.WorkStatus)
	}
}

func TestProjectTaskSummaryTracksCanceledWithoutAttention(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	if _, appErr := s.CancelTask(userID, TaskCancelInput{TaskID: task.ID, Reason: "user stop"}); appErr != nil {
		t.Fatalf("cancel task: %v", appErr)
	}

	projectState, appErr := s.GetProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("get project: %v", appErr)
	}
	if projectState.TaskSummary.CanceledCount != 1 {
		t.Fatalf("expected canceled_count=1, got %#v", projectState.TaskSummary)
	}
	if projectState.TaskSummary.WorkStatus != "idle" {
		t.Fatalf("expected idle work status after cancel, got %s", projectState.TaskSummary.WorkStatus)
	}
}

func TestArchiveProjectBlocksAppendingPlanningMessages(t *testing.T) {
	s, userID, _, _, project := seedWorkflowState(t)
	planningTask, appErr := s.CreateTaskPlanning(userID, project.ID, "Need login")
	if appErr != nil {
		t.Fatalf("create planning task: %v", appErr)
	}

	if _, appErr := s.ArchiveProject(Scope{UserID: userID}, project.ID); appErr != nil {
		t.Fatalf("archive project: %v", appErr)
	}

	_, appErr = s.AppendTaskMessage(userID, planningTask.ID, "还想补充一个需求", nil)
	if appErr == nil {
		t.Fatal("expected append message to fail for archived project")
	}
	if appErr.Code != "PROJECT_ARCHIVED" {
		t.Fatalf("expected PROJECT_ARCHIVED, got %s", appErr.Code)
	}
}

func TestArchiveProjectResetsInProgressWorkToPending(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	inProgressTask, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create in-progress task: %v", appErr)
	}
	if _, appErr := s.RecordSequentialTodoDispatch(inProgressTask.ID, "todo-1"); appErr != nil {
		t.Fatalf("dispatch todo: %v", appErr)
	}

	doneTask, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement dashboard",
		Description: "Add dashboard page",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-2",
				Title:          "Build dashboard page",
				Description:    "Implement UI",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create done task: %v", appErr)
	}
	if _, _, appErr := s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: doneTask.ID,
		TodoID: "todo-2",
		Result: model.TodoResult{
			Summary: "done",
			Output:  "dashboard implemented",
		},
	}); appErr != nil {
		t.Fatalf("complete todo: %v", appErr)
	}

	failedTask, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Investigate flaky test",
		Description: "Reproduce test issue",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-3",
				Title:          "Collect logs",
				Description:    "Inspect latest logs",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create failed task: %v", appErr)
	}
	if _, appErr := s.FailTodoByNode(developer.NodeID, TodoFailInput{
		TaskID: failedTask.ID,
		TodoID: "todo-3",
		Error:  "tests are still flaky",
	}); appErr != nil {
		t.Fatalf("fail todo: %v", appErr)
	}

	projectState, appErr := s.ArchiveProject(Scope{UserID: userID}, project.ID)
	if appErr != nil {
		t.Fatalf("archive project: %v", appErr)
	}
	if projectState.Status != "archived" {
		t.Fatalf("expected archived project status, got %s", projectState.Status)
	}

	inProgressState, appErr := s.GetTask(userID, inProgressTask.ID)
	if appErr != nil {
		t.Fatalf("get in-progress task: %v", appErr)
	}
	if inProgressState.Status != "pending" {
		t.Fatalf("expected task pending after archive, got %s", inProgressState.Status)
	}
	if inProgressState.Todos[0].Status != "pending" {
		t.Fatalf("expected todo pending after archive, got %s", inProgressState.Todos[0].Status)
	}
	if inProgressState.Todos[0].StartedAt != nil {
		t.Fatal("expected todo started_at to be cleared after archive")
	}

	doneState, appErr := s.GetTask(userID, doneTask.ID)
	if appErr != nil {
		t.Fatalf("get done task: %v", appErr)
	}
	if doneState.Status != "done" {
		t.Fatalf("expected done task to stay done, got %s", doneState.Status)
	}

	failedState, appErr := s.GetTask(userID, failedTask.ID)
	if appErr != nil {
		t.Fatalf("get failed task: %v", appErr)
	}
	if failedState.Status != "failed" {
		t.Fatalf("expected failed task to stay failed, got %s", failedState.Status)
	}

	agent, appErr := s.GetAgent(userID, developer.ID)
	if appErr != nil {
		t.Fatalf("get developer agent: %v", appErr)
	}
	if agent.Status != "online" {
		t.Fatalf("expected developer to be online after archive reset, got %s", agent.Status)
	}
}

func TestArchiveProjectBlocksTaskExecutionMutations(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	if _, appErr := s.ArchiveProject(Scope{UserID: userID}, project.ID); appErr != nil {
		t.Fatalf("archive project: %v", appErr)
	}

	assertArchivedError := func(name string, appErr *transport.AppError) {
		t.Helper()
		if appErr == nil {
			t.Fatalf("expected %s to fail for archived project", name)
		}
		if appErr.Code != "PROJECT_ARCHIVED" {
			t.Fatalf("expected PROJECT_ARCHIVED for %s, got %s", name, appErr.Code)
		}
	}

	_, appErr = s.RecordTodoDispatch(userID, task.ID, "todo-1")
	assertArchivedError("manual dispatch", appErr)

	_, appErr = s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID:  task.ID,
		TodoID:  "todo-1",
		Message: "started",
	})
	assertArchivedError("todo progress", appErr)

	_, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
	})
	assertArchivedError("todo complete", appErr)

	_, appErr = s.FailTodoByNode(developer.NodeID, TodoFailInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Error:  "failed",
	})
	assertArchivedError("todo fail", appErr)

	_, appErr = s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement dashboard",
		Description: "Add dashboard page",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-2",
				Title:          "Build dashboard page",
				Description:    "Implement UI",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	assertArchivedError("task create", appErr)
}
