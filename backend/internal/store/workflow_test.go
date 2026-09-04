package store

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

func TestSyncAgentPresenceMarksOfflineAndBusy(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

	_, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	updated := s.SyncAgentPresence([]AgentPresence{
		{NodeID: developer.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	if updated != 2 {
		t.Fatalf("unexpected updated count: %d", updated)
	}

	projectState, appErr := s.GetProject(Scope{UserID: s.agents[pm.ID].UserID}, project.ID)
	if appErr != nil {
		t.Fatalf("get project: %v", appErr)
	}
	if projectState.PMAgent.Status != "offline" {
		t.Fatalf("expected project pm to be offline, got %s", projectState.PMAgent.Status)
	}

	taskState, appErr := s.GetTask(s.agents[pm.ID].UserID, s.projectTasks[project.ID][0])
	if appErr != nil {
		t.Fatalf("get task: %v", appErr)
	}
	if taskState.PMAgent.Status != "offline" {
		t.Fatalf("expected task pm to be offline, got %s", taskState.PMAgent.Status)
	}
	if s.agents[developer.ID].Status != "online" {
		t.Fatalf("expected developer to stay online before starting work, got %s", s.agents[developer.ID].Status)
	}

	taskState, appErr = s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID:  taskState.ID,
		TodoID:  taskState.Todos[0].ID,
		Message: "started",
	})
	if appErr != nil {
		t.Fatalf("todo progress: %v", appErr)
	}
	if s.agents[developer.ID].Status != "busy" {
		t.Fatalf("expected developer to be busy after progress, got %s", s.agents[developer.ID].Status)
	}
	if taskState.Todos[0].Assignee.NodeID != developer.NodeID {
		t.Fatalf("unexpected todo assignee node id: %s", taskState.Todos[0].Assignee.NodeID)
	}
}

func TestAgentUsageAndDeleteConflictDetails(t *testing.T) {
	s, userID, pm, developer, project := seedWorkflowState(t)

	_, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	agents := s.ListAgents(userID)
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	var pmUsage, developerUsage model.AgentUsage
	for _, agent := range agents {
		switch agent.ID {
		case pm.ID:
			pmUsage = agent.Usage
		case developer.ID:
			developerUsage = agent.Usage
		}
	}

	if !pmUsage.InUse || pmUsage.ProjectCount != 1 || pmUsage.TaskCount != 1 || pmUsage.TodoCount != 0 || pmUsage.TotalCount != 2 {
		t.Fatalf("unexpected pm usage: %#v", pmUsage)
	}
	if !developerUsage.InUse || developerUsage.ProjectCount != 0 || developerUsage.TaskCount != 0 || developerUsage.TodoCount != 1 || developerUsage.TotalCount != 1 {
		t.Fatalf("unexpected developer usage: %#v", developerUsage)
	}

	agent, appErr := s.GetAgent(userID, developer.ID)
	if appErr != nil {
		t.Fatalf("get agent: %v", appErr)
	}
	if !agent.Usage.InUse || agent.Usage.TodoCount != 1 {
		t.Fatalf("unexpected get agent usage: %#v", agent.Usage)
	}

	// 删除有引用的 agent 应该软删除（归档）而非报错
	appErr = s.DeleteAgent(userID, developer.ID)
	if appErr != nil {
		t.Fatalf("expected soft delete to succeed, got: %v", appErr)
	}

	// 归档后 ListAgents 不应包含该 agent
	agentsAfterArchive := s.ListAgents(userID)
	for _, a := range agentsAfterArchive {
		if a.ID == developer.ID {
			t.Fatal("archived agent should not appear in ListAgents")
		}
	}

	// GetAgent 仍可查看归档 agent
	archivedAgent, appErr := s.GetAgent(userID, developer.ID)
	if appErr != nil {
		t.Fatalf("GetAgent on archived agent should succeed: %v", appErr)
	}
	if !archivedAgent.Archived {
		t.Fatal("expected agent to be archived")
	}

	// 再次删除归档 agent 应返回 not found
	appErr = s.DeleteAgent(userID, developer.ID)
	if appErr == nil || appErr.Status != 404 {
		t.Fatalf("expected not found for already archived agent, got: %v", appErr)
	}
}

func TestTaskCreateIdempotencyByMessageID(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

	task1, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "msg-task-create-1", TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				Title:          "Build backend API",
				Description:    "Implement auth endpoints",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("first task.create: %v", appErr)
	}

	task2, appErr := s.CreateTaskByPMNodeWithMessageID(pm.NodeID, "msg-task-create-1", TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login duplicate",
		Description: "duplicate retry",
		Todos: []TaskCreateTodoInput{
			{
				Title:          "Build backend API duplicate",
				Description:    "retry",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("duplicate task.create should be ignored, got: %v", appErr)
	}
	if task1.ID != task2.ID {
		t.Fatalf("expected same task id for duplicate message: %s vs %s", task1.ID, task2.ID)
	}
	if len(s.projectTasks[project.ID]) != 1 {
		t.Fatalf("expected single task record, got %d", len(s.projectTasks[project.ID]))
	}
}

func TestTodoCompleteIdempotencyByMessageID(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

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

	result := model.TodoResult{
		Summary:  "done",
		Output:   "implemented",
		Metadata: map[string]any{"duration_ms": 100},
	}
	task1, _, appErr := s.CompleteTodoByNodeWithMessageID(developer.NodeID, "msg-todo-complete-1", TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: result,
	})
	if appErr != nil {
		t.Fatalf("first todo.complete: %v", appErr)
	}
	task2, _, appErr := s.CompleteTodoByNodeWithMessageID(developer.NodeID, "msg-todo-complete-1", TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: result,
	})
	if appErr != nil {
		t.Fatalf("duplicate todo.complete should be ignored, got: %v", appErr)
	}
	if task2.Status != "done" || task1.ID != task2.ID {
		t.Fatalf("unexpected duplicate todo.complete result: task1=%s task2=%s status=%s", task1.ID, task2.ID, task2.Status)
	}
	events, appErr := s.ListTaskEvents(task1.UserID, task.ID)
	if appErr != nil {
		t.Fatalf("list task events: %v", appErr)
	}
	completedEvents := 0
	for _, event := range events {
		if event.EventType == "todo_completed" {
			completedEvents++
		}
	}
	if completedEvents != 1 {
		t.Fatalf("expected a single todo_completed event, got %d", completedEvents)
	}
}

