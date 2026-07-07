package protocol

import "trustmesh/backend/internal/model"

type WebhookPayload struct {
	NodeID     string         `json:"nodeId"`
	Type       string         `json:"type"`
	From       string         `json:"from"`
	SessionKey string         `json:"sessionKey"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata"`
}

type TaskCreatePayload struct {
	ProjectID   string                  `json:"project_id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Todos       []TaskCreateTodoPayload `json:"todos"`
}

type TaskReplyPayload struct {
	TaskID   string          `json:"task_id"`
	Content  string          `json:"content"`
	UIBlocks []model.UIBlock `json:"ui_blocks,omitempty"`
}

type TaskPlanReadyPayload struct {
	TaskID      string                  `json:"task_id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Todos       []TaskCreateTodoPayload `json:"todos"`
}

type TaskCreateTodoPayload struct {
	ID             string `json:"id"`
	Order          int    `json:"order,omitempty"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	AssigneeNodeID string `json:"assignee_node_id"`
}

type TodoProgressPayload struct {
	TaskID  string `json:"task_id"`
	TodoID  string `json:"todo_id"`
	Message string `json:"message"`
}

type TodoCompletePayload struct {
	TaskID string           `json:"task_id"`
	TodoID string           `json:"todo_id"`
	Result model.TodoResult `json:"result"`
}

type TodoFailPayload struct {
	TaskID string `json:"task_id"`
	TodoID string `json:"todo_id"`
	Error  string `json:"error"`
}

type TaskCommentPayload struct {
	TaskID  string `json:"task_id"`
	TodoID  string `json:"todo_id,omitempty"`
	Content string `json:"content"`
}

type TaskMentionPayload struct {
	TaskID          string `json:"task_id"`
	ProjectID       string `json:"project_id"`
	CommentID       string `json:"comment_id"`
	TodoID          string `json:"todo_id,omitempty"`
	TaskTitle       string `json:"task_title"`
	TaskDescription string `json:"task_description,omitempty"`
	TaskStatus      string `json:"task_status"`
	TaskPriority    string `json:"task_priority"`
	TodoTitle       string `json:"todo_title,omitempty"`
	AuthorName      string `json:"author_name"`
	UserContent     string `json:"user_content"`
	Content         string `json:"content"`
}

type TaskCreatedPayload struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
}

type TaskPlanRejectedPayload struct {
	TaskID   string `json:"task_id"`
	Feedback string `json:"feedback"`
}

type TaskStatusChangedPayload struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	ActorNodeID string `json:"actor_node_id,omitempty"`
	Cause       string `json:"cause,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Version     int    `json:"version"`
}

type TodoExecBrief struct {
	Objective    string `json:"objective"`
	MustUseSkill string `json:"must_use_skill"`
}

type TodoAssignedPayload struct {
	TaskID       string            `json:"task_id"`
	TodoID       string            `json:"todo_id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Content      string            `json:"content"`
	ExecBrief    *TodoExecBrief    `json:"exec_brief,omitempty"`
	TaskContext  *TaskContext      `json:"task_context,omitempty"`
	PriorResults []TodoPriorResult `json:"prior_results,omitempty"`
	AttachedFiles []TaskAttachedFileRef `json:"attached_files,omitempty"`
}

// TaskAttachedFileRef carries file info embedded in task messages.
type TaskAttachedFileRef struct {
	ID          string `json:"id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	MimeType    string `json:"mime_type"`
	Source      string `json:"source"`
	DownloadUrl string `json:"download_url,omitempty"`
}

// TaskAttachedFilesToRefs converts model-level attached files to protocol refs.
func TaskAttachedFilesToRefs(files []model.TaskAttachedFile) []TaskAttachedFileRef {
	if len(files) == 0 {
		return nil
	}
	out := make([]TaskAttachedFileRef, len(files))
	for i, f := range files {
		out[i] = TaskAttachedFileRef{
			ID:       f.ID,
			FileName: f.FileName,
			FileSize: f.FileSize,
			MimeType: f.MimeType,
			Source:   f.Source,
		}
	}
	return out
}

