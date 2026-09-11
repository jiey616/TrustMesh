---
title: 节点能力查询与写回 — 研发设计文档
---

# 节点能力查询与写回 — 研发设计文档

## 0. 文档定位

本文档是 `capability-contract.md`（草案 v2）在 **TrustMesh 侧**的研发落地设计，目标是将契约转化为可执行的代码级任务。

> **重要修正**：本文档替代上一轮《TrustMesh_Hermes 能力展示方案》中 C 分支（数据通道）与 D 分支（配置语义）的结论。上一轮假设"TrustMesh 直连节点 `/v1/skills`、`/v1/models`"在真实拓扑下不成立——云端 TrustMesh 访问不到内网 hermes 节点的本地 API，必须经 NATS 网格 `capability.*` 穿透。详见 §2。



---

## 1. 背景与目标

### 1.1 需求

TrustMesh 希望在 Agent 详情页为 hermes 节点展示「产品徽章 + 能力 Tab」，并支持在云端对节点的**技能 / 模型 / 定时任务（cron）**&#x8FDB;行查询与写回。

### 1.2 能力范围（三类 CRUD）

| target  | 读来源                              | 写机制                                                     | 重启 gateway |
| ------- | -------------------------------- | ------------------------------------------------------- | ---------- |
| `skill` | gateway `GET /v1/skills`         | 改 `config.yaml` `skills.external_dirs`（managed 键）+ 托管目录 | 是          |
| `model` | `config.yaml` `custom_providers` | 改 `config.yaml` `custom_providers` + `model` 默认         | 是          |
| `cron`  | gateway `GET /api/jobs`          | 代理 gateway 原生 `/api/jobs` 端点                            | 否          |

> 经 hermes 0.16.0 源码核查：gateway 无 `admin_config_rw` 能力（仅只读 `/v1/skills`、`/v1/models`、`/v1/capabilities`），也无热重载。skill/model 写回必须改 `config.yaml` 并重启 gateway。cron 是 gateway 原生 CRUD，无需重启、风险更低。

### 1.3 目标

-TrustMesh 侧以最小改动接入契约定义的旁挂 daemon 端点。

- 前端在 hermes 节点详情页提供只读展示 + 写回操作 UI。
- 读写失败优雅降级，不影响节点在线状态与既有功能。

---

## 2. 架构与网络拓扑（关键修正）

### 2.1 拓扑事实

```
┌─────────────────────────────────────────────────────────────────┐
│  云端（TrustMesh）                                                │
│                                                                  │
│  ┌──────────────┐   HTTP    ┌─────────────────────┐              │
│  │ TrustMesh    │──────────▶│ 旁挂 daemon          │              │
│  │ backend      │  本地直连 │ (trustmesh-server    │              │
│  │ (Go/Gin)     │           │  clawsynapse 容器)   │              │
│  └──────────────┘           └─────────┬───────────┘              │
│                                       │                          │
└───────────────────────────────────────┼──────────────────────────┘
                                        │ NATS 网格
                                        │ capability.* 消息
                   ┌────────────────────┼────────────────────┐
                   │                    │                    │
            ┌──────▼──────┐      ┌──────▼──────┐      ┌──────▼──────┐
            │ hermes 节点 A │      │ hermes 节点 B │      │  ...更多节点 │
            │ (内网)        │      │ (内网)        │      │              │
            │ ┌───────────┐ │      │ ┌───────────┐ │      │              │
            │ │ gateway   │ │      │ │ gateway   │ │      │              │
            │ │ :8642     │ │      │ │ :8642     │ │      │              │
            │ │ config.yaml│ │      │ │ config.yaml│ │      │              │
            │ └───────────┘ │      │ └───────────┘ │      │              │
            └───────────────┘      └───────────────┘      └──────────────┘
```

### 2.2 关键修正点

| 项       | 上一轮方案（错误）                                | 本设计（正确）                                                                                             |
| ------- | ---------------------------------------- | --------------------------------------------------------------------------------------------------- |
| 读通道     | TrustMesh 直连节点 `/v1/skills`、`/v1/models` | TrustMesh 调旁挂 daemon `GET /v1/peers/{nodeId}/capabilities`，daemon 经 NATS `capability.query` 穿透到目标节点 |
| 写通道     | "写回适配器"（未细化）                             | TrustMesh 调旁挂 daemon `POST /v1/peers/{nodeId}/capabilities`，daemon 经 NATS `capability.set` 到目标节点    |
| webhook | 以为要加 `agent.*`/`capability.*` 分派         | **不动**——response/set_response 经 daemon 同步 HTTP 返回，不走 webhook                                        |

### 2.3 旁挂 daemon 的角色

TrustMesh 后端只与**自己的旁挂 daemon**（`trustmesh-server` clawsynapse 容器）通信，后者是 ClawSynapse 网格中的一个枢纽节点。daemon 负责：

1. 接收 TrustMesh 的本地 HTTP 请求；
2. 将请求翻译为 NATS `capability.*` 消息发往目标节点；
3. 等待目标节点的 `capability.response` / `capability.set_response`（5s 超时）；
4. 将结果同步返回 TrustMesh。

> 这与 TrustMesh 现有 `clawsynapse/client.go` 的模式完全一致——client 已通过 `baseURL` 调 daemon 的 `/v1/publish`、`/v1/peers`、`/v1/trust/*`、`/v1/transfers`、`/v1/health`。新增的 `/v1/peers/{nodeId}/capabilities` 是同构扩展。

---

## 3. 协议设计（capability.* 模块）

### 3.1 Subject 与 messageType