func TestTaskResultAggregationFromCompletedTodos(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

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
			{
				ID:             "todo-2",
				Title:          "Write rollout notes",
				Description:    "Document changes",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	task, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: model.TodoResult{
			Summary:  "API ready",
			Output:   "Added register/login endpoints",
			Metadata: map[string]any{"duration_ms": 1200},
		},
	})
	if appErr != nil {
		t.Fatalf("complete first todo: %v", appErr)
	}

	if task.Status != "in_progress" {
		t.Fatalf("expected in_progress after first completion, got %s", task.Status)
	}
	if task.Result.Metadata["completed_todo_count"] != 1 {
		t.Fatalf("expected completed_todo_count=1, got %#v", task.Result.Metadata["completed_todo_count"])
	}
	if task.Result.Metadata["pending_todo_count"] != 1 {
		t.Fatalf("expected pending_todo_count=1, got %#v", task.Result.Metadata["pending_todo_count"])
	}

	task, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-2",
		Result: model.TodoResult{
			Summary:  "Docs ready",
			Output:   "Added rollout checklist",
			Metadata: map[string]any{"duration_ms": 400},
		},
	})
	if appErr != nil {
		t.Fatalf("complete second todo: %v", appErr)
	}

	if task.Status != "done" {
		t.Fatalf("expected done after all todos complete, got %s", task.Status)
	}
	if task.Result.Summary == "" || task.Result.FinalOutput == "" {
		t.Fatalf("expected aggregated task result, got %+v", task.Result)
	}
	if task.Result.Metadata["completed_todo_count"] != 2 {
		t.Fatalf("expected completed_todo_count=2, got %#v", task.Result.Metadata["completed_todo_count"])
	}
}

func TestSaveArtifactAndFillOnTaskQuery(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Deliver report",
		Description: "Upload final report",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Upload report",
				Description:    "Send report PDF",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	// Save an artifact via transfer.received path.
	artifact := model.TaskArtifact{
		TransferID: "tf_report_123",
		TaskID:     task.ID,
		TodoID:     "todo-1",
		FileName:   "report.pdf",
		FileSize:   2048,
		LocalPath:  "/tmp/report.pdf",
		MimeType:   "application/pdf",
		FromNodeID: developer.NodeID,
		CreatedAt:  time.Now(),
	}
	if appErr := s.SaveArtifact(artifact); appErr != nil {
		t.Fatalf("save artifact: %v", appErr)
	}

	// Query task should include the artifact.
	fetched, appErr := s.GetTask(task.UserID, task.ID)
	if appErr != nil {
		t.Fatalf("get task: %v", appErr)
	}
	if len(fetched.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(fetched.Artifacts))
	}
	a := fetched.Artifacts[0]
	if a.TransferID != "tf_report_123" {
		t.Fatalf("unexpected transfer_id: %s", a.TransferID)
	}
	if a.FileName != "report.pdf" {
		t.Fatalf("unexpected file_name: %s", a.FileName)
	}
	if a.MimeType != "application/pdf" {
		t.Fatalf("unexpected mime_type: %s", a.MimeType)
	}

	// Duplicate save should overwrite, not duplicate.
	artifact.FileSize = 4096
	if appErr := s.SaveArtifact(artifact); appErr != nil {
		t.Fatalf("save duplicate artifact: %v", appErr)
	}
	fetched2, _ := s.GetTask(task.UserID, task.ID)
	if len(fetched2.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact after dedup, got %d", len(fetched2.Artifacts))
	}
	if fetched2.Artifacts[0].FileSize != 4096 {
		t.Fatalf("expected updated file_size=4096, got %d", fetched2.Artifacts[0].FileSize)
	}
}

