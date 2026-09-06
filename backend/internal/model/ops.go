package model

import "time"

// 运维工单状态机：
//
//	open ──► diagnosing ──► guiding ──► resolved   规则不再满足且过观察期
//	                            │
//	                            ├──────► escalated 干预次数用尽仍无改善，转人工
//	                            └──────► ignored   人工忽略
//
// 终态（resolved / escalated / ignored）不再参与自动干预。
const (
	OpsStatusOpen       = "open"       // 刚发现，尚未归因
	OpsStatusDiagnosing = "diagnosing" // 归因中（LLM 调用中）
	OpsStatusGuiding    = "guiding"    // 已下发修复指引，等待执行体响应
	OpsStatusResolved   = "resolved"   // 规则不再满足且已过观察期
	OpsStatusEscalated  = "escalated"  // 干预次数用尽仍无改善，转人工
	OpsStatusIgnored    = "ignored"    // 人工忽略
)

// 一期规则 ID。新增规则必须同步注册到规则引擎的判定表。
const (
	RuleTodoStalled        = "todo_stalled"         // todo 处于 in_progress 但长时间无状态变化
	RuleTaskSilent         = "task_silent"          // 沉默型：任务非终态且长时间无新事件
	RuleDeliverableUnbound = "deliverable_unbound"  // 交付物已上传但未绑定输出位
	RuleDeliverableReject  = "deliverable_rejected" // 交付物上传被拒
)

// 严重级别。
const (
	OpsSeverityWarn     = "warn"
	OpsSeverityCritical = "critical"
)

// 归因来源。LLM 不可用时为 none，此时改用模板指引并标记需人工复核，
// 绝不静默放弃。
const (
	OpsAttrTemplate = "template" // 命中预设模板库
	OpsAttrLLM      = "llm"      // LLM 归因
	OpsAttrNone     = "none"     // 无归因（LLM 不可用 / 未配置）
)

// 干预等级。
const (
	OpsLevelL0 = "L0" // 仅记录工单 + 通知人
	OpsLevelL1 = "L1" // 给执行体下发修复指引（自动，带节流）
)

// 动作类型（OpsAction.Kind）。
const (
	OpsActionCreated    = "created"    // 工单创建
	OpsActionDiagnosed  = "diagnosed"  // 归因完成
	OpsActionGuided     = "guided"     // 下发修复指引
	OpsActionReminded   = "reminded"   // timeout_monitor 催办留痕（不占指导预算）
	OpsActionEscalated  = "escalated"  // 升级人工
	OpsActionResolved   = "resolved"   // 关闭
	OpsActionIgnored    = "ignored"    // 人工忽略
	OpsActionReopened   = "reopened"   // 观察期内复发，重新打开
)

// OpsIncident 是一次异常的生命周期聚合。
//
// 🔴 OrgID 跟随「被扫描的资源」推导（task → project → 执行 agent → 个人租户兜底），
// 而不是巡检器自身的身份 —— 后台巡检是 Go 协程，没有 HTTP 请求上下文，
// 拿不到 X-Org-Id。推导逻辑见 store.resolveEventOrgUnsafe 同款思路。
type OpsIncident struct {
	ID        string `json:"id" bson:"_id"`
	OrgID     string `json:"org_id,omitempty" bson:"org_id,omitempty"`
	UserID    string `json:"user_id" bson:"user_id"`
	DedupeKey string `json:"dedupe_key" bson:"dedupe_key"` // 主体+规则ID，唯一索引做硬去重
	RuleID    string `json:"rule_id" bson:"rule_id"`
	Status    string `json:"status" bson:"status"`
	Severity  string `json:"severity" bson:"severity"`

	Title   string `json:"title" bson:"title"`
	Summary string `json:"summary,omitempty" bson:"summary,omitempty"`     // 现象描述
	RootCause string `json:"root_cause,omitempty" bson:"root_cause,omitempty"` // 根因
	AttrSource string `json:"attr_source,omitempty" bson:"attr_source,omitempty"`

	// 关联主体（按需填充，不必全有）
	ProjectID string `json:"project_id,omitempty" bson:"project_id,omitempty"`
	TaskID    string `json:"task_id,omitempty" bson:"task_id,omitempty"`
	TodoID    string `json:"todo_id,omitempty" bson:"todo_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty" bson:"agent_id,omitempty"`
	NodeID    string `json:"node_id,omitempty" bson:"node_id,omitempty"`

	// 干预计数：达到 OpsGuideMaxPerTodo 后不再自动下发，转 escalated
	GuideCount int `json:"guide_count" bson:"guide_count"`

	// 活跃标记：dedupe_key 的部分唯一索引只对 active=true 生效，
	// 终态工单保留历史且不阻止同类新工单创建（同一问题复发场景）。
	// 状态迁移点必须同步维护（见 store 层 UpdateOpsIncidentStatusUnsafe）。
	Active bool `json:"active" bson:"active"`

	Actions  []OpsAction `json:"actions" bson:"actions"`
	EventIDs []string    `json:"event_ids,omitempty" bson:"event_ids,omitempty"` // 关联事件，供时间线展示

	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" bson:"updated_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty" bson:"resolved_at,omitempty"`
}

// OpsAction 是工单内的一次动作记录，由统一干预编排器产出。
// 每次下发（含被节流跳过的）都要留痕，否则无法回溯"到底通知过 agent 没有"。
type OpsAction struct {
	ID         string         `json:"id" bson:"id"`
	At         time.Time      `json:"at" bson:"at"`
	Level      string         `json:"level" bson:"level"`                         // L0 | L1
	Kind       string         `json:"kind" bson:"kind"`                           // 见 OpsAction* 常量
	TemplateID string         `json:"template_id,omitempty" bson:"template_id,omitempty"`
	Target     string         `json:"target,omitempty" bson:"target,omitempty"`   // 目标节点
	Content    string         `json:"content,omitempty" bson:"content,omitempty"` // 实际下发的指引全文
	Result     string         `json:"result" bson:"result"`                       // sent | throttled | failed | skipped
	Detail     string         `json:"detail,omitempty" bson:"detail,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

// 动作结果。
const (
	OpsResultSent      = "sent"      // 已下发
	OpsResultThrottled = "throttled" // 被节流跳过
	OpsResultFailed    = "failed"    // 下发失败
	OpsResultSkipped   = "skipped"   // 主动跳过（如已达上限 / 人工忽略）
)

// IsTerminal 判断工单是否处于终态。
func (i *OpsIncident) IsTerminal() bool {
	switch i.Status {
	case OpsStatusResolved, OpsStatusEscalated, OpsStatusIgnored:
		return true
	}
	return false
}

// IsActive 判断工单是否处于活跃状态（参与去重与自动干预）。
func (i *OpsIncident) IsActive() bool {
	return !i.IsTerminal()
}

// CanGuide 判断是否还能自动下发修复指引。
// 已达上限或处于终态时返回 false —— 防止 agent 被反复唤醒。
func (i *OpsIncident) CanGuide(maxPerTodo int) bool {
	if i.IsTerminal() {
		return false
	}
	if maxPerTodo <= 0 {
		maxPerTodo = 3
	}
	return i.GuideCount < maxPerTodo
}
