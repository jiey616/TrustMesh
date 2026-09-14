package model

import "time"

type MeetingStatus string

const (
	MeetingWaiting    MeetingStatus = "waiting"
	MeetingInProgress MeetingStatus = "in_progress"
	MeetingCompleted  MeetingStatus = "completed"
)

type AgendaAssignee struct {
	AgentID string `json:"agent_id" bson:"agent_id"`
	Weight  int    `json:"weight" bson:"weight"` // 发言权重，越高越优先
}

type MeetingAgendaItem struct {
	ID          string           `json:"id" bson:"id"`
	Order       int              `json:"order" bson:"order"`
	Description string           `json:"description" bson:"description"`
	Assignees   []AgendaAssignee `json:"assignees" bson:"assignees"`
}

type MeetingTodoItem struct {
	ID                   string `json:"id" bson:"id"`
	Description          string `json:"description" bson:"description"`
	ResponsibleAgentID   string `json:"responsible_agent_id" bson:"responsible_agent_id"`
	ResponsibleAgentName string `json:"responsible_agent_name" bson:"responsible_agent_name"`
	Status               string `json:"status" bson:"status"` // pending | converted
	ConvertedTaskID      string `json:"converted_task_id,omitempty" bson:"converted_task_id,omitempty"`
}

type MeetingAttachedFile struct {
	ID       string `json:"id" bson:"id"`
	FileName string `json:"file_name" bson:"file_name"`
	FileSize int64  `json:"file_size,omitempty" bson:"file_size,omitempty"`
	MimeType string `json:"mime_type,omitempty" bson:"mime_type,omitempty"`
	Source   string `json:"source,omitempty" bson:"source,omitempty"`
}

type MeetingParticipant struct {
	AgentID   string `json:"agent_id" bson:"agent_id"`
	AgentName string `json:"agent_name" bson:"agent_name"`
	NodeID    string `json:"node_id" bson:"node_id"`
	Status    string `json:"status" bson:"status"` // invited | joined | left
}

type Meeting struct {
	ID            string                `json:"id" bson:"_id"`
	ProjectID     string                `json:"project_id" bson:"project_id"`
	OrgID         string                `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	Title         string                `json:"title" bson:"title"`
	Agenda        string                `json:"agenda" bson:"agenda"`
	CreatorID     string                `json:"creator_id" bson:"creator_id"`
	HostAgentID   string                `json:"host_agent_id" bson:"host_agent_id"`
	Participants  []MeetingParticipant  `json:"participants" bson:"participants"`
	AgendaItems   []MeetingAgendaItem   `json:"agenda_items" bson:"agenda_items"`
	Todos         []MeetingTodoItem     `json:"todos" bson:"todos"`
	FileIDs       []string              `json:"file_ids,omitempty" bson:"file_ids,omitempty"`
	AttachedFiles []MeetingAttachedFile `json:"attached_files,omitempty" bson:"attached_files,omitempty"`
	SummaryFileID string                `json:"summary_file_id,omitempty" bson:"summary_file_id,omitempty"`
	Minutes       string                `json:"minutes,omitempty" bson:"minutes,omitempty"`
	MinutesFileID string                `json:"minutes_file_id,omitempty" bson:"minutes_file_id,omitempty"`
	Status        MeetingStatus         `json:"status" bson:"status"`
	CreatedAt     time.Time             `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at" bson:"updated_at"`

	// T2.1 乐观锁版本号：每次写路径 +1，UpdateOne 的 filter 带 {_id, version}，
	// 防止并发写互相覆盖（丢失更新）。
	//
	// bson 刻意**不带** omitempty：否则 0 / 缺省值不落库，filter 里的
	// version 会永远匹配不上，导致该会议任何更新都判定为冲突。
	// 存量文档（T2.1 之前写入）没有这个字段，由启动时
	// backfillMeetingVersions 统一补成 1。
	Version int `json:"version" bson:"version"`

	// T1.2 会话态镜像：纯派生自会议消息（PM 声明的 phase / 点名 target / agent 发言轮数），
	// 让后端重启后仍能回答「会议进行到哪一步、上一次点到谁、纪要是否已上传」。
	// 不改动任何编排语义（编排仍由主持人 agent 负责）；omitempty 保证对存量文档向后兼容，
	// 存量会议的取值由 loadMongoState 的 backfillMeetingSessionUnsafe 按历史消息回填。
	LastPhase      string    `json:"last_phase,omitempty" bson:"last_phase,omitempty"`
	LastTarget     string    `json:"last_target,omitempty" bson:"last_target,omitempty"`
	SpeakerTurns   int       `json:"speaker_turns,omitempty" bson:"speaker_turns,omitempty"`
	LastActivityAt time.Time `json:"last_activity_at,omitempty" bson:"last_activity_at,omitempty"`
}

type MeetingMessage struct {
	ID           string    `json:"id" bson:"_id"`
	OrgID        string    `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属
	MeetingID    string    `json:"meeting_id" bson:"meeting_id"`
	SenderType   string    `json:"sender_type" bson:"sender_type"` // user | agent | system
	SenderID     string    `json:"sender_id" bson:"sender_id"`
	SenderName   string    `json:"sender_name" bson:"sender_name"`
	Phase        string    `json:"phase,omitempty" bson:"phase,omitempty"`                 // meeting phase tag
	Target       string    `json:"target,omitempty" bson:"target,omitempty"`               // intended recipient (agent_id/node_id/all/host)
	ContextBrief string    `json:"context_brief,omitempty" bson:"context_brief,omitempty"` // distilled context summary from host
	Content      string    `json:"content" bson:"content"`
	UIBlocks     []UIBlock `json:"ui_blocks,omitempty" bson:"ui_blocks,omitempty"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
}