func TestSaveArtifactBindsTodoOutput(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Bind output report",
		Description: "Upload final report with outputName",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-1",
				Title:          "Upload report",
				Description:    "Send report PDF",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	// Save an artifact carrying an outputName binding hint.
	artifact := model.TaskArtifact{
		TransferID: "tf_bound_1",
		TaskID:     task.ID,
		TodoID:     "todo-1",
		FileName:   "report.pdf",
		FileSize:   2048,
		LocalPath:  "/tmp/report.pdf",
		MimeType:   "application/pdf",
		FromNodeID: developer.NodeID,
		CreatedAt:  time.Now(),
		OutputName: "剧本正文",
	}
	if appErr := s.SaveArtifact(artifact); appErr != nil {
		t.Fatalf("save artifact: %v", appErr)
	}

	fetched, appErr := s.GetTask(task.UserID, task.ID)
	if appErr != nil {
		t.Fatalf("get task: %v", appErr)
	}
	var todo *model.Todo
	for i := range fetched.Todos {
		if fetched.Todos[i].ID == "todo-1" {
			todo = &fetched.Todos[i]
		}
	}
	if todo == nil {
		t.Fatalf("todo-1 not found")
	}
	if len(todo.Outputs) != 1 {
		t.Fatalf("expected 1 todo output, got %d", len(todo.Outputs))
	}
	if todo.Outputs[0].OutputName != "剧本正文" {
		t.Fatalf("unexpected output name: %s", todo.Outputs[0].OutputName)
	}
	if todo.Outputs[0].ArtifactID != "tf_bound_1" {
		t.Fatalf("unexpected artifact id: %s", todo.Outputs[0].ArtifactID)
	}
	if todo.Outputs[0].FileRef == "" {
		t.Fatalf("expected non-empty FileRef (ProjectFile id)")
	}

	// Re-uploading the same outputName should overwrite, not duplicate.
	artifact.TransferID = "tf_bound_2"
	artifact.FileSize = 4096
	if appErr := s.SaveArtifact(artifact); appErr != nil {
		t.Fatalf("re-save artifact: %v", appErr)
	}
	fetched2, _ := s.GetTask(task.UserID, task.ID)
	for i := range fetched2.Todos {
		if fetched2.Todos[i].ID == "todo-1" {
			todo = &fetched2.Todos[i]
		}
	}
	if len(todo.Outputs) != 1 {
		t.Fatalf("expected 1 todo output after re-save, got %d", len(todo.Outputs))
	}
	if todo.Outputs[0].ArtifactID != "tf_bound_2" {
		t.Fatalf("expected updated artifact id tf_bound_2, got %s", todo.Outputs[0].ArtifactID)
	}
}

func TestRecordTodoDispatchMarksTaskInProgress(t *testing.T) {
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

	task, appErr = s.RecordTodoDispatch(userID, task.ID, "todo-1")
	if appErr != nil {
		t.Fatalf("dispatch todo: %v", appErr)
	}

	if task.Status != "in_progress" {
		t.Fatalf("expected task in_progress after dispatch, got %s", task.Status)
	}
	if task.Todos[0].Status != "in_progress" {
		t.Fatalf("expected todo in_progress after dispatch, got %s", task.Todos[0].Status)
	}
	if task.Todos[0].StartedAt == nil {
		t.Fatal("expected todo started_at to be set after dispatch")
	}
	if s.agents[developer.ID].Status != "busy" {
		t.Fatalf("expected developer to be busy after dispatch, got %s", s.agents[developer.ID].Status)
	}
	if task.Result.Summary == "" {
		t.Fatalf("expected in-progress task result summary after dispatch, got %+v", task.Result)
	}
}

func TestTaskResultAggregationOnTodoFailure(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

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

	task, appErr = s.FailTodoByNode(developer.NodeID, TodoFailInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Error:  "missing oauth credentials",
	})
	if appErr != nil {
		t.Fatalf("fail todo: %v", appErr)
	}

	if task.Status != "failed" {
		t.Fatalf("expected failed task, got %s", task.Status)
	}
	if task.Result.Summary == "" || task.Result.FinalOutput == "" {
		t.Fatalf("expected failed task aggregation, got %+v", task.Result)
	}
	if task.Result.Metadata["failed_todo_count"] != 1 {
		t.Fatalf("expected failed_todo_count=1, got %#v", task.Result.Metadata["failed_todo_count"])
	}
	if len(task.Artifacts) != 0 {
		t.Fatalf("expected no artifacts on failed todo, got %d", len(task.Artifacts))
	}
}

func TestTaskTodoOrderAndSequentialExecutionGuards(t *testing.T) {
	s, _, pm, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByPMNode(pm.NodeID, TaskCreateInput{
		ProjectID:   project.ID,
		Title:       "Implement login",
		Description: "Support email password login",
		Todos: []TaskCreateTodoInput{
			{
				ID:             "todo-2",
				Order:          2,
				Title:          "Build frontend UI",
				Description:    "Implement the login form after backend API is ready",
				AssigneeNodeID: developer.NodeID,
			},
			{
				ID:             "todo-1",
				Order:          1,
				Title:          "Build backend API",
				Description:    "Implement auth endpoints first",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	if len(task.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(task.Todos))
	}
	if task.Todos[0].ID != "todo-1" || task.Todos[0].Order != 1 {
		t.Fatalf("expected first todo to be todo-1/order=1, got %+v", task.Todos[0])
	}
	if task.Todos[1].ID != "todo-2" || task.Todos[1].Order != 2 {
		t.Fatalf("expected second todo to be todo-2/order=2, got %+v", task.Todos[1])
	}

	_, appErr = s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID:  task.ID,
		TodoID:  "todo-2",
		Message: "trying to skip ahead",
	})
	if appErr == nil || appErr.Code != "TODO_BLOCKED_BY_PREVIOUS" {
		t.Fatalf("expected TODO_BLOCKED_BY_PREVIOUS for out-of-order progress, got %#v", appErr)
	}

	task, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: model.TodoResult{
			Summary:  "API ready",
			Output:   "backend done",
			Metadata: map[string]any{},
		},
	})
	if appErr != nil {
		t.Fatalf("complete first todo: %v", appErr)
	}

	task, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-2",
		Result: model.TodoResult{
			Summary:  "UI ready",
			Output:   "frontend done",
			Metadata: map[string]any{},
		},
	})
	if appErr != nil {
		t.Fatalf("complete second todo: %v", appErr)
	}

	if task.Status != "done" {
		t.Fatalf("expected task done after ordered completion, got %s", task.Status)
	}
}

