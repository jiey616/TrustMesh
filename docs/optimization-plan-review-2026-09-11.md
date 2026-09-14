# 《完整分阶段优化方案》交叉评审意见

> 评审日期：2026-09-11
> 被审文档：`docs/optimization-plan-2026-09-11.md`（架构师产出）
> 评审人：产品经理 许清楚 / 工程师 寇豆码 / QA 工程师 严过关（独立并行复审）
> 汇编：主理人 齐活林
> 共识结论：**不能直接开工** —— 技术判断扎实（证据锚定到 `file:line`、P0 收敛方向正确），但存在 3 处阻塞级技术缺陷、多处遗漏、以及「质量保障设计比代码改动落后一个阶段」的结构性问题。

---

## 0. 三方结论速览

| 评审人 | 结论 | 最大风险 |
|---|---|---|
| 工程师 寇豆码 | ❌ 不能直接开工 | T0.2 / T0.6 / T0.7 三条「怎么改」存在实质性技术缺陷 |
| 产品经理 许清楚 | ❌ 不建议直接实施 | 「多租户仍是单用户」未作产品就绪度验证 +「重启不丢数据」过度承诺 |
| QA 严过关 | ❌ 不建议直接开工 | 阶段0 要动越权裁决代码却无回归测试；4 个任务零验收条目 |

正面评价（三方一致）：**file:line 引用准确度高**（工程师抽查 6 处全部命中）、P0 识别准确、任务可验证、回滚清晰、阶段划分骨架正确。

---

## A. 阻塞级技术缺陷（工程师，必须修后才能开工）

### A1. T0.2 放宽创建期 —— 签名不匹配 + 全量越权敞口
- **签名不匹配**：方案写「把 `agentCanWriteTaskUnsafe` 的 3 选一前置到创建期」，但 `agentCanWriteTaskUnsafe(task, agent)`（`scope.go:141`）要求传入**已存在的 task**，而 `CreateTaskByPMNode`（`workflow.go:296`）此刻只有 `project`，task 尚未创建 → **无法直接复用**。
- **越权敞口（严重）**：照搬 3 选一会把 `project.OrgID == ""` 的**存量个人项目**当成「同 org」匹配 → 存量个人项目的任务可被**任意 agent 指派 = 全量跨用户越权放开**。
- **测试不足**：`TestCreateTaskAssignsSharedOrgAgent` 只是单一 happy path，缺反向用例。
- **修法**：新增 `agentCanAssignableToProjectUnsafe(agent, project)`（project 维度）；**必须保留空 org 短路** `if project.OrgID == "" { return agent.UserID == project.UserID }`（严格 user 相等）；补反向用例（跨 org agent → 403；存量空 org 项目 → 跨 user agent → 403）；风险等级由「中」上调。

### A2. T0.6 协议信封强 schema —— 会打断生产 persona 节点
- `protocol/clawsynapse.go:76-81` 明确记载：**山雨 persona 型节点会发送「手写 JSON 字符串」作为回报**，这类消息**是合法 JSON 但无 `protocol` 字段**。按方案「未知/缺失 protocol **直接拒**」→ 破坏现有兼容。
- 方案风险栏只写「需与节点侧跨仓协调」，**未写双读过渡、灰度开关、回退窗口**。
- **修法**：改为**双读过渡** —— ① 阶段0 只加「未知/缺失 protocol」的**计数与日志**（不拒）；② 节点侧发带 `protocol` 的新镜像并灰度；③ 观测未知 protocol 计数归零后，阶段1 才切「直接拒」，且加**配置开关可一键回退**。验收补「双读期内零误拒」。

