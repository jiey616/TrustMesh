# TrustMesh 测试与可靠性视角调研报告

> 调研人：Edward / 严过关（QA 工程师）
> 调研性质：测试覆盖盘点 + 系统可靠性风险分析（不写业务代码，只做质量保障分析）
> 证据来源：实际 Grep/Glob/Read 了仓库 `backend/` 与 `frontend*/` 源码、测试文件、`AGENTS.md`、`docs/multi-tenant-enterprise-plan.md`、`backend/scripts/smoke-task-flow.sh`

---

## 0. 调研方法与证据基线

| 证据项 | 实测结果 |
|---|---|
| 后端 `*_test.go` 总数 | **40 个**，分布在 10 个包（store 21、clawsynapse 10、handler 2、app 2、auth 1、protocol 1、knowledge 1、assistant 1、model 1、agentfile 1） |
| 前端测试文件（`*.test.*` / `*.spec.*`） | **0 个**（frontend/ 与 frontend-v2/ 均无） |
| 前端测试框架 | 两个 `package.json` 的 `scripts` 仅有 `dev/build/lint/preview`，**无 test 脚本、无 vitest/jest 依赖** |
| 官方测试预期 | `AGENTS.md` 第 24–25 行：「Backend coverage is test-driven… 单测放 `*_test.go`」；「There is no frontend test suite yet, so at minimum run `npm run build` and `npm run lint`」 |
| 端到端脚本 | 仅 `backend/scripts/smoke-task-flow.sh`（单用户、单步骤 happy path） |
| 多租户隔离逻辑 | `backend/internal/middleware/org_scope.go`（OrgScope 中间件，含 401 越权拦截分支） |
| 跨用户越权修复证据 | `store/meeting.go:204`、`store/meeting.go:281` 注释明确记载 fail-open → fail-closed 的 P0 修复 |

---

## 1. 测试覆盖现状盘点

### 1.1 后端单测分布（按包）

| 包 (package) | 模块职责 | 是否有单测 | 测试文件数 | 关键**未测**源文件 | 风险等级 |
|---|---|---|---|---|---|
| `store` | 全内存状态机 / CRUD / 超时 / 调度 | ✅ 有 | 21 | `store_task.go`、`store_meeting.go`、`store_project_file.go`、`store_join_request.go`、`store_events.go`、`store_knowledge.go`、`store_market.go`、`store_planning.go`、`notification.go`、`store_presence.go`、`streams.go`、`user_streams.go`、`cleanup.go`、`bootstrap.go` | **中高** |
| `clawsynapse` | ClawSynapse webhook / skill / plan 校验 | ✅ 有 | 10 | （包整体覆盖较好，无显著空白） | 低 |
| `handler` | HTTP 接口层（23 个文件） | ⚠️ 极少 | 2 | `auth.go`、`clawsynapse.go`、`transfer.go`、`task.go`、`project.go`、`meeting.go`、`project_file.go`、`knowledge.go`、`external_app.go`、`organization.go`、`dashboard.go`、`join_request.go`、`user.go`、`ops.go`、`assistant.go`、`llm_config.go`、`workflow_template.go`、`notification.go` 等 21 个 | **高** |
| `middleware` | **鉴权 + 多租户隔离** | ❌ 无 | 0 | `org_scope.go`（隔离裁决）、`auth.go`（RequireAuth）、`rate_limit.go`、`cors.go`、`recovery.go`、`logging.go` | **高（P0 关联）** |
| `project` | Mongo 存储层 (`storage.go`) | ❌ 无 | 0 | `storage.go` | 中 |
| `knowledge` | 知识库 | ⚠️ 部分 | 1（仅 `chunker_test.go`） | `qdrant.go`、`storage.go`、`processor.go` | 中 |
| `embedding` | 向量化 | ❌ 无 | 0 | `embedding.go`、`openai.go` | 低 |
| `auth` | JWT / 外部 token | ✅ 有 | 1 | — | 低 |
| `app` | 路由 / webhook 入口 | ✅ 有 | 2 | — | 低 |
| `model` | 模型校验 | ✅ 有 | 1 | — | 低 |
| `assistant` / `agentfile` / `protocol` | 工具 / agentfile / 协议 | ✅ 有 | 各 1 | — | 低 |
| `logger` / `transport` | 日志 / 响应封装 | ❌ 无 | 0 | 低风险工具层 | 低 |

