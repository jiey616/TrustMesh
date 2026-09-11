# TrustMesh 智能体页改造方案（Hermes 品牌 + Gateway API 接入）

> 版本：v4（基于 github.com/jiey616/clawsynapse **最新 main 分支代码核实**，2026-07-13 更新）
> 结论先行：ClawSynapse 的 `HermesAdapter` **已经完全从 CLI 切换为 Hermes Gateway HTTP API**——`chat.*` → `POST /v1/responses`（有状态续接），`task.*`/`todo.*` → `POST /v1/runs`（创建+轮询）。配置（`hermesGatewayUrl`/`hermesGatewayKey`/`hermesModel`）、Gateway HTTP 客户端、消息路由、会话续接（`previous_response_id`/`session_id`）**均已在上游实现**。原 v3 方案 §3「ClawSynapse 侧补全 Gateway 逻辑」的大部分工作已被上游完成。本方案据此**大幅缩减 ClawSynapse 侧工作量**（仅剩「能力发现/管理类扩展 `agent.*`」），并把重心放在 **TrustMesh 侧的 gateway 服务 + 前端改造**。

---

## 0. 与 v3 方案的关键差异（重要）

| v3 假设 | v4 核实结果 | 影响 |
|---------|------------|------|
| HermesAdapter 只封装 `hermes chat -q` CLI，Gateway 逻辑未实现 | ❌ 已改为 Gateway HTTP API（`responses`/`runs`） | ClawSynapse 侧 Gateway 客户端、消息路由**已无需开发** |
| 需新增 `HermesGatewayURL/Key` 配置 | ❌ 配置已存在（含 env 支持） | 仅部署时填值即可 |
| 回传类型 = `chat.message.response` / `task.run.response` | ❌ 实际 = `chat.response` / `task.response`（`<前缀>.response`） | **webhook 分派逻辑必须修正**，否则收不到回复 |
| 对话走 CLI 阻塞式，可选切 Gateway | 已无 CLI，对话即 Gateway `responses` | 对话必然是 Gateway，无需「融合方式」决策 |
| `task.*` 仅指运行任务流，PM/Executor 自主流保持 CLI | 当前 `task.*`/`todo.*` **一律**走 `/v1/runs` | 需确认与现有 PM/Executor 协议关系（见 §9 决策点 3） |

---

## 1. 现状盘点（已核实）

### 1.1 ClawSynapse 侧（最新代码）

| 项 | 现状（已核实） | 缺口 |
|----|------|------|
| `HermesAdapter.DeliverMessage`（`internal/adapter/hermes.go`） | 按消息类型路由：非 `task`/`todo` 前缀 → `deliverViaResponses` → `POST /v1/responses`（stateful，`previous_response_id` 续接）；`task.*`/`todo.*` → `deliverViaRuns` → `POST /v1/runs` + `pollRun` 轮询到 terminal | 无 skills/jobs/sessions/capabilities/models 暴露；无 SSE 流式（`runs` 是轮询到完成） |
| `HermesConfig` | 已含 `BaseURL`(默认 `http://127.0.0.1:8642/v1`)、`APIKey`、`Model`(默认 `hermes-agent`)、`SessionStore`、`AgentRole` | — |
| Gateway HTTP 客户端 | 已实现（内联 `callJSON`：`Bearer` 鉴权、JSON 收发、`/v1/responses`、`/v1/runs`、`/v1/runs/{id}`、`/health`） | 仅覆盖 responses/runs/health，缺 skills/toolsets/capabilities/models/jobs/sessions/stop/approval |
| 配置（`config.go`） | `HermesGatewayURL`/`HermesGatewayKey`/`HermesModel` 均已存在，支持 env `HERMES_GATEWAY_URL`/`HERMES_GATEWAY_KEY`/`HERMES_MODEL`；`agent-adapter=hermes` 已支持 | — |
| 会话续接 | 复用 `store.SessionState`（hermes 命名空间），chat 用 `previous_response_id`、task 用 `session_id`；含 `isGatewayUnknownSessionError` 检测与重试 | — |
| 回复机制（`messaging/service.go`） | `replyToSender` → `Publish` 回 sender inbox（**NATS**，非 HTTP webhook）；回复类型 = `replyTypeFor(orig.Type)` = `<前缀>.response`（如 `chat.response`、`task.response`、`agent.response`） | 无 `.event` 流式回传 |
| 可投递前缀 | 默认 `chat,task,todo`；`isDeliverableType` 按前缀匹配 | **无 `agent` 前缀**——能力发现消息当前会被拒 |
| `AgentAdapter` 接口 | 仅 `DeliverMessage` + `GetStatus` | 无 `GatewayCapable` 能力接口 |

