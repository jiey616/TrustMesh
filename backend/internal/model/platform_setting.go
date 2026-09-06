package model

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// 平台 / 租户 LLM 配置（B2 决策：每租户可独立覆盖 LLM）。
//
// 两层结构，解析优先级：租户配置 > 平台默认 > env 兜底（config.go）：
//   - OrgID == ""                → 平台默认配置（仅平台管理员可改）
//   - OrgID == "<org_id>"        → 该租户的覆盖配置（org owner/admin 可改）
//
// C1 热生效：每次调用解析，保存即生效，无需重启。
// D1 write-only：APIKey 入库但接口只回掩码，留空 = 保持不变。
// ─────────────────────────────────────────────────────────────────────────────

const (
	LLMSourceOrg      = "org"      // 生效配置来自租户覆盖
	LLMSourcePlatform = "platform" // 生效配置来自平台默认
	LLMSourceEnv      = "env"      // 生效配置来自 env 兜底（未做任何 UI 配置）
)

// PlatformLLMSetting 是一层 LLM 配置的存储实体。
// Mongo 集合 platform_settings，org_id 唯一索引（"" 为平台默认单例）。
type PlatformLLMSetting struct {
	OrgID     string    `json:"org_id" bson:"org_id"` // "" = 平台默认
	APIURL    string    `json:"api_url" bson:"api_url"`
	APIKey    string    `json:"-" bson:"api_key"` // write-only，绝不出现在 JSON
	Model     string    `json:"model" bson:"model"`
	OpsModel  string    `json:"ops_model,omitempty" bson:"ops_model,omitempty"` // 运维归因模型，空则回落 Model
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
	UpdatedBy string    `json:"updated_by" bson:"updated_by"`
}

// LLMConfigView 是给前端的掩码视图：永不携带明文 key。
type LLMConfigView struct {
	OrgID        string    `json:"org_id,omitempty"`
	APIURL       string    `json:"api_url"`
	APIKeyMasked string    `json:"api_key_masked"` // 形如 sk-***1234；空 = 该层未配 key
	Model        string    `json:"model"`
	OpsModel     string    `json:"ops_model,omitempty"`
	Source       string    `json:"source"` // org | platform | env（解析后的实际生效来源）
	HasOverride  bool      `json:"has_override"` // 该层是否存在落库配置（false = 展示的是回退值）
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
	UpdatedBy    string    `json:"updated_by,omitempty"`
}

// LLMConfigInput 是 PUT 请求体。APIKey 为空 = 保持已有不变；
// ResetAPIKey 为 true 时清空该层 key（回到回退链）。
type LLMConfigInput struct {
	APIURL     string `json:"api_url"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	OpsModel   string `json:"ops_model"`
	ResetAPIKey bool  `json:"reset_api_key"`
}

// LLMConfigTestRequest 是连接测试请求：字段全部可选——
// 提供了就用提交值测（测未保存的表单值），没提供就用解析后的生效配置测。
type LLMConfigTestRequest struct {
	OrgID  string `json:"org_id"`
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}
