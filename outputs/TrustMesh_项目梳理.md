# TrustMesh 项目梳理

> 梳理日期：2026-07-30 · 工作空间：`D:\AiWorkspace\TrustMesh`
> 定位：以 AI Agent 为核心执行者的任务编排与项目管理平台，构建在 ClawSynapse 网络之上，作为其中的一个节点参与多 Agent 通信。

---

## 一、项目定位与核心思想

- **本质**：参考 Asana 的项目管理模型，把"执行者"从人类替换为 AI Agent。用户提需求 → PM Agent 规划 → 执行 Agent 完成任务。
- **网络基础**：建立在 [ClawSynapse](https://github.com/yuanjun5681/clawsynapse) 之上（NATS 消息总线 + clawsynapsed 节点 + Adapter）。
- **通信铁律**：Agent 之间不直接通信，一切经过 TrustMesh 中转。
  - 出站：TrustMesh 调 `clawsynapsed` Local API `POST /v1/publish`
  - 入站：ClawSynapse `WebhookAdapter` 回调 `POST /webhook/clawsynapse`
  - 节点发现：TrustMesh 周期 `GET /v1/peers` 同步 Agent 在线状态

---

## 二、技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25（go.mod: 1.25.1）、Gin、MongoDB Driver v2、zap、golang-jwt、go-openai |
| 通信网络 | ClawSynapse（clawsynapsed + NATS）；默认外部 NATS `nats://220.168.146.21:9414` |
| 向量库 | Qdrant（知识库 embedding 检索，OpenAI embedding） |
| 前端 | React 19 + TypeScript + Vite 7 + Tailwind CSS v4 + Base UI（`@base-ui/react`，原 Radix 已迁移）+ shadcn 样式 + recharts |
| 状态管理 | TanStack Query（服务端）+ Zustand（客户端 auth/theme） |
| HTTP 客户端 | ky（自动附 JWT，401 跳登录） |
| 基础设施 | Docker Compose（mongo / qdrant / backend / clawsynapse / frontend） |

> 注意：CLAUDE.md 仍写"shadcn/ui (Radix UI)"，但 `frontend/package.json` 实际依赖是 `@base-ui/react ^1.3.0` + `shadcn ^4.0.8`，UI 底座已迁移到 Base UI。

---

## 三、代码结构与核心模块

### 后端（`backend/internal/`，101 个 .go 文件）

分层与 CLAUDE.md 一致，但业务模块已显著超出 MVP：

| 模块 | 职责 | 关键文件 |
|---|---|---|
| `cmd/server` | 入口：.env → config → logger → app → http.Server | `main.go` |
| `app` | 应用组装、Gin 路由、依赖注入 | `router.go`、`webhook_test.go` |
| `handler` | HTTP 处理层（~20 个 handler） | `auth.go`、`project.go`、`task.go`、`meeting.go`、`market.go`、`agent_chat.go`、`knowledge.go`、`assistant.go`、`realtime.go`、`sse.go`、`clawsynapse.go`、`join_request.go`、`notification.go`、`dashboard.go`、`transfer.go`、`platform.go`、`project_file.go` |
| `store` | **核心业务层**：内存 map + 可选 MongoDB 持久化，含全部业务逻辑与事件流 | `store.go`、`store_task.go`、`store_meeting.go`、`store_market.go`、`store_agent.go`、`store_project.go`、`store_knowledge.go`、`store_agent_chat.go`、`workflow.go`、`planning.go`、`meeting.go`、`streams.go`、`bootstrap.go`、`timeout_monitor.go`、`mongo_state.go` |
| `model` | 领域模型（单文件/分文件） | `agent.go`、`task.go`、`conversation.go`、`meeting.go`、`market.go`、`project.go`、`knowledge.go`、`agent_chat.go`、`event.go`、`comment.go`、`join_request.go`、`dashboard.go`、`user.go`、`project_file.go` |
| `clawsynapse` | webhook 入站处理、发布客户端、Peer 同步、信任同步、上下文校验 | `client.go`、`webhook.go`、`sync.go`、`trust_sync.go`、`context_verify_test.go` |
| `protocol` | ClawSynapse 协议层 | `clawsynapse.go` |
| `auth` | JWT 签发/验证 | `jwt.go` |
| `middleware` | JWT 认证、CORS、日志、Recovery、限流 | `auth.go`、`cors.go`、`logging.go`、`recovery.go`、`rate_limit.go` |
| `assistant` | **平台内置 LLM 助手**（基于 OpenAI，意图识别/工具调用） | `intent.go`、`llm.go`、`prompt.go`、`tools.go`、`types.go` |
| `knowledge` | 知识库：切片、处理、Qdrant 检索、存储 | `chunker.go`、`processor.go`、`qdrant.go`、`storage.go` |
| `embedding` | OpenAI embedding 封装 | `embedding.go`、`openai.go` |
| `agentfile` / `project` | Agent 文件 / 项目文件存储 | `agentfile.go`、`project/storage.go` |
| `config` / `logger` / `transport` | 环境变量、日志、统一响应格式 | `config.go`、`logger.go`、`response.go` |

**存储架构**：内存优先（`sync.RWMutex` 保护的 map）+ `MONGO_ENABLED=true` 时同步写 MongoDB（`persist*Unsafe`）；启动 `bootstrap.go` 从 Mongo 加载到内存；方法名含 `Unsafe` 后缀表示调用者已持锁。

### 前端（`frontend/src/`）

- API 客户端：`src/api/client.ts`（ky，自动 JWT + 401 跳转）
- 路由：`react-router-dom` v7
- 状态：TanStack Query + Zustand
- UI：Base UI + Tailwind v4 + shadcn 样式 + recharts（图表）
- 按 feature 组织：`pages` / `components` / `api` / `hooks` / `stores` / `lib` / `types`

### 技能（`skills/`，4 个 SKILL.md）

- `tm-meeting-host` —— 会议主持人（PM Agent）技能
- `tm-meeting-participant` —— 会议参会者（执行 Agent）技能
- `tm-task-plan` —— 任务规划技能（PM）
- `tm-task-exec` —— 任务执行技能（执行 Agent）

> ⚠️ **关键架构事实**：PM / 执行 Agent 是**外部 ClawSynapse 节点**，其技能从自身运行时技能库加载，**不**从 TrustMesh 仓库 `skills/` 目录加载。改 `skills/*.md` 不会自动生效到外部 Agent 节点。

### 部署（`deploy/`，75 个 .py + 2 个 .sh）

- `deploy/deploy_test.py` —— 测试环境部署脚本（paramiko SSH）
- `deploy/clawsynapse/` —— 上游 clawsynapse 节点容器构建定义（从 GitHub 拉源码编译 `clawsynapsed`）
- 注意：`deploy_test.py` 只推 backend/frontend 镜像，不同步 `skills/`，也不推 `compose` 文件。

---

## 四、核心业务功能（已落地 / 设计中）

1. **任务编排（MVP 主线）**
   - 领域模型：`Project 1→1 PM Agent`、`Project 1→N Conversation`、`Conversation 1→1 Task`、`Task ◇→N TodoItem`、`Task 1→N TaskEvent`。
   - Todo 是最小执行单元，Task 状态由 Todos 自动聚合（`failed` 优先级最高）。
   - 消息类型：`conversation.message/reply`、`task.create/created/status_changed`、`todo.assigned/progress/complete/fail/status_changed`。
   - PM 门禁：PM 不在线时禁止用户发消息（`PM_AGENT_OFFLINE`）。

2. **中心化调度会议**（中心化会议架构，`docs/meeting-architecture.md` + `model/meeting.go` + `handler/meeting.go`）
   - 主持人（PM Agent）轮询各执行 Agent，round-robin 发言、冲突调解、用户可插话。
   - 会前准备 → 会议讨论 → 自动生成纪要（markdown）+ 待办（绑定 Agent 责任人）+ 上传纪要文件。

3. **工作市场模块**（`handler/market.go`、`store/store_market.go`、`model/market.go`，对应 `docs/TrustMesh工作市场模块设计文档.md`）

4. **知识库 / RAG**（`internal/knowledge/*` + Qdrant + OpenAI embedding）：上传文档 → 切片 → 向量化 → 检索。

5. **平台内置助手**（`internal/assistant/*` + OpenAI）：意图识别 + 工具调用的 LLM 助手。

6. **Hermes Gateway 集成（规划中）**（`docs/hermes-integration-design.md`）：将 Agent 接入 Hermes Gateway HTTP API，新增 `agent.*` 能力发现（`skills`/`jobs`/`sessions`/`capabilities`），前端 Agent 页扩展为 7 个 Tab。当前 `task.*`/`todo.*` 在上游已统一走 `/v1/runs`。

7. **Agent 文件 / 项目文件**：`agentfile`、`project_file` 模块 + 共享传输卷 `trustmesh-transfer-data`。

---

## 五、部署与运行

### 本地 / 全栈（Docker Compose）

```bash
docker compose up -d --build      # mongo / qdrant / backend / clawsynapse / frontend
```
- 前端 `:3000`、后端 `:8080`（健康 `:8080/healthz`）、ClawSynapse `:18080`、MongoDB `:27017`、Qdrant `:6333`。
- 外部依赖：NATS `nats://220.168.146.21:9414`。

### 测试环境（团队记忆）

- 服务器 `175.27.135.91`，**SSH 端口 62000**（用户 cloudUser），HTTP 网关走容器端口：前端 `:3000`、后端 `:8080`。
- 远程目录 `/opt/trustmesh-test`，镜像 tag `test`。
- 部署用 `deploy/deploy_test.py`（paramiko）。**改 backend 镜像后必须 `docker compose up -d --force-recreate backend`**（tag 不变时 `up -d` 不会重建）。
- 持久化卷：`trustmesh-files-data`（项目文件/纪要/产物）必须挂载 backend 的 `/var/lib/trustmesh-files`，否则重部署会丢数据。
- 本地无 go：backend 镜像用 `docker build --platform linux/amd64`。

### 后端关键环境变量

`JWT_SECRET`（必填）、`MONGO_ENABLED`/`MONGO_URI`/`MONGO_DATABASE`、`CLAWSYNAPSE_API_URL`/`PEER_SYNC_INTERVAL`、`EMBEDDING_*`、`QDRANT_URL`、`KNOWLEDGE_STORAGE_PATH`、`FILES_STORAGE_PATH`、`TRUSTMESH_EXTERNAL_URL`、`ASSISTANT_API_URL`/`KEY`/`MODEL`。

---

## 六、关键约定与已知事项（团队记忆沉淀）

1. **消息类型统一**：会议消息已与 1:1 对话统一走 `chat.message`（出站 + 入站）。`metadata.context=="meeting"` / `metadata.meeting_id` / `sessionKey` 命中 `GetMeeting` 即识别会议语境。旧 `meeting.message`/`meeting.control` 仅作兼容别名保留。
2. **ACK 回执过滤**：智能体发完正式回复后会补发 `ACK <类型>` 回执。所有入站分支必须过 `isSilentACK()` 静默吞掉；`cleanConversationNoise` 用 `stripLeadingAck` 剥离首部 ACK 令牌保留真实内容，避免误杀"前缀 + 真实发言"。
3. **会议纪要懒恢复**：纪要字节丢失时 `GetContent` 从 `meeting.Minutes`(DB) 透明重建；`agent_artifact` 字节不在 DB，**无法**自动恢复，需 Agent 重新产出。
4. **技能加载架构（重要）**：外部 Agent 节点技能需在其自身运行时部署更新；改仓库 `skills/` 对测试环境 Agent 不自动生效。PM Agent 节点运行位置 / 技能部署路径尚未完全澄清。
5. **webhook 路由** `POST /webhook/clawsynapse` 当前无签名鉴权；合成测试可令 `nodeId` 为空跳过本地节点校验。

---

## 七、文档索引（`docs/`，18 篇）

| 文档 | 主题 |
|---|---|
| `mvp-design.md` | MVP 设计 |
| `api-design.md` / `api-reference.md` | API 设计与参考 |
| `data-model.md` | MongoDB 数据模型 |
| `message-protocol.md` / `message-protocol-redesign.md` | 消息协议现状 / 精简改造提案（回声去除） |
| `meeting-architecture.md` | 中心化调度会议架构 |
| `hermes-integration-design.md` | Hermes Gateway 集成方案（v4） |
| `frontend-design.md` / `project-structure.md` | 前端 / 项目结构 |
| `realtime-architecture.md` | 实时推送架构（SSE） |
| `installation-guide.md` / `quick-start-guide.md` | 安装 / 快速上手 |
| `TrustMesh工作市场模块设计文档.md` / `TrustMesh工作岗位市场技术方案.md` | 工作市场模块 |
| `production-cost-estimation.md` | 生产成本估算 |
| `prod-meeting-fix-2026-07-14.md` | 生产会议修复记录 |

---

## 八、一句话总结

TrustMesh 是一个"Agent 即执行者"的多智能体任务编排平台：后端 Go/Gin + MongoDB + Qdrant，前端 React 19 + Base UI，底层通过 ClawSynapse（NATS）与 PM / 执行 Agent 节点通信；核心能力已覆盖任务流、中心化会议、工作市场、知识库 RAG 与内置 LLM 助手，Hermes Gateway 集成在规划中。团队记忆沉淀了消息类型统一、ACK 过滤、部署卷挂载、外部 Agent 技能加载等关键约定。
