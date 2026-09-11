# TrustMesh × Hermes 节点能力（capability）端到端测试报告

- 日期：2026-08-04
- 环境：本地 Docker（trustmesh-backend :8080 / trustmesh-clawsynapse 枢纽 :18080 / clawsynapse hermes 节点 :18081 / mongo / qdrant / frontend :3000）
- 测试对象：hermes 节点（nodeId `n1-9881a46a564a9c5ab06663b461bd8615`，Agent「测试PM」`cf27fa0b8f9551448baa9cb7`，product=hermes）
- 消息总线：NATS `nats://175.27.135.91:4222`（远程）

---

## 一、结论速览

| 链路 | 结果 | 说明 |
|------|------|------|
| 阶段 0：Agent.Product 字段 + 审批同步 + 详情徽章数据 | ✅ 通过 | Agent 数据含 `product: "hermes"`，与 JoinRequest 审批同步链路一致 |
| 阶段 1：能力查询（skills/models/jobs 只读） | ✅ 通过 | `GET /agents/{id}/capabilities` 实时返回 80 技能 / 2 模型 / 1 cron |
| 阶段 2：cron 写回（create/pause/run/resume/delete） | ✅ 通过 | 全部 `ok:true`，`restartStatus:"none"`（契约 cron 无重启语义） |
| 阶段 2：model 写回（add/switch/delete） | ✅ 通过（含 1 个 TrustMesh 侧修复 + 1 个 hermes 限制） | add 写入 config.yaml + 重启返回 `restarted`；switch 仅对 custom provider 生效 |
| 阶段 2：skill 上传 + add | ⚠️ 阻塞（daemon 侧缺口） | TrustMesh 端点已实现，但旁挂 daemon **无文件上传端点**（契约 §12 #3 未定义） |

---

## 二、读取链路（✅ 全部通过）

### 2.1 Agent 数据（阶段 0 验证）

```
GET /api/v1/agents → {"id":"cf27fa0b8f9551448baa9cb7","name":"测试PM",
  "node_id":"n1-9881...","product":"hermes","status":"online",...}
```

- `product: "hermes"` 正确显示（来源：JoinRequest 审批同步 → Agent.Product，阶段 0 实现生效）。
- 前端详情页徽章数据源就绪。

### 2.2 能力查询（阶段 1 验证）

```
GET /api/v1/agents/cf27fa0b8f9551448baa9cb7/capabilities
→ {"data":{"product":"hermes","available":true,
    "skills":[...80 项...],"models":[...],"jobs":[...],"reason":""}}
```

- **skills（80 项）**：clawsynapse / tm-meeting-host / tm-meeting-participant / tm-task-exec / tm-task-plan / yuanbao / claude-code / codex / computer-use / hermes-agent / opencode 等。
- **models（2 项）**：`agnes → agnes-2.0-flash (isDefault=false)`、`deepseek → deepseek-v4-flash (isDefault=true)`。
- **jobs（1 项）**：`rebuild-verify / 0 4 * * * / enabled=true`。
- 链路：TrustMesh → hub daemon `GET /v1/peers/{nodeId}/capabilities` → NATS `capability.query` → hermes 节点 adapter `Capabilities()` → 返回。

### 2.3 前端

- 详情页在 `product === 'hermes'` 时显示 3 个 Tab（定时任务 / 技能 / 模型），数据来自上述端点。
- 非 hermes 节点返回 `available:false, reason:"not a hermes node"`，前端降级显示。

---

## 三、写回链路

### 3.1 cron（✅ 全通过）

| 动作 | 请求 | 响应 |
|------|------|------|
| create | `{target:cron, action:create, job:{name:"e2e-test-job",...}}` | `ok:true, restartStatus:"none"` |
| pause | `{..., action:pause, jobId}` | `ok:true` |
| run | `{..., action:run, jobId}` | `ok:true` |
| resume | `{..., action:resume, jobId}` | `ok:true` |
| delete | `{..., action:delete, jobId}` | `ok:true` |

- hermes 节点日志铁证：`capability set applied, adapter:hermes, target:cron, action:create/pause/run/resume/delete`。
- 复查 jobs：e2e-test-job 创建后出现、删除后消失（读缓存有数秒延迟）。
- 符合契约：cron 代理 gateway 原生 `/api/jobs`，**无需重启**。

### 3.2 model（✅ 通过，含 1 个 TrustMesh 修复 + 1 个 hermes 限制）