### A3. T0.7 超时递增退避 —— 治标不治本，且会加剧故障
- **根因未治**：`liveness.go:8` 已记载 ——「TD_05 stalled 8h across 14 reminders, every one of them with **remind_count=1 because each error report reset it**」。计数器被错误上报**重置**才是卡死主因，**单纯提高上限不会让计数器更容易到顶，只会让卡死时间更长**。
- **与硬截止时间撞车**：阶梯 15m→30m→1h→2h 累计 **225min ≈ 3.75h**，而 `timeout_monitor.go:39` `defaultHardDeadline=4h`（普通步）——普通 todo 会在阶梯走完前被硬截止**直接判失败**，「升级人工」形同虚设。
- **混淆三个计数器**：`MaxReminders`(催办) / `MaxRetries`(重派) / `MaxReworks`(返工) 是三回事（`timeout_monitor.go:19/21/22`），方案并列书写且**漏了 `defaultMaxReworks=3`**。
- **修法**：① 先修根因（错误上报不得重置 remind_count，与 `LastProgressAt` 对齐）；② 再调阶梯，明确规定「阶梯累计时长 < 对应 kind 的 hardDeadline」（普通 <4h、heavy <8h）；③ 分开写三个计数器语义与目标值。

---

## B. 工程视角遗漏项（8 条）

| # | 遗漏点 | 证据 | 建议归位 |
|---|---|---|---|
| B1 | `WebhookHandler` 运行时 setter 并发改写 → **data race** | `SetKnowledgeComponents:72`/`SetAgentFileConfig:83`/`SetMeetingActivityNotifier:92` | 阶段0 新增 T0.10（成本 1–2h）：构造期 DI 或 `atomic.Value` |
| B2 | `FlexibleTodoResult` 字符串优先解码 | `protocol/clawsynapse.go:84` | 并入 T0.6（同属协议解析，同一改动窗口） |
| B3 | CRLF/LF 混用（仓库卫生） | `store/` 内 24/45 文件 CRLF | 阶段1 独立小任务；**按包分批** normalize（勿全量，否则巨 diff 与「单 commit + revert」回滚策略冲突） |
| B4 | 其余 `_ = c.ShouldBindJSON` 未纳入 | `task.go:410`、`llm_config.go:164,257`、`external_app.go:160`、`ops.go:103,116` | 扩充 T0.4（现只列 2 个文件） |
| B5 | `store_agent.go:700` 遗漏 | `ev.UserID == agent.UserID`（pmStatsUnsafe） | T0.1 明确枚举（现只列 `:722`） |
| B6 | `ops_incident.go:237/239`、`store_join_request.go:204` 被「等」字带过 | 见左 | T0.1 明确枚举，避免漏改 |
| B7 | `*Unsafe` 持锁契约无护栏 | 全 `store/` | 阶段1：把 `go test -race ./internal/store/...` 加进 CI |
| B8 | 包级全局 `meetingOutgoingDedup` | `clawsynapse/webhook.go:783` | 阶段1/2：收进 `WebhookHandler` 实例字段 |

### 其他工程修正
- **T0.1 风险标注误导**：写「风险低（Go 强类型，漏改编译报错）」—— 真实风险是**语义**：`visibleToScope` 在 OrgID 存在时是**租户级共享**，与原 `x.UserID == userID` 的**用户级独占**不同，机械替换会静默放宽/收紧；且部分函数（如 `executorStatsUnsafe`）根本没有 userID 入参，需跨调用方透传 Scope。→ 风险改「中」，补「逐函数比对语义差异 + 越权单测」。
- **T0.8 停机 flush 可能挂死**：「persist 失败转 error 阻断停机」在 Mongo 不可用时会导致**无限挂起**，最终被 `docker stop` 超时 SIGKILL，**反而必丢数据**。→ 改为**有界等待**（如 30s）+ 超时 fail-fast 强告警；同步配置 compose `stop_grace_period`。

### 依赖与工期
1. 阶段0「1–2 周做 T0.1–T0.9（含跨仓 T0.6）」不现实 → **T0.6 拆两段**，阶段0 只做「计数+日志+双读准备」，强制 schema 移到阶段1。
2. 缺依赖连线：T0.1 → T0.2（org 语义须先收敛，否则半收敛状态放宽权限）；T0.9 → T3.1（NATS 不隔离，阶段3 多实例会双派发）。
3. T0.8 与 T0.9 都改 `docker-compose.yml`，存在**同文件冲突**，需协调提交顺序。
4. T1.3（定前端 EOL）应先于 T1.5（前端测试基线），否则给即将 EOL 的旧轨写基线是浪费。