**结论**：后端「test-driven」基本属实，但覆盖**高度集中在 store 与 clawsynapse 的内部算法**（状态聚合、webhook 解析、plan 校验），而**最该被测试守卫的「边界」——中间件隔离、handler 鉴权、外部 Mongo/存储层——反而最薄弱**。这正是已知越权缺陷的根因画像。

### 1.2 端到端（smoke）覆盖

`smoke-task-flow.sh` 实际覆盖路径：
`注册 → 建 PM/Dev 两个 agent → 建 project → 建 conversation → 发布 task.create webhook → 发布 todo.complete webhook → 读 task`。

**覆盖缺口**：
- ❌ 多租户隔离：全程单用户单 token，无任何第二用户 / `X-Org-Id` / 跨用户 404/403 断言。
- ❌ 多步骤推进：**只建 1 个 todo**，从未验证「todo-1 完成 → todo-2 被调度推进」（即「卡在第 1 步」症状无法被本脚本发现）。
- ❌ 失败/超时路径：从不触发 3 次提醒判失败、不验证 `interrupted`/failed 态。
- ❌ 会议 / 归档 / 重启持久化：完全未触达。
- ❌ 负向鉴权：不死测 401（错误 token）、越权访问。

### 1.3 前端「零测试」现状定性

- `frontend/`（shadcn 旧轨）与 `frontend-v2/`（AntD v5 + Electron 桌面轨）**均无单测、无组件测试、无 E2E**。
- 当前质量门禁只有 `npm run build` + `npm run lint`（AGENTS.md 明示）。
- **风险定性（高）**：多租户改造正在两个前端并行加功能（组织切换、租户头 `X-Org-Id`、跨租户数据可见性）。无测试意味着：(a) 多租户 UI 越权/串数据只能靠人工点检；(b) 双轨合并/迁移时回归无从保障；(c) 构建通过 ≠ 行为正确（lint 不校验状态逻辑）。前端是目前整个质量体系中**最大的盲点**。

---

## 2. 可靠性风险清单（按 P0 / P1 / P2）

| 等级 | 风险 | 触发条件 | 影响 | 当前是否被任何测试覆盖 | 建议拦截手段 |
|---|---|---|---|---|---|
| **P0** | **跨用户同租户越权（会议/任务 404 被 fail-open 吞掉）** | 用户 A 带他人 `X-Org-Id` 或伪造会议 ID 访问；旧实现「不在内存就不裁决」直接回空/放行 | 越权读他人会议/任务内容；生产已出过 P0（代码注释 `meeting.go:281` 证实已修但无回归保护） | ❌ `middleware/org_scope.go` 无测试；`store/meeting.go` 无测试；仅代码层 fail-closed | 补 `OrgScope` 401 单测 + `meetingVisible`/`AddMeetingMessage` 越权单测；smoke 增加「用户 B 用 X-Org-Id 访问用户 A 资源应 404/403」断言 |
| **P0** | **任务卡死在 interrupted（"Operation interrupted" 反复上报）** | agent 因上下文裁剪/网关截断，循环上报含 `Operation interrupted` 的评论，平台误当进度续命 | todo 永远 `in_progress`，不超时、不失败，任务永久卡死 | ⚠️ `liveness_test.go` 有 `TestIsErrorCommentClassification`，但**未包含** `"Operation interrupted"` 与 `"budget="` 真实生产串（grep 确认无命中） | 在 liveness 测试表追加 `"Operation interrupted"`→`true`、`"budget=60/60"`→`true`；加集成断言「含中断串的评论不会 reset 超时计数器」 |
| **P1** | **任务卡在第 1 步不进第 2 步（skill pruning 不按设计执行）** | workflow 完成 todo-1 后，agent 因上下文压缩把执行 skill 裁剪掉，不推进 todo-2；或状态机未触发下一 todo 调度 | 多步流水线停滞，山雨编剧矩阵、《军旅微电影》等同症 | ⚠️ `dispatch_reconciler_test.go`/`ops_dispatcher_test.go` 覆盖重调度算法；`clawsynapse/context_verify_test.go`、`plan_validation_test.go` 覆盖合规校验；但「完成 step1→step2 推进」的端到端 + agent 实际执行合规**无集成测试** | smoke 扩展为多 todo 流水线，断言 step2 进入 `in_progress`；clawsynapse 增加「plan 被 pruning 后拒绝执行」契约测试 |
| **P1** | **会议数据仅内存、容器重启即丢** | backend 容器重启，内存 `s.meetings` 清空 | 历史会议/纪要丢失（旧缺陷） | ⚠️ 代码已加 Mongo 持久化分支（`meeting.go` 的 `mongoEnabled`/InsertOne/FindOne 懒加载），但**无 meeting_test**，重启 rehydrate 路径未验证 | 补 `CreateMeeting→重启→GetMeeting` 持久化单测；CI 跑「写会议→重启 backend→读回」集成用例 |
| **P1** | **部署链路脆弱（build→stop→poll→rm→up -d 手动编排）** | 人工编排任一步失败/顺序错；前端构建式部署；改 Mongo 前须停 backend | 发布事故、脏数据、服务空窗 | ❌ 无任何部署/冒烟自动化测试 | 部署脚本加 healthcheck 门禁；smoke 跑在 deploy 后自动触发；引入蓝绿/滚动以减少空窗 |
| **P2** | **agent 超时 3 次判失败、不自动重试、不再问用户** | todo `in_progress` 静默超 30min×3 次提醒无响应 | 任务判失败，需人工介入（设计如此，但用户无感知） | ✅ `timeout_monitor_test.go` 覆盖 3 次→failed + 升级事件 | 已覆盖；建议补「升级事件 `todo_remind_escalated` 触发前端告警」的集成断言 |
| **P2** | **前端多租户越权/串数据** | 前端携带错误 `X-Org-Id`、组织切换状态错乱 | 看到别租户数据 | ❌ 零测试 | 前端引入 Vitest+RTL 后优先覆盖「组织切换后请求头/数据隔离」组件测试 |