// TaskContext provides task-level context for the assigned agent.
// Included only on the agent's first todo in a task; omitted for subsequent
// todos where the session already contains this information.
type TaskContext struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Todos       []TodoSummary `json:"todos"`
}

type TodoSummary struct {
	TodoID       string `json:"todo_id"`
	Order        int    `json:"order"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	AssigneeName string `json:"assignee_name"`
	IsCurrent    bool   `json:"is_current,omitempty"`
}

// TodoPriorResult carries the result of a previously completed todo.
// For first-time agents: all prior results are included.
// For returning agents: only cross-agent results (work done by other agents
// that this agent hasn't seen in the current session).
type TodoPriorResult struct {
	TodoID  string `json:"todo_id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Output  string `json:"output,omitempty"`
}

type TodoStatusChangedPayload struct {
	TaskID      string `json:"task_id"`
	TodoID      string `json:"todo_id"`
	Status      string `json:"status"`
	ActorNodeID string `json:"actor_node_id,omitempty"`
	Cause       string `json:"cause,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Version     int    `json:"version"`
	Message     string `json:"message,omitempty"`
}

// Knowledge base protocol payloads

type KnowledgeQueryPayload struct {
	QueryID   string  `json:"query_id"`
	Query     string  `json:"query"`
	ProjectID string  `json:"project_id"`
	TopK      int     `json:"top_k,omitempty"`
	MinScore  float64 `json:"min_score,omitempty"`
}

type KnowledgeResultPayload struct {
	QueryID   string                `json:"query_id"`
	ProjectID string                `json:"project_id"`
	Results   []KnowledgeResultItem `json:"results"`
	Error     string                `json:"error,omitempty"`
}

type KnowledgeResultItem struct {
	ChunkID       string         `json:"chunk_id"`
	DocumentID    string         `json:"document_id"`
	DocumentTitle string         `json:"document_title"`
	Content       string         `json:"content"`
	Score         float64        `json:"score"`
	ChunkIndex    int            `json:"chunk_index"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// task.context.query — Agent requests task context on demand
type ContextQueryPayload struct {
	TaskID string `json:"task_id"`
}

// task.context.result — TrustMesh responds with full task context snapshot
type ContextResultPayload struct {
	TaskID      string            `json:"task_id"`
	TaskContext *TaskContext      `json:"task_context"`
	AllResults  []TodoPriorResult `json:"all_results,omitempty"`
}

type PMTaskProject struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PMTaskAgent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	NodeID       string   `json:"node_id"`
	Role         string   `json:"role"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
}

// PMTaskMessage is published by TrustMesh to PM Agent during planning phase.
type PMTaskMessage struct {
	SchemaVersion   string               `json:"schema_version"`
	TaskID          string               `json:"task_id"`
	ProjectID       string               `json:"project_id"`
	Content         string               `json:"content"`
	UserContent     string               `json:"user_content"`
	IsInitial       bool                 `json:"is_initial_message"`
	UserUIResponse  *model.UIResponse    `json:"user_ui_response,omitempty"`
	Project         *PMTaskProject       `json:"project,omitempty"`
	CandidateAgents []PMTaskAgent        `json:"candidate_agents,omitempty"`
	AttachedFiles   []TaskAttachedFileRef `json:"attached_files,omitempty"`
}

// ——— PM Agent dynamic todo management ———

// TodoAddPayload is published by PM Agent to dynamically append a new TODO.
type TodoAddPayload struct {
	TaskID          string `json:"task_id"`
	Title           string `json:"title"`
	Description    string `json:"description"`
	AssigneeNodeID string `json:"assignee_node_id"`
	BeforeTodoID   string `json:"before_todo_id,omitempty"` // empty = append to end
}

// TodoModifyPayload is published by PM Agent to dynamically update a TODO.
type TodoModifyPM struct {
	TaskID          string `json:"task_id"`
	TodoID          string `json:"todo_id"`
	Title           string `json:"title,omitempty"`
	Description    string `json:"description,omitempty"`
	AssigneeNodeID string `json:"assignee_node_id,omitempty"`
}
