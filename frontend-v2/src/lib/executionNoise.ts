/**
 * 任务「执行过程」里的**机械噪声**事件 —— 默认折叠进「详细日志」，不占主视图。
 *
 * 🔴 为什么要折叠而不是删掉：
 *   这些事件描述的是**平台内部怎么把自己跑起来**（进度上报、状态机自动推进、派发重试、
 *   数字员工上下线、清单结构变更），不改变"用户需要做什么"。它们会以每条 1~2 行的密度
 *   持续刷屏，把真正要看的节点（开始/完成/失败/待确认/提问）淹掉。
 *   但它们是**唯一的审计轨迹** —— 排查硬超时、派发失败、产物错绑全靠它，
 *   所以前端**不能**不渲染，只能默认收起、可展开。
 *
 * 与「通知收窄」的分工不同（别混）：
 *   - 通知侧（`backend/internal/store/store_notification_internal.go`）是**后端直接不生成**；
 *   - 这里只是**前端默认不展开**，数据一条没少，切个开关就能看全。
 *
 * ⚠️ 本清单刻意**不含**承载"故事线"的节点（`todo_assigned` / `todo_started` / `todo_completed`）：
 *    每个步骤的分配→开始→完成是执行过程的骨架，不是噪声。
 *    也**不含**任何失败/确认类（`todo_failed` / `todo_hard_deadline_failed` / `todo_awaiting_review` …），
 *    那些是必须第一眼看到的。
 */
export const EXECUTION_NOISE_EVENTS: readonly string[] = [
  'todo_progress', // 执行侧进度上报（同一 todo 内反复推送，最主要的噪声）
  'agent_status_changed', // 数字员工上线/离线/忙碌
  'todo_auto_advanced', // 状态机自动推进
  'agent_step_stalled', // 步骤疑似停滞（诊断信号）
  'todo_timeout_remind', // 超时提醒（重复催促）
  'todo_dispatch_failed', // 派发失败，后台会自动补派
  'todo_run_budget_exhausted', // 预算耗尽（诊断信号，真卡死另有事件）
  'todo_updated', // 清单条目字段变更
  'todo_removed', // 清单条目删除
  'todos_reordered', // 清单重排
  'todo_output_bound_manually', // 手工绑定产出
  'artifact_bound_after_terminal', // 终态后补绑产物
]

/** 该事件是否属于"机械噪声"（默认折叠、不占主视图）。 */
export function isExecutionNoise(eventType: string | null | undefined): boolean {
  return typeof eventType === 'string' && EXECUTION_NOISE_EVENTS.includes(eventType)
}
