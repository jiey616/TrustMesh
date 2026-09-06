package assistant

import (
	"context"
	"fmt"

	"trustmesh/backend/internal/model"
)

// ─────────────────────────────────────────────────────────────────────────────
// 运维 LLM 归因（F 分支）：把工单快照交给 LLM，输出结构化根因。
// 由 app 层注入 store（store 不能 import assistant，反向依赖）。
//
// 降级契约（F 决策）：LLM 不可用/失败时返回 error，store 侧降级为模板指引
// 并标记需人工复核——绝不静默失效，也绝不让归因失败阻塞指引下发。
// ─────────────────────────────────────────────────────────────────────────────

const opsAttributionSystemPrompt = `你是 AIGC 生产平台的运维归因助手。基于给出的平台状态快照，判断异常的最可能根因，并给出简短的修复建议。要求：
1. 只输出 3-6 句话，不要客套。
2. 优先考虑这些常见根因：执行 agent 的 LLM 上下文压缩后失忆、上传文件漏带 --metadata 参数、工作流步骤声明的 agent_id 与 todo 指派不一致、节点离线、上游 todo 未完成导致输入缺失。
3. 如果快照信息不足以判断，明确说"证据不足"，并列出需要补充取证的方向。
4. 用中文。`

// OpsAttributor 生成 store.SetOpsAttributionHook 需要的归因函数。
// B2：LLM 配置按租户解析——工单归属哪个租户，就用哪个租户的生效配置；
// 工单 OrgID 为空（个人租户兜底）时走平台默认/env。解析层保证 ops_model
// 兜底为对话模型；配置不可用返回 error（store 侧降级模板 + 标记人工复核）。
func OpsAttributor(provider *LLMProvider) func(ctx context.Context, inc *model.OpsIncident, snapshot string) (string, error) {
	return func(ctx context.Context, inc *model.OpsIncident, snapshot string) (string, error) {
		if provider == nil {
			return "", fmt.Errorf("llm provider unavailable")
		}
		client := provider.AttributionClientFor(inc.OrgID)
		if client == nil {
			return "", fmt.Errorf("llm not configured for org %q", inc.OrgID)
		}
		user := fmt.Sprintf("工单 %s（规则 %s，严重级别 %s）\n\n%s", inc.ID, inc.RuleID, inc.Severity, snapshot)
		return client.Complete(ctx, opsAttributionSystemPrompt, user)
	}
}
