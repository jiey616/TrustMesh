package model

import "time"

type AgentChatMessage struct {
	ID              string           `json:"id" bson:"id"`
	SenderType      string           `json:"sender_type" bson:"sender_type"`
	Direction       string           `json:"direction" bson:"direction"`
	Content         string           `json:"content" bson:"content"`
	Status          string           `json:"status" bson:"status"`
	RemoteMessageID string           `json:"remote_message_id,omitempty" bson:"remote_message_id,omitempty"`
	Attachments     []ChatAttachment `json:"attachments,omitempty" bson:"attachments,omitempty"`
	CreatedAt       time.Time        `json:"created_at" bson:"created_at"`
}

type AgentChatDetail struct {
	ID          string             `json:"id" bson:"id"`
	AgentID     string             `json:"agent_id" bson:"agent_id"`
	AgentNodeID string             `json:"agent_node_id" bson:"agent_node_id"`
	SessionKey  string             `json:"session_key" bson:"session_key"`
	Status      string             `json:"status" bson:"status"`
	Messages    []AgentChatMessage `json:"messages" bson:"messages"`
	CreatedAt   time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at" bson:"updated_at"`
}

type AgentChatSessionSummary struct {
	ID                 string     `json:"id" bson:"id"`
	AgentID            string     `json:"agent_id" bson:"agent_id"`
	SessionKey         string     `json:"session_key" bson:"session_key"`
	Status             string     `json:"status" bson:"status"`
	MessageCount       int        `json:"message_count" bson:"message_count"`
	LastMessagePreview string     `json:"last_message_preview" bson:"last_message_preview"`
	LastMessageAt      *time.Time `json:"last_message_at,omitempty" bson:"last_message_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" bson:"updated_at"`
}

type AgentChat struct {
	ID          string             `bson:"_id"`
	OrgID       string             `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属
	UserID      string             `bson:"user_id"`
	AgentID     string             `bson:"agent_id"`
	AgentNodeID string             `bson:"agent_node_id"`
	SessionKey  string             `bson:"session_key"`
	Status      string             `bson:"status"`
	Messages    []AgentChatMessage `bson:"messages"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}