func TestCancelTaskStopsFurtherTodoUpdates(t *testing.T) {
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
			{
				ID:             "todo-2",
				Title:          "Write docs",
				Description:    "Document rollout",
				AssigneeNodeID: developer.NodeID,
			},
		},
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	task, appErr = s.RecordSequentialTodoDispatch(task.ID, "todo-1")
	if appErr != nil {
		t.Fatalf("dispatch first todo: %v", appErr)
	}
	if task.Status != "in_progress" {
		t.Fatalf("expected in_progress before cancel, got %s", task.Status)
	}

	task, appErr = s.CancelTask(userID, TaskCancelInput{
		TaskID: task.ID,
		Reason: "manual stop",
	})
	if appErr != nil {
		t.Fatalf("cancel task: %v", appErr)
	}
	if task.Status != "canceled" {
		t.Fatalf("expected canceled task status, got %s", task.Status)
	}
	if task.CancelReason == nil || *task.CancelReason != "manual stop" {
		t.Fatalf("unexpected cancel reason: %#v", task.CancelReason)
	}
	if task.CanceledBy == nil || task.CanceledBy.ActorID != userID {
		t.Fatalf("unexpected canceled_by: %#v", task.CanceledBy)
	}
	if task.Todos[0].Status != "canceled" || task.Todos[1].Status != "canceled" {
		t.Fatalf("expected unfinished todos to be canceled: %#v", task.Todos)
	}
	if task.Result.Metadata["canceled_todo_count"] != 2 {
		t.Fatalf("expected canceled_todo_count=2, got %#v", task.Result.Metadata["canceled_todo_count"])
	}

	_, appErr = s.UpdateTodoProgressByNode(developer.NodeID, TodoProgressInput{
		TaskID:  task.ID,
		TodoID:  "todo-1",
		Message: "late progress",
	})
	if appErr == nil || appErr.Code != "TASK_CANCELED" {
		t.Fatalf("expected TASK_CANCELED for progress after cancel, got %#v", appErr)
	}

	_, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: "todo-1",
		Result: model.TodoResult{Summary: "late", Output: "late"},
	})
	if appErr == nil || appErr.Code != "TASK_CANCELED" {
		t.Fatalf("expected TASK_CANCELED for complete after cancel, got %#v", appErr)
	}

	_, appErr = s.RecordSequentialTodoDispatch(task.ID, "todo-2")
	if appErr == nil || appErr.Code != "TASK_CANCELED" {
		t.Fatalf("expected TASK_CANCELED for dispatch after cancel, got %#v", appErr)
	}
}

func TestCreateTaskByUser(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "Implement login",
		Description:     "Support email password login",
		Priority:        "high",
		AssigneeAgentID: developer.ID,
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	if task.Title != "Implement login" {
		t.Fatalf("unexpected title: %s", task.Title)
	}
	if task.Status != "pending" {
		t.Fatalf("expected pending, got %s", task.Status)
	}
	if task.Priority != "high" {
		t.Fatalf("expected high priority, got %s", task.Priority)
	}
	if len(task.Todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(task.Todos))
	}
	todo := task.Todos[0]
	if todo.Title != "Implement login" {
		t.Fatalf("todo title should match task title, got %s", todo.Title)
	}
	if todo.Assignee.AgentID != developer.ID {
		t.Fatalf("unexpected assignee: %s", todo.Assignee.AgentID)
	}
	if todo.Status != "pending" {
		t.Fatalf("expected todo pending, got %s", todo.Status)
	}

	// Verify task appears in project tasks
	fetched, appErr := s.GetTask(userID, task.ID)
	if appErr != nil {
		t.Fatalf("get task: %v", appErr)
	}
	if fetched.ID != task.ID {
		t.Fatalf("fetched task id mismatch")
	}
}

func TestCreateTaskByUserValidation(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)

	// Missing title
	_, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "",
		Description:     "desc",
		AssigneeAgentID: developer.ID,
	})
	if appErr == nil {
		t.Fatal("expected validation error for missing title")
	}

	// Invalid priority
	_, appErr = s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "Test",
		Description:     "desc",
		Priority:        "critical",
		AssigneeAgentID: developer.ID,
	})
	if appErr == nil {
		t.Fatal("expected validation error for invalid priority")
	}

	// Default priority
	task, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "Test",
		Description:     "desc",
		AssigneeAgentID: developer.ID,
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}
	if task.Priority != "medium" {
		t.Fatalf("expected default medium priority, got %s", task.Priority)
	}

	// Archived project
	s.mu.Lock()
	s.projects[project.ID].Status = "archived"
	s.mu.Unlock()
	_, appErr = s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "Test",
		Description:     "desc",
		AssigneeAgentID: developer.ID,
	})
	if appErr == nil || appErr.Code != "PROJECT_ARCHIVED" {
		t.Fatalf("expected PROJECT_ARCHIVED, got %v", appErr)
	}
}

