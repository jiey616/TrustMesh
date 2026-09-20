package model

import (
	"time"
)

// PlatformGuide 全局「使用引导」文档（单篇，覆盖更新）。
//
// 存储与 desktop_releases 同策略：**Mongo 权威，不进全内存状态机** ——
// 平台级低频管理数据，无需 FlushPersistAll 镜像，重启后从 Mongo 直接读。
// 固定 _id="global"（全局单篇，重复上传即覆盖）。
//
// 渲染端（前端）必须用 sandbox iframe 呈现 HTML（默认禁脚本）：
// 后端不做 HTML 消毒 —— iframe sandbox 是比任何服务端消毒都更强的边界，
// 两层都做会出现「消毒被绕过但仍有兜底」与「消毒误杀合法内容」的双向风险，
// 这里只守上传格式，把执行边界交给浏览器沙箱。
type PlatformGuide struct {
	ID       string `json:"id" bson:"_id"` // 固定 "global"
	FileName string `json:"file_name" bson:"file_name"`
	// Size 是 HTML 字节数（UTF-8），用于管理端展示与上传前上限校验对账。
	Size int `json:"size" bson:"size"`
	// HTML 是文档原文。整个文档（含 html）通过 authed /guide 下发给登录用户。
	HTML string `json:"html" bson:"html"`
	// UpdatedBy 是最近一次上传者的 user_id（审计口径用 user_id 而非邮箱）。
	UpdatedBy string    `json:"updated_by" bson:"updated_by"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// PlatformGuideMeta 是无正文的管理投影（列表/状态展示用）。
type PlatformGuideMeta struct {
	FileName  string    `json:"file_name"`
	Size      int       `json:"size"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlatformGuideGlobalID 是全局单篇文档的固定主键。
const PlatformGuideGlobalID = "global"

// Meta 返回无正文投影。
func (g *PlatformGuide) Meta() PlatformGuideMeta {
	return PlatformGuideMeta{
		FileName:  g.FileName,
		Size:      g.Size,
		UpdatedBy: g.UpdatedBy,
		UpdatedAt: g.UpdatedAt,
	}
}