### 1.2 TrustMesh 侧

| 项 | 现状 | 缺口 |
|----|------|------|
| `clawsynapse.Client`（`backend/internal/clawsynapse/client.go`） | 纯 HTTP 客户端：`Publish`(POST `/v1/publish`)、`GetPeers`、`AuthChallenge` 等 | 无 NATS 订阅能力；回复靠 ClawSynapse 节点转 webhook |
| 接收回复 | `POST /webhook/clawsynapse` → `webhook.go` 按 `type` 分派 | 原 v3 把类型写成 `chat.message.response`，**实际是 `chat.response`**，需修正 |
| Agent 模型（`model/agent.go`） | 有 `NodeID` | 无 `Adapter`/`Product` 字段 |
| AgentChat | `POST /api/v1/agents/:id/chat/messages` → Publish `chat.message` → 回复经 webhook → 存库 → SSE | 回复类型需从 `chat.message` 改为 `chat.response` 匹配 |
| 前端 Agent 页 | `AgentDetailPage` 4 Tab，品牌 TrustMesh | 改 Hermes 品牌；缺技能/定时任务/运行 Tab |

### 1.3 Hermes Gateway API（运行在 Agent 节点 `127.0.0.1:8642`，需 `API_SERVER_KEY`）

- **已被 ClawSynapse 调用**：`POST /v1/responses`、`POST /v1/runs`、`GET /v1/runs/{id}`、`GET /health`
- **尚未被 ClawSynapse 调用**：`POST /v1/chat/completions`、`GET /v1/models`、`GET /v1/capabilities`、`GET /v1/skills`、`GET /v1/toolsets`、`GET /v1/runs/{id}/events`(SSE)、`POST /v1/runs/{id}/stop`、`POST /v1/runs/{id}/approval`、`GET/POST /api/jobs`、`GET/POST /api/sessions`、`POST /api/sessions/{id}/fork`、`POST /api/sessions/{id}/chat/stream`(SSE)

---

## 2. 网络与传输模型（修正后）

```
┌──────────────────────────┐        NATS publish          ┌──────────────────────────────┐
│  TrustMesh 后端           │  type="chat.message" /       │  ClawSynapse 节点 (无公网IP)  │
│  clawsynapse.Client       │        "task.run.create" /   │  订阅 clawsynapse.msg.<id>.inbox│
│  .Publish(...)            │        "agent.skills" ...    │                              │
│                           │ ───────────────────────────▶ │  Adapter (按消息类型路由)      │
│                           │                              │   ├─ chat.*  → POST /v1/responses│
│                           │                              │   ├─ task.*/todo.* → POST /v1/runs │
│                           │                              │   └─ agent.* → 🆕 能力 API (待扩)│
│                           │   NATS publish 回 sender     │                              │
│  (TrustMesh 侧           │ ◀─────────────────────────── │  replyToSender → Publish 回    │
│   clawsynapse 节点收)     │   type="chat.response" /     │   sender inbox (TrustMesh 节点) │
│                           │        "task.response" /     │                              │
│                           │        "agent.response"      │                              │
└───────────┬──────────────┘                              └──────────────────────────────┘
            │
            │  TrustMesh 侧 clawsynapse 节点收到回复 → 转 POST /webhook/clawsynapse（既有机制）
            ▼
┌──────────────────────────┐
│  TrustMesh backend webhook.go │ 按 <前缀>.response 分派 → 关联 SessionKey → 推用户 SSE
└──────────────────────────┘
```

