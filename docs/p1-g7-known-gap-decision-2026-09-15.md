# P1 决策：G7 缺口（新租户无 PM agent）— 接受为已知产品缺口

> 决策人：用户（jiey）　日期：2026-09-15
> 结论：**选 B —— 接受为已知缺口**，不在生产部署常驻 PM agent 节点，也不改 `pm_agent_id` 必填约束。
> 关联：T1.7 验收报告 §5 缺口表（G7）、`docs/t1.7-g7-localnode-plan-2026-09-14.md` §7 实测。

## 1. 缺口定义

**症状**：新建企业租户（如验收中的 Org2）在开通后**无法自助建项目** —— `POST /projects` 返回
`PROJECT_PM_AGENT_INVALID: pm_agent_id must reference a PM agent of current user`。

**根因**：建项目要求 `pm_agent_id`，且该 agent 必须属于当前用户/租户（`store_project.go:86` 判空 + `:97 pmAgentForScopeUnsafe` 校验 role=pm 且租户可见未归档）。新租户**没有任何 agent**，因此无可用 `pm_agent_id`。这不是代码缺陷，而是**「agent 入企」是「建项目」的前置依赖**，而入企这一步没有自助路径。

**影响面**：仅影响**新企业租户首次开通**这一刻。已有 agent 的租户、以及个人空间（会自动带个人 agent）不受影响。

**证据**：T1.7 Step5 实测被挡（`t17_step5_out.txt` L3-5）；`GET /agents/join-requests` 返回 `count:0`（生产无空闲节点可发起入企）。

## 2. 决策与理由

选择 **B（接受为已知缺口）**，理由：

- 触发频率低：只在开新企业租户时遇到，且当前生产只有 1 个真实企业租户（画宗）。
- 有已验证的运维变通方案（见 §3），阻塞可由运维在分钟级解除，不是死局。
- 方案 A（生产常驻一个 PM agent 节点）会引入长期运维负担与信任/节点治理成本，收益与当前租户规模不匹配。

**明确不做**：
- 不在生产常驻/预留 PM agent 节点；
- 不放宽 `pm_agent_id` 必填（放宽会让项目失去 PM 归属，破坏规划链路）；
- 不为它加临时开关。

## 3. 当前变通方案（运维代配，T1.7 已实测闭环）

1. 起一个全新 agent 节点容器（新 data 卷 = 未注册身份），NATS 指向生产 `nats://175.27.135.91:4222`，`trustMode: tofu`。
2. 用租户 owner token 带 `X-Org-Id` 调 `GET /agents/invite-prompt`，取生产 daemon node_id 与 challenge 命令。
3. 节点内执行 `clawsynapse auth challenge` + `trust request --target <prodNodeID> --reason '{"user_id":...,"org_id":...}'`（reason 必须带 user_id/org_id，归属才锁对该租户）。
4. 平台侧 `GET /agents/join-requests` 可见 pending → owner 调 `POST /agents/join-requests/:id/approve` 批准。
5. 批准后生成**归属该租户**的 PM agent，此时 `POST /projects` 放行（T1.7 实测 201 / active / pm online）。

> 补充事实（避免误判）：`POST /projects` **不校验 agent 在线**，`offline → PM_AGENT_OFFLINE` 门禁只在**建任务触发 PM 规划**时生效（`store_planning.go:51/163/453`）。所以只要 agent 存在且归属正确即可，不必保持在线。

## 4. 复评触发条件（满足任一即重新评估）

- 出现**第二个及以上真实企业租户**需要自助开通；
- 「企业自助开通」被列入产品需求（届时应当做的是自助 agent 入企引导，而非常驻节点）；
- 运维代配在真实开租户时**实际成为阻塞**（例如租户 owner 无法提供节点、或审批链路无人操作）。

## 5. 状态

- 生产现状：无常驻 PM agent；验收用的临时节点 `clawsynapse-t17` 与 PM agent `b3abe589…` 均已按约回滚删除，复验 `GET /agents`（Org2）→ `items:[]`。
- 本缺口**不阻塞** T1.7 验收结论（七步全绿，G7 已回归并留下闭环证据）与阶段 1 收口。