---

## C. 产品视角：2 项完全遗漏 + 6 项不妥

### C-遗漏
| # | 遗漏点 | 为什么重要 | 建议 |
|---|---|---|---|
| A1 | **真实第二租户冷启动端到端验证** | 「多租户改造完成却仍是单用户」是**产品就绪度**问题，不是代码问题。现有 T1.5 只有合成 A/B org 的技术断言，证明不了「真能开一家企业」。且 multi-tenant 原「阶段4 前端」（租户切换器/企业管理/成员权限/节点审批 UI）**在方案里无归属** | 阶段1：真实环境创建第二用户+企业租户，走完「邀请→审批→跨成员共享 agent→跨成员建项目/会议/任务→切企业隔离→配额展示」**产品验收** |
| A2 | **用户可感知的端到端验收 + 产品级 KPI** | 方案全是工程断言，回答不了「我的流水线能不能自己跑完」。无 KPI 就无法证明 P0 真被解决 | 每阶段末加一行「用户视角可交付」；补 KPI：一次跑通率 / 平均停滞时长 / 无人介入完成率 / 重启零丢失率 |
| A3 | **Agent 合规在阶段0 的平台侧 interim** | T1.4 依赖跨仓同批发版，滑期则「产出合规」这根 P0 支柱在阶段0/1 持续空窗 | 阶段0 先做平台侧独立兜底：关键 skill 标记不可裁剪、重注入系统约束、入库前校验产出结构与必填字段 |
| A4 | **非优雅停机 + 会议「会话态」恢复** | T0.8 只保优雅停机；crash/`kill -9`/OOM 时屏障不触发。且消息持久化了但**会话态**（轮到谁发言、轮次、纪要进度）仍在内存 → 重启后「记录还在、会却开不下去」 | 验收补「kill -9 后不丢」；会议验收写「重启后可读回**且可继续召开**」 |

### C-不妥
| # | 问题 | 建议 |
|---|---|---|
| B1 | 阶段0/1 宣称「重启不丢数据」**过度承诺**（根治在阶段2 write-through） | ①**会议（低写入量）单独提前做 write-through**；②其余加周期性 checkpoint；③表述降级为「优雅停机不丢」 |
| B2 | Agent 合规从 P0 **降级为 P1** | 回调 P0 或至少：阶段0 加 interim + 把「产出合规」写进阶段1 可交付状态 |
| B3 | 砍旧前端排阶段3（3–5 月后）**太晚** | 证据显示 frontend-v2 页面集合已是旧轨**超集**。阶段1 先做 feature-parity 审计，若确为超集则**前置到阶段1/2 之交**下线 |
| B4 | 岗位市场 T4.2 **产品模型未澄清** | 原始设计是本地技能包模式，T4.2 却写成在线服务闭环；frontend-v2 实为 **Electron 桌面应用**，天然适合「一键安装到本地」。**先定产品模型再写代码** |
| B5 | T4.3 与 T1.5 重复；产出物质量门禁笼统且排最后 | CI/PR 门禁合并进阶段1；**产出物质量门禁提至阶段1/2** 并给可判定规则 |
| B6 | 阶段2「1–2 月重构 50+ map」**不现实**（对照：290 处 UserID 就分了 8 批） | 按价值分批：**Meeting → Task → Project 先做 Mongo 权威**，全量 Repository 抽离延后；工期改 2–3 月 |

---

## D. QA 视角：验收不可验证 + 测试落后一个阶段

### D1. 阶段0 有 4 个任务「零验收条目」
阶段0 六条验收只覆盖 T0.2/T0.4/T0.5/测试/NATS；**T0.1（隔离收尾）、T0.3（SystemScope 断言）、T0.6（协议信封）、T0.8（持久化落盘）无任何验收**。→ 必须补齐。