**铁律（已核实）**：
- 出站只用 `clawsynapse.Client.Publish`（与现有 `chat.message` 一致）。消息类型：`chat.*`（对话）、`task.*`/`todo.*`（运行/任务流）、`agent.*`（能力发现/管理，**待 ClawSynapse 扩展**）。
- 入站回传：ClawSynapse 回复是 **NATS publish 回 sender（TrustMesh）inbox**（`replyToSender`），由 TrustMesh 侧 clawsynapse 节点收到后转 `POST /webhook/clawsynapse`（既有机制）。**不是 ClawSynapse 直推 HTTP webhook**。
- 回复类型 = `<前缀>.response`（由 `replyTypeFor` 生成）：`chat.message` → `chat.response`；`task.run.create` → `task.response`；`agent.skills` → `agent.response`。**绝不要写成 `chat.message.response`**（原 v3 错误）。
- 关联键统一用 `SessionKey`：Publish 时生成/传递，ClawSynapse `replyToSender` 原样带回，webhook 按此关联。
- 消息类型产品无关：`chat.*`/`task.*`/`todo.*`/`agent.*` 不出现 `hermes` 字样，具体网关调用由 ClawSynapse adapter 决定。

---

## 3. ClawSynapse 侧改造（仅剩能力扩展）

### 3.1 已完成（上游已实现，P0 无需改动）
- ✅ Gateway HTTP 客户端（`callJSON` + Bearer 鉴权）
- ✅ `chat.*` → `/v1/responses`（stateful 续接）
- ✅ `task.*`/`todo.*` → `/v1/runs`（创建 + 轮询）
- ✅ 配置 `HermesGatewayURL/Key/Model`
- ✅ 会话续接（`store.SessionState` + unknown session 重试）
- ✅ `GetStatus` → `/health`

### 3.2 仍需做：扩展 `HermesAdapter` 暴露 Gateway 能力（新增方法，不改 `DeliverMessage`）

建议新增 `GatewayCapable` 接口（或直接在 adapter 上加方法），当前仅 `HermesAdapter` 实现：
```go
type GatewayCapable interface {
    // 能力发现
    ListSkills(ctx) ([]Skill, error)        // GET /v1/skills
    ListToolsets(ctx) ([]Toolset, error)    // GET /v1/toolsets
    GetCapabilities(ctx) (*Capabilities, error) // GET /v1/capabilities
    ListModels(ctx) ([]Model, error)        // GET /v1/models
    // 定时任务 (Jobs)
    ListJobs/CreateJob/GetJob/UpdateJob/DeleteJob/PauseJob/ResumeJob/RunJob(...)
    // 会话管理 (Sessions)
    ListSessions/GetSession/CreateSession/ForkSession(...)
    // 运行控制
    StopRun(ctx, runID) error               // POST /v1/runs/{id}/stop
    ApproveRun(ctx, runID, ok bool) error   // POST /v1/runs/{id}/approval
}
```
- 同步类（list/get）直接返回结果；流式类（runs events / session chat stream）**当前 ClawSynapse 是同步模型**，建议 P2 再支持。

### 3.3 新增 `agent.*` 消息路由

- `DeliverablePrefixes` 配置增加 `agent`（默认 `chat,task,todo,agent`）。
- `AdapterMessageHandler` 或 `messaging.Service` 增加 `agent.*` 分派：收到 `agent.skills` → 调 `adapter.(GatewayCapable).ListSkills` → 结果经 `replyToSender` 回 `agent.response`（自动 `<前缀>.response`）。
- 消息词汇表（通用，产品无关）：

