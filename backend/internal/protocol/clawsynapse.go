package protocol

import (
	"encoding/json"
	"time"

	"trustmesh/backend/internal/model"
)

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
	// SourceTaskID records the task whose action item created this task
	// (action-items → tasks conversion, for traceability).
	SourceTaskID string `json:"source_task_id,omitempty"`
}

// TaskResultPayload is pushed backend → PM after a todo completes with
// unconverted action items, so the PM can convert them into new tasks.
type TaskResultPayload struct {
	TaskID      string             `json:"task_id"`
	TodoID      string             `json:"todo_id"`
	TodoTitle   string             `json:"todo_title"`
	ProjectID   string             `json:"project_id"`
	ActionItems []model.ActionItem `json:"action_items"`
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
	// DeliverScope 允许 PM 声明本次交付到工作流的哪一步为止（用户只要求部分交付时）。
	// 为空表示必须覆盖工作流的全部步骤（历史行为）。
	DeliverScope *PlanDeliverScope `json:"deliver_scope,omitempty"`
}

// PlanDeliverScope 声明任务规划的交付范围。
// UpToStep 是工作流中的步骤名（如「资产制作」）：校验时只要求覆盖到该步（含）
// 为止，其后的步骤允许不规划。UpToStep 必须真实存在于工作流中，否则为无效声明。
type PlanDeliverScope struct {
	UpToStep string `json:"up_to_step,omitempty"`
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

// FlexibleTodoResult tolerates LLM agents that report "result" as a plain
// string instead of the structured object. The string is treated as the
// result summary. Measured 2026-09-04: the 山雨 screenwriter node sent a
// correct todo.complete whose "result" was a hand-written JSON string; the
// strict object decode 400-rejected it ("invalid todo.complete message"),
// the node then retried over the chat channel and stalled the task.
type FlexibleTodoResult model.TodoResult

func (r *FlexibleTodoResult) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*r = FlexibleTodoResult{Summary: text}
		return nil
	}
	var base model.TodoResult
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	*r = FlexibleTodoResult(base)
	return nil
}

type TodoCompletePayload struct {
	TaskID  string             `json:"task_id"`
	TodoID  string             `json:"todo_id"`
	Result  FlexibleTodoResult `json:"result"`
	Content string             `json:"content"` // 兼容外部 Agent 用 content 回报完成摘要而非 result

	// NeedReview marks the completed todo as awaiting human/PM review. While
	// pending_approval, sequential dispatch of later todos is blocked until
	// the reviewer approves (or rejects → rework).
	NeedReview *bool `json:"need_review,omitempty"`
	// ReturnPrevious asks to send the previous todo (order-1, the todo this
	// one reviews) back for rework. Used by review-style todos (e.g. a quality
	// gate) whose completion verdict is that the predecessor's output failed.
	ReturnPrevious *bool `json:"return_previous,omitempty"`
	// ReworkReason carries the concrete rework verdict (e.g. the reviewer's
	// must-fix list) when ReturnPrevious is set. It is forwarded verbatim to
	// the reworked assignee's todo.assigned so the executor knows exactly what
	// to fix instead of reworking blind.
	ReworkReason string `json:"rework_reason,omitempty"`
}

// TodoReviewPayload is carried by the todo.review message type: a human
// reviewer (via UI) or the PM agent (via ClawSynapse) approves or rejects a
// completed todo that is awaiting review.
type TodoReviewPayload struct {
	TaskID string `json:"task_id"`
	TodoID string `json:"todo_id"`
	// Action: "approve" | "reject".
	Action string `json:"action"`
	// Reason is required when Action == "reject" (shown to the assignee on
	// rework); optional when approving.
	Reason string `json:"reason,omitempty"`
}

type TodoFailPayload struct {
	TaskID string `json:"task_id"`
	TodoID string `json:"todo_id"`
	Error  string `json:"error"`
}

// TodoAskPayload — agent requests human input while executing a todo.
// Backend parks the todo in waiting_user until the user answers.
type TodoAskPayload struct {
	TaskID     string   `json:"task_id"`
	TodoID     string   `json:"todo_id"`
	QuestionID string   `json:"question_id"`
	Question   string   `json:"question"`
	Options    []string `json:"options,omitempty"`
	// Required: when true (default) the todo stays parked until answered;
	// when false the todo auto-resumes after AskTimeout.
	Required *bool `json:"required,omitempty"`
}