### D2. 测试覆盖遗漏（应前置到阶段0）
| 遗漏测试 | 为什么关键 | 断言要点 |
|---|---|---|
| `liveness` 测试表补**生产真实失败串** | `liveness.go` 共 17 个错误模式，测试仅覆盖 ~1/3；**未覆盖** `"Operation interrupted"`（`liveness.go:23`，生产「卡死 interrupted」直接模式）、`"budget="`、`Context length exceeded` 等 | `IsErrorComment("...Operation interrupted...")==true`；并补「不续命」断言（`RemindCount`/`LastProgressAt` 不被刷新） |
| `workflow` 跨步骤推进单测 | 防 P0①「卡第1步」；方案只有阶段1 的 6 步 E2E（慢、粗、定位差） | 完成 todo-1 → `todo-2.Status=="in_progress"` 且 `DispatchedAt` 非空 |
| **会议/任务重启 rehydrate 测试** | P0⑤ 核心。`meeting.go:66-81` 的 Mongo 懒加载分支**零测试**；方案只做「停机 flush + 快照校验」，**没有「读回」断言** | `CreateMeeting` → 清空内存 → `GetMeeting` 成功且字段一致；rehydrate 后重跑越权测试确保仍 fail-closed |
| T0.2 裁决函数 **8 个调用点**回归 | `agentCanWriteTaskUnsafe` 被 `workflow.go:684/769/911/1155/1235/2189` + `store_artifact.go:217/487` 共 8 处调用，方案只列 1 个测试 | 表驱动：每调用点「同租户共享 agent 可写 / 跨租户不可写」两例 |
| T0.6 协议信封负向测试 | 「未知/缺失 protocol 直接拒」是 P0④ 核心修复，无验收即无保护 | 无 `protocol` → 拒绝且不 fallback chat 解析 |
| 可观测性三件套埋点 | 方案**未采纳**：`step_advance` 延迟埋点（卡第1步最直接早期指标）、`todo in_progress 时长 metric`、`agent 合规哨兵` | metric 端点可查；告警规则存在性单测；agent 连续 N 次不推进 → `agent_step_stalled` 事件 |

### D3. 不合适项
- **C1/C2（最严重）**：阶段0 验收要求补 `org_scope` 单测，但**阶段0 任务列表没有创建它的任务**（排在阶段1 T1.5）。结果：T0.1/T0.2 直接动越权裁决代码，验证手段却只有 `go build` —— **编译通过 ≠ 隔离正确**，已修 P0② 在阶段0 期间**零回归保护**。→ 把越权回归测试写成 T0.1/T0.2 的 **DoD/准入条件**（测试先行），而非「风险」栏备注。样板：照抄 `TestMeetingScopedVisibility`（`store_org_test.go:1116`）。
- **C3**：T1.5「补 `meeting.go` 越权单测」**与现状重复** —— 该测试已存在且覆盖读写 + 状态不被改写（`store_org_test.go:1116-1184`），另有 SystemScope 旁路测试（:1189）。→ 改为「补 meeting **重启 rehydrate** 持久化测试」。
- **C4**：**T0.7b 可能已完工** —— 递增退避已在 `timeout_monitor.go:105-118`（`remindBackoff`）实现，升级人工事件 `todo_remind_escalated` 已在 `:264-279`。另方案引用 `timeout_monitor.go:21` `defaultMaxRetries=3`，但该常量在 `checkTodoTimeouts` 中未见使用。→ 排期前先复核现状，T0.7b 从「新建」改为「核对 + 补测试 + 补监控」。
- **C5**：前端测试基线排阶段1，但阶段0+1 前端仍在改多租户 → **零测试窗口 3–6 周**，恰覆盖串数据风险最高期。→ 最小集（请求头 + 缓存 key 两例）**前置阶段0**。
- **C6**：行号漂移 —— 方案写 `SystemScope`（`scope.go:47-49`），实测 `SystemScope` 在 `scope.go:72`、`visibleToScope` 在 `:45`、`agentCanWriteTaskUnsafe` 在 `:141`。