func TestCreateTaskByUserTodoWorkflow(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)

	task, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "Build API",
		Description:     "REST endpoints",
		AssigneeAgentID: developer.ID,
	})
	if appErr != nil {
		t.Fatalf("create task: %v", appErr)
	}

	// Dispatch the todo
	task, appErr = s.RecordTodoDispatch(userID, task.ID, task.Todos[0].ID)
	if appErr != nil {
		t.Fatalf("dispatch todo: %v", appErr)
	}
	if task.Status != "in_progress" {
		t.Fatalf("expected in_progress after dispatch, got %s", task.Status)
	}

	// Complete the todo
	task, _, appErr = s.CompleteTodoByNode(developer.NodeID, TodoCompleteInput{
		TaskID: task.ID,
		TodoID: task.Todos[0].ID,
		Result: model.TodoResult{Summary: "done"},
	})
	if appErr != nil {
		t.Fatalf("complete todo: %v", appErr)
	}
	if task.Status != "done" {
		t.Fatalf("expected done after completing only todo, got %s", task.Status)
	}
}

func seedWorkflowState(t *testing.T) (*Store, string, stringAgent, stringAgent, projectRef) {
	t.Helper()

	s := New()
	user, appErr := s.CreateUser("user@example.com", "User", "hash")
	if appErr != nil {
		t.Fatalf("create user: %v", appErr)
	}
	pm, appErr := s.CreateAgent(user.ID, "node-pm-001", "PM Agent", "pm", "pm", []string{"plan"})
	if appErr != nil {
		t.Fatalf("create pm: %v", appErr)
	}
	developer, appErr := s.CreateAgent(user.ID, "node-dev-001", "Developer", "developer", "dev", []string{"backend"})
	if appErr != nil {
		t.Fatalf("create developer: %v", appErr)
	}
	s.SyncAgentPresence([]AgentPresence{
		{NodeID: pm.NodeID, LastSeenAt: time.Now().UTC()},
		{NodeID: developer.NodeID, LastSeenAt: time.Now().UTC()},
	}, time.Now().UTC())
	project, appErr := s.CreateProject(Scope{UserID: user.ID}, "TrustMesh MVP", "demo", pm.ID)
	if appErr != nil {
		t.Fatalf("create project: %v", appErr)
	}
	return s, user.ID, stringAgent{ID: pm.ID, NodeID: pm.NodeID}, stringAgent{ID: developer.ID, NodeID: developer.NodeID}, projectRef{ID: project.ID}
}

type stringAgent struct {
	ID     string
	NodeID string
}

type projectRef struct {
	ID string
}

func TestProjectWorkflowRefAndProgress(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)

	// 设置项目总流程（3 步）
	wfs := []model.Workflow{{
		Name: "总流程",
		Steps: []model.WorkflowStep{
			{Name: "剧本创作", Role: "developer"},
			{Name: "导演拆解", Role: "developer"},
			{Name: "质量验收", Role: "developer"},
		},
	}}
	idx := 0
	if _, appErr := s.UpdateProject(Scope{UserID: userID}, project.ID, UpdateProjectInput{
		Workflows:            wfs,
		PrimaryWorkflowIndex: &idx,
	}); appErr != nil {
		t.Fatalf("set primary workflow: %v", appErr)
	}

	// 任务 A 负责步骤 0-1（快照应裁剪为 2 步）
	taskA, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "前段",
		Description:     "前段任务",
		Priority:        "medium",
		AssigneeAgentID: developer.ID,
		WorkflowIndex:   wfIdxPtr(0),
		StepFrom:        0,
		StepTo:          1,
	})
	if appErr != nil {
		t.Fatalf("create task A: %v", appErr)
	}
	if taskA.WorkflowRef == nil || taskA.WorkflowRef.WorkflowIndex != 0 || taskA.WorkflowRef.StepFrom != 0 || taskA.WorkflowRef.StepTo != 1 {
		t.Fatalf("unexpected workflow_ref: %+v", taskA.WorkflowRef)
	}
	if taskA.Workflow == nil || len(taskA.Workflow.Steps) != 2 || taskA.Workflow.Steps[0].Name != "剧本创作" {
		t.Fatalf("snapshot should be trimmed to owned steps, got %+v", taskA.Workflow)
	}

	// 任务 B 负责步骤 2（快照应裁剪为 1 步）
	taskB, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "后段",
		Description:     "后段任务",
		Priority:        "medium",
		AssigneeAgentID: developer.ID,
		WorkflowIndex:   wfIdxPtr(0),
		StepFrom:        2,
		StepTo:          2,
	})
	if appErr != nil {
		t.Fatalf("create task B: %v", appErr)
	}
	if taskB.WorkflowRef == nil || taskB.WorkflowRef.StepFrom != 2 || taskB.WorkflowRef.StepTo != 2 {
		t.Fatalf("unexpected workflow_ref B: %+v", taskB.WorkflowRef)
	}
	if taskB.Workflow == nil || len(taskB.Workflow.Steps) != 1 {
		t.Fatalf("snapshot B should be 1 step, got %+v", taskB.Workflow)
	}

	// 进度聚合：步骤 0/1 归任务 A，步骤 2 归任务 B
	progress, appErr := s.GetProjectWorkflowProgress(userID, project.ID)
	if appErr != nil {
		t.Fatalf("get progress: %v", appErr)
	}
	if progress.WorkflowName != "总流程" || len(progress.Steps) != 3 {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	if progress.Steps[0].TaskID != taskA.ID || progress.Steps[1].TaskID != taskA.ID {
		t.Fatalf("steps 0/1 should belong to task A: %+v", progress.Steps)
	}
	if progress.Steps[2].TaskID != taskB.ID {
		t.Fatalf("step 2 should belong to task B: %+v", progress.Steps[2])
	}
	if progress.Steps[0].Status != "pending" {
		t.Fatalf("step 0 should be pending (no todo yet), got %q", progress.Steps[0].Status)
	}

	// 跨任务查找：步骤 0（剧本创作）应由任务 A 负责，且能匹配到其 todo
	srcTask, srcTodos, ok := s.FindTaskForWorkflowStep(userID, project.ID, "总流程", "剧本创作")
	if !ok {
		t.Fatalf("cross-task step lookup failed")
	}
	if srcTask.ID != taskA.ID {
		t.Fatalf("source task should be A, got %s", srcTask.ID)
	}
	if len(srcTodos) == 0 || srcTodos[0] == nil || srcTodos[0].ID == "" {
		t.Fatalf("source todo should be matched")
	}
	// 不存在的步骤名返回 false
	if _, _, ok := s.FindTaskForWorkflowStep(userID, project.ID, "总流程", "不存在的步骤"); ok {
		t.Fatalf("lookup of unknown step should fail")
	}
}