// TodoAnswerPayload — backend pushes the user's answer to the assignee agent,
// resuming the parked todo.
type TodoAnswerPayload struct {
	TaskID     string    `json:"task_id"`
	TodoID     string    `json:"todo_id"`
	QuestionID string    `json:"question_id"`
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	AnsweredBy string    `json:"answered_by,omitempty"`
	AnsweredAt time.Time `json:"answered_at"`
	// TimedOut is true when the answer was auto-generated by the timeout
	// (required=false, no user response); Answer == "__timeout__".
	TimedOut bool `json:"timed_out,omitempty"`
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
	TaskID        string                `json:"task_id"`
	TodoID        string                `json:"todo_id"`
	Title         string                `json:"title"`
	Description   string                `json:"description"`
	Content       string                `json:"content"`
	ExecBrief     *TodoExecBrief        `json:"exec_brief,omitempty"`
	TaskContext   *TaskContext          `json:"task_context,omitempty"`
	PriorResults  []TodoPriorResult     `json:"prior_results,omitempty"`
	AttachedFiles []TaskAttachedFileRef `json:"attached_files,omitempty"`
	// Inputs are the upstream workflow outputs this step declared as its
	// inputs. Each entry either carries a resolved download_url (Resolved=true)
	// pointing at the predecessor's produced file, or surfaces the declared
	// expectation with Resolved=false (source not ready yet — soft hint, never
	// blocks dispatch).
	Inputs []TodoInputRef `json:"inputs,omitempty"`
	// Outputs are the workflow output slots this step declares as its
	// deliverables. The executing agent MUST copy `name` verbatim into
	// `--metadata outputName=...` when uploading the final deliverable,
	// otherwise the file is filed as a process artifact and never reaches
	// downstream steps. Empty when the step declares no outputs.
	Outputs []TodoOutputRef `json:"outputs,omitempty"`
}

// TodoOutputRef carries a workflow-declared output slot. `Name` is an
// identifier (often a human-facing naming template such as
// "剧名_剧本类型_版本_时间"), NOT a file name — the agent must copy it
// verbatim rather than substituting real values into the placeholders.
type TodoOutputRef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
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

// TodoArtifactRef references a product file produced by a completed todo,
// so the executing agent can fetch the predecessor's output by download_url.
type TodoArtifactRef struct {
	TodoID      string `json:"todo_id,omitempty"`
	TransferID  string `json:"transfer_id,omitempty"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	MimeType    string `json:"mime_type,omitempty"`
	DownloadUrl string `json:"download_url,omitempty"`
}

// TodoInputRef is one declared workflow input that links to an upstream
// step's produced output file. When Resolved is true the agent can fetch the
// predecessor's actual file via DownloadUrl; when false the declared
// expectation is still surfaced so the agent knows what to expect (source
// file not ready — non-blocking by design).
type TodoInputRef struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	SourceStep   string `json:"source_step,omitempty"`    // resolved upstream step name/title
	SourceTodoID string `json:"source_todo_id,omitempty"` // todo that produced the file
	OutputName   string `json:"output_name,omitempty"`    // matched StepOutput.Name
	FileName     string `json:"file_name,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	ArtifactID   string `json:"artifact_id,omitempty"` // transfer id of the artifact
	FileRef      string `json:"file_ref,omitempty"`    // ProjectFile id
	DownloadUrl  string `json:"download_url,omitempty"`
	Resolved     bool   `json:"resolved"`
}

// TodoPriorResult carries the result of a previously completed todo.
// For first-time agents: all prior results are included.
// For returning agents: only cross-agent results (work done by other agents
// that this agent hasn't seen in the current session).
type TodoPriorResult struct {
	TodoID    string            `json:"todo_id"`
	Title     string            `json:"title"`
	Summary   string            `json:"summary"`
	Output    string            `json:"output,omitempty"`
	Artifacts []TodoArtifactRef `json:"artifacts,omitempty"`
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
	SchemaVersion  string            `json:"schema_version"`
	TaskID         string            `json:"task_id"`
	ProjectID      string            `json:"project_id"`
	Content        string            `json:"content"`
	UserContent    string            `json:"user_content"`
	IsInitial      bool              `json:"is_initial_message"`
	MustUseSkill   string            `json:"must_use_skill,omitempty"`
	UserUIResponse *model.UIResponse `json:"user_ui_response,omitempty"`
	Project        *PMTaskProject    `json:"project,omitempty"`
	// Workflow carries the task's workflow snapshot (if any). When present the
	// PM must produce a plan that covers every step in order with matching
	// assignee roles; backend enforces it on task.plan_ready.
	Workflow        *model.Workflow       `json:"workflow,omitempty"`
	CandidateAgents []PMTaskAgent         `json:"candidate_agents,omitempty"`
	AttachedFiles   []TaskAttachedFileRef `json:"attached_files,omitempty"`
}

// ——— PM Agent dynamic todo management ———