---

## 3. 建议的测试策略（分层、可落地、按优先级）

### 3.1 后端：核心包必须补的单测（3~5 个最该补的场景 + 断言要点）

1. **`middleware/org_scope.go` — 多租户隔离裁决（P0）**
   - 场景 A：带 `X-Org-Id` 且用户是成员 → `Scope.OrgID/Role` 正确注入。
   - 场景 B：带 `X-Org-Id` 且用户**非**成员 → 必须 `401 Unauthorized`（断言 `c.IsAborted()` 且 body 含 "not a member"）。
   - 场景 C：不带 `X-Org-Id` 的存量客户端 → 仅注入 `UserID`，行为与改造前一致（不 401）。
   - 断言要点：越权路径**绝不**落到 handler；用 `gin.New()` + `httptest` 注入 header 验证。

2. **`store/meeting.go` — 归属 fail-closed（P0，回归保护）**
   - 场景：用户 A 创建会议 → 用户 B（非成员）`GetMeeting`/`ListMeetingMessages`/`AddMeetingMessage` → 必须 `transport.NotFound`（404），**不能**回空或放行。
   - 断言要点：对照 `meeting.go:204`、`:281` 修复点，确保 fail-open 不回归。

3. **`store/liveness.go` — 生产失败串识别（P0）**
   - 场景：`IsErrorComment("...Operation interrupted...")` == `true`；`IsBudgetExhaustedComment("Turn ended with pending tool result ... budget=60/60")` == `true`。
   - 断言要点：这些串**不得**被当作 progress 重置超时计数（直接补进 `TestIsErrorCommentClassification` 表）。

4. **`store/workflow.go` — 跨步骤推进（P1）**
   - 场景：`CompleteTodoByNode` 完成 todo-1 → 断言 todo-2 被置 `in_progress` 且触发调度（`aggregateTaskStatus` 聚合正确）。
   - 断言要点：防「卡在第 1 步」；可复用 `workflow_test.go` 现有 fixture。

5. **`store/meeting.go` — 重启持久化（P1）**
   - 场景：`mongoEnabled=true` 下 `CreateMeeting` → 清空内存 map → `GetMeeting` 从 Mongo 懒加载回灌成功。
   - 断言要点：覆盖 `meeting.go` 的 `FindOne` rehydrate 分支。

### 3.2 端到端：smoke 脚本应扩展的关键流

在 `smoke-task-flow.sh` 基础上新增（建议抽成 `smoke-multitenant.sh` / `smoke-workflow.sh`）：
- **多租户隔离流**：注册用户 A、B；A 建组织并邀请 B；B 带错误 `X-Org-Id` 访问 A 资源 → 断言 404/403；B 带正确 `X-Org-Id` → 断言 200。
- **多步骤推进流**：任务含 `todo-1`/`todo-2` 两个步骤；完成 step1 后 `sleep` 并轮询，断言 step2 进入 `in_progress` 且最终 task 完成（拦截「卡第 1 步」）。
- **失败/超时流（缩短阈值）**：通过环境变量把 `defaultTodoTimeout` 调到秒级，注入静默 todo，断言 3 次提醒后判 `failed` 并产出 `todo_remind_escalated` 事件。
- **重启持久化流**：写会议/任务 → 重启 backend 容器 → 读回断言存在。

