# TrustMesh 智能体产品标识与 Hermes 能力展示/配置 方案

> 整理自 2026-07-30 的需求澄清对话（grill-me）。结论：**需求可实现**，但 Hermes 能力数据的契约依赖 ClawSynapse hermes 适配器侧暴露端点，需先对齐。

## 一、需求结论

| 维度 | 结论 |
|------|------|
| 产品标识展示 | 可实现（低风险） |
| Hermes 技能/模型展示 | 可实现，依赖适配器暴露 `GET /v1/skills`、`/v1/models` |
| Hermes 技能/模型配置（写回） | 可实现，额外依赖适配器暴露写端点 |
| 整体可行性 | ✅ 可以做，唯一外部风险是 hermes 适配器 API 契约未定义 |

## 二、已对齐的决策树（A–E）

| 分支 | 决策 | 说明 |
|------|------|------|
| **A 数据来源** | 审批 JoinRequest 时同步 `AgentProduct` → `Agent.Product` | 复用既有「加入申请带产品标识」机制；取值采用 JoinRequest 中填的值，**不预设白名单**，为空则默认 `"trustmesh"` |
| **B 展示范围** | 仅详情页徽章 | 列表与筛选不动，最小侵入 |
| **C 数据通道** | TrustMesh 主动拉取 | 给 ClawSynapse 客户端新增 `GET /v1/skills`、`/v1/models`，查看 hermes 详情时实时拉；拉取失败/超时/404 优雅降级为「能力信息暂不可用」 |
| **D 配置语义** | 写回适配器 | 用户在前端启用/禁用技能、选默认模型，TrustMesh 调 hermes 适配器写接口回写 |
| **E 前端形态** | 新增「Hermes 能力」Tab | 仅当 `product==hermes` 时显示，与现有 4 个 Tab（概览/对话/工作记录/活动日志）并列 |

## 三、现状与缺口

| 项 | 现状 | 缺口 |
|----|------|------|
| `Agent` 模型 | `backend/internal/model/agent.go` **无** `Product` 字段 | 需新增 `Product string` |
| JoinRequest 采集 | `model/join_request.go:14` 已有 `AgentProduct` | 审批通过后未同步到 Agent |
| 审批落库点 | `store/store_join_request.go:175` `ApproveJoinRequest(...)` 返回 `*model.Agent` | 在此把 `AgentProduct` 写入 `Agent.Product` |
| ClawSynapse 客户端 | `client.go` 仅有 `/v1/publish`、`/v1/peers`、`/v1/trust/*` 等 | 缺 `/v1/skills`、`/v1/models` 及写回端点 |
| webhook 分派 | `webhook.go` 的 switch 无 `agent.*` | 本方案用**拉取**模型，无需新增 webhook 分派（降低风险） |
| 前端详情页 | `AgentDetailPage.tsx` 仅 4 个 Tab | 需加产品徽章 + 条件渲染 Hermes 能力 Tab |

> 注：`clawsynapse/trust_sync.go:116` 也会从 peer profile 读 `AgentProduct`，属信任同步路径；本方案按用户选择以 **JoinRequest 审批** 为唯一来源。

## 四、实现方案

### 4.1 后端

1. **模型字段**（`model/agent.go`）
   - 新增 `Product string \`json:"product" bson:"product"\``，默认 `"trustmesh"`。

2. **审批同步**（`store/store_join_request.go` `ApproveJoinRequest`）
   - 创建/激活 Agent 时：`agent.Product = strings.TrimSpace(in.AgentProduct)`；为空则 `"trustmesh"`。

3. **ClawSynapse 客户端能力端点**（`clawsynapse/client.go`）
   - `GetSkills(ctx, nodeID) ([]Skill, error)` → `GET /v1/skills?node=<nodeID>`
   - `GetModels(ctx, nodeID) ([]Model, error)` → `GET /v1/models?node=<nodeID>`
   - 写回（契约待定）：`SetSkillEnabled(ctx, nodeID, skillID, enabled)`、`SetDefaultModel(ctx, nodeID, modelID)` → `POST /v1/skills` / `POST /v1/models`（路径与 body 待 hermes 适配器契约）
   - 统一错误处理：非 200 / 超时 → 返回明确错误，供 handler 降级。

4. **新增 HTTP 接口**（`handler` + `app/router.go`）
   - `GET /api/agents/:id/capabilities`：仅对 `product==hermes` 的 Agent 调 ClawSynapse 拉取 skills+models；非 hermes 返回空或 404。
   - `POST /api/agents/:id/skills/:skillId`（`{enabled:bool}`）：写回适配器。
   - `POST /api/agents/:id/model`（`{modelId}`）：设默认模型，写回适配器。

5. **降级与缓存**：拉取失败时接口返回 `available:false` + 原因；可选加短时缓存（TTL 数十秒）减轻适配器压力。

### 4.2 前端（`frontend/src/pages/AgentDetailPage.tsx`）

1. **产品徽章**：详情页头部新增徽章组件，读 `agent.product`，映射显示名（如 `hermes`→"Hermes"、`trustmesh`→"TrustMesh"）。
2. **条件 Tab**：`product==="hermes"` 时插入第 5 个 Tab「Hermes 能力」。
3. **Tab 内容**：
   - 技能列表：名称、描述、启用开关（toggle）→ 调写回接口。
   - 模型列表：单选默认模型 → 写回接口。
   - 加载/错误态：拉取中 spinner；失败显示「能力信息暂不可用」+重试。

## 五、需 hermes 适配器 owner 确认的契约（关键阻塞项）

1. ClawSynapse 节点是否暴露 `GET /v1/skills`、`GET /v1/models`？**响应 schema**（字段名、分页、节点作用域）。
2. 写回端点：启用/禁用技能、设默认模型的 **HTTP 方法与路径、请求体、鉴权方式**。
3. 节点作用域：这些端点是按 `nodeID` 区分，还是当前节点全局？
4. 鉴权：是否需要额外 token / 走现有 ClawSynapse 信任链。

> 以上任一项未定义，C/D 分支都只是假设，必须先与适配器侧对齐再动手。

## 六、风险与建议

- **R1（高）**：hermes 适配器契约未定义 —— 建议先由适配器 owner 给出端点草案，TrustMesh 侧按契约实现客户端与接口。
- **R2（低）**：拉取实时性 —— 实时拉保证新鲜，但适配器抖动会影响详情页；用降级 + 可选缓存缓解。
- **R3（低）**：写回一致性 —— 写回后建议立即重新拉取一次以刷新 UI，避免本地状态与适配器不一致。
- **建议**：先交付「只读展示 + 产品徽章」（A+B+C 只读部分），把「写回」（D）作为第二阶段，待契约明确后再做，降低被上游阻塞的范围。
