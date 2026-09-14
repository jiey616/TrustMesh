package model

import "time"

// WorkflowStep 是工作流中的一个有序步骤。步骤顺序即执行顺序。
// 绑定方式：AgentID 精确绑定具体数字员工；Role 兜底匹配（Agent.Role / Agent.Name
// 模糊匹配）。二者可同时存在，校验时 AgentID 优先。
//
// Inputs / Outputs 声明该步骤的输入与产出文件，用于把"上一个流程的输出文件"
// 关联为本步骤的输入（见 StepIOLink）。两者均为可选（非必填）。
type WorkflowStep struct {
	Name       string       `json:"name" bson:"name"`
	Role       string       `json:"role,omitempty" bson:"role,omitempty"`
	AgentID    string       `json:"agent_id,omitempty" bson:"agent_id,omitempty"`
	NeedReview bool         `json:"need_review,omitempty" bson:"need_review,omitempty"`
	Inputs     []StepInput  `json:"inputs,omitempty" bson:"inputs,omitempty"`
	Outputs    []StepOutput `json:"outputs,omitempty" bson:"outputs,omitempty"`
}

// StepOutput 声明一个步骤会产出的文件（逻辑名，与运行时真实文件解耦）。
type StepOutput struct {
	Name        string `json:"name" bson:"name"` // 逻辑名，如 "剧本正文"
	Description string `json:"description,omitempty" bson:"description,omitempty"`
	MimeType    string `json:"mime_type,omitempty" bson:"mime_type,omitempty"` // 提示，如 text/markdown
}

// StepIOLink 把输入关联到上游某个步骤的某个输出。
// Step 为来源步骤名；关键字 "prev" 表示紧邻的上一步。
// Output 必须匹配来源步骤某个 StepOutput.Name。
type StepIOLink struct {
	Step   string `json:"step" bson:"step"`
	Output string `json:"output" bson:"output"`
}

// StepInput 声明一个步骤需要的输入文件，并指明其来源（上一个流程的输出）。
type StepInput struct {
	Name        string     `json:"name" bson:"name"`
	Description string     `json:"description,omitempty" bson:"description,omitempty"`
	MimeType    string     `json:"mime_type,omitempty" bson:"mime_type,omitempty"`
	Source      StepIOLink `json:"source" bson:"source"`
}

// Clone returns a deep copy of the step (inputs/outputs slices are copied).
func (s *WorkflowStep) Clone() *WorkflowStep {
	if s == nil {
		return nil
	}
	out := *s
	out.Inputs = append([]StepInput(nil), s.Inputs...)
	out.Outputs = append([]StepOutput(nil), s.Outputs...)
	return &out
}

// Workflow 是预定义的有序工作流，PM 规划任务时必须按 Steps 顺序产出 todos。
// ID 是全局唯一标识（全局模板与项目内克隆副本都有独立 ID），所有引用以 ID 为准。
// ParentTemplateID / TemplateVersion / TemplateSnapshot 仅在"继承自全局模板"时填充：
// 继承 = 把模板 Steps 克隆进项目并记录快照，之后项目可二次修改；同步 = 按步骤名
// 三路合并（base=TemplateSnapshot, ours=项目当前, theirs=模板最新版）。
type Workflow struct {
	ID string `json:"id,omitempty" bson:"id,omitempty"`
	// ParentTemplateID 指向该工作流继承的全局模板；空表示纯项目私有工作流。
	ParentTemplateID string `json:"parent_template_id,omitempty" bson:"parent_template_id,omitempty"`
	// TemplateVersion 是已同步到的模板版本号（继承时=模板当时版本，同步成功后递增）。
	TemplateVersion int `json:"template_version,omitempty" bson:"template_version,omitempty"`
	// TemplateSnapshot 记录继承/上次同步时模板的 Steps 快照，是三路合并的 base。
	TemplateSnapshot []WorkflowStep `json:"template_snapshot,omitempty" bson:"template_snapshot,omitempty"`

	Name  string         `json:"name" bson:"name"`
	Steps []WorkflowStep `json:"steps" bson:"steps"`
}