### D4. 验收标准可验证性修正
- **A5「smoke 注入 `clawsynapse stop` 90s，3min 内自动补派」→ flaky 不可靠**：`defaultDispatchReconcileInterval=2min`（`dispatch_reconciler.go:12`），3min 只够 1.5 个扫描周期，属临界值断言；且依赖外部进程编排。→ 改为**确定性单测**（复用 `liveness_test.go:63-72` 手法，手置 `LastActivityAt`/`RemindCount` 后直接调 `reconcilePendingDispatches()`，秒级零 flaky）+ E2E 改为「轮询不变量，超时 10min 判失败」，「3min」标注为**目标 SLO** 而非断言阈值。
- **A9 前端「≥2 用例」不够** → ≥5 用例并点名（列表隔离/详情越权/请求头正确/缓存失效/401 处理）+ 源码级断言（`queryKey` 与 client header 均含 orgId）。**缓存残留是前端串数据头号根因**。
- A4「403/404 二选一有歧义」→ 钉死：越权→403、不存在→404，断言「绝不返回 200+空数组」。
- A10「字符串匹配不再为主路径」**模糊不可验证** → 改为：给定 `type=todo.error` 时断言走 type 分支且 `IsErrorComment` 未被调用。

---

## E. 开工前最小前置（必须）

1. **修 A 表三条阻塞技术缺陷**：T0.2（project 维度函数 + 空 org 短路 + 反向用例）、T0.6（双读过渡 + 灰度 + 回退开关）、T0.7（先治计数器重置根因 + 阶梯对齐 hardDeadline + 分清三个计数器）。
2. **测试前置到阶段0**：org_scope 越权单测 + liveness 生产真实串用例 + workflow 跨步骤推进单测 + meeting rehydrate，与 T0.1/T0.2/T0.7 **同 PR（测试先行）**。
3. **补齐 T0.1/T0.3/T0.6/T0.8 四个「零验收」任务的验收条目**。
4. **产品层补**：真实第二租户冷启动（阶段1）、每阶段末用户视角可交付 + KPI、会议 write-through 提前、Agent 合规保持 P0 并加阶段0 interim。
5. **smoke 卡死断言**改为「确定性单测 + 轮询不变量」；新增独立可观测性任务与验收。

## F. 用户决策（3 条，✅ 已拍板 2026-09-12）

| # | 决策点 | 用户决定 | 对方案的影响 |
|---|---|---|---|
| 1 | 砍旧 frontend 时机 | ✅ **这次就砍掉，归档** | 删除 T3.2（阶段3砍旧轨）与 T1.3（定 EOL 时间表）；**新增独立任务「旧 frontend 归档」前置到阶段1 开头**（可与阶段0 并行），含 ①feature-parity 审计（确认 v2 是旧轨超集）②归档动作（git 归档 tag / 移 `archive/` / 从 compose 移除 `frontend` 服务）③文档更新。P1⑦ 升级为「阶段1 内彻底解决」；前端测试只需覆盖 **v2 单轨**（工程师提的「T1.3 应先于 T1.5」依赖因归档消解）；阶段3 描述改为「弹性与水平扩展」 |
| 2 | 阶段2 存储重构范围 | ✅ **按价值分批** | `Meeting → Task → Project` 先做成 Mongo 权威（直接兑现用户可感知的「不丢数据」），全量 50+ map Repository 抽离**延后/降级**；工期 1–2 月 → **2–3 月**，给出分批交付点；保留双写过渡 + 校验脚本（`org_backfill`/`org_verify` 模式）+ 分集合灰度 |
| 3 | 岗位市场产品模型 | ⏸️ **暂不动**（用户表示模型未想好） | **T4.2（岗位市场一键闭环）从任务列表移除**，移入文档末尾「已暂缓 / 待产品模型决策」清单，标注「产品模型未定，暂不实施」。阶段4 仅保留 T4.1 货币化护栏、T4.3 质量门禁、T4.4 工作流模板沉淀。`TrustMesh工作市场模块设计文档.md` 保留在待决策区备查 |

> 决策 3 的产品模型二选一仍未关闭（在线服务闭环 vs 本地技能包 + Electron 桌面端一键安装）。frontend-v2 实为 **Electron 桌面应用**，天然适合后者；待用户想清楚后再单独立项，不阻塞本次优化。