新增模块 `capability`，遵循 `clawsynapse.<module>.<scope>.<action>` 规范：

| 类别   | Subject                                              | messageType               | 说明           |
| ---- | ---------------------------------------------------- | ------------------------- | ------------ |
| 能力查询 | `clawsynapse.capability.<targetNodeId>.query`        | `capability.query`        | 查询目标节点能力     |
| 能力查询 | `clawsynapse.capability.<targetNodeId>.response`     | `capability.response`     | 返回能力清单       |
| 能力写回 | `clawsynapse.capability.<targetNodeId>.set`          | `capability.set`          | 写回技能/模型/cron |
| 能力写回 | `clawsynapse.capability.<targetNodeId>.set_response` | `capability.set_response` | 写回结果         |

### 3.2 公共封套

复用 `docs/protocol.md` 公共消息封套：

| 字段                | 类型       | 必填 | 说明                                                                     |
| ----------------- | -------- | -- | ---------------------------------------------------------------------- |
| `messageId`       | `string` | 是  | 消息唯一 ID                                                                |
| `messageType`     | `string` | 是  | 见上表                                                                    |
| `from`            | `string` | 是  | 发起方 nodeId                                                             |
| `to`              | `string` | 是  | 目标 nodeId                                                              |
| `requestId`       | `string` | 是  | 关联 query/set 与 response                                                |
| `ts`              | `number` | 是  | Unix 毫秒时间戳                                                             |
| `signature`       | `string` | 是  | Ed25519 签名（`messageType`+`subject`+`from`+`to`+`ts`+`sha256(payload)`） |
| `protocolVersion` | `string` | 否  | 默认 `v1`                                                                |

所有 `capability.*` 消息参与签名；`set` / `set_response` 须带签名，沿用现有签名与重放保护。

### 3.3 `capability.query` 请求

```json
{
  "requestId": "req-uuid",
  "nodeId": "opc-founder-001"
}
```

### 3.4 `capability.response` 响应

```json
{
  "requestId": "req-uuid",
  "product": "hermes",
  "available": true,
  "skills": [
    { "name": "tm-task-plan", "description": "...", "category": "task" }
  ],
  "models": [
    { "id": "provider-1", "provider": "openai", "model": "gpt-4o", "isDefault": true }
  ],
  "jobs": [
    { "id": "job-1", "name": "daily-report", "schedule": "0 9 * * *", "enabled": true, "prompt": "...", "skills": ["tm-task-exec"], "nextRun": "2026-08-04T09:00:00Z" }
  ],
  "reason": ""
}
```

- 非 hermes 适配器：`available:false`，`skills`/`models`/`jobs` 为空。
- gateway 不可达：`available:false` + `reason`（如 `"gateway unreachable"`）。
- **`models` 不回显 `api_key`**。

### 3.5 `capability.set` 请求

```json
{
  "requestId": "req-uuid",
  "target": "skill",
  "action": "add",
  "skill": "my-custom-skill",
  "fileIds": ["file-abc123"]
}
```

字段说明：

| 字段          | 类型         | 必填                                           | 说明                                                      |
| ----------- | ---------- | -------------------------------------------- | ------------------------------------------------------- |
| `requestId` | `string`   | 是                                            | 请求关联 ID                                                 |
| `target`    | `string`   | 是                                            | `skill` | `model` | `cron`                              |
| `action`    | `string`   | 是                                            | 见 §3.6 动作映射                                             |
| `skill`     | `string`   | target=skill                                 | 技能名                                                     |
| `fileIds`   | `string[]` | target=skill 的 add/update                    | 已传到节点的 `fileId`                                         |
| `model`     | `string`   | target=model                                 | 模型/provider 标识                                          |
| `provider`  | `object`   | target=model 的 add                           | `{api_mode, transport, model, default_model, api_key?}` |
| `job`       | `object`   | target=cron 的 create/update                  | cron job 配置                                             |
| `jobId`     | `string`   | target=cron 的 update/delete/pause/resume/run | 目标 job ID                                               |

### 3.6 动作映射

| target  | action    | 语义          | 实现                                           |
| ------- | --------- | ----------- | -------------------------------------------- |
| `skill` | `add`     | 部署新技能       | `fileId` → 托管目录；注册 `external_dirs` managed 键 |
| `skill` | `update`  | 覆盖技能内容      | `fileId` → 覆盖托管目录文件                          |
| `skill` | `enable`  | 启用技能        | 注册 `external_dirs` managed 键                 |
| `skill` | `disable` | 停用技能        | 注销 managed 键（保留文件，可恢复）                       |
| `model` | `add`     | 新增 provider | 写 `custom_providers` 条目                      |
| `model` | `switch`  | 设默认模型       | 写 `config.model`                             |
| `model` | `delete`  | 删除 provider | 删 `custom_providers` 条目（删当前默认在校验阶段拦截）        |
| `cron`  | `create`  | 新建定时任务      | 代理 `POST /api/jobs`                          |
| `cron`  | `update`  | 改定时任务       | 代理 `PATCH /api/jobs/{id}`                    |
| `cron`  | `delete`  | 删除定时任务      | 代理 `DELETE /api/jobs/{id}`                   |
| `cron`  | `pause`   | 暂停          | 代理 `POST /api/jobs/{id}/pause`               |
| `cron`  | `resume`  | 恢复          | 代理 `POST /api/jobs/{id}/resume`              |
| `cron`  | `run`     | 立即运行        | 代理 `POST /api/jobs/{id}/run`                 |

> 一期 `skill` 不含 `delete`（保留文件、可恢复）。

### 3.7 `capability.set_response` 响应