// TodoAddPayload is published by PM Agent to dynamically append a new TODO.
type TodoAddPayload struct {
	TaskID         string `json:"task_id"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	AssigneeNodeID string `json:"assignee_node_id"`
	BeforeTodoID   string `json:"before_todo_id,omitempty"` // empty = append to end
}

// TodoModifyPayload is published by PM Agent to dynamically update a TODO.
type TodoModifyPM struct {
	TaskID         string `json:"task_id"`
	TodoID         string `json:"todo_id"`
	Title          string `json:"title,omitempty"`
	Description    string `json:"description,omitempty"`
	AssigneeNodeID string `json:"assignee_node_id,omitempty"`
}

// ——— Meeting Room protocol types (v2 — dedicated meeting protocol) ———

// MeetingInstructionPayload is sent from platform to the host (PM Agent)
// to initiate or update a meeting. Replaces PMTaskMessage + chat.message.
type MeetingInstructionPayload struct {
	SchemaVersion string                  `json:"schema_version"`
	MeetingID     string                  `json:"meeting_id"`
	ProjectID     string                  `json:"project_id"`
	Content       string                  `json:"content"`      // 完整会议指令（系统提示 + 用户消息）
	UserContent   string                  `json:"user_content"` // 用户原始输入
	MeetingTitle  string                  `json:"meeting_title"`
	MeetingAgenda string                  `json:"meeting_agenda"`
	AgendaItems   []MeetingAgendaItemRef  `json:"agenda_items,omitempty"`
	Participants  []MeetingParticipantRef `json:"participants,omitempty"`
	AttachedFiles []TaskAttachedFileRef   `json:"attached_files,omitempty"`
}

// MeetingAgendaItemRef describes a single agenda item for meeting creation.
type MeetingAgendaItemRef struct {
	ID          string               `json:"id"`
	Order       int                  `json:"order"`
	Description string               `json:"description"`
	Assignees   []MeetingAssigneeRef `json:"assignees,omitempty"`
}

// MeetingAssigneeRef identifies a single assignee on an agenda item.
type MeetingAssigneeRef struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	NodeID  string `json:"node_id"`
	Weight  int    `json:"weight"`
}

// MeetingParticipantRef describes a meeting participant (executor agent).
type MeetingParticipantRef struct {
	AgentID      string   `json:"agent_id"`
	Name         string   `json:"name"`
	NodeID       string   `json:"node_id"`
	Role         string   `json:"role"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// MeetingEndPayload is broadcast to all participants when a meeting ends.
// Replaces PMTaskMessage + chat.message with [会议已结束] content.
type MeetingEndPayload struct {
	SchemaVersion string `json:"schema_version"`
	MeetingID     string `json:"meeting_id"`
	ProjectID     string `json:"project_id"`
	Title         string `json:"title"`
	Message       string `json:"message"`
}

// MeetingChatPayload is the unified inbound type for ALL meeting conversation
// messages. Replaces:
//   - chat.message (with metadata.context=meeting)
//   - task.reply (with task_id carrying meeting_id)
//   - task.response (with task_id matching a meeting)
//   - task.comment (with task_id carrying meeting_id)
//
// Role field distinguishes the sender:
//   - "host"        — PM Agent coordinating the meeting
//   - "participant" — executor Agent responding to @mention
//
// Phase / Target / ContextBrief implement the structured meeting protocol
// described in the multi-agent meeting design: the host distills prior
// discussion into ContextBrief (<=200 chars) and routes to a single Target
// (agent_id | node_id | "all" | "host"), eliminating fragile @-parsing on the
// participant side and the "context explosion" problem.
type MeetingChatPayload struct {
	MeetingID    string          `json:"meeting_id"`
	Role         string          `json:"role"`                    // "host" | "participant"
	Phase        string          `json:"phase,omitempty"`         // init|speak|review|clarify|summary|confirm|end|prep|cross-review|follow-up
	Target       string          `json:"target,omitempty"`        // agent_id | node_id | "all" | "host" | ""(fallback to @mention)
	ContextBrief string          `json:"context_brief,omitempty"` // <=200 char distilled prior-discussion summary (host->participant)
	Content      string          `json:"content"`
	UIBlocks     []model.UIBlock `json:"ui_blocks,omitempty"`
}

// MeetingControlPayloadV2 is the enhanced meeting control type.
// Supports: start, conclude, minutes, ping, status.
type MeetingControlPayloadV2 struct {
	MeetingID   string              `json:"meeting_id"`
	Action      string              `json:"action"` // start | conclude | minutes | ping | status
	Content     string              `json:"content,omitempty"`
	MinutesData *MeetingMinutesData `json:"minutes_data,omitempty"`
}

// MeetingMinutesData carries structured meeting minutes for the "minutes" action.
type MeetingMinutesData struct {
	Title        string   `json:"title"`
	Participants []string `json:"participants"`
	AgendaItems  []string `json:"agenda_items"`
	Discussion   []string `json:"discussion"`
	Decisions    []string `json:"decisions"`
	ActionItems  []string `json:"action_items"`
	FullMarkdown string   `json:"full_markdown"` // complete minutes markdown
}

// ——— Legacy Meeting Room protocol types (deprecated, kept for compatibility) ———

// MeetingMessagePayload is sent by user or agents during a meeting.
// Deprecated: use MeetingChatPayload + meeting.chat type instead.
type MeetingMessagePayload struct {
	MeetingID string `json:"meeting_id"`
	Content   string `json:"content"`
	SenderID  string `json:"sender_id"`
}

// MeetingControlPayload is sent by the PM agent (host) to control the meeting.
// Deprecated: use MeetingControlPayloadV2 + meeting.control type instead.
type MeetingControlPayload struct {
	MeetingID string `json:"meeting_id"`
	Action    string `json:"action"` // start | summarize | conclude
	Content   string `json:"content,omitempty"`
}
