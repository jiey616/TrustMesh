package model

import "time"

// PlatformManual 平台操作手册（单篇结构化文档，覆盖更新）。
//
// 与 platform_guides 的区别：
//   - platform_guides 存**整篇 HTML**，前端用 sandbox iframe 渲染（旧机制，保留兼容）；
//   - platform_manual 存**结构化章节树**，前端用平台自身的 antd 组件渲染，
//     视觉与平台完全一致，且内容可由管理员在界面内**表单化编辑**（无需懂 HTML）。
//
// 存储同 desktop_releases / platform_guides：**Mongo 权威，不进全内存状态机** ——
// 平台级低频管理数据，无需 FlushPersistAll 镜像，重启后从 Mongo 直接读。
// 固定 _id="global"（全局单篇，保存即覆盖）。
//
// 🔴 图片不内联 data URI：章节里的 image block 只存**URL**（走 FILES_STORAGE_PATH
// 文件存储），避免单文档逼近 BSON 16MiB 硬顶、也避免每次读取全量传输。
type PlatformManual struct {
	ID string `json:"id" bson:"_id"` // 固定 "global"
	// Title/Subtitle 是手册封面标题（前端 hero 区）。
	Title    string `json:"title" bson:"title"`
	Subtitle string `json:"subtitle" bson:"subtitle"`
	// Footer 是页脚说明（版本/日期等自由文本）。
	Footer string `json:"footer" bson:"footer"`
	// Sections 是章节目录（顺序即渲染顺序）。
	Sections []ManualSection `json:"sections" bson:"sections"`
	// UpdatedBy 是最近一次编辑者的 user_id（审计口径）。
	UpdatedBy string    `json:"updated_by" bson:"updated_by"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// ManualSection 是一章：标题 + 该章下的内容块序列。
type ManualSection struct {
	// ID 是稳定标识（锚点跳转 + 编辑定位用）。由前端生成短随机串，章节重排不变。
	ID string `json:"id" bson:"id"`
	// Title 是章节标题（如「登录与工作台」）。
	Title string `json:"title" bson:"title"`
	// Lead 是章节导语（可选，一句话概述）。
	Lead string `json:"lead" bson:"lead"`
	// Blocks 是内容块序列（顺序即渲染顺序）。
	Blocks []ManualBlock `json:"blocks" bson:"blocks"`
}

// ManualBlock 是一个内容块。Type 决定 Data 的形态与前端渲染组件。
//
// 支持的 Type（前端一一对应渲染，未知 Type 一律跳过、不报错 —— 向前兼容）：
//   - "paragraph" : Data.Text                → 段落（支持 **加粗** 内联标记）
//   - "heading"   : Data.Text                → 小节标题（h3）
//   - "steps"     : Data.Items[]             → 有序步骤列表
//   - "tip"       : Data.Text                → 蓝色提示条
//   - "warn"      : Data.Text                → 橙色警示条
//   - "image"     : Data.URL/Alt/Caption     → 配图（URL 来自文件存储）
//   - "table"     : Data.Head[]/Rows[][]     → 表格
//   - "flow"      : Data.Items[]             → 流程链（箭头连接）
//   - "qa"        : Data.Items[]             → 问答对（Q/A）
type ManualBlock struct {
	Type string          `json:"type" bson:"type"`
	Data ManualBlockData `json:"data" bson:"data"`
}

// ManualBlockData 是内容块的统一数据体。
//
// 用「扁平可选字段」而非 interface/多态：BSON 解码到 interface 会把数字变成
// float64、丢失类型信息，扁平结构序列化稳定、前后端都好校验。各 Type 只用
// 其中若干字段，未用字段留空即可（omitempty 保证存储紧凑）。
type ManualBlockData struct {
	// Text 用于 paragraph / heading / tip / warn。
	Text string `json:"text,omitempty" bson:"text,omitempty"`
	// Items 用于 steps / flow（每项一句）/ qa（见 QAPairs）。
	Items []string `json:"items,omitempty" bson:"items,omitempty"`
	// QAPairs 用于 qa：问题 + 答案成对，避免用 Items 奇偶位隐式编码。
	QAPairs []ManualQA `json:"qa_pairs,omitempty" bson:"qa_pairs,omitempty"`
	// URL/Alt/Caption 用于 image。URL 必须是可访问地址（文件存储返回）。
	URL     string `json:"url,omitempty" bson:"url,omitempty"`
	Alt     string `json:"alt,omitempty" bson:"alt,omitempty"`
	Caption string `json:"caption,omitempty" bson:"caption,omitempty"`
	// Head/Rows 用于 table：Head 是表头，Rows 是数据行（每行长度应与 Head 对齐）。
	Head []string   `json:"head,omitempty" bson:"head,omitempty"`
	Rows [][]string `json:"rows,omitempty" bson:"rows,omitempty"`
}

// ManualQA 是一条问答。
type ManualQA struct {
	Q string `json:"q" bson:"q"`
	A string `json:"a" bson:"a"`
}

// PlatformManualMeta 是无正文的管理投影（状态展示用）。
type PlatformManualMeta struct {
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle"`
	Sections  int       `json:"sections"`
	UpdatedBy string    `json:"updated_by"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PlatformManualGlobalID 是全局单篇文档的固定主键。
const PlatformManualGlobalID = "global"

// Meta 返回无正文投影。
func (m *PlatformManual) Meta() PlatformManualMeta {
	return PlatformManualMeta{
		Title:     m.Title,
		Subtitle:  m.Subtitle,
		Sections:  len(m.Sections),
		UpdatedBy: m.UpdatedBy,
		UpdatedAt: m.UpdatedAt,
	}
}

// ManualImageAsset 是手册配图（走文件存储，DB 只留元数据 + URL）。
//
// 与 platform_manual 分开集合存储：图片是**追加写、只增不改**的资源，
// 混进单篇文档会让每次保存都重写全部图片元数据。
type ManualImageAsset struct {
	// ID 是资源标识（生成的随机串，也是文件名前缀）。
	ID string `json:"id" bson:"_id"`
	// FileName 是原始文件名（展示/审计用）。
	FileName string `json:"file_name" bson:"file_name"`
	// Size 是字节数。
	Size int64 `json:"size" bson:"size"`
	// ContentType 是 MIME（image/png 等，前端 <img> 不依赖它，但排障有用）。
	ContentType string `json:"content_type" bson:"content_type"`
	// Path 是磁盘落点（存储内部用，不下发给前端）。
	Path string `json:"-" bson:"path"`
	// URL 是可访问地址（/api/v1/manual/images/{id}），前端 img src 用它。
	URL string `json:"url" bson:"url"`
	// UploadedBy 是上传者 user_id。
	UploadedBy string    `json:"uploaded_by" bson:"uploaded_by"`
	UploadedAt time.Time `json:"uploaded_at" bson:"uploaded_at"`
}
