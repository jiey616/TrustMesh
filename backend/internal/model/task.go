package model

import "time"

type TaskSummary struct {
	ID                 string    `json:"id" bson:"id"`
	Title              string    `json:"title" bson:"title"`
	Status             string    `json:"status" bson:"status"`
	Priority           string    `json:"priority" bson:"priority"`
	TodoCount          int       `json:"todo_count" bson:"todo_count"`
	CompletedTodoCount int       `json:"completed_todo_count" bson:"completed_todo_count"`
	CreatedAt          time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" bson:"updated_at"`
}

type TodoAssignee struct {
	AgentID string `json:"agent_id" bson:"agent_id"`
	Name    string `json:"name" bson:"name"`
	NodeID  string `json:"node_id" bson:"node_id"`
}

type TodoResult struct {
	Summary  string         `json:"summary" bson:"summary"`
	Output   string         `json:"output" bson:"output"`
	Metadata map[string]any `json:"metadata" bson:"metadata"`
}

type Todo struct {
	ID           string       `json:"id" bson:"id"`
	Order        int          `json:"order" bson:"order"`
	Title        string       `json:"title" bson:"title"`
	Description  string       `json:"description" bson:"description"`
	Status       string       `json:"status" bson:"status"`
	Assignee     TodoAssignee `json:"assignee" bson:"assignee"`
	StartedAt    *time.Time   `json:"started_at" bson:"started_at"`
	CompletedAt  *time.Time   `json:"completed_at" bson:"completed_at"`
	FailedAt     *time.Time   `json:"failed_at" bson:"failed_at"`
	CanceledAt   *time.Time   `json:"canceled_at" bson:"canceled_at"`
	Error        *string      `json:"error" bson:"error"`
	CancelReason *string      `json:"cancel_reason" bson:"cancel_reason"`
	Result       TodoResult   `json:"result" bson:"result"`
	CreatedAt    time.Time    `json:"created_at" bson:"created_at"`
	AssignedAt   *time.Time   `json:"assigned_at,omitempty" bson:"assigned_at,omitempty"`
	RetryCount   int          `json:"retry_count" bson:"retry_count"`
	MaxRetries   int          `json:"max_retries" bson:"max_retries"`
}

type ActorRef struct {
	ActorType string `json:"actor_type" bson:"actor_type"`
	ActorID   string `json:"actor_id" bson:"actor_id"`
	ActorName string `json:"actor_name" bson:"actor_name"`
}

type TaskArtifact struct {
	TransferID    string    `json:"transfer_id" bson:"_id"`
	TaskID        string    `json:"task_id" bson:"task_id"`
	TodoID        string    `json:"todo_id,omitempty" bson:"todo_id,omitempty"`
	FileName      string    `json:"file_name" bson:"file_name"`
	FileSize      int64     `json:"file_size" bson:"file_size"`
	LocalPath     string    `json:"-" bson:"local_path"`
	MimeType      string    `json:"mime_type" bson:"mime_type"`
	FromNodeID    string    `json:"from_node_id" bson:"from_node_id"`
	FromAgentID   string    `json:"from_agent_id" bson:"from_agent_id"`
	FromAgentName string    `json:"from_agent_name" bson:"from_agent_name"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
}

type TaskResult struct {
	Summary     string         `json:"summary" bson:"summary"`
	FinalOutput string         `json:"final_output" bson:"final_output"`
	Metadata    map[string]any `json:"metadata" bson:"metadata"`
}

type TaskListItem struct {
	ID                 string         `json:"id" bson:"id"`
	ProjectID          string         `json:"project_id" bson:"project_id"`
	Title              string         `json:"title" bson:"title"`
	Description        string         `json:"description" bson:"description"`
	Status             string         `json:"status" bson:"status"`
	Priority           string         `json:"priority" bson:"priority"`
	PMAgent            PMAgentSummary `json:"pm_agent" bson:"pm_agent"`
	TodoCount          int            `json:"todo_count" bson:"todo_count"`
	CompletedTodoCount int            `json:"completed_todo_count" bson:"completed_todo_count"`
	FailedTodoCount    int            `json:"failed_todo_count" bson:"failed_todo_count"`
	CreatedAt          time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at" bson:"updated_at"`
}

type TaskDetail struct {
	ID           string             `json:"id" bson:"_id"`
	UserID       string             `json:"-" bson:"user_id"`
	ProjectID    string             `json:"project_id" bson:"project_id"`
	Title        string             `json:"title" bson:"title"`
	Description  string             `json:"description" bson:"description"`
	Status       string             `json:"status" bson:"status"`
	Priority     string             `json:"priority" bson:"priority"`
	PMAgentID    string             `json:"-" bson:"pm_agent_id"`
	PMAgent      PMAgentSummary     `json:"pm_agent" bson:"pm_agent"`
	Messages     []TaskMessage      `json:"messages,omitempty" bson:"messages,omitempty"`
	Todos        []Todo             `json:"todos" bson:"todos"`
	Artifacts    []TaskArtifact     `json:"artifacts" bson:"-"`
	AttachedFiles []TaskAttachedFile `json:"attached_files" bson:"attached_files"`
	Result       TaskResult         `json:"result" bson:"result"`
	Version      int                `json:"version" bson:"version"`
	CanceledAt   *time.Time         `json:"canceled_at" bson:"canceled_at"`
	CanceledBy   *ActorRef          `json:"canceled_by" bson:"canceled_by"`
	CancelReason *string            `json:"cancel_reason" bson:"cancel_reason"`
	CreatedAt    time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at" bson:"updated_at"`
}

// TaskAttachedFile is a lightweight reference to a project file attached to a task.
type TaskAttachedFile struct {
	ID       string `json:"id" bson:"id"`
	FileName string `json:"file_name" bson:"file_name"`
	FileSize int64  `json:"file_size" bson:"file_size"`
	MimeType string `json:"mime_type" bson:"mime_type"`
	Source   string `json:"source" bson:"source"` // "user_upload" | "agent_artifact"
}

func (t *TaskDetail) NextDispatchableTodo() *Todo {
	if t == nil {
		return nil
	}
	for i := range t.Todos {
		todo := &t.Todos[i]
		switch todo.Status {
		case "done":
			continue
		case "pending":
			return todo
		default:
			return nil
		}
	}
	return nil
}

func (t *TaskDetail) CanDispatchTodo(todoID string) bool {
	next := t.NextDispatchableTodo()
	return next != nil && next.ID == todoID
}