func TestCreateTaskInvalidStepRange(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)

	wfs := []model.Workflow{{Name: "总流程", Steps: []model.WorkflowStep{{Name: "一步", Role: "writer"}}}}
	idx := 0
	if _, appErr := s.UpdateProject(Scope{UserID: userID}, project.ID, UpdateProjectInput{Workflows: wfs, PrimaryWorkflowIndex: &idx}); appErr != nil {
		t.Fatalf("set primary workflow: %v", appErr)
	}
	// 越界 step_to
	if _, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "坏任务",
		Description:     "bad",
		Priority:        "medium",
		AssigneeAgentID: developer.ID,
		WorkflowIndex:   wfIdxPtr(0),
		StepFrom:        0,
		StepTo:          5,
	}); appErr == nil {
		t.Fatalf("out-of-range step_to should fail")
	}
	// 不存在的工作流 index
	if _, appErr := s.CreateTaskByUser(userID, UserTaskCreateInput{
		ProjectID:       project.ID,
		Title:           "坏任务2",
		Description:     "bad",
		Priority:        "medium",
		AssigneeAgentID: developer.ID,
		WorkflowIndex:   wfIdxPtr(9),
		StepFrom:        0,
		StepTo:          0,
	}); appErr == nil {
		t.Fatalf("invalid workflow_index should fail")
	}
}

func wfIdxPtr(i int) *int { return &i }

