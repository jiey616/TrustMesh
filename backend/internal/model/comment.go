package model

import "time"

type CommentMention struct {
	AgentID   string `json:"agent_id" bson:"agent_id"`
	AgentName string `json:"agent_name" bson:"agent_name"`
	NodeID    string `json:"node_id" bson:"node_id"`
	Role      string `json:"role" bson:"role"`
}

type CommentMentionDelivery struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type Comment struct {
	ID        string           `json:"id" bson:"_id"`
	UserID    string           `json:"-" bson:"user_id"`
	OrgID     string           `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	TaskID    string           `json:"task_id" bson:"task_id"`
	TodoID    string           `json:"todo_id,omitempty" bson:"todo_id,omitempty"`
	ActorType string           `json:"actor_type" bson:"actor_type"`
	ActorID   string           `json:"actor_id" bson:"actor_id"`
	ActorName string           `json:"actor_name" bson:"actor_name"`
	Content   string           `json:"content" bson:"content"`
	Mentions  []CommentMention `json:"mentions,omitempty" bson:"mentions,omitempty"`
	CreatedAt time.Time        `json:"created_at" bson:"created_at"`
}