| 动作 | 结果 |
|------|------|
| switch → agnes | ✅ 生效：agnes 变 `isDefault=true`（config.yaml model.default 更新 + gateway 重启） |
| add provider | ✅ 生效：写入 config.yaml `custom_providers`（含 api_key），`ok:true, restartStatus:"restarted"` |
| delete provider | ✅ 生效：从 custom_providers 移除，`restartStatus:"restarted"` |
| switch → deepseek（built-in） | ⚠️ 被 hermes 拒：`capability.invalid: built-in provider switch is unsupported`（hermes 限制：built-in provider 仅经部署 env 配置，不支持经 API switch） |

- 测试期间发现的 **TrustMesh 侧 bug（已修复）**：
  1. **写回超时 3s 不够**：model/skill 写回会重启 gateway（数秒窗口），`CLAWSYNAPSE_TIMEOUT=3s` 导致 `context deadline exceeded`（switch 实际成功但 TrustMesh 误报失败）。→ 修复：`Client` 新增独立 `writeClient`（写回/上传用 ≥30s 超时）。
  2. **ProviderConfig 缺 `name` 字段**：hermes 校验 `model add requires provider.name`，但 Go `ProviderConfig` 无 name 字段导致序列化丢弃。→ 修复：补 `Name string json:"name,omitempty"`；前端 `ModelAddDialog` 补 `name`。

### 3.3 skill（⚠️ 阻塞于 daemon 侧缺口）

| 步骤 | 结果 |
|------|------|
| TrustMesh 上传端点 `POST /agents/{id}/skills/upload` | ✅ 实现（multipart 收文件 → 转发 daemon） |
| daemon 上传端点 | ❌ **不存在**：`POST /v1/peers/{nodeId}/transfers` → 404；`POST /v1/transfer/upload` → 405（仅 DELETE/GET/HEAD） |

- **根因**：契约 §2.5 说 skill 写回需先经 `clawsynapse.transfer.*` 传文件拿 fileId，但**旁挂 daemon 未实现文件上传端点**（契约 §12 待确认 #3 悬而未决）。TrustMesh 侧 `UploadSkillFile`（当前指向 `/v1/peers/{nodeId}/transfers`，假设路径）与前端「部署技能」表单均已就位，**等 daemon 提供真实上传端点契约后即可打通**。
- 建议：ClawSynapse 侧确认上传端点路径/响应格式（如 `POST /v1/transfer` multipart 返回 `{data:{fileId}}`），TrustMesh 侧 `client.UploadSkillFile` 按契约微调即可。

---

## 四、测试中发现并已修复的 TrustMesh 侧问题

| # | 问题 | 根因 | 修复 |
|---|------|------|------|
| 1 | model switch 报 `context deadline exceeded`（实际已生效） | 共享 `httpClient` 3s 超时 < gateway 重启窗口 | `Client` 增加 `writeClient`（写回/上传 ≥30s）；`SetCapabilities`/`UploadSkillFile` 改用之 |
| 2 | model add 报 `capability.invalid: requires provider.name` | Go `ProviderConfig` 缺 `name` 字段，前端传的 name 被序列化丢弃 | 后端补 `Name` 字段；前端 `ModelAddDialog` 传 `provider.name` |

两个修复均已通过 `go build` + 单测 + 重建镜像验证。

---

## 五、环境注意事项（测试过程沉淀）

1. **Mongo 双模 store 的内存缓存**：直接改 Mongo 数据后必须重启 backend 才生效；且 backend 运行中会把内存态写回 Mongo（peer presence 同步），**不要**靠改 DB 改归属，应走 API 流程。
2. **测试数据还原**：hermes 容器重启后 entrypoint 保留 `model.default/provider already set` 配置；测试用的 e2e-test-job 已删、默认模型已还原为 deepseek、测试 provider 已删除。
3. **能力查询的读缓存**：写回后立即查询可能看不到最新数据（数秒延迟），复查需间隔几秒。
4. **hermes 适配器读侧**：`capability.response.models` 的读实现未完整反映所有 custom_providers（add 后 config.yaml 有、查询列表可能不显示）——hermes 侧实现细节，不影响 TrustMesh 链路。

---

## 六、遗留待办

1. **ClawSynapse 侧**：实现旁挂 daemon 的文件上传端点（skill 写回前置），确认路径/响应契约。
2. **ClawSynapse 侧**：`capability.response.models` 读实现核对（应完整列出 custom_providers）。
3. **TrustMesh 侧**：`client.UploadSkillFile` 按真实 daemon 契约微调（当前为假设路径）。
4. **前端**：三个 Tab 的写回 UI 已就位，待 daemon 上传端点就绪后联调「部署技能」。