### 3.3 前端：最小可行测试引入方案

- **框架选型（从简）**：`Vitest` + `@testing-library/react`(RTL) + `jsdom`；复用现有 Vite/TS/ESLint 基建，零额外构建链。
- **落地顺序**：
  1. 两个前端 `package.json` 加 `test:unit`(vitest run) / `test:watch` 脚本；CI 门禁 `npm run test:unit`。
  2. 先补**多租户相关组件**（组织切换器、带 `X-Org-Id` 的 API client、跨租户数据列表）——这正是当前最高风险面。
  3. 再补核心交互（任务看板、对话流）的 RTL 组件测试。
- **不追求 100%**：先覆盖「租户隔离」「关键用户流」的冒烟级用例，建立回归基线。

### 3.4 可观测性 / 混沌：提前发现「卡住」「不合规」

- **步骤推进埋点**：每个 todo 状态变更上报 metric（`todo_in_progress_duration`、`step_advance_latency`）；对「in_progress 超过 N 分钟未推进」做告警（先于 30min 超时提醒，给 SRE 早入场）。
- **agent 合规哨兵**：clawsynapse 侧对「plan 被 pruning / 未按要求执行」打 structured log + 计数；当某角色 agent 连续 N 次不推进 step，触发告警（定位 skill-pruning 类问题）。
- **超时/失败看板**：把 `todo_timeout_failed` / `todo_hard_deadline_failed` / `todo_remind_escalated` 事件接入监控，按项目维度聚合，主动发现「批量卡死」。
- **混沌演练**：定期注入「backend 重启」「agent 静默」「跨租户越权请求」三类故障，验证测试与告警能否兜住（Chaos 工程佐证测试有效性）。

---

## 4. QA 视角 Top 3 行动建议（按性价比排序）

1. **【最高性价比·P0】补 `middleware/org_scope.go` + `store/meeting.go` 的越权单测，并把「跨用户越权」加进 smoke。**
   理由：这是已发生过的生产 P0，根因（fail-open）已在代码层修，但**零回归测试**，随时可能回退；单测成本低、收益极高，且能直接守卫多租户改造这条主线。

2. **【高性价比·P1】把 smoke 从「单用户单步骤」升级为「多租户隔离 + 多步骤推进 + 失败超时」三条流水线，并接入 CI 在每次部署后自动跑。**
   理由：当前 smoke 完全发现不了「卡第 1 步」「卡 interrupted」「越权」三类最高频故障；扩展成本主要是脚本编写，却能覆盖绝大多数生产痛点。

3. **【战略投资·P1】前端引入 Vitest+RTL，优先补「多租户隔离」组件测试。**
   理由：双轨前端零测试 + 多租户改造并行 = 最高盲点；先以最小框架成本建立租户隔离回归基线，可避免改造引入的越权串数据类事故。

---

## 5. 附录：后端测试文件清单（40 个）

- `store/`(21)：store_copy, store_notification, workflow_progress_align, store_agent, store_workflow_template, workflow_review, workflow, dashboard, store_user, store_external_app, ops_scanner, ops_dispatcher, workflow_cancel_notify, liveness, dispatch_reconciler, store_agent_chat, store_org, store_project, reopen_orphan, timeout_monitor, bind_step_output
- `clawsynapse/`(10)：context_verify, upload_skill, webhook_json, context, plan_validation, webhook_unwrap, webhook_input, webhook_ack, webhook_transfer, client_publish_retry
- `handler/`(2)：agent_capability, agent_chat
- `app/`(2)：webhook, router
- `auth/`(1)：external_token
- `protocol/`(1)：clawsynapse
- `knowledge/`(1)：chunker
- `assistant/`(1)：tools
- `model/`(1)：workflow_validate
- `agentfile/`(1)：agentfile

**零测试包**：`middleware`(6 文件)、`project`(storage.go)、`embedding`(2)、`logger`(1)、`transport`(1)、`knowledge`(qdrant/storage/processor)、以及 store 下 14 个未列出的子文件（见 1.1）。
