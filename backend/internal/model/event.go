package model

import "time"

type Event struct {
	ID        string         `json:"id" bson:"_id"`
	OrgID     string         `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属
	UserID    string         `json:"-" bson:"user_id"`
	ProjectID string         `json:"project_id" bson:"project_id"`
	TaskID    string         `json:"task_id,omitempty" bson:"task_id,omitempty"`
	TodoID    string         `json:"todo_id,omitempty" bson:"todo_id,omitempty"`
	ActorType string         `json:"actor_type" bson:"actor_type"`
	ActorID   string         `json:"actor_id" bson:"actor_id"`
	ActorName string         `json:"actor_name" bson:"actor_name"`
	EventType string         `json:"event_type" bson:"event_type"`
	Content   *string        `json:"content" bson:"content"`
	Metadata  map[string]any `json:"metadata" bson:"metadata"`
	CreatedAt time.Time      `json:"created_at" bson:"created_at"`
}

type Notification struct {
	ID        string `json:"id" bson:"_id"`
	OrgID     string `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属
	UserID    string `json:"-" bson:"user_id"`
	EventID   string `json:"event_id" bson:"event_id"`
	ProjectID string `json:"project_id" bson:"project_id"`
	TaskID    string `json:"task_id,omitempty" bson:"task_id,omitempty"`
	ActorType string `json:"actor_type" bson:"actor_type"`
	ActorID   string `json:"actor_id,omitempty" bson:"actor_id,omitempty"`
	ActorName string `json:"actor_name" bson:"actor_name"`
	Title     string `json:"title" bson:"title"`
	Body      string `json:"body" bson:"body"`
	Category  string `json:"category" bson:"category"`
	Priority  string `json:"priority" bson:"priority"`
	// SourceEvent：**机器可读**的来源键，客户端据此判类（例如桌面端只弹「待人工确认」与「任务完成」）。
	// 取值 = 产生该通知的事件类型；`task_status_changed` 再追加终态，
	// 形如 `task_status_changed.done` / `.failed` / `.canceled` —— 否则客户端分不出「完成」与「失败/取消」。
	//
	// 🔴 客户端**不要**拿 Title 文本判类：标题是写给人看的，后端哪天改文案，
	// 客户端筛选就会**静默**失配（不报错、只是再也不弹），且跨仓耦合没有任何编译期/测试期能拦。
	SourceEvent string     `json:"source_event,omitempty" bson:"source_event,omitempty"`
	IsRead      bool       `json:"is_read" bson:"is_read"`
	ReadAt      *time.Time `json:"read_at" bson:"read_at"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`
}

type UserStreamEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Payload    map[string]any `json:"payload"`
}
