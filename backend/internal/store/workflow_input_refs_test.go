package store

import (
	"testing"
	"time"

	"trustmesh/backend/internal/model"
)

// inputRefsTask builds the minimum registerTask needs: the recorder only looks
// the task up by ID in s.tasks and then finds the todo by ID.
func inputRefsTask(id, todoStatus string) *model.TaskDetail {
	now := time.Now().UTC()
	return &model.TaskDetail{
		ID:        id,
		UserID:    "user-1",
		Status:    "in_progress",
		CreatedAt: now,
		UpdatedAt: now,
		Todos: []model.Todo{
			{ID: "TD_01", Order: 1, Title: "资产制作", Status: todoStatus, CreatedAt: now},
		},
	}
}

// The store is a pure carrier: it must persist whatever the caller observed,
// including the timestamp the caller stamped. So the fixture stamps one.
func sampleInputRefs(state string) []model.TodoInput {
	checkedAt := time.Now().UTC()
	return []model.TodoInput{
		{
			Name:       "分组视频生成提示词",
			SourceStep: "分镜分组",
			OutputName: "分组视频生成提示词",
			State:      state,
			CheckedAt:  &checkedAt,
		},
	}
}

func TestRecordTodoInputRefsStoresSnapshot(t *testing.T) {
	s := New()
	task := inputRefsTask("task-refs", "in_progress")
	registerTask(s, "proj-1", task)

	if appErr := s.RecordTodoInputRefs(task.ID, "TD_01", sampleInputRefs(model.InputStateMissing)); appErr != nil {
		t.Fatalf("record: %v", appErr)
	}

	got := s.tasks[task.ID].Todos[0].Inputs
	if len(got) != 1 {
		t.Fatalf("inputs count = %d, want 1", len(got))
	}
	if got[0].Name != "分组视频生成提示词" {
		t.Errorf("name = %q", got[0].Name)
	}
	if got[0].SourceStep != "分镜分组" {
		t.Errorf("source_step = %q", got[0].SourceStep)
	}
	if got[0].State != model.InputStateMissing {
		t.Errorf("state = %q, want %q", got[0].State, model.InputStateMissing)
	}
	if got[0].CheckedAt == nil {
		t.Error("checked_at should be recorded by the caller; the store must not drop it")
	}
}

// The stored value is the LATEST observation, not a history (history lives in
// the event stream) — so a second call must replace, not append.
func TestRecordTodoInputRefsReplacesSnapshot(t *testing.T) {
	s := New()
	task := inputRefsTask("task-refs", "in_progress")
	registerTask(s, "proj-1", task)

	if appErr := s.RecordTodoInputRefs(task.ID, "TD_01", sampleInputRefs(model.InputStateMissing)); appErr != nil {
		t.Fatalf("record 1: %v", appErr)
	}
	if appErr := s.RecordTodoInputRefs(task.ID, "TD_01", sampleInputRefs(model.InputStateResolved)); appErr != nil {
		t.Fatalf("record 2: %v", appErr)
	}

	got := s.tasks[task.ID].Todos[0].Inputs
	if len(got) != 1 {
		t.Fatalf("inputs count = %d, want 1 (replace, not append)", len(got))
	}
	if got[0].State != model.InputStateResolved {
		t.Errorf("state = %q, want %q", got[0].State, model.InputStateResolved)
	}
}

// "Nothing was resolved this time" must never erase a failure the user still
// needs to see. A todo with no declared inputs legitimately yields an empty
// slice, and that must be a no-op rather than a clear.
func TestRecordTodoInputRefsEmptyIsNoop(t *testing.T) {
	s := New()
	task := inputRefsTask("task-refs", "in_progress")
	registerTask(s, "proj-1", task)

	if appErr := s.RecordTodoInputRefs(task.ID, "TD_01", sampleInputRefs(model.InputStateMissing)); appErr != nil {
		t.Fatalf("record: %v", appErr)
	}
	if appErr := s.RecordTodoInputRefs(task.ID, "TD_01", nil); appErr != nil {
		t.Fatalf("empty record must be a harmless no-op, got: %v", appErr)
	}

	got := s.tasks[task.ID].Todos[0].Inputs
	if len(got) != 1 || got[0].State != model.InputStateMissing {
		t.Fatalf("empty slice must not clear the previous snapshot, got %+v", got)
	}
}

// A failed / canceled todo is exactly the one whose unresolved inputs the user
// inspects before deciding to re-dispatch — so the terminal-state guards that
// RecordTodoDispatch applies must NOT apply here.
func TestRecordTodoInputRefsWritesTerminalTodo(t *testing.T) {
	for _, status := range []string{"failed", "canceled", "done"} {
		t.Run(status, func(t *testing.T) {
			s := New()
			task := inputRefsTask("task-refs", status)
			registerTask(s, "proj-1", task)

			if appErr := s.RecordTodoInputRefs(task.ID, "TD_01", sampleInputRefs(model.InputStateMissing)); appErr != nil {
				t.Fatalf("record on a %s todo: %v", status, appErr)
			}
			if got := s.tasks[task.ID].Todos[0].Inputs; len(got) != 1 {
				t.Fatalf("expected the snapshot to land on a %s todo, got %+v", status, got)
			}
		})
	}
}

func TestRecordTodoInputRefsUnknownTargets(t *testing.T) {
	s := New()
	task := inputRefsTask("task-refs", "in_progress")
	registerTask(s, "proj-1", task)

	if appErr := s.RecordTodoInputRefs("no-such-task", "TD_01", sampleInputRefs(model.InputStateMissing)); appErr == nil {
		t.Error("expected an error for an unknown task id")
	}
	if appErr := s.RecordTodoInputRefs(task.ID, "no-such-todo", sampleInputRefs(model.InputStateMissing)); appErr == nil {
		t.Error("expected an error for an unknown todo id")
	}
	if got := s.tasks[task.ID].Todos[0].Inputs; len(got) != 0 {
		t.Fatalf("failed writes must not mutate anything, got %+v", got)
	}
}
