# 平台内置运维智能体（OpsAgent）一期实施计划

> 决策来源：2026-09-06 决策树定稿（见 `.workbuddy/memory/2026-09-06.md`）  
> 一句话形态：**Go 内置运维大脑（规则发现 + LLM 归因 + 干预编排）+ 专属 Hermes 运维节点（只读取证）**  
> 一期范围：**任务卡滞 + 交付物异常** 两类规则的完整闭环（发现 → 归因 → 指导下发 → 工单状态追踪）



---

## 0. 勘察结论：三处现成接口可复用（决定了改造量）

| 现有设施                               | 位置                                       | 一期的用法                                                                                   |
| ---------------------------------- | ---------------------------------------- | --------------------------------------------------------------------------------------- |
| `remindHook` / `planningStallHook` | `store/store.go:155,162`                 | timeout_monitor **已把"计时判死"与"下发"解耦**。统一编排器不必重构它，只需把这两个 hook 改指到 `InterventionDispatcher` |
| `notifySystemWarningToAgent`       | `clawsynapse/webhook.go:2701`            | 现成的"发 `task.mention` 让执行体自纠"实现，含目标解析与 fire-and-forget。作为 L1 下发的底层通道                     |
| `Scope` / `currentScope`           | `store/scope.go`、`handler/helpers.go:15` | 租户裁决与 HTTP 取 Scope 的入口都已就绪，助手工具层补 Scope 有现成抓手                                           |

⚠️ **一处必须先行修复**：`assistant.ToolExecutor` 全部方法直接收 `userID`，无 `Scope` 参数 → 多租户下必然跨租户泄漏。此项列为阶段 0 的硬前置。

---

## 1. 一期目标与非目标

**做**：

1. 规则引擎发现两类异常（任务卡滞 / 交付物异常），含**沉默型故障**（不发任何事件的卡死）
2. 异常聚合成 `ops_incident` 工单，按「主体 + 规则」去重
3. 归因：模板库命中优先，未命中才调 LLM
4. L1 干预：自动给执行体发修复指引 `task.mention`，带节流 / 去重 / 次数上限
5. 工单状态机 + 完整动作时间线，规则反向验证加观察期后自动关闭
6. 前端运维工单页面（列表 / 详情 / 动作时间线）

**不做**（二期及以后）：

- 专属 ops 节点部署与运维 skill（走 JoinRequest 接入）——一期诊断全在平台侧，不下探宿主
- 节点健康 / 质量异常两类规则（信号源已有，加规则即可，留作第二批）
- L2 系统变更（重启容器 / 改配置）——需人工确认入口与 ops 节点配合，二期做

---


## 2. 数据流

```
[输入 A: 定时巡检]  ──按租户分片扫描──┐
                                      ├─→ 规则引擎（确定性判据）──→ 命中
[输入 B: 事件流]    ──webhook 异常信号─┘                              │
                                                                     ▼
                                                    ops_incident 工单（dedupe_key 去重）
                                                                     │
                                            ┌────────────────────────┼────────────────────┐
                                            ▼                        ▼                    ▼
                                    模板库命中？              未命中 → LLM 归因      记录 L0（工单+通知人）
                                    │是                        │                          │
                                    ▼                          ▼                          │
                                    └──────→ 生成修复指引 ←──────┘                          │
                                                    │                                     │
                                                    ▼                                     │
                                    InterventionDispatcher（唯一出口）                     │
                                    · 节流 / 去重 / 次数计数                                │
                                    · 调 notifySystemWarningToAgent 发 task.mention         │
                                    · 写 ops_action 留痕 ←────────────────────────────────┘
                                                    │
                                                    ▼
                                    规则反向验证 + 观察期 → resolved / escalated
```

---

## 3. 数据模型


### 3.1 `model/ops.go`（新建）