| TrustMesh 发 → | 语义 | ClawSynapse 处理 | 回传（NATS）→ |
|----------------|------|------------------|---------------|
| `chat.message` | 对话（页面/自主流共用） | `DeliverMessage` → `/v1/responses` | `chat.response` |
| `task.run.create` | 创建运行 | `DeliverMessage` → `/v1/runs` | `task.response`(run_id) |
| `todo.*` | 待办 | `DeliverMessage` → `/v1/runs` | `task.response` |
| `agent.skills` | 技能列表 | 🆕 `ListSkills` | `agent.response` |
| `agent.toolsets` | 工具集 | 🆕 `ListToolsets` | `agent.response` |
| `agent.capabilities` | 能力 | 🆕 `GetCapabilities` | `agent.response` |
| `agent.models` | 模型 | 🆕 `ListModels` | `agent.response` |
| `agent.jobs.list` 等 | 定时任务 | 🆕 Jobs 方法 | `agent.response` |
| `agent.sessions.list`/`.fork` | 会话管理 | 🆕 Sessions 方法 | `agent.response` |

> **扩展性**：接入 OpenClaw 等新产品时，只需新增 adapter 实现 `GatewayCapable`，**TrustMesh 侧消息类型、REST 端点、前端零改动**。

---

## 4. TrustMesh 侧改造

### 4.1 Agent 模型加字段（`model/agent.go`）
```go
Adapter       string `json:"adapter" bson:"adapter"`         // "hermes" | "default" | "openclaw" ...
Product       string `json:"product" bson:"product"`         // "hermes" | "trustmesh"（品牌标识）
GatewayEnabled bool   `json:"gateway_enabled" bson:"gateway_enabled"` // 支持 Gateway API
```
- 创建/JoinRequest 同步时落库（复用 `JoinRequest.AgentProduct`）。

### 4.2 `clawsynapse.Client` 无需改
`Publish(targetNode, type, message, sessionKey, metadata)` 通用，直接发 `chat.*`/`task.*`/`agent.*`。

### 4.3 `webhook.go` 修正回复类型分派（⚠️ 关键修正）
- 原 v3 误用 `chat.message.response` / `task.run.response`；**实际 ClawSynapse 回传是 `chat.response` / `task.response` / `agent.response`**（`<前缀>.response`）。
- `chat.response` → 复用 `handleChatMessage` 追加到 AgentChat（实现「对话」Tab）。
- `task.response` → 运行结果落库 / 推 SSE。
- `agent.response` → `gatewayService.Dispatch(sessionKey, payload)` 推 SSE（技能/定时任务/会话/能力等）。

### 4.4 新增 `gateway` 服务（`backend/internal/gateway/`）
```
GatewayService:
  - 内存映射 sessionKey → 等待中的 SSE channel
  - Request(agent, op, payload) (reqID, error): 生成/复用 SessionKey，Publish <op>，登记映射
  - Dispatch(sessionKey, payload): webhook 回传（<前缀>.response）时查映射，推对应 channel / 用户 SSE
  - REST handlers（见 4.5）
```

### 4.5 后端 REST 端点（新增，`/api/v1/agents/:id/gateway/...`）
| 端点 | 作用 | 对应消息类型 |
|------|------|--------------|
| `POST /chat` | 发起/继续对话 | `chat.message` |
| `GET  /skills` | 技能列表 | `agent.skills` |
| `GET  /toolsets` | 工具集 | `agent.toolsets` |
| `GET  /capabilities` | 能力/模型 | `agent.capabilities` / `agent.models` |
| `GET/POST /jobs` `.../jobs/:jid` `.../jobs/:jid/pause\|resume\|run` | 定时任务 | `agent.jobs.*` |
| `POST /runs` `GET /runs/:rid` `POST /runs/:rid/stop` `POST /runs/:rid/approval` | 运行 | `task.run.*` |
| `GET/POST /sessions` `.../sessions/:sid` `.../sessions/:sid/fork` | 会话管理 | `agent.sessions.*` |

> 端点路径用 `/gateway/` 保持通用，换产品不变。

### 4.6 前端 SSE 流式
- 复用 `/events/stream` 用户级 SSE，新增事件类型 `task.response`、`agent.response`、`chat.response` 等，前端 reducer 处理（与现有 `agent_chat.updated` 同机制）。

---

## 5. 前端 Agent 页改造（Hermes 品牌 + 功能）