```json
{
  "requestId": "req-uuid",
  "ok": true,
  "target": "skill",
  "action": "add",
  "skill": "my-custom-skill",
  "restartStatus": "restarted",
  "error": ""
}
```

| 字段              | 类型        | 说明                                                |
| --------------- | --------- | ------------------------------------------------- |
| `requestId`     | `string`  | 对应 set 的 `requestId`                              |
| `ok`            | `boolean` | 是否成功                                              |
| `target`        | `string`  | `skill` | `model` | `cron`                        |
| `action`        | `string`  | 执行的动作                                             |
| `skill`         | `string`  | 影响到的技能名（如有）                                       |
| `model`         | `string`  | 影响到的模型（如有）                                        |
| `jobId`         | `string`  | 影响到的 job（如有）                                      |
| `restartStatus` | `string`  | `none`（cron 无需重启）/ `restarted` / `restart_failed` |
| `error`         | `string`  | 失败原因（含错误码）                                        |

---

## 4. TrustMesh 后端实现

### 4.1 Agent 模型扩展：新增 Product 字段

**文件**：`backend/internal/model/agent.go`

现状：`Agent` 结构体无 `Product` 字段（已核对）。`JoinRequest` 已有 `AgentProduct` 字段（`model/join_request.go:14`）。

```go
type Agent struct {
    // ... 现有字段 ...
    NodeID string `json:"nodeId" bson:"nodeId"`
    UserID string `json:"userId" bson:"userId"` // 创始人归属，已存在
    // 新增
    Product string `json:"product" bson:"product"` // "trustmesh"(默认) / "hermes" / "opc" / ...
}
```

**取值规则**：不预设白名单，直接采用 JoinRequest 审批里填的 `AgentProduct`；为空则默认 `"trustmesh"`。

### 4.2 审批同步：JoinRequest.AgentProduct → Agent.Product

**文件**：`backend/internal/store/store_join_request.go`

**落库点**：`ApproveJoinRequest`（约 `:175`），审批通过创建/激活 Agent 时，把 `JoinRequest.AgentProduct` 同步写入 `Agent.Product`。

```go
func (s *Store) ApproveJoinRequest(ctx context.Context, reqID string, reviewerID string) (*Agent, error) {
    // ... 现有逻辑：取 JoinRequest、校验、创建 Agent ...
    agent := &Agent{
        NodeID:  jr.NodeID,
        UserID:  jr.UserID,
        Product: jr.AgentProduct,           // 新增：同步产品标识
        // 若为空回退默认值
    }
    if agent.Product == "" {
        agent.Product = "trustmesh"
    }
    // ... 保存 ...
}
```

> **注意 §七 bug**：`discovery/service.go`（ClawSynapse 侧）当前把 announce 的 `AgentProduct` 硬编码为 `"clawsynapse"`，hermes 节点运行时上报错误。因此 **TrustMesh 徽章必须用存储的 `Agent.Product`**（来自 JoinRequest 审批），不依赖运行时 `capability.response.product`。后者仅作运行时校验/告警用。

### 4.3 client.go 新增方法

**文件**：`backend/internal/clawsynapse/client.go`

现状（已核对）：client 通过 `baseURL` 调旁挂 daemon，已有 `Publish`、`GetPeers`、`GetTransfer`、`ListTransfers`、`GetHealth` 等；**无** `GetSkills`/`GetModels`，**无** 文件上传方法。

#### 4.3.1 能力查询（读）

```go
// CapabilityInfo 是 capability.response 的 Go 映射
type CapabilityInfo struct {
    Product   string         `json:"product"`
    Available bool           `json:"available"`
    Skills    []SkillInfo    `json:"skills"`
    Models    []ModelInfo    `json:"models"`
    Jobs      []CronJobInfo  `json:"jobs"`
    Reason    string         `json:"reason"`
    Ts        int64          `json:"ts"`
}

type SkillInfo struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Category    string `json:"category"`
}

type ModelInfo struct {
    ID        string `json:"id"`
    Provider  string `json:"provider"`
    Model     string `json:"model"`
    IsDefault bool   `json:"isDefault"`
    // 注意：api_key 不回显，不在此结构
}

type CronJobInfo struct {
    ID       string   `json:"id"`
    Name     string   `json:"name"`
    Schedule string   `json:"schedule"`
    Enabled  bool     `json:"enabled"`
    Prompt   string   `json:"prompt"`
    Skills   []string `json:"skills,omitempty"`
    NextRun  string   `json:"nextRun,omitempty"`
}

// GetCapabilities 查询目标节点的能力（技能/模型/cron）。
// 内部经旁挂 daemon 发 NATS capability.query，同步等待 response（5s 超时）。
func (c *Client) GetCapabilities(ctx context.Context, nodeID string) (*CapabilityInfo, error) {
    nodeID = strings.TrimSpace(nodeID)
    if nodeID == "" {
        return nil, fmt.Errorf("nodeId is required")
    }
    url := c.baseURL + "/v1/peers/" + url.PathEscape(nodeID) + "/capabilities"
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, fmt.Errorf("build capabilities request: %w", err)
    }
    resp, err := c.httpClient.Do(req)
    if err != nil {
        // 降级：返回 available:false，不报错给上层
        return &CapabilityInfo{Available: false, Reason: "daemon unreachable"}, nil
    }
    defer resp.Body.Close()
    // HTTP 恒 200，body 内 available/reason 表达真实状态
    var out struct {
        Code string          `json:"code"`
        Data CapabilityInfo  `json:"data"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("decode capabilities response: %w", err)
    }
    if out.Code != "" && out.Code != "ok" {
        return &CapabilityInfo{Available: false, Reason: out.Code}, nil
    }
    return &out.Data, nil
}
```

#### 4.3.2 能力写回（写）

```go
// SetCapabilityRequest 是 POST /v1/peers/{nodeId}/capabilities 的请求体
type SetCapabilityRequest struct {
    Target   string         `json:"target"`   // skill|model|cron
    Action   string         `json:"action"`   // 见 §3.6
    Skill    string         `json:"skill,omitempty"`
    FileIds  []string       `json:"fileIds,omitempty"`
    Model    string         `json:"model,omitempty"`
    Provider *ProviderConfig `json:"provider,omitempty"`
    Job      map[string]any `json:"job,omitempty"`
    JobID    string         `json:"jobId,omitempty"`
}