```go
// 工单状态机：open → diagnosing → guiding → (resolved | escalated | ignored)
const (
    OpsStatusOpen       = "open"       // 刚发现，未归因
    OpsStatusDiagnosing = "diagnosing" // 归因中（LLM 调用中）
    OpsStatusGuiding    = "guiding"    // 已下发修复指引，等待执行体响应
    OpsStatusResolved   = "resolved"   // 规则不再满足且过观察期
    OpsStatusEscalated  = "escalated"  // 干预次数用尽仍无改善 → 转人工
    OpsStatusIgnored    = "ignored"    // 人工忽略
)

// 一期规则 ID
const (
    RuleTodoStalled        = "todo_stalled"        // todo in_progress 超时无状态变化
    RuleTaskSilent         = "task_silent"         // 沉默型：任务非终态且长时间无新事件
    RuleDeliverableUnbound = "deliverable_unbound" // 交付物未绑定输出位
    RuleDeliverableReject  = "deliverable_rejected"// 上传被拒
)

type OpsIncident struct {
    ID         string       `json:"id" bson:"_id"`
    OrgID      string       `json:"org_id,omitempty" bson:"org_id,omitempty"` // 跟随资源推导，非巡检器身份
    UserID     string       `json:"user_id" bson:"user_id"`
    DedupeKey  string       `json:"dedupe_key" bson:"dedupe_key"`   // 主体+规则ID，唯一索引
    RuleID     string       `json:"rule_id" bson:"rule_id"`
    Status     string       `json:"status" bson:"status"`
    Severity   string       `json:"severity" bson:"severity"`       // warn | critical
    Title      string       `json:"title" bson:"title"`
    Summary    string       `json:"summary,omitempty" bson:"summary,omitempty"`       // 现象
    RootCause  string       `json:"root_cause,omitempty" bson:"root_cause,omitempty"` // LLM 归因
    AttrSource string       `json:"attr_source,omitempty" bson:"attr_source,omitempty"` // template | llm | none
    // 关联主体
    ProjectID  string       `json:"project_id,omitempty" bson:"project_id,omitempty"`
    TaskID     string       `json:"task_id,omitempty" bson:"task_id,omitempty"`
    TodoID     string       `json:"todo_id,omitempty" bson:"todo_id,omitempty"`
    AgentID    string       `json:"agent_id,omitempty" bson:"agent_id,omitempty"`
    NodeID     string       `json:"node_id,omitempty" bson:"node_id,omitempty"`
    // 干预计数
    GuideCount int          `json:"guide_count" bson:"guide_count"`
    Actions    []OpsAction  `json:"actions" bson:"actions"`
    EventIDs   []string     `json:"event_ids,omitempty" bson:"event_ids,omitempty"` // 关联事件（时间线）
    CreatedAt  time.Time    `json:"created_at" bson:"created_at"`
    UpdatedAt  time.Time    `json:"updated_at" bson:"updated_at"`
    ResolvedAt *time.Time   `json:"resolved_at,omitempty" bson:"resolved_at,omitempty"`
}

type OpsAction struct {
    ID         string         `json:"id" bson:"id"`
    At         time.Time      `json:"at" bson:"at"`
    Level      string         `json:"level" bson:"level"`            // L0 | L1
    Kind       string         `json:"kind" bson:"kind"`              // created | diagnosed | guided | escalated | resolved
    TemplateID string         `json:"template_id,omitempty" bson:"template_id,omitempty"`
    Target     string         `json:"target,omitempty" bson:"target,omitempty"` // 目标节点
    Content    string         `json:"content,omitempty" bson:"content,omitempty"` // 实际下发的指引全文
    Result     string         `json:"result" bson:"result"`          // sent | throttled | failed | skipped
    Detail     string         `json:"detail,omitempty" bson:"detail,omitempty"`
    Metadata   map[string]any `json:"metadata,omitempty" bson:"metadata,omitempty"`
}
```

### 3.2 Mongo 集合 `ops_incidents`

| 索引     | 字段                             | 用途                        |
| ------ | ------------------------------ | ------------------------- |
| 唯一     | `dedupe_key`                   | 去重硬保证（同一主体+规则仅一个 open 工单） |
| 复合     | `org_id + status + created_at` | 租户内列表与筛选                  |
| 复合     | `task_id + status`             | 任务维度反查                    |
| TTL 候选 | `resolved_at`                  | 二期定保留期，一期先不设              |

模式照抄现有集合：`mongoOpsIncidents` 字段 + `loadOpsIncidents()` + `persistOpsIncidentUnsafe()` + `ensureMongoIndexes()` 注册。

---

## 4. 阶段拆分

### 阶段 0：地基（无依赖，可独立验收）

| #   | 改动                                                      | 文件                                          | 风险            |
| --- | ------------------------------------------------------- | ------------------------------------------- | ------------- |
| 0.1 | 新增 Ops 配置项                                              | `config/config.go`                          | 低             |
| 0.2 | 新建 `OpsIncident` / `OpsAction` 模型与常量                    | `model/ops.go`（新建）                          | 低             |
| 0.3 | Mongo 集合 + 索引 + load/persist                            | `store/mongo_state.go`                      | 低             |
| 0.4 | 🔴 **修助手工具层租户泄漏**：签名 `userID string` → `sc store.Scope` | `assistant/tools.go`、`handler/assistant.go` | **中**（改签名波及面） |
| 0.5 | 前端助手对话请求带上 `X-Org-Id`                                   | `frontend-v2/src/api/*.ts`                  | 低             |

新增配置：

