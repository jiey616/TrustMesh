package model

import "time"

// Todo review gate statuses (Todo.ReviewStatus).
const (
	ReviewNone     = ""                 // no review required
	ReviewPending  = "pending_approval" // completed, waiting for human/PM review
	ReviewApproved = "approved"         // review passed; pipeline continues
	ReviewRejected = "rejected"         // review failed; rework triggered
)

// Task-level statuses. "awaiting_review" is aggregated when any todo in the
// task is blocked on human/PM review.
const (
	TaskStatusAwaitingReview = "awaiting_review"
	TaskStatusWaitingUser    = "waiting_user"
)

// Todo-level status for a todo parked on a human input request (todo.ask).
const TodoStatusWaitingUser = "waiting_user"

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

// TodoCancelNotice 描述一个被取消且可能在执行节点上仍有在途执行的 todo。
// 应用层把这些通知转发给执行节点（adapter lifecycle 取消链路），节点据此
// 调 gateway /stop 停掉在途 run。
type TodoCancelNotice struct {
	TodoID string `json:"todo_id"`
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`
}

type TodoResult struct {
	Summary  string         `json:"summary" bson:"summary"`
	Output   string         `json:"output" bson:"output"`
	Metadata map[string]any `json:"metadata" bson:"metadata"`
	// ActionItems are structured follow-up items declared by the executing
	// agent. They can be converted into new tasks by the PM (auto) or the user
	// (project "待办" tab). Per-report cap: 5 (agent should consolidate).
	ActionItems []ActionItem `json:"action_items,omitempty" bson:"action_items,omitempty"`
}

// ActionItem statuses.
const (
	ActionItemPending              = "pending"               // ready to convert
	ActionItemAwaitingConfirmation = "awaiting_confirmation" // PM unsure; user decides
	ActionItemConverted            = "converted"             // already turned into a task
)

// ActionItem is a structured follow-up item produced by an executing agent
// that may be turned into a new task assigned to a specific agent.
type ActionItem struct {
	Title           string    `json:"title" bson:"title"`
	Description     string    `json:"description,omitempty" bson:"description,omitempty"`
	AssigneeNodeID  string    `json:"assignee_node_id,omitempty" bson:"assignee_node_id,omitempty"`
	AssigneeRole    string    `json:"assignee_role,omitempty" bson:"assignee_role,omitempty"`
	Status          string    `json:"status" bson:"status"`
	ConvertedTaskID string    `json:"converted_task_id,omitempty" bson:"converted_task_id,omitempty"`
	ConfirmedBy     string    `json:"confirmed_by,omitempty" bson:"confirmed_by,omitempty"`
	CreatedAt       time.Time `json:"created_at" bson:"created_at"`
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
	// LastActivityAt is the most recent time the assignee reported progress on
	// this todo (todo.progress / todo.complete / todo.fail). The timeout
	// monitor uses it instead of AssignedAt so long-running tasks that keep
	// reporting progress are never spuriously retried.
	LastActivityAt *time.Time `json:"last_activity_at,omitempty" bson:"last_activity_at,omitempty"`
	// LastProgressAt records the last time the todo made PRODUCTIVE progress
	// (todo.progress / todo.complete / artifact received). Unlike
	// LastActivityAt it is never refreshed by error reports. The hard deadline
	// gate uses it so a todo that has been busy but unproductive for hours is
	// failed even if its reminder counter never reaches the limit.
	LastProgressAt *time.Time `json:"last_progress_at,omitempty" bson:"last_progress_at,omitempty"`
	RetryCount     int        `json:"retry_count" bson:"retry_count"`
	MaxRetries     int        `json:"max_retries" bson:"max_retries"`

	// RemindCount tracks how many timeout reminders have been sent to the
	// assignee without any response. When it reaches maxReminders the todo is
	// marked failed instead of being auto-redispatched (which caused duplicate
	// executions). Any agent activity (progress/complete/fail/ask) resets it.
	RemindCount int        `json:"remind_count" bson:"remind_count"`
	RemindAt    *time.Time `json:"remind_at,omitempty" bson:"remind_at,omitempty"`

	// Dispatch tracking: makes a failed sequential dispatch observable and
	// self-healable. Without these, dispatchNextTodo's silent returns leave
	// the todo stuck in pending with no trace (2026-09-10: TD_04 stalled 56min
	// because a single failed publish was swallowed without retry, event or
	// notification).
	DispatchAttempts int        `json:"dispatch_attempts,omitempty" bson:"dispatch_attempts,omitempty"`
	LastDispatchAt   *time.Time `json:"last_dispatch_at,omitempty" bson:"last_dispatch_at,omitempty"`
	LastDispatchErr  *string    `json:"last_dispatch_err,omitempty" bson:"last_dispatch_err,omitempty"`

	// ReopenCount counts how many times a terminal todo (failed / canceled /
	// done) was brought back to in_progress so late work could still land.
	// Capped by maxReopens: without a cap, an agent that keeps uploading after
	// its todo failed could keep the todo alive forever and the task would
	// never settle.
	ReopenCount int `json:"reopen_count,omitempty" bson:"reopen_count,omitempty"`

	// ReviewStatus tracks the human/PM review gate for a completed todo.
	// Values: "" (no review needed), "pending_approval" (waiting for review),
	// "approved" (review passed), "rejected" (review rejected; rework triggered).
	// While pending_approval the sequential dispatch is blocked until the
	// reviewer approves.
	ReviewStatus string `json:"review_status,omitempty" bson:"review_status,omitempty"`
	// NeedReview marks the todo as requiring human/PM review after completion.
	// Inherited from the workflow step (step.need_review) when the PM finalizes
	// a plan against a workflow; an executor may also declare it on todo.complete.
	NeedReview bool `json:"need_review,omitempty" bson:"need_review,omitempty"`
	// ReviewReason records the rejection reason when a todo is sent back for
	// rework (from the human reviewer or the PM agent).
	ReviewReason *string `json:"review_reason,omitempty" bson:"review_reason,omitempty"`
	// ReworkCount counts how many times this todo has been sent back for
	// rework via a review rejection. Independent from RetryCount (timeout
	// retries). Default max is 3; exceeding it fails the todo.
	ReworkCount int `json:"rework_count,omitempty" bson:"rework_count,omitempty"`
	// MaxReworks is the upper bound for ReworkCount (defaults to 3 when 0).
	MaxReworks int `json:"max_reworks,omitempty" bson:"max_reworks,omitempty"`
	// Questions are human input requests made by the assignee while executing
	// (todo.ask). While an unanswered required question exists the todo is
	// parked in waiting_user; answers are appended in place so the full
	// question/answer history is preserved for review and audit.
	Questions []TodoQuestion `json:"questions,omitempty" bson:"questions,omitempty"`

	// Outputs records the actual files produced by this todo, bound to the
	// workflow step's declared StepOutput by name. Populated when an agent
	// uploads an artifact with `--metadata outputName=<name>`. Downstream
	// steps resolve their StepInput.Source against these entries to fetch the
	// "上一个流程的输出文件".
	Outputs []TodoOutput `json:"outputs,omitempty" bson:"outputs,omitempty"`
}

// TodoOutput binds a produced artifact to a workflow step's declared output.
type TodoOutput struct {
	OutputName string `json:"output_name" bson:"output_name"` // 匹配 StepOutput.Name
	ArtifactID string `json:"artifact_id,omitempty" bson:"artifact_id,omitempty"`
	FileRef    string `json:"file_ref,omitempty" bson:"file_ref,omitempty"` // ProjectFile ID
}

// TodoQuestion is a single human-input request recorded on a todo.
type TodoQuestion struct {
	ID       string   `json:"id" bson:"id"`
	Question string   `json:"question" bson:"question"`
	Options  []string `json:"options,omitempty" bson:"options,omitempty"`
	Required bool     `json:"required" bson:"required"`
	// AskedAt is when the agent posted the question (todo.ask received).
	AskedAt time.Time `json:"asked_at" bson:"asked_at"`
	// Answer is empty while pending; set once the user answers or the
	// required=false timeout fires (Answer == "__timeout__").
	Answer     string     `json:"answer,omitempty" bson:"answer,omitempty"`
	AnsweredBy string     `json:"answered_by,omitempty" bson:"answered_by,omitempty"`
	AnsweredAt *time.Time `json:"answered_at,omitempty" bson:"answered_at,omitempty"`
	TimedOut   bool       `json:"timed_out,omitempty" bson:"timed_out,omitempty"`
}

type ActorRef struct {
	ActorType string `json:"actor_type" bson:"actor_type"`
	ActorID   string `json:"actor_id" bson:"actor_id"`
	ActorName string `json:"actor_name" bson:"actor_name"`
}

// Artifact file-nature kinds.
const (
	// ArtifactKindDeliverable marks a declared final deliverable bound to a
	// workflow step output (upload carried metadata outputName).
	ArtifactKindDeliverable = "deliverable"
	// ArtifactKindProcess marks intermediate/process files. They stay visible
	// in the file tree but never participate in step-input resolution.
	ArtifactKindProcess = "process"
)

type TaskArtifact struct {
	TransferID    string    `json:"transfer_id" bson:"_id"`
	OrgID string `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属
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
	// ProjectFileID links this artifact to the auto-created ProjectFile so the
	// pipeline progress panel can resolve it for preview/download even when the
	// agent did not declare an output name.
	ProjectFileID string `json:"project_file_id,omitempty" bson:"project_file_id,omitempty"`
	// Kind classifies the file nature: "deliverable" (declared an outputName
	// and bound to a workflow step output) or "process" (everything else).
	// Only deliverables participate in downstream step-input resolution;
	// process files stay visible in the file tree but never feed dispatch.
	Kind string `json:"kind,omitempty" bson:"kind,omitempty"`
	// OutputName is the binding to a workflow StepOutput.Name. Set from
	// transfer metadata outputName; a non-empty value marks the artifact as a
	// deliverable and populates the owning todo's Outputs.
	OutputName string `json:"output_name,omitempty" bson:"output_name,omitempty"`
	// Orphan marks a deliverable that arrived AFTER its todo had already
	// reached a terminal state. Surfaced so "failed todo carrying a done
	// deliverable" reads as a late arrival instead of a contradiction
	// (2026-09-10 TD_06: failed at 23:08, 4/6 videos actually delivered and
	// bound at 00:08 with no way to reconcile the two).
	Orphan bool `json:"orphan,omitempty" bson:"orphan,omitempty"`
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
	ID          string `json:"id" bson:"_id"`
	UserID      string `json:"-" bson:"user_id"`
	OrgID       string `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	ProjectID   string `json:"project_id" bson:"project_id"`
	Title       string `json:"title" bson:"title"`
	Description string `json:"description" bson:"description"`
	Status      string `json:"status" bson:"status"`
	Priority    string `json:"priority" bson:"priority"`
	// SourceTaskID links this task back to the task whose action item created
	// it (action-items → tasks conversion).
	SourceTaskID string `json:"source_task_id,omitempty" bson:"source_task_id,omitempty"`
	// Workflow is the workflow snapshot this task plans against. Copied from
	// the project default at creation time (or overridden by the request).
	// The PM must honor it when producing task.plan_ready.
	Workflow *Workflow `json:"workflow,omitempty" bson:"workflow,omitempty"`
	// WorkflowRef records where this task sits in the project's overall
	// pipeline (the primary workflow): which workflow it belongs to and the
	// contiguous step range (by index into the workflow's Steps) this task
	// owns. Empty when the task is not part of a project pipeline.
	WorkflowRef   *WorkflowRef       `json:"workflow_ref,omitempty" bson:"workflow_ref,omitempty"`
	// DeliverScope 记录 PM 声明的交付终点步骤（部分交付场景，如「只要到资产图」）。
	// 为空表示全量交付。仅作记录与审计，不参与派发。
	DeliverScope  *TaskDeliverScope  `json:"deliver_scope,omitempty" bson:"deliver_scope,omitempty"`
	PMAgentID     string             `json:"-" bson:"pm_agent_id"`
	PMAgent       PMAgentSummary     `json:"pm_agent" bson:"pm_agent"`
	Messages      []TaskMessage      `json:"messages,omitempty" bson:"messages,omitempty"`
	Todos         []Todo             `json:"todos" bson:"todos"`
	Artifacts     []TaskArtifact     `json:"artifacts" bson:"-"`
	AttachedFiles []TaskAttachedFile `json:"attached_files" bson:"attached_files"`
	Result        TaskResult         `json:"result" bson:"result"`
	Version       int                `json:"version" bson:"version"`
	CanceledAt    *time.Time         `json:"canceled_at" bson:"canceled_at"`
	CanceledBy    *ActorRef          `json:"canceled_by" bson:"canceled_by"`
	CancelReason  *string            `json:"cancel_reason" bson:"cancel_reason"`
	CreatedAt     time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at" bson:"updated_at"`
}

// TaskAttachedFile is a lightweight reference to a project file attached to a task.
type TaskAttachedFile struct {
	ID       string `json:"id" bson:"id"`
	FileName string `json:"file_name" bson:"file_name"`
	FileSize int64  `json:"file_size" bson:"file_size"`
	MimeType string `json:"mime_type" bson:"mime_type"`
	Source   string `json:"source" bson:"source"` // "user_upload" | "agent_artifact"
}

// TaskDeliverScope 记录任务的交付范围：规划只覆盖到工作流的 UpToStep 步（含）为止。
// 由 PM 在 task.plan_ready 的 deliver_scope 中声明；为空表示全量交付。
type TaskDeliverScope struct {
	UpToStep string `json:"up_to_step" bson:"up_to_step"`
}

func (t *TaskDetail) NextDispatchableTodo() *Todo {
	if t == nil {
		return nil
	}
	for i := range t.Todos {
		todo := &t.Todos[i]
		switch todo.Status {
		case "done":
			// A completed todo awaiting human/PM review blocks the sequential
			// pipeline: later todos must not dispatch until approval.
			if todo.ReviewStatus == ReviewPending {
				return nil
			}
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
