package model

import "time"

// 桌面端发行版（Electron 安装包）—— 自建更新源的载体，集合 desktop_releases。
// 方案与实测依据：docs/desktop-app-update-plan-2026-09-18.md。
//
// 与其余业务资源不同，本集合**不进全内存状态机**（同 audit_logs 的先例）：
// 它是平台级低频管理数据，读者只有两类（平台管理员的列表页、每个桌面客户端启动时
// 拉一次 latest.yml），且状态迁移（发布 / 回滚）要求「同一时刻只有一个 published」。
// 直接以 Mongo 为权威可省掉一张需要 FlushPersistAll 镜像的 map，避免陈旧内存快照
// 覆盖库中状态（T3.1 双实例事故的根因类型）。
const (
	DesktopReleaseStatusDraft     = "draft"
	DesktopReleaseStatusPublished = "published"
	DesktopReleaseStatusArchived  = "archived"

	// DesktopReleaseChannelStable 是当前唯一启用的通道。
	// channel 字段为演进预留（方案 §2「明确不做」：本期不做 stable/beta 分流）。
	DesktopReleaseChannelStable = "stable"

	// DesktopReleaseRetain 是保留的版本数上限：每次发布后，超出部分从**最旧的
	// archived** 开始删除（published 永不参与清理，见 store.PruneDesktopReleases）。
	DesktopReleaseRetain = 5
)

// DesktopRelease 一条发行版记录。
//
// 版本号纪律：Version 必须与打包时 package.json 的 version 逐字相同，否则
// electron-updater 会静默判定「无更新」（方案 §3.7）。
type DesktopRelease struct {
	ID string `json:"id" bson:"_id"`
	// Version 是 semver 字符串（如 "0.3.0"）。
	Version string `json:"version" bson:"version"`
	// Channel 预留：当前恒 DesktopReleaseChannelStable。
	Channel string `json:"channel" bson:"channel"`
	// FileName 是安装包对外暴露的文件名（= electron-builder 的 artifactName，
	// 如 "TrustMesh-Setup-0.3.0.exe"）。公开 feed 的 :filename 段即取此值。
	FileName string `json:"file_name" bson:"file_name"`
	// FilePath 是安装包在**本地磁盘的绝对路径**（LocalFileStorage 返回的 uri）。
	// 标记 json:"-" —— 磁盘布局不得外泄，公开 feed 只用 FileName。
	FilePath string `json:"-" bson:"file_path"`
	// Size 是安装包字节数（latest.yml 的 files[].size 用它）。
	Size int64 `json:"size" bson:"size"`
	// Sha512 是**base64 编码的 sha512 摘要**（electron-builder 产出的 latest.yml
	// 即此格式；下载端用它对安装包做完整性校验）。
	Sha512 string `json:"sha512" bson:"sha512"`
	// BlockMapFileName / BlockMapPath / BlockMapSize 是差分下载所需的 .blockmap。
	// 缺省时 electron-updater 退化为每次全量下载（方案 §8），不报错。
	BlockMapFileName string `json:"block_map_file_name,omitempty" bson:"block_map_file_name,omitempty"`
	BlockMapPath     string `json:"-" bson:"block_map_path,omitempty"`
	// BlockMapSize 会写进 latest.yml 的 files[].blockMapSize（下载端据此判断能否走差分）。
	BlockMapSize int64 `json:"block_map_size,omitempty" bson:"block_map_size,omitempty"`

	// Notes 是发布说明（release.json 的 notes，可选）。
	Notes string `json:"notes,omitempty" bson:"notes,omitempty"`
	// Status 取 DesktopReleaseStatus* 之一；published 全局至多一条（部分唯一索引兜底）。
	Status string `json:"status" bson:"status"`
	// PublishedAt 是最近一次被置为 published 的时间（回滚也会刷新它）。
	PublishedAt time.Time `json:"published_at,omitempty" bson:"published_at,omitempty"`
	// UploadedBy 是上传者的用户 ID。
	UploadedBy string    `json:"uploaded_by,omitempty" bson:"uploaded_by,omitempty"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
}

// IsDesktopReleaseStatus 判断状态取值是否合法。
func IsDesktopReleaseStatus(v string) bool {
	switch v {
	case DesktopReleaseStatusDraft, DesktopReleaseStatusPublished, DesktopReleaseStatusArchived:
		return true
	}
	return false
}

// DesktopReleaseInput 是上传（新建）一条发行版的入参。
// 元数据来自随包上传的 release.json（由 _deploy_desktop.py 产出，不手填）。
type DesktopReleaseInput struct {
	Version string
	Channel string
	// FileName 是落库的对外文件名；空则由调用方按 artifactName 规则兜底。
	FileName string
	Size     int64
	Sha512   string
	Notes    string
	// UploadedBy 由 handler 从请求上下文注入。
	UploadedBy string
}