### 5.1 品牌改造
- `AgentDetailPage` 头部、Logo、空状态 → 统一 "Hermes Agent"（读 `agent.product`）。
- 平台级（登录页/标题）保持 TrustMesh。
- UI 不写死 hermes，读 `agent.product`。

### 5.2 Tab 结构（4 → 7）
| Tab | 数据来源 | 融合/新增 |
|-----|----------|-----------|
| 概览 | `agent.capabilities`+`agent.models`+`agent.skills`+`agent.toolsets` | 🆕 |
| 对话 | `chat.message` → `chat.response`（复用现有 AgentChat 存储/UI） | 🔄 融合（ClawSynapse 已支持） |
| 技能 | `agent.skills` 列表 | 🆕 |
| 定时任务 | `agent.jobs.*` CRUD | 🆕 |
| 运行 | `task.run.*` 列表 + stop/approve | 🆕 |
| 工作记录 | 现有 task 流 | 🔄 保留 |
| 活动日志 | 现有 | 🔄 保留 |

---

## 6. 功能映射总表（Hermes API → 通用消息 → 端点 → UI）

| Hermes Gateway API | 通用消息类型 | ClawSynapse 现状 | TrustMesh 端点 | UI Tab |
|--------------------|--------------|------------------|----------------|--------|
| `POST /v1/responses` | `chat.message` | ✅ 已实现 | `POST /agents/:id/gateway/chat` | 对话 |
| `POST /v1/runs` (+verbs) | `task.run.*` | ✅ 已实现（创建+轮询） | `/agents/:id/gateway/runs*` | 运行 |
| `GET /v1/skills` | `agent.skills` | 🆕 待扩 | `GET /agents/:id/gateway/skills` | 技能 |
| `GET /v1/toolsets` | `agent.toolsets` | 🆕 待扩 | `GET /agents/:id/gateway/toolsets` | 概览 |
| `GET /v1/capabilities` `/v1/models` | `agent.capabilities`/`agent.models` | 🆕 待扩 | `GET /agents/:id/gateway/capabilities` | 概览 |
| `GET/POST /api/jobs` (+verbs) | `agent.jobs.*` | 🆕 待扩 | `/agents/:id/gateway/jobs*` | 定时任务 |
| `GET/POST /api/sessions` (+fork) | `agent.sessions.*` | 🆕 待扩 | `/agents/:id/gateway/sessions*` | 对话/概览 |

---

## 7. 融合 vs 新增分析

| Hermes 能力 | 策略 | 说明 |
|-------------|------|------|
| 对话 Chat（`/v1/responses`） | 🔄 融合（ClawSynapse 已支持） | 复用 AgentChat store + SSE；消息 `chat.message`，回复 `chat.response`；P0 即可闭环 |
| 运行 Runs（`/v1/runs`） | 🔄 已实现（创建+轮询） | 回复 `task.response`；stop/approve 待 ClawSynapse 扩 |
| Skills | 🆕 新增 | 需 ClawSynapse `ListSkills` + `agent.skills` 路由 |
| Jobs 定时任务 | 🆕 新增 | 需 ClawSynapse Jobs 方法 + `agent.jobs.*` 路由 |
| Sessions 会话 | 🆕 新增 | 需 ClawSynapse Sessions 方法 + `agent.sessions.*` 路由；fork 新增 |
| Capabilities/Models | 🆕 新增 | 概览 Tab；需 ClawSynapse 暴露 |

> **注意**：ClawSynapse 当前对 `task.*`/`todo.*` **一律**走 `/v1/runs`（含 PM/Executor 自主流）。这与「自主任务流保持不变」的旧假设不同，需确认（见 §9 决策点 3）。

---

## 8. 实施分期（修订）

