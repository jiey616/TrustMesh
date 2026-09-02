package model

import "time"

type Event struct {
	ID        string         `json:"id" bson:"_id"`
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
	ID        string     `json:"id" bson:"_id"`
	UserID    string     `json:"-" bson:"user_id"`
	EventID   string     `json:"event_id" bson:"event_id"`
	ProjectID string     `json:"project_id" bson:"project_id"`
	TaskID    string     `json:"task_id,omitempty" bson:"task_id,omitempty"`
	ActorType string     `json:"actor_type" bson:"actor_type"`
	ActorID   string     `json:"actor_id,omitempty" bson:"actor_id,omitempty"`
	ActorName string     `json:"actor_name" bson:"actor_name"`
	Title     string     `json:"title" bson:"title"`
	Body      string     `json:"body" bson:"body"`
	Category  string     `json:"category" bson:"category"`
	Priority  string     `json:"priority" bson:"priority"`
	IsRead    bool       `json:"is_read" bson:"is_read"`
	ReadAt    *time.Time `json:"read_at" bson:"read_at"`
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
}

type UserStreamEvent struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	Payload    map[string]any `json:"payload"`
}
