package model

import "time"

// ─── UI Block 类型（交互式问题澄清） ───

type UIBlockOption struct {
	Value       string `json:"value" bson:"value"`
	Label       string `json:"label" bson:"label"`
	Description string `json:"description,omitempty" bson:"description,omitempty"`
}

type UIBlock struct {
	ID           string          `json:"id" bson:"id"`
	Type         string          `json:"type" bson:"type"`
	Label        string          `json:"label" bson:"label"`
	Options      []UIBlockOption `json:"options,omitempty" bson:"options,omitempty"`
	Multiple     bool            `json:"multiple,omitempty" bson:"multiple,omitempty"`
	Placeholder  string          `json:"placeholder,omitempty" bson:"placeholder,omitempty"`
	Required     *bool           `json:"required,omitempty" bson:"required,omitempty"`
	Content      string          `json:"content,omitempty" bson:"content,omitempty"`
	Default      []string        `json:"default,omitempty" bson:"default,omitempty"`
	ConfirmLabel string          `json:"confirm_label,omitempty" bson:"confirm_label,omitempty"`
	CancelLabel  string          `json:"cancel_label,omitempty" bson:"cancel_label,omitempty"`
}

type UIBlockResponse struct {
	Selected  []string `json:"selected,omitempty" bson:"selected,omitempty"`
	Text      string   `json:"text,omitempty" bson:"text,omitempty"`
	Confirmed *bool    `json:"confirmed,omitempty" bson:"confirmed,omitempty"`
}

type UIResponse struct {
	Blocks map[string]UIBlockResponse `json:"blocks" bson:"blocks"`
}

type TaskMessage struct {
	ID          string           `json:"id" bson:"id"`
	Role        string           `json:"role" bson:"role"`
	Content     string           `json:"content" bson:"content"`
	UIBlocks    []UIBlock        `json:"ui_blocks,omitempty" bson:"ui_blocks,omitempty"`
	UIResponse  *UIResponse      `json:"ui_response,omitempty" bson:"ui_response,omitempty"`
	Attachments []ChatAttachment `json:"attachments,omitempty" bson:"attachments,omitempty"`
	CreatedAt   time.Time        `json:"created_at" bson:"created_at"`
}
