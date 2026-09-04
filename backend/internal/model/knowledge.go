package model

import "time"

type KnowledgeDocument struct {
	ID          string         `json:"id" bson:"_id"`
	UserID      string         `json:"-" bson:"user_id"`
	OrgID       string         `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	ProjectID   *string        `json:"project_id" bson:"project_id"`
	Title       string         `json:"title" bson:"title"`
	Description string         `json:"description" bson:"description"`
	DocType     string         `json:"doc_type" bson:"doc_type"`
	MimeType    string         `json:"mime_type" bson:"mime_type"`
	SourceURI   string         `json:"-" bson:"source_uri"`
	FileSize    int64          `json:"file_size" bson:"file_size"`
	Status      string         `json:"status" bson:"status"`
	ChunkCount  int            `json:"chunk_count" bson:"chunk_count"`
	Tags        []string       `json:"tags" bson:"tags"`
	Metadata    map[string]any `json:"metadata" bson:"metadata"`
	CreatedAt   time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" bson:"updated_at"`
}

type KnowledgeChunk struct {
	ID         string         `json:"id" bson:"_id"`
	DocumentID string         `json:"document_id" bson:"document_id"`
	UserID     string         `json:"-" bson:"user_id"`
	OrgID      string         `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属（阶段 0 仅加字段）
	ProjectID  *string        `json:"-" bson:"project_id"`
	ChunkIndex int            `json:"chunk_index" bson:"chunk_index"`
	Content    string         `json:"content" bson:"content"`
	TokenCount int            `json:"token_count" bson:"token_count"`
	Metadata   map[string]any `json:"metadata" bson:"metadata"`
	CreatedAt  time.Time      `json:"created_at" bson:"created_at"`
}

const (
	KnowledgeDocStatusProcessing = "processing"
	KnowledgeDocStatusReady      = "ready"
	KnowledgeDocStatusFailed     = "failed"
)