```go
OpsEnabled          bool          // 总开关，默认 false（未配 LLM 也能跑规则+模板）
OpsScanInterval     time.Duration // 巡检周期，默认 5 分钟
OpsSilentThreshold  time.Duration // 沉默型判定时长，默认 30 分钟
OpsGuideMaxPerTodo  int           // 每 todo 指导下发上限，默认 3
OpsGuideCooldown    time.Duration // 同一工单指导下发冷却，默认 15 分钟
OpsResolveObserve   time.Duration // 反向验证观察期，默认 10 分钟
OpsModel            string        // 归因模型，空则回落 AssistantModel
```

**验收**：编译通过；`ops_incidents` 索引建立；单测覆盖「租户 A 的助手工具查不到租户 B 数据」。

---


### 阶段 1：发现层（只读，不改动任何既有写路径）

| #   | 改动                                            | 文件                          | 风险         |
| --- | --------------------------------------------- | --------------------------- | ---------- |
| 1.1 | 规则引擎：4 条规则判定函数（纯函数，输入快照，输出命中列表）               | `store/ops_rules.go`（新建）    | 低          |
| 1.2 | 🔴 **按租户分片扫描**：先按 `org_id` 分组，聚合在片内完成         | `store/ops_scanner.go`（新建）  | **高**（泄漏点） |
| 1.3 | 工单创建 / 追加（dedupe_key 去重）                      | `store/ops_incident.go`（新建） | 中          |
| 1.4 | 启动巡检协程（复用 `StartTimeoutMonitor` 同款 ticker 模式） | `app/router.go`             | 低          |

规则判据（初值，均可配置）：

| 规则                     | 判据                                             | 信号源                         |
| ---------------------- | ---------------------------------------------- | --------------------------- |
| `todo_stalled`         | todo 处于 `in_progress` 且 `now - 最后状态变化 > 30min` | 现有 `checkTodoTimeouts` 同款字段 |
| `task_silent`          | 任务非终态且 `now - 最后一条事件 > 30min`（**抓沉默型**）        | `events` / `task.UpdatedAt` |
| `deliverable_unbound`  | 已上传但未绑定输出位                                     | webhook `unboundWarned`     |
| `deliverable_rejected` | 上传被拒（元数据缺失等）                                   | webhook `rejectedWarned`    |

> ⚠️ 1.2 是**最隐蔽的泄漏点**：`unboundWarned` / `rejectedWarned` 目前是 webhook 里的局部 map，提升到 store 层时必须按租户隔离存放，否则异常信号本身会串租户。

**验收**（可测）：

- 造一个卡住的 todo → 巡检后生成 1 个工单；连续 3 个巡检周期 → **仍只有 1 个工单**，actions 累加
- 造一个沉默任务（山雨矩阵同款：第 1 步完成不进第 2 步）→ `task_silent` 命中
- 租户 A 造异常 → 租户 B 的工单列表为空（**跨租户零泄漏硬指标**）

---


### 阶段 2：干预层（写路径，风险最高）

| #   | 改动                                                                  | 文件                               | 风险    |
| --- | ------------------------------------------------------------------- | -------------------------------- | ----- |
| 2.1 | 修复指引模板库（4 类故障 × 模板，含具体命令参数）                                         | `assistant/ops_templates.go`（新建） | 低     |
| 2.2 | `InterventionDispatcher`：唯一出口，接管 `remindHook` / `planningStallHook` | `store/ops_dispatch.go`（新建）      | **高** |
| 2.3 | LLM 归因接线（复用 `assistant.LLMClient`，强制结构化输出）                          | `assistant/ops_diagnose.go`（新建）  | 中     |
| 2.4 | 工单状态机 + 反向验证 + 观察期关闭                                                | `store/ops_incident.go`          | 中     |
| 2.5 | L0 通知写入 `notifications`                                             | 复用现有                             | 低     |

`InterventionDispatcher` 核心职责：

```go
type InterventionDispatcher struct {
    store    *store.Store
    publisher *clawsynapse.Client
    log      *zap.Logger
}

// Dispatch 是对执行体下发的唯一出口。
// 所有 remind / 运维指导 / 系统警告都必须经过它，以保证：
//   1. 节流与去重不会各算各的（避免刚发指导就被判 failed）
//   2. 每次下发都有 ops_action 留痕
func (d *InterventionDispatcher) Dispatch(ctx context.Context, req DispatchReq) DispatchResult

type DispatchReq struct {
    Level      string // L0 | L1
    RuleID     string
    TaskID     string
    TodoID     string
    Content    string // 修复指引全文
    TemplateID string
    IdempotKey string // 幂等键，用于去重
}
```

**改造点**：`app/router.go` 中 `s.SetRemindHook(...)` 与 `s.SetPlanningStallHook(...)` 改指到 dispatcher。`timeout_monitor.go` 本体**不改**（它只负责计时与判死）。

> 🔴 **必须处理的撞车**：运维 task 会被 `timeout_monitor` 管辖（10min remind / 3 次判 failed）。诊断类动作耗时不可控，需给运维 task 单独设 `MaxRetries` 与超时档位，否则会被误判失败。