// WorkflowTemplate 是用户级全局工作流模板（跨项目复用）。项目通过"继承"克隆
// 一份副本进项目（见 Workflow.ParentTemplateID），模板每次保存 Version 自动递增。
type WorkflowTemplate struct {
	ID          string         `json:"id" bson:"_id"`
	OrgID       string         `json:"org_id,omitempty" bson:"org_id,omitempty"` // 多租户：租户归属
	UserID      string         `json:"-" bson:"user_id"`
	Name        string         `json:"name" bson:"name"`
	Description string         `json:"description,omitempty" bson:"description,omitempty"`
	Steps       []WorkflowStep `json:"steps" bson:"steps"`
	Version     int            `json:"version" bson:"version"`
	// Curated 是「策展标记」：模板作者可把优质模板标为精选（curated=true），
	// 模板库按 curated 优先排序展示。CuratedAt 记录最近一次标为精选的时间。
	Curated   bool      `json:"curated,omitempty" bson:"curated,omitempty"`
	CuratedAt time.Time `json:"curated_at,omitempty" bson:"curated_at,omitempty"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// WorkflowRef records the slice of a project's primary workflow ("项目总流程")
// that a task owns. Steps are referenced by index into Workflow.Steps
// (inclusive range StepFrom..StepTo). It lets the frontend render the project
// pipeline and lets downstream tasks resolve cross-task step inputs against
// the predecessor task's produced outputs.
type WorkflowRef struct {
	WorkflowIndex int    `json:"workflow_index" bson:"workflow_index"`               // index into Project.Workflows（旧引用，兼容存量任务）
	WorkflowID    string `json:"workflow_id,omitempty" bson:"workflow_id,omitempty"` // 新引用，优先于 WorkflowIndex
	WorkflowName  string `json:"workflow_name" bson:"workflow_name"`
	StepFrom      int    `json:"step_from" bson:"step_from"`
	StepTo        int    `json:"step_to" bson:"step_to"`
}

// IsEmpty reports whether the workflow has no steps (treat as unset).
func (w *Workflow) IsEmpty() bool {
	return w == nil || len(w.Steps) == 0
}

// Clone returns a deep copy so task snapshots never alias the project config.
func (w *Workflow) Clone() *Workflow {
	if w == nil {
		return nil
	}
	out := &Workflow{
		ID:               w.ID,
		ParentTemplateID: w.ParentTemplateID,
		TemplateVersion:  w.TemplateVersion,
		Name:             w.Name,
		Steps:            make([]WorkflowStep, len(w.Steps)),
		TemplateSnapshot: make([]WorkflowStep, len(w.TemplateSnapshot)),
	}
	copy(out.Steps, w.Steps)
	copy(out.TemplateSnapshot, w.TemplateSnapshot)
	return out
}

// WorkflowStepProgress is one step of the project's overall pipeline
// (the primary workflow), with execution status derived from the owning task's
// todo and the produced output files.
type WorkflowStepProgress struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Role      string `json:"role,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Status    string `json:"status"` // pending|in_progress|awaiting_review|done|failed|canceled|unassigned
	TaskID    string `json:"task_id,omitempty"`
	TaskTitle string `json:"task_title,omitempty"`
	// DeclaredOutputs 是该步骤在流程里声明的输出位名称。手工绑定交付物时前端据此
	// 限定可选范围：声明了就只能选这些（避免拼出下游步骤取不到的名字），
	// 没声明才允许自由命名。
	DeclaredOutputs []string                `json:"declared_outputs,omitempty"`
	Outputs         []WorkflowStepOutputRef `json:"outputs,omitempty"`
}

// WorkflowStepOutputRef is the final produced file of a pipeline step.
type WorkflowStepOutputRef struct {
	OutputName string `json:"output_name"`
	FileID     string `json:"file_id,omitempty"`     // ProjectFile ID (可预览/下载)
	ArtifactID string `json:"artifact_id,omitempty"` // agent artifact transfer ID
	FileName   string `json:"file_name,omitempty"`
	MimeType   string `json:"mime_type,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
}

// WorkflowProgress is the project's overall pipeline progress.
type WorkflowProgress struct {
	WorkflowName string                 `json:"workflow_name"`
	Steps        []WorkflowStepProgress `json:"steps"`
}

// WorkflowSyncChangeKind 描述一次模板同步里单个步骤的处置方式。
type WorkflowSyncChangeKind string

const (
	WorkflowSyncAdd           WorkflowSyncChangeKind = "add"            // 模板新增步骤：将加入项目
	WorkflowSyncUpdate        WorkflowSyncChangeKind = "update"         // 模板更新且项目未改：将覆盖为模板新版
	WorkflowSyncKeep          WorkflowSyncChangeKind = "keep"           // 冲突：项目改过，保留项目版本
	WorkflowSyncKeepProject   WorkflowSyncChangeKind = "keep_project"   // 项目自加步骤，模板没有：保留
	WorkflowSyncRemovePending WorkflowSyncChangeKind = "remove_pending" // 模板删除了该步骤：待用户确认
)

// WorkflowSyncChange 是一次模板同步里对单个步骤的处置。Order 为项目目标步骤顺序
// （应用同步后步骤在该工作流中的最终位置），仅对保留进项目的新增/更新/保留步骤有效。
type WorkflowSyncChange struct {
	Name            string                 `json:"name"`
	Kind            WorkflowSyncChangeKind `json:"kind"`
	ProjectModified bool                   `json:"project_modified,omitempty"` // 项目是否改动过该步骤（keep 时为 true）
}

// WorkflowSyncDiff 是"模板版本 X → 模板版本 Y"对项目内某个继承工作流的合并预览。
// 前端据此展示差异并让用户确认后再应用。
type WorkflowSyncDiff struct {
	TemplateID     string               `json:"template_id"`
	TemplateName   string               `json:"template_name"`
	CurrentVersion int                  `json:"current_version"` // 模板当前版本
	ProjectVersion int                  `json:"project_version"` // 项目已同步到的版本
	Changes        []WorkflowSyncChange `json:"changes"`
}