### P0 — 品牌 + 对话闭环（ClawSynapse 零改动）
1. **ClawSynapse（部署配置）**：确保运行版本含 Gateway 实现，配置 `agent-adapter=hermes` + `hermes-gateway-url` + `hermes-gateway-key` + `hermes-model`。无需改代码。
2. **TrustMesh**：Agent 模型加 `Adapter/Product/GatewayEnabled`；`webhook.go` 修正为接收 `chat.response`（非 `chat.message.response`）；前端 Agent 页品牌改 Hermes + 对话 Tab 验证闭环。
3. **验证**：Agent 页发消息 → ClawSynapse 调 `/v1/responses` → 回 `chat.response` → 前端回显。

### P1 — 技能 / 定时任务 / 运行 / 会话（ClawSynapse 能力扩展 + TrustMesh 新 Tab）
4. **ClawSynapse**：`HermesAdapter` 新增能力方法（`GatewayCapable`）+ `agent.*` 消息路由 + `DeliverablePrefixes` 加 `agent`。
5. **TrustMesh**：`gateway` 服务 + REST（skills/jobs/runs/sessions/capabilities）+ `webhook.go` 处理 `agent.response` / `task.response`。
6. **前端**：新增 技能/定时任务/运行/概览 Tab。

### P2 — 增强
7. Runs 的 SSE 流式 events + stop/approve（需 ClawSynapse 改同步模型）；Sessions fork + chat/stream；多 Agent 协同。
8. 错误/超时/重连加固；`SessionKey` 关联可靠性。

---

## 9. 待确认决策点

1. **回传通道（已澄清）**：ClawSynapse 回复是 **NATS publish 回 sender（TrustMesh）inbox**（`replyToSender`），由 TrustMesh 侧 clawsynapse 节点转 `POST /webhook/clawsynapse`（既有机制）。回复类型 = `<前缀>.response`（如 `chat.response`）。✅ 无需新 webhook 配置，但 **必须修正原 v3 误写的 `chat.message.response`**。
2. **对话是否要 SSE 流式**：当前 ClawSynapse 同步 `responses`（等完整文本再回 `chat.response`），无打字机效果。选项 A：接受同步（P0 即可用，无流式）；选项 B：要求 ClawSynapse 改 `/v1/responses stream:true` + 逐段回传 `chat.event`（需 ClawSynapse 扩展，P2）。**推荐 A 起步**。
3. **`task.*`/`todo.*` 统一走 `/v1/runs` 的影响**：当前 ClawSynapse 把所有 `task.*`/`todo.*` 路由到 runs（含 PM/Executor 自主流 `task.message`/`todo.assigned`）。需确认这是否与现有 PM/Executor 协议冲突，还是本就期望 Hermes 用 runs 执行任务。
4. **Agent 模型字段**：`Adapter`/`Product` 命名与落库时机（创建 or JoinRequest 同步）。
5. **认证**：`API_SERVER_KEY` 存 ClawSynapse 配置（每节点）即可，推荐仅 ClawSynapse 持有（Gateway 在 Agent 节点本地）。
6. **品牌范围**：仅智能体标识改 Hermes，平台级保持 TrustMesh（品牌读 `agent.product`，不写死）。

---

## 10. 风险与注意

- **回复类型命名（致命）**：必须用 `<前缀>.response`（`chat.response` / `task.response` / `agent.response`），**不要用原 v3 误写的 `chat.message.response`**，否则 webhook 分派收不到回复。
- **`task/todo → runs` 对自主流的影响**（见决策点 3）：PM/Executor 协议可能已被改变，上线前需回归测试。
- **非流式（同步）限制**：ClawSynapse 当前是同步响应（runs 轮询到完成），对话无打字机、运行无实时事件流，P2 才支持流式。
- **配置缺失**：部署 ClawSynapse 必须设 `agent-adapter=hermes` + `hermes-gateway-*` 三件套，否则走 default adapter（不调 Gateway）。
- **扩展性**：消息类型与 REST 路径保持产品无关（`/gateway/`、通用 op），新增 adapter 时 TrustMesh 零改动——这是相对 v2（`hermes.*` 前缀）的核心改进。
- **会话连续性**：ClawSynapse 用 `SessionKey` 做 `previous_response_id` 续接；TrustMesh 发 `chat.message` 时**必须传稳定 SessionKey**（同一对话不变），否则每次都开新会话。