**验收**：

- 同一工单 15 分钟内重复命中 → 第 2 次下发结果为 `throttled`，不重复唤醒 agent
- 指导下发 3 次仍无改善 → 工单转 `escalated`，**不再自动下发**
- 造"下发指导后 5 分钟任务恢复、8 分钟后又卡住" → 工单**不关闭**（观察期未过）
- 全链路 actions 留痕完整（谁、何时、发了什么、结果）

---

### 阶段 3：前端（依赖阶段 1、2）

| #   | 改动                                                                                                                        | 文件                                          |
| --- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| 3.1 | 运维工单 API：`GET /ops/incidents`、`GET /ops/incidents/:id`、`POST /ops/incidents/:id/resolve`、`POST /ops/incidents/:id/ignore` | `handler/ops.go`（新建）+ `app/router.go`       |
| 3.2 | 运维工单页：列表（按状态/规则筛选）+ 详情（根因、动作时间线、关联任务跳转）                                                                                   | `frontend-v2/src/pages/OpsPage.tsx`（新建）     |
| 3.3 | 侧边栏入口 + 未处理工单数量角标                                                                                                         | `frontend-v2/src/components/MainLayout.tsx` |

**验收**：能看到工单的状态、根因、每一次干预动作与结果；人工关闭 / 忽略生效。

---

### 阶段 4（二期，本期不做）

专属 ops 节点：走现有 `JoinRequest` 审批链路注册为特殊 role agent，挂载运维 skill，负责下探宿主取证（拉日志、查 `state.db`、查进程/磁盘）。业务节点保持无特权。

---

## 5. 之前 vs 之后

| 维度          | 之前                                            | 之后（一期）                             |
| ----------- | --------------------------------------------- | ---------------------------------- |
| 卡住的任务       | 只有 `todo.remind` 三连催，3 次后判 failed，**无根因、无记录** | 生成工单，记录现象 / 根因 / 每次干预与结果           |
| 沉默型卡死（山雨矩阵） | 完全无人发现，只能人工翻                                  | `task_silent` 规则主动命中               |
| 给执行体的指导     | 硬编码在 `notifySystemWarningToAgent` 里的一段文本      | 模板库（可审计可迭代）+ LLM 兜底                |
| 对执行体的下发     | remind / 系统警告 / 运维指导各自为政，计数独立                 | 统一 `InterventionDispatcher`，节流去重留痕 |
| 处理状态        | 无处可查                                          | `ops_incident` 状态机 + 动作时间线         |
| 租户隔离        | 助手工具层无 Scope（**泄漏**）                          | 全程 Scope，巡检按租户分片                   |

---

## 6. 风险与降级

| 风险                    | 影响                | 降级方案                                                       |
| --------------------- | ----------------- | ---------------------------------------------------------- |
| LLM 未配或不可用            | 无归因               | 规则照常发现、工单照常生成，`attr_source=none`，改用模板指引并标记需人工复核。**不会静默失效** |
| 巡检误报刷屏                | 工单噪音              | dedupe_key 去重 + 观察期 + 人工 ignore                            |
| 指导下发失控（agent 被反复唤醒）   | token 成本、agent 跑偏 | 冷却 15min + 每 todo 上限 3 次 + 超限转 escalated                   |
| 改造撞车（指导 vs remind 判死） | 指导白给              | 统一 dispatcher + 运维 task 单独超时档位                             |
| 阶段 0.4 改签名波及面         | 编译失败              | 先跑 `go build ./...`，用容器编译（本地无 Go 工具链）                      |

---

## 7. 执行约定（沿用既有铁律）

- 后端改完**必须 `docker-compose build backend` + 换容器**才生效（`data/` 是挂载，但 Go 代码改动要重建）
- 前端改完**必须 build + 换容器**；验证用 `docker exec grep` 命中中文文案指纹
- 改纯 CRLF 文件的多行替换须用 Python 整份补丁，改完 grep / 编译验证
- 冒烟不能刚 `up -d` 就打，等 healthy 后再等 6–8 秒；**容器重启后 JWT 失效，必须重新登录取 token**
- 提交按路径精确 `git add`，**禁用 `git add -A`**

---

## 8. 建议的推进节奏

| 批次    | 内容                      | 可独立验收            |
| ----- | ----------------------- | ---------------- |
| 第 1 批 | 阶段 0（0.1–0.3）+ 0.4 租户修复 | ✅ 编译 + 单测        |
| 第 2 批 | 阶段 1（发现层）               | ✅ 造异常看工单、验跨租户零泄漏 |
| 第 3 批 | 阶段 2（干预层）               | ✅ 指导下发留痕、节流与撞车验证 |
| 第 4 批 | 阶段 3（前端）                | ✅ 页面可见           |

每批之间跑一次完整冒烟，确认无回归再进下一批。
