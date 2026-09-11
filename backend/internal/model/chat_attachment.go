package model

// ChatAttachment describes a user-uploaded file attached to a chat message
// (数字员工对话 / 任务对话). The URL field is a signed, short-lived download
// link that is populated at read/delivery time and never persisted (bson:"-").
type ChatAttachment struct {
	ID       string `json:"id" bson:"id"`
	FileName string `json:"file_name" bson:"file_name"`
	FileSize int64  `json:"file_size" bson:"file_size"`
	MimeType string `json:"mime_type" bson:"mime_type"`
	URL      string `json:"url,omitempty" bson:"-"`
}
