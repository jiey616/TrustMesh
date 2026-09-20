package model

import "time"

// MobileAppRelease 移动端安装包（Android APK）的**当前版本**记录。
//
// 与 desktop_releases 的差异：桌面端需要 draft/published/rollback 三态与
// latest.yml 发布指针（electron-updater 协议），移动端扫码下载只需要
// 「一个可下载的最新包」，因此收敛为单条 current 记录 + 磁盘文件，
// 上传即覆盖（同 desktop 的 Mongo 权威、不进内存状态机策略）。
//
// FilePath 不序列化到 API（json:"-"），只存在库里供下载端点解析磁盘路径。
type MobileAppRelease struct {
	ID       string `json:"id" bson:"_id"` // 固定 "current"
	Version  string `json:"version" bson:"version"`
	FileName string `json:"file_name" bson:"file_name"`
	FilePath string `json:"-" bson:"file_path"`
	Size     int64  `json:"size" bson:"size"`
	// Sha512 是服务端落盘后重算的摘要，仅用于审计留证与人工核对
	//（浏览器下载链路自身有 TLS 完整性，客户端不消费此字段）。
	Sha512    string    `json:"sha512" bson:"sha512"`
	UpdatedBy string    `json:"updated_by" bson:"updated_by"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// MobileAppReleaseCurrentID 是当前版本记录的固定主键。
const MobileAppReleaseCurrentID = "current"
