package model

import "time"

type User struct {
	ID           string `json:"id" bson:"_id"`
	Email        string `json:"email" bson:"email"`
	Name         string `json:"name" bson:"name"`
	PasswordHash string `json:"-" bson:"password_hash"`
	// 平台管理员（A1）：首个注册用户自动获得；可改平台级 LLM 配置。
	IsAdmin   bool      `json:"is_admin" bson:"is_admin,omitempty"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
	// Disabled 由平台管理员禁用（平台用户管理）：登录 / refresh 一律拒绝；
	// 已签发的 access token 在 TTL（默认 15min）内自然过期。
	Disabled   bool       `json:"disabled" bson:"disabled,omitempty"`
	DisabledAt *time.Time `json:"disabled_at,omitempty" bson:"disabled_at,omitempty"`
}