type ProviderConfig struct {
    APIMode      string `json:"api_mode"`
    Transport    string `json:"transport"`
    Model        string `json:"model"`
    DefaultModel string `json:"default_model,omitempty"`
    APIKey       string `json:"api_key,omitempty"` // 写回时携带，读时不回显
}

type SetCapabilityResult struct {
    OK            bool   `json:"ok"`
    Target        string `json:"target"`
    Action        string `json:"action"`
    Skill         string `json:"skill,omitempty"`
    Model         string `json:"model,omitempty"`
    JobID         string `json:"jobId,omitempty"`
    RestartStatus string `json:"restartStatus"` // none|restarted|restart_failed
    Error         string `json:"error,omitempty"`
}

// SetCapabilities 写回目标节点的能力。
func (c *Client) SetCapabilities(ctx context.Context, nodeID string, body *SetCapabilityRequest) (*SetCapabilityResult, error) {
    nodeID = strings.TrimSpace(nodeID)
    if nodeID == "" {
        return nil, fmt.Errorf("nodeId is required")
    }
    payload, err := json.Marshal(body)
    if err != nil {
        return nil, fmt.Errorf("marshal set request: %w", err)
    }
    url := c.baseURL + "/v1/peers/" + url.PathEscape(nodeID) + "/capabilities"
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
    if err != nil {
        return nil, fmt.Errorf("build set request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return &SetCapabilityResult{OK: false, Error: "daemon unreachable"}, nil
    }
    defer resp.Body.Close()
    var out struct {
        Code string              `json:"code"`
        Data SetCapabilityResult `json:"data"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, fmt.Errorf("decode set response: %w", err)
    }
    return &out.Data, nil
}
```

#### 4.3.3 文件上传（skill 写回前置，需补）

**缺口**：`client.go` 现有 `GetTransfer`/`ListTransfers` 仅只读，**无 upload/create transfer 方法**。skill 的 add/update 需先经 `clawsynapse.transfer.*` 把技能文件传到节点拿 `fileId`。

```go
// UploadSkillFile 上传技能文件包到目标节点，返回 fileId。
// 待 ClawSynapse 侧确认 transfer 端点的上传契约后实现。
func (c *Client) UploadSkillFile(ctx context.Context, nodeID string, reader io.Reader, filename string) (string, error) {
    // TODO: 待确认 daemon 的文件上传端点（POST /v1/peers/{nodeId}/transfers ?）
    // 期望返回 fileId，供后续 capability.set 的 fileIds 引用。
    // 这是 skill 写回链路的真实缺口，需与 ClawSynapse 侧对齐后补全。
    return "", fmt.Errorf("upload transfer not implemented yet")
}
```

> **此方法是 skill 写回的阻塞项**，需确认 ClawSynapse 侧文件传输上传端点契约（见 §12 待确认 #3）。

### 4.4 handler 层 API

**文件**：`backend/internal/handler/agent.go`（或新建 `handler/capability.go`）

新增两个 HTTP 端点供前端调用：

```
GET  /api/agents/{agentId}/capabilities
POST /api/agents/{agentId}/capabilities
```

```go
// GetAgentCapabilities 前端查询某 Agent 节点的能力。
func (h *Handler) GetAgentCapabilities(c *gin.Context) {
    agentID := c.Param("agentId")
    agent, ok := h.store.GetAgentByID(agentID)
    if !ok {
        c.JSON(404, gin.H{"error": "agent not found"})
        return
    }
    // 仅 hermes 节点提供能力查询
    if agent.Product != "hermes" {
        c.JSON(200, gin.H{"available": false, "reason": "not a hermes node"})
        return
    }
    info, err := h.clawClient.GetCapabilities(c.Request.Context(), agent.NodeID)
    if err != nil {
        c.JSON(200, gin.H{"available": false, "reason": err.Error()})
        return
    }
    c.JSON(200, gin.H{"data": info})
}

// SetAgentCapabilities 前端写回某 Agent 节点的能力。
func (h *Handler) SetAgentCapabilities(c *gin.Context) {
    agentID := c.Param("agentId")
    agent, ok := h.store.GetAgentByID(agentID)
    if !ok {
        c.JSON(404, gin.H{"error": "agent not found"})
        return
    }
    if agent.Product != "hermes" {
        c.JSON(403, gin.H{"error": "capability writeback only for hermes nodes"})
        return
    }
    var body SetCapabilityRequest
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    result, err := h.clawClient.SetCapabilities(c.Request.Context(), agent.NodeID, &body)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"data": result})
}
```

路由注册（`internal/app` 或 `handler/router`）：

```go
api.GET("/agents/:agentId/capabilities", h.GetAgentCapabilities)
api.POST("/agents/:agentId/capabilities", h.SetAgentCapabilities)
```

### 4.5 webhook 不变

**文件**：`backend/internal/clawsynapse/webhook.go`

已核对：`webhook.go:126` 的 switch 含 `chat.*`/`task.*`/`todo.*`/`knowledge.*`/`transfer.*`/`meeting.*`，**无 `capability.*` 且无需新增**。`capability.response`/`set_response` 经旁挂 daemon 同步 HTTP 返回，不走 webhook 入站分派。

### 4.6 技能文件上传端点（前端→backend→daemon）

前端上传技能文件时，需要一个 backend 端点中转：

```
POST /api/agents/{agentId}/skills/upload   (multipart/form-data)
```

backend 接收文件 → 调 `client.UploadSkillFile(nodeID, reader, filename)` → 返回 `fileId` → 前端再带 `fileId` 调 `POST /api/agents/{agentId}/capabilities`（`{target:"skill", action:"add", skill, fileIds:[fileId]}`）。

---

## 5. TrustMesh 前端实现

### 5.1 产品徽章

**文件**：`frontend/src/pages/AgentDetailPage.tsx`

在详情页头部显示产品标识徽章：

```tsx
// 根据 agent.product 显示徽章
const productBadge = agent.product === "hermes"
  ? <Badge variant="purple">hermes</Badge>
  : agent.product === "opc"
    ? <Badge variant="teal">opc</Badge>
    : <Badge variant="neutral">{agent.product ?? "trustmesh"}</Badge>
```

- 徽章用**存储的 `agent.product`**（来自 API 返回的 Agent 对象），不依赖运行时查询。
- 仅当 `agent.product === "hermes"` 时显示「Hermes 能力」Tab。

### 5.2 Hermes 能力 Tab

在现有 4 个 Tab（概览/对话/工作记录/活动日志）后新增第 5 个 Tab「Hermes 能力」，仅 `product==="hermes"` 时渲染。

**新增文件**：`frontend/src/components/agent/HermesCapabilityTab.tsx`

```tsx
type Props = { agentId: string }

export function HermesCapabilityTab({ agentId }: Props) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["agent-capabilities", agentId],
    queryFn: () => api.get(`/agents/${agentId}/capabilities`).then(r => r.data.data),
    staleTime: 30_000,
  })

  if (isLoading) return <Spinner />
  if (!data?.available) {
    return <Callout intent="neutral">能力信息暂不可用：{data?.reason ?? "未知原因"}</Callout>
  }

  return (
    <div className="space-y-6">
      <SkillList agentId={agentId} skills={data.skills} />
      <ModelList agentId={agentId} models={data.models} />
      <CronList agentId={agentId} jobs={data.jobs} />
    </div>
  )
}
```

### 5.3 各能力 CRUD UI

#### 5.3.1 技能列表（SkillList）

- 只读列表：`name / description / category`。
- 写回操作（二期）：
  - **添加技能**：上传文件（`POST /agents/{id}/skills/upload` 拿 fileId）→ 填技能名 → `POST /capabilities` `{target:"skill", action:"add", skill, fileIds:[fileId]}`。
  - **启用/停用**：`action:"enable"` / `"disable"`。
  - **更新**：重新上传文件 → `action:"update"`。
- 写回后展示 `restartStatus`（restarted / restart_failed），失败时提示重试。

#### 5.3.2 模型列表（ModelList）

- 只读列表：`id / provider / model / isDefault`（**不显示 api_key**）。
- 写回操作（二期）：
  - **新增 provider**：表单填 `api_mode / transport / model / default_model / api_key` → `action:"add"`，`provider` 内联配置。
  - **设默认**：`action:"switch"`。
  - **删除**：`action:"delete"`（当前默认模型禁止删除，前端灰掉按钮 + 后端拦截）。
- ⚠️ `api_key` 经 NATS 明文传输，见 §8.4 安全。

#### 5.3.3 定时任务列表（CronList）

- 只读列表：`name / schedule / enabled / nextRun`。
- 写回操作（一期可做，风险低、无重启）：
  - **新建**：表单填 job 配置 → `action:"create"`。
  - **编辑**：`action:"update"` + `jobId`。
  - **删除**：`action:"delete"` + `jobId`。
  - **暂停/恢复**：`action:"pause"` / `"resume"`。
  - **立即运行**：`action:"run"`。

### 5.4 TypeScript 类型定义

**新增文件**：`frontend/src/types/capability.ts`

```ts
export interface CapabilityInfo {
  product: string
  available: boolean
  skills: SkillInfo[]
  models: ModelInfo[]
  jobs: CronJobInfo[]
  reason?: string
}

export interface SkillInfo {
  name: string
  description: string
  category: string
}

export interface ModelInfo {
  id: string
  provider: string
  model: string
  isDefault: boolean
}

export interface CronJobInfo {
  id: string
  name: string
  schedule: string
  enabled: boolean
  prompt: string
  skills?: string[]
  nextRun?: string
}

export interface SetCapabilityRequest {
  target: "skill" | "model" | "cron"
  action: string
  skill?: string
  fileIds?: string[]
  model?: string
  provider?: ProviderConfig
  job?: Record<string, unknown>
  jobId?: string
}

export interface ProviderConfig {
  api_mode: string
  transport: string
  model: string
  default_model?: string
  api_key?: string
}

export interface SetCapabilityResult {
  ok: boolean
  target: string
  action: string
  skill?: string
  model?: string
  jobId?: string
  restartStatus: "none" | "restarted" | "restart_failed"
  error?: string
}
```

---

## 6. ClawSynapse 侧实现要求（契约边界）

> 以下为 ClawSynapse 侧需实现的内容，TrustMesh 侧不涉及，但作为契约依赖记录于此。

### 6.1 capability NATS 模块

- 新增 `capability` 模块，处理 `capability.query` / `capability.set`。
- 信任校验：仅信任对等方可查/写，非信任回 `capability.denied`。
- 签名验证：复用现有 Ed25519 签名 + 重放保护。

### 6.2 旁挂 daemon 端点

- `GET /v1/peers/{nodeId}/capabilities`：发 `capability.query` → 等 `capability.response`（5s 超时）→ 返回。超时/拒绝/离线 → `{available:false, reason}`，HTTP 200。
- `POST /v1/peers/{nodeId}/capabilities`：发 `capability.set` → 等 `capability.set_response`（5s 超时）→ 返回。超时/拒绝/离线 → `{ok:false, error}`，HTTP 200。

### 6.3 读处理（节点侧）

1. 收到 `capability.query` → 信任校验。
2. 调适配器 `Capabilities(ctx)`：
   - `skill`：并发透传 gateway `GET /v1/skills`（短 TTL 缓存）。
   - `model`：解析本地 `config.yaml` 的 `custom_providers`（**非 gateway**，因 gateway `/v1/models` 恒返回 1 个 agent 名，列不出 provider）。
   - `cron`：代理 gateway `GET /api/jobs`。
3. 组装 `capability.response` 回发。

### 6.4 写处理（节点侧）

**skill / model（改 config + 重启）**

1. 信任校验。
2. skill add/update：按 `fileId` 取本地文件 → 落到托管目录 `~/.hermes/skills/clawsynapse-managed/<skill>/`（**禁止任意路径**）。
3. 维护 `skills.clawsynapse_managed` 键；计算有效 `external_dirs = 原 base + managed`（保留用户 base，不破坏式覆盖）。
4. **先校验**：YAML 解析 + 技能文件结构（或 model provider 字段合法性）→ 失败则**回滚、不重启**、报 `capability.invalid`。
5. model 写回额外校验：禁止删除当前默认模型（须先 `switch` 再删）。
6. 成功 → 定位 `:8642` 上的 gateway PID → kill → 同 env 重 exec `hermes gateway run` → health 检查 → 回报 `restartStatus`。

**cron（代理 gateway 原生端点，无重启）**

1. 信任校验。
2. 按 `action` 直接代理对应 gateway `/api/jobs` 端点（Bearer 鉴权复用 `HermesGatewayKey`）。
3. 返回 gateway 结果 → `restartStatus: "none"`。

### 6.5 文件传输

- skill add/update 需先走 ClawSynapse **现有文件传输通道**（`clawsynapse.transfer.*`）把技能文件传到节点拿 `fileId`，再在 `capability.set` 中引用。协议消息内不内联大文件。
- model / cron 不依赖文件传输。

### 6.6 待修 bug（§七）

- **`discovery/service.go` AgentProduct 硬编码**：当前把 announce 的 `AgentProduct` 硬编码为 `"clawsynapse"`，hermes 节点上报错误。需按 `agentAdapter` 配置修正，否则运行时 product 校验失效。
- **`tm-task-exec` 技能缺失**：executor 角色引用了不存在的技能，能力展示上线后会暴露空技能清单。

---

## 7. 数据流

### 7.1 读流程（查询能力）

```
前端 HermesCapabilityTab
  │ GET /api/agents/{agentId}/capabilities
  ▼
TrustMesh handler.GetAgentCapabilities
  │ 校验 agent.product == "hermes"
  ▼
client.GetCapabilities(nodeId)
  │ GET {daemon}/v1/peers/{nodeId}/capabilities
  ▼
旁挂 daemon (trustmesh-server clawsynapse)
  │ 发 NATS capability.query → 等 capability.response (5s)
  ▼
目标 hermes 节点
  │ 信任校验 → 适配器 Capabilities(ctx)
  │   skill: gateway GET /v1/skills
  │   model: 解析 config.yaml custom_providers
  │   cron:  gateway GET /api/jobs
  ▼
capability.response → daemon → client → handler → 前端
```

### 7.2 写流程 — skill（add）

```
前端：上传技能文件
  │ POST /api/agents/{agentId}/skills/upload (multipart)
  ▼
handler → client.UploadSkillFile(nodeId, reader, filename)
  │ 经 transfer 通道传文件到目标节点
  ▼
返回 fileId

前端：写回技能
  │ POST /api/agents/{agentId}/capabilities
  │   {target:"skill", action:"add", skill:"xxx", fileIds:[fileId]}
  ▼
handler.SetAgentCapabilities → client.SetCapabilities
  │ POST {daemon}/v1/peers/{nodeId}/capabilities
  ▼
daemon → NATS capability.set → 目标节点
  │ 信任校验 → fileId 落托管目录 → 注册 managed 键
  │ 校验 config → 改 config.yaml → kill gateway → 重 exec
  │ health 检查
  ▼
capability.set_response {restartStatus:"restarted"} → 前端展示结果
```

### 7.3 写流程 — model（add）

```
前端：新增 provider 表单
  │ POST /api/agents/{agentId}/capabilities
  │   {target:"model", action:"add", provider:{api_mode,transport,model,default_model,api_key}}
  ▼
（同上路径，无文件传输）
  │ 节点侧：校验 provider 字段 → 写 custom_providers → 改 config.model
  │ → 重启 gateway
  ▼
set_response {restartStatus:"restarted"}
```

### 7.4 写流程 — cron（create）

```
前端：新建定时任务表单
  │ POST /api/agents/{agentId}/capabilities
  │   {target:"cron", action:"create", job:{name,schedule,prompt,...}}
  ▼
  │ 节点侧：信任校验 → 代理 gateway POST /api/jobs（Bearer HermesGatewayKey）
  │ 无重启
  ▼
set_response {restartStatus:"none"}
```

---

## 8. 安全设计

### 8.1 权限

- 读写均**仅信任对等方**（复用现有签名验证 + trust-mode 检查）。
- 非信任节点的 `query` / `set` 回 `capability.denied`。
- 与 OPC 模型一致：只有运营审批通过的创始人节点才被信任，才能被查/写。

### 8.2 托管目录隔离

- skill 新/改只落到 `~/.hermes/skills/clawsynapse-managed/<skill>/`，**绝不接受任意路径**。
- 把 RCE 爆炸半径锁死在托管区。

### 8.3 写回审计日志

- 每次 `capability.set` 记 `sender / action / target / skill|model|jobId / fileId / ts`。
- `api_key` 脱敏后记录。

### 8.4 密钥不回显

- `models` 读响应不明文返回 `api_key`。
- `provider` 写回时携带 `api_key`，但**不回显**。

### 8.5 ⚠️ model api_key 经 NATS 明文传输（需确认）

- model 的 `add` 在 `provider` 字段内联 `api_key`，经 NATS 传输。
- 当前测试环境 NATS **无鉴权**（与外部 NATS 一致），若有恶意/被攻陷 peer，`api_key` 可被嗅探。
- **建议**：上 model 写回前，给 NATS 加 TLS 或 token（与 NATS 安全化工作衔接）。
- 或：一期 model 只做读 + `switch`/`delete`（不含 `api_key`），`add`（含 `api_key`）放二期并配合 NATS 安全化。

### 8.6 重启安全性

- gateway 会话落盘 `~/.hermes/state.db`（SQLite），`/v1/responses` 的 `previous_response_id` 续聊血缘重启后从磁盘恢复。
- 仅数秒不可用窗口（需实测确认）。
- **并发写回竞争**：契约未提 config.yaml 写锁，多人同时改技能/模型可能竞争同一文件。需确认 ClawSynapse 侧是否串行化（见 §12 待确认 #2）。

---

## 9. 错误处理与降级

### 9.1 错误码

| 错误码                         | 含义                    | TrustMesh 侧处理          |
| --------------------------- | --------------------- | ---------------------- |
| `capability.denied`         | 未授权（非信任对等方）           | 前端提示"无权限"              |
| `capability.unavailable`    | peer 离线 / gateway 不可达 | 前端降级显示"能力信息暂不可用"       |
| `capability.invalid`        | 校验失败（已回滚，未重启）         | 前端提示校验错误 + 原因          |
| `capability.restart_failed` | 重启后 health 未通过        | 前端提示"写回已生效但重启失败，需人工检查" |
| `capability.timeout`        | 网格响应超时                | 前端降级 + 可重试             |

### 9.2 降级原则

- 读写失败**不影响节点在线状态与既有功能**。
- 读失败：HTTP 仍 200，`{available:false, reason}`，前端显示降级提示。
- 写失败：HTTP 仍 200，`{ok:false, error}`，前端提示错误 + 可重试。

---

## 10. 分期交付计划

### 阶段 0：无外部依赖项（TrustMesh 侧可立即启动）

| 任务                              | 文件                                | 说明               |
| ------------------------------- | --------------------------------- | ---------------- |
| Agent 模型加 `Product` 字段          | `model/agent.go`                  | 默认 `"trustmesh"` |
| 审批同步 `AgentProduct` → `Product` | `store/store_join_request.go:175` | 空 → 默认值          |
| Agent 详情 API 返回 `product`       | `handler/agent.go`                | 序列化已含，确认即可       |
| 前端产品徽章                          | `AgentDetailPage.tsx`             | 用存储值，不依赖运行时      |

> 此阶段不依赖 ClawSynapse 侧任何改动，可独立交付。

### 阶段 1：只读能力展示（依赖 ClawSynapse 侧 capability 模块 + daemon 端点）

| 任务                                          | 文件                           | 说明     |
| ------------------------------------------- | ---------------------------- | ------ |
| `client.GetCapabilities`                    | `clawsynapse/client.go`      | §4.3.1 |
| handler `GET /api/agents/{id}/capabilities` | `handler/agent.go`           | §4.4   |
| 前端 Hermes 能力 Tab（只读）                        | `HermesCapabilityTab.tsx`    | §5.2   |
| 三类只读列表 UI                                   | SkillList/ModelList/CronList | §5.3   |

### 阶段 2：写回（依赖 ClawSynapse 侧 set 实现 + 文件传输）

| 任务                                            | 文件                      | 说明     |
| --------------------------------------------- | ----------------------- | ------ |
| `client.SetCapabilities`                      | `clawsynapse/client.go` | §4.3.2 |
| handler `POST /api/agents/{id}/capabilities`  | `handler/agent.go`      | §4.4   |
| `client.UploadSkillFile`（**待确认契约**）           | `clawsynapse/client.go` | §4.3.3 |
| handler `POST /api/agents/{id}/skills/upload` | `handler/agent.go`      | §4.6   |
| 技能写回 UI（add/update/enable/disable）            | SkillList               | §5.3.1 |
| 模型写回 UI（add/switch/delete）                    | ModelList               | §5.3.2 |
| cron 写回 UI（CRUD + pause/resume/run）           | CronList                | §5.3.3 |

> cron 写回风险低（无重启），可与阶段 1 同步做前端。

---

## 11. 依赖与阻塞

### 11.1 阻塞项（ClawSynapse 侧，TrustMesh 无法绕过）

| 阻塞项                                               | 影响                        | 状态               |
| ------------------------------------------------- | ------------------------- | ---------------- |
| `capability` NATS 模块未实现                           | 阶段 1/2 全部阻塞               | 待 ClawSynapse 实现 |
| 旁挂 daemon `/v1/peers/{nodeId}/capabilities` 端点未实现 | 阶段 1/2 全部阻塞               | 待 ClawSynapse 实现 |
| `discovery/service.go` AgentProduct 硬编码 bug       | 运行时 product 错误（徽章用存储值可绕过） | 待 ClawSynapse 修  |
| `tm-task-exec` 技能缺失                               | 技能清单暴露空                   | 待补齐              |
| 文件传输上传端点契约未定义                                     | skill 写回阻塞                | 待确认              |

### 11.2 TrustMesh 侧需补

| 任务                        | 工作量    | 阶段 |
| ------------------------- | ------ | -- |
| `Agent.Product` 字段 + 审批同步 | 小      | 0  |
| 前端产品徽章                    | 小      | 0  |
| `client.GetCapabilities`  | 小      | 1  |
| handler 读端点               | 小      | 1  |
| 前端能力 Tab + 只读列表           | 中      | 1  |
| `client.SetCapabilities`  | 小      | 2  |
| `client.UploadSkillFile`  | 中（待契约） | 2  |
| handler 写端点 + 上传中转        | 中      | 2  |
| 前端写回 UI（技能/模型/cron）       | 中偏大    | 2  |

### 11.3 环境依赖

- 测试环境 clawsynapse 仍连外部 NATS `nats://220.168.146.21:9414`（已不可达）。capability 模块端到端验证需先把 clawsynapse 切到本地 NATS（`nats://trustmesh-nats:4222`，已装好含 JetStream）。
- hermes 节点需与 `trustmesh-server` 枢纽节点在**同一 NATS**，capability 网格才可达。

---

## 12. 待确认问题清单（给 ClawSynapse 侧）

1. **capability 模块实现进度**：`capability.*` NATS 模块 + 旁挂 daemon `GET/POST /v1/peers/{nodeId}/capabilities` 端点，预计何时可用？
2. **config.yaml 并发写回串行化**：多个 `capability.set` 同时到达同一节点改 config.yaml，是否有写锁/队列？否则可能产生写冲突导致 config 损坏。
3. **文件传输上传端点契约**：TrustMesh 如何把技能文件传到目标节点拿 `fileId`？是 `POST /v1/peers/{nodeId}/transfers`（multipart）还是别的？响应格式？——这是 skill 写回的硬依赖，client.go 当前无上传方法。
4. **cron 是否一期范围**：TrustMesh 侧成本极低，但前端需多做一套 UI。是否一期就含 cron CRUD？
5. **model api_key 传输安全**：`provider.api_key` 经 NATS 明文，当前 NATS 无鉴权。是否一期 model 只做读 + switch/delete（不含 add 的 api_key），add 放二期配合 NATS 加 TLS/token？
6. **gateway 重启实测**：契约称"数秒不可用窗口"，是否实测过？重启期间在途的 `/v1/responses` 请求如何处理（排队/拒绝）？
7. **capability.response.product 与存储 Agent.Product 不一致时**：TrustMesh 用存储值做徽章，运行时 product 仅校验。若不一致，是否需要 TrustMesh 告警/日志？

---

## 13. 与历史决策的对齐 / 修正

| 历史决策（上一轮 Hermes 能力展示方案）                                 | 本设计修正                                                                          |
| ------------------------------------------------------- | ------------------------------------------------------------------------------ |
| A 来源：审批 JoinRequest 同步 `AgentProduct` → `Agent.Product` | **保留**，不变                                                                      |
| B 展示：仅详情页徽章                                             | **保留**，不变                                                                      |
| C 通道：TrustMesh 主动拉取，直连节点 `/v1/skills`、`/v1/models`      | **修正**：经旁挂 daemon `GET /v1/peers/{nodeId}/capabilities`，NATS 穿透，不直连节点          |
| D 配置：写回适配器                                              | **细化**：写回经 `capability.set`，skill/model 改 config.yaml + 重启 gateway，cron 代理原生端点 |
| E 前端：仅 product==hermes 显示第 5 个 Tab                      | **保留**，并扩展 Tab 内容为技能/模型/cron 三类                                                |

---

## 附录 A：相关文件索引

| 文件                                             | 说明                                                                |
| ---------------------------------------------- | ----------------------------------------------------------------- |
| `capability-contract.md`                       | ClawSynapse 侧起草的契约（本设计的依据）                                        |
| `docs/hermes-integration-design.md`            | 早期 hermes 集成设计（含 `agent.*` 提议，已被本契约取代）                            |
| `backend/internal/model/agent.go`              | Agent 模型（待加 Product 字段）                                           |
| `backend/internal/model/join_request.go`       | JoinRequest 模型（已有 AgentProduct）                                   |
| `backend/internal/store/store_join_request.go` | ApproveJoinRequest（审批同步落库点）                                       |
| `backend/internal/clawsynapse/client.go`       | Local API 客户端（待加 GetCapabilities/SetCapabilities/UploadSkillFile） |
| `backend/internal/clawsynapse/webhook.go`      | webhook 分派（不变）                                                    |
| `backend/internal/handler/agent.go`            | Agent handler（待加能力端点）                                             |
| `frontend/src/pages/AgentDetailPage.tsx`       | 详情页（待加徽章 + Tab）                                                   |

---

*本文档随 ClawSynapse 侧契约确认与实现进展持续更新。*