// TestFindTaskForWorkflowStepSkipsCanceled verifies that the cross-task step
// resolver ignores canceled tasks and prefers the most recently updated active
// task when several tasks cover the same workflow step. This prevents a
// terminated test run from supplying a downstream step's inputs.
func TestFindTaskForWorkflowStepSkipsCanceled(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)
	projectID := project.ID

	s.projects[projectID].Workflows = []model.Workflow{
		{Name: "画宗AIGC无人工厂产线工作流", Steps: []model.WorkflowStep{{Name: "剧本创作", Role: "developer"}}},
	}
	s.projects[projectID].PrimaryWorkflowIndex = 0

	canceled := &model.TaskDetail{
		ID:        "task-canceled",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "canceled",
		Workflow:  &model.Workflow{Name: "画宗AIGC无人工厂产线工作流", Steps: []model.WorkflowStep{{Name: "剧本创作", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName: "画宗AIGC无人工厂产线工作流",
			StepFrom:      0,
			StepTo:        0,
		},
		Todos: []model.Todo{{
			ID: "tc", Order: 1, Title: "剧本创作", Status: "done",
			Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID},
		}},
		UpdatedAt: time.Now().UTC().Add(-time.Hour),
	}
	active := &model.TaskDetail{
		ID:        "task-active",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "done",
		Workflow:  &model.Workflow{Name: "画宗AIGC无人工厂产线工作流", Steps: []model.WorkflowStep{{Name: "剧本创作", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName: "画宗AIGC无人工厂产线工作流",
			StepFrom:      0,
			StepTo:        0,
		},
		Todos: []model.Todo{{
			ID: "ta", Order: 1, Title: "剧本创作", Status: "done",
			Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID},
		}},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[canceled.ID] = canceled
	s.tasks[active.ID] = active
	s.projectTasks[projectID] = append(s.projectTasks[projectID], canceled.ID, active.ID)

	// Mistyped workflow name must not match anything.
	if _, _, ok := s.FindTaskForWorkflowStep(userID, projectID, "画宗AIGC无人工厂产线工作线工作流", "剧本创作"); ok {
		t.Fatal("did not expect a match for a mistyped workflow name")
	}

	task, todos, ok := s.FindTaskForWorkflowStep(userID, projectID, "画宗AIGC无人工厂产线工作流", "剧本创作")
	if !ok {
		t.Fatal("expected to find an active task for the step")
	}
	if task.ID != active.ID {
		t.Fatalf("expected active (non-canceled) task %q, got %q", active.ID, task.ID)
	}
	if len(todos) != 1 || todos[0].ID != "ta" {
		t.Fatalf("expected todo %q, got %v", "ta", todoIDs(todos))
	}
}

// TestFindTaskForWorkflowStepReturnsAllTodosForStep verifies that a single
// workflow step split into several same-agent todos (preflight + deliverable)
// returns ALL matched todos — the produced artifact may be linked to any of
// them, and the previous "first todo only" behavior silently dropped the
// step's real output, leaving downstream inputs Resolved=false.
func TestFindTaskForWorkflowStepReturnsAllTodosForStep(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)
	projectID := project.ID

	s.projects[projectID].Workflows = []model.Workflow{
		{Name: "画宗AIGC无人工厂产线工作流", Steps: []model.WorkflowStep{
			{Name: "剧本创作", Role: "developer"},
			{Name: "分镜拆解", Role: "developer"},
		}},
	}
	s.projects[projectID].PrimaryWorkflowIndex = 0

	task := &model.TaskDetail{
		ID:        "task-storyboard",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "done",
		Workflow:  &model.Workflow{Name: "画宗AIGC无人工厂产线工作流", Steps: []model.WorkflowStep{{Name: "分镜拆解", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName:  "画宗AIGC无人工厂产线工作流",
			StepFrom:      1,
			StepTo:        1,
		},
		Todos: []model.Todo{
			{ID: "TD_01", Order: 1, Title: "核对分镜", Status: "done",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
			{ID: "TD_02", Order: 2, Title: "产出分镜表", Status: "done",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
		},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[task.ID] = task
	s.projectTasks[projectID] = append(s.projectTasks[projectID], task.ID)

	_, todos, ok := s.FindTaskForWorkflowStep(userID, projectID, "画宗AIGC无人工厂产线工作流", "分镜拆解")
	if !ok {
		t.Fatal("expected to find the storyboard task for the step")
	}
	if len(todos) != 2 {
		t.Fatalf("expected 2 matched todos, got %v", todoIDs(todos))
	}
	got := map[string]bool{}
	for _, td := range todos {
		got[td.ID] = true
	}
	if !got["TD_01"] || !got["TD_02"] {
		t.Fatalf("expected both TD_01 and TD_02, got %v", todoIDs(todos))
	}
}

func todoIDs(todos []*model.Todo) []string {
	out := make([]string, 0, len(todos))
	for _, td := range todos {
		if td == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, td.ID)
	}
	return out
}

// TestPrimaryStepAggregatesMultipleTodos verifies that when a single pipeline
// step is split into several same-agent todos (e.g. a preflight check plus the
// real deliverable), the progress node aggregates both the status and the
// outputs of all of them instead of only the first matched todo.
func TestPrimaryStepAggregatesMultipleTodos(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)
	projectID := project.ID

	s.projects[projectID].Workflows = []model.Workflow{
		{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "分镜拆解", Role: "developer"}}},
	}
	s.projects[projectID].PrimaryWorkflowIndex = 0

	task := &model.TaskDetail{
		ID:        "task-multi",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "done",
		Workflow:  &model.Workflow{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "分镜拆解", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName: "测试流程",
			StepFrom:      0,
			StepTo:        0,
		},
		Todos: []model.Todo{
			{ID: "t1", Order: 1, Title: "前置检查", Status: "done",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
			{ID: "t2", Order: 2, Title: "分镜拆解执行", Status: "done",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
		},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[task.ID] = task
	s.taskArtifacts[task.ID] = []model.TaskArtifact{
		{TransferID: "tr1", TodoID: "t1", ProjectFileID: "pf1", FileName: "check.md", FileSize: 10, Kind: model.ArtifactKindDeliverable},
		{TransferID: "tr2", TodoID: "t2", ProjectFileID: "pf2", FileName: "storyboard.md", FileSize: 20, Kind: model.ArtifactKindDeliverable},
	}
	s.projectTasks[projectID] = append(s.projectTasks[projectID], task.ID)

	prog, appErr := s.GetProjectWorkflowProgress(userID, projectID)
	if appErr != nil {
		t.Fatalf("progress: %v", appErr)
	}
	if len(prog.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(prog.Steps))
	}
	step := prog.Steps[0]
	if step.Status != "done" {
		t.Fatalf("expected done, got %s", step.Status)
	}
	if len(step.Outputs) != 2 {
		t.Fatalf("expected 2 aggregated outputs, got %d: %+v", len(step.Outputs), step.Outputs)
	}

	// An in-progress sub-todo must make the whole step in_progress.
	task.Todos[1].Status = "in_progress"
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if prog.Steps[0].Status != "in_progress" {
		t.Fatalf("expected in_progress, got %s", prog.Steps[0].Status)
	}

	// A failed sub-todo dominates the aggregated status.
	task.Todos[1].Status = "failed"
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if prog.Steps[0].Status != "failed" {
		t.Fatalf("expected failed, got %s", prog.Steps[0].Status)
	}

	// Restored to done: all outputs remain aggregated.
	task.Todos[1].Status = "done"
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if len(prog.Steps[0].Outputs) != 2 {
		t.Fatalf("expected 2 outputs after restore, got %d", len(prog.Steps[0].Outputs))
	}
}

// TestPipelineExcludesProcessArtifacts verifies that workflow progress nodes
// bind only FINAL deliverables: process artifacts (中间稿) and legacy records
// without an explicit kind must never surface as step outputs.
func TestPipelineExcludesProcessArtifacts(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)
	projectID := project.ID

	s.projects[projectID].Workflows = []model.Workflow{
		{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "剧本创作", Role: "developer"}}},
	}
	s.projects[projectID].PrimaryWorkflowIndex = 0

	task := &model.TaskDetail{
		ID:        "task-kind",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "done",
		Workflow:  &model.Workflow{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "剧本创作", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName:  "测试流程",
			StepFrom:      0,
			StepTo:        0,
		},
		Todos: []model.Todo{
			{ID: "k1", Order: 1, Title: "剧本创作执行", Status: "done",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
		},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[task.ID] = task
	s.taskArtifacts[task.ID] = []model.TaskArtifact{
		// process 中间稿：必须被排除。
		{TransferID: "tr-p", TodoID: "k1", ProjectFileID: "pf-p", FileName: "01-intake.md", FileSize: 5, Kind: model.ArtifactKindProcess},
		// 遗留记录（kind 为空）：同样不上工作流。
		{TransferID: "tr-l", TodoID: "k1", ProjectFileID: "pf-l", FileName: "legacy-draft.md", FileSize: 6},
		// 真正的交付文件：唯一可见。
		{TransferID: "tr-d", TodoID: "k1", ProjectFileID: "pf-d", FileName: "final-script.md", FileSize: 99, Kind: model.ArtifactKindDeliverable},
	}
	s.projectTasks[projectID] = append(s.projectTasks[projectID], task.ID)

	prog, appErr := s.GetProjectWorkflowProgress(userID, projectID)
	if appErr != nil {
		t.Fatalf("progress: %v", appErr)
	}
	step := prog.Steps[0]
	if len(step.Outputs) != 1 {
		t.Fatalf("expected 1 deliverable output, got %d: %+v", len(step.Outputs), step.Outputs)
	}
	if step.Outputs[0].FileName != "final-script.md" {
		t.Fatalf("unexpected output file: %s", step.Outputs[0].FileName)
	}
}

// TestCancelledTaskKeepsCompletedSteps verifies that cancelling a task does
// NOT wipe the pipeline history of its finished steps: without an active
// owner, a cancelled task remains the fallback owner — done todos stay done,
// a cancelled todo shows as cancelled. A newly dispatched active task takes
// the step back over.
func TestCancelledTaskKeepsCompletedSteps(t *testing.T) {
	s, userID, _, developer, project := seedWorkflowState(t)
	projectID := project.ID

	s.projects[projectID].Workflows = []model.Workflow{
		{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "步骤A", Role: "developer"}}},
	}
	s.projects[projectID].PrimaryWorkflowIndex = 0

	task := &model.TaskDetail{
		ID:        "task-cancel",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "canceled",
		Workflow:  &model.Workflow{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "步骤A", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName:  "测试流程",
			StepFrom:      0,
			StepTo:        0,
		},
		Todos: []model.Todo{
			{ID: "c1", Order: 1, Title: "步骤A执行", Status: "done",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
		},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[task.ID] = task
	s.projectTasks[projectID] = append(s.projectTasks[projectID], task.ID)

	// Cancelled owner, done todo: step must stay done (history preserved).
	prog, appErr := s.GetProjectWorkflowProgress(userID, projectID)
	if appErr != nil {
		t.Fatalf("progress: %v", appErr)
	}
	if prog.Steps[0].Status != "done" {
		t.Fatalf("expected done after cancel, got %s", prog.Steps[0].Status)
	}
	if prog.Steps[0].TaskID != task.ID {
		t.Fatalf("cancelled task should still own the step, got %+v", prog.Steps[0])
	}

	// Cancelled owner, cancelled todo: step shows cancelled.
	task.Todos[0].Status = "canceled"
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if prog.Steps[0].Status != "canceled" {
		t.Fatalf("expected cancelled step, got %s", prog.Steps[0].Status)
	}

	// Cancelled owner with NO todo at all: node falls back to unassigned.
	task.Todos = nil
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if prog.Steps[0].Status != "unassigned" {
		t.Fatalf("expected unassigned, got %s", prog.Steps[0].Status)
	}
	task.Todos = []model.Todo{
		{ID: "c1", Order: 1, Title: "步骤A执行", Status: "done",
			Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
	}

	// A just-planned active task WITHOUT todos must NOT blanket-hide the
	// cancelled task's completed history.
	planning := &model.TaskDetail{
		ID:        "task-planning",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "planning",
		Workflow:  &model.Workflow{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "步骤A", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName:  "测试流程",
			StepFrom:      0,
			StepTo:        0,
		},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[planning.ID] = planning
	s.projectTasks[projectID] = append(s.projectTasks[projectID], planning.ID)
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if prog.Steps[0].Status != "done" || prog.Steps[0].TaskID != task.ID {
		t.Fatalf("cancelled history must survive a planning task: status=%s task=%s", prog.Steps[0].Status, prog.Steps[0].TaskID)
	}

	// An active task WITH todos claiming the same step overrides everything.
	active := &model.TaskDetail{
		ID:        "task-active",
		UserID:    userID,
		ProjectID: projectID,
		Status:    "in_progress",
		Workflow:  &model.Workflow{Name: "测试流程", Steps: []model.WorkflowStep{{Name: "步骤A", Role: "developer"}}},
		WorkflowRef: &model.WorkflowRef{
			WorkflowIndex: 0,
			WorkflowName:  "测试流程",
			StepFrom:      0,
			StepTo:        0,
		},
		Todos: []model.Todo{
			{ID: "a1", Order: 1, Title: "步骤A执行", Status: "in_progress",
				Assignee: model.TodoAssignee{AgentID: developer.ID, Name: "developer", NodeID: developer.NodeID}},
		},
		UpdatedAt: time.Now().UTC(),
	}
	s.tasks[active.ID] = active
	s.projectTasks[projectID] = append(s.projectTasks[projectID], active.ID)
	prog, _ = s.GetProjectWorkflowProgress(userID, projectID)
	if prog.Steps[0].TaskID != active.ID {
		t.Fatalf("active task should override cancelled owner, got task %s", prog.Steps[0].TaskID)
	}
	if prog.Steps[0].Status != "in_progress" {
		t.Fatalf("expected in_progress, got %s", prog.Steps[0].Status)
	}
}
