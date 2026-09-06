package store

import (
	"fmt"
	"strings"
	"time"

	"trustmesh/backend/internal/model"
)

// ─────────────────────────────────────────────────────────────────────────────
// 修复指引模板库（C.1：模板优先 + LLM 兜底）。
//
// 实证依据：agent 手搓 JSON 是惯犯，指引越具体、越结构化，自纠成功率越高；
// 模糊建议（"请检查你的配置"）等于没说。所有模板必须带可执行的具体命令与参数。
// webhook 侧既有的两条交付物警告文案（warnUnboundDeliverable /
// warnTransferRejected）是实测有效的模板，此处与其口径保持一致。
// ─────────────────────────────────────────────────────────────────────────────

// opsGuideContext 生成指引所需的现场快照（持锁阶段采集）。
type opsGuideContext struct {
	RuleID      string
	TaskID      string
	ProjectID   string
	TodoID      string
	TaskTitle   string
	TaskStatus  string
	TodoTitle   string
	NodeID      string
	StalledFor  time.Duration // 距最近一次活动多久
	Threshold   time.Duration // 静默阈值
	RootCause   string        // LLM 归因结果，可为空
}

// opsGuideContent 按规则生成结构化修复指引全文。
// 返回 templateID 供 OpsAction.TemplateID 留痕。
func opsGuideContent(ctx opsGuideContext) (content, templateID string) {
	stalled := "超过 " + humanizeDuration(ctx.StalledFor)
	if ctx.Threshold > 0 {
		stalled = fmt.Sprintf("超过 %s（阈值 %s）", humanizeDuration(ctx.StalledFor), humanizeDuration(ctx.Threshold))
	}

	var b strings.Builder
	switch ctx.RuleID {
	case model.RuleTodoStalled:
		templateID = "todo_stalled_v1"
		b.WriteString("【运维修复指引】todo 长时间无进展\n\n")
		b.WriteString("## 现象\n")
		b.WriteString(fmt.Sprintf("任务《%s》中的 todo《%s》处于执行中已 %s 无任何上报。\n\n", ctx.TaskTitle, ctx.TodoTitle, stalled))
		b.WriteString("## 要求\n")
		b.WriteString("1. 这是对既有任务的步骤推进提醒，不是新任务指派，禁止从头重跑整个流程，应基于已完成的工作继续。\n")
		b.WriteString("2. 若工作已完成：立即用 todo.complete 交付；文件上传必须用 `clawsynapse transfer send` 并携带 --metadata taskId=… --metadata todoId=…（声明输出位的步骤还要加 --metadata outputName=<输出位名>）。\n")
		b.WriteString("3. 若仍在进行：立即用 todo.progress 回报当前进度与剩余工作。\n")
		b.WriteString("4. 若被阻塞无法继续：用 todo.fail 说明具体原因（缺输入 / 权限不足 / 技术故障）。\n")

	case model.RuleTaskSilent:
		templateID = "task_silent_v1"
		b.WriteString("【运维修复指引】任务长时间无任何活动\n\n")
		b.WriteString("## 现象\n")
		b.WriteString(fmt.Sprintf("任务《%s》处于 %s 状态已 %s 无事件、无 todo 上报（沉默型卡滞）。\n\n", ctx.TaskTitle, ctx.TaskStatus, stalled))
		b.WriteString("## 要求\n")
		b.WriteString("1. 请回报当前所处步骤与状态：能继续则立即推进，并用 todo.progress 回报进度。\n")
		b.WriteString("2. 已完成的部分直接 todo.complete 交付，不要等全部做完一起交。\n")
		b.WriteString("3. 确实卡住（缺上游输入 / 依赖未就绪）用 todo.fail 说明具体阻塞点，平台会通知相关方。\n")

	case model.RuleDeliverableUnbound:
		templateID = "deliverable_unbound_v1"
		b.WriteString("【运维修复指引】交付物已上传但未绑定输出位\n\n")
		b.WriteString("## 现象\n")
		b.WriteString(fmt.Sprintf("任务《%s》收到文件但未能自动绑定到步骤输出位（已 %s）。\n\n", ctx.TaskTitle, stalled))
		b.WriteString("## 修复步骤\n")
		b.WriteString("重新上传并声明输出位：`clawsynapse transfer send --metadata taskId=… --metadata todoId=… --metadata outputName=<输出位名> <文件路径>`。\n")
		b.WriteString("outputName 必须与工作流步骤声明的输出位名完全一致（大小写敏感）。\n")

	case model.RuleDeliverableReject:
		templateID = "deliverable_rejected_v1"
		b.WriteString("【运维修复指引】文件上传未入库\n\n")
		b.WriteString("## 现象\n")
		b.WriteString(fmt.Sprintf("任务《%s》的文件传输被平台拒绝（已 %s）。\n\n", ctx.TaskTitle, stalled))
		b.WriteString("## 修复步骤\n")
		b.WriteString("1. 核对上传命令：`clawsynapse transfer send` 必须携带 --metadata taskId=… --metadata todoId=…。\n")
		b.WriteString("2. 核对文件路径与大小限制后重新上传；多次被拒请先回报 todo.fail。\n")

	default:
		templateID = "generic_v1"
		b.WriteString("【运维修复指引】\n\n")
		b.WriteString(fmt.Sprintf("任务《%s》触发运维规则 %s（%s）。\n\n", ctx.TaskTitle, ctx.RuleID, stalled))
		b.WriteString("请回报当前状态：能继续则用 todo.progress 推进；已完成用 todo.complete 交付；被阻塞用 todo.fail 说明原因。\n")
	}

	if ctx.RootCause != "" {
		b.WriteString("\n## 根因分析（AI 归因，供参考）\n")
		b.WriteString(strings.TrimSpace(ctx.RootCause))
		b.WriteString("\n")
	}
	b.WriteString("\n## 验证方式\n")
	b.WriteString("平台巡检将确认你的上报；若仍无进展会再次催办，多次无改善将转人工处理。\n")
	return b.String(), templateID
}

// humanizeDuration 简短可读的时长（巡检文案用，不需要精确到 ns）。
func humanizeDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%.1f 天", d.Hours()/24)
	case d >= time.Hour:
		return fmt.Sprintf("%.1f 小时", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	default:
		return d.String()
	}
}
