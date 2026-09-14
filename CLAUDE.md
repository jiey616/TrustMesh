# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

TrustMesh 是一个以 AI Agent 为核心执行者的任务编排与项目管理平台。参考 Asana 的项目管理模型，将执行者从人类替换为 AI Agent。

TrustMesh 构建在 [ClawSynapse](https://github.com/yuanjun5681/clawsynapse) 网络之上，作为 ClawSynapse 的一个节点参与多 Agent 通信。

## 开发命令

### 后端 (Go)

```bash
# 启动基础设施 (MongoDB)
docker compose up -d mongo

# 运行后端（从项目根目录）
cd backend && go run ./cmd/server

# 运行所有测试
cd backend && go test ./...

# 运行单个包测试
cd backend && go test ./internal/app/
cd backend && go test ./internal/store/

# 运行单个测试
cd backend && go test ./internal/app/ -run TestHappyPathAuthToConversation

# 构建
cd backend && go build ./cmd/server
```

### 前端 (React)

```bash
cd frontend-v2 && npm install
cd frontend-v2 && npm run dev      # 开发服务器
cd frontend-v2 && npm run build    # 生产构建 (tsc -b && vite build)
cd frontend-v2 && npm run lint     # ESLint
```

### Docker Compose (全栈)

```bash
docker compose up -d            # 启动所有服务 (mongo, backend, clawsynapse, frontend)
docker compose down             # 停止
```

服务端口：后端 `:8080`，前端 `:3000`，ClawSynapse `:18080`，MongoDB `:27017`

## 架构

```
ClawSynapse 网络 (共享 NATS Server)
│
├── TrustMesh 节点 (clawsynapsed + WebhookAdapter)
│   ├── 发送：clawsynapsed Local API (POST /v1/publish)
│   ├── 接收：WebhookAdapter → POST /webhook/clawsynapse
│   ├── 后端 (Go/Gin + MongoDB)
│   └── 前端 (React) ←→ REST API (/api/v1/*) ←→ 后端
│
├── PM Agent 节点 (clawsynapsed + Adapter)
└── 执行 Agent 节点 (clawsynapsed + Adapter)
```

- Agent 间不直接通信，一切通过 TrustMesh 中转
- 前端通过 `/api/v1` 前缀的 REST API + JWT 认证访问后端
- TrustMesh 通过 ClawSynapse Local API (`POST /v1/publish`) 向 Agent 发消息
- Agent 通过 WebhookAdapter 推送到 `POST /webhook/clawsynapse` 端点回传消息

### 后端分层

| 层 | 路径 | 职责 |
|---|---|---|
| 入口 | `cmd/server/main.go` | 加载 .env → config → logger → app → http.Server |
| 应用组装 | `internal/app/router.go` | 创建 Gin engine、注入所有依赖、注册路由 |
| Handler | `internal/handler/` | 请求解析、调用 Store、返回响应 |
| Store | `internal/store/` | **核心业务层**，内存 map + 可选 MongoDB 持久化，含所有业务逻辑 |
| Model | `internal/model/types.go` | 所有领域模型定义（单文件） |
| Transport | `internal/transport/response.go` | 统一响应格式和 AppError |
| ClawSynapse | `internal/clawsynapse/` | webhook 处理、消息发布客户端、Peer 同步 |
| Config | `internal/config/config.go` | 环境变量加载，全部通过 `getEnv()` 读取 |
| Auth | `internal/auth/jwt.go` | JWT 签发与验证 |
| Middleware | `internal/middleware/` | JWT 认证、CORS、日志、Recovery |

### Store 的双模式存储

Store 采用"内存优先 + MongoDB 持久化"架构：
- 所有数据首先操作内存 map（`sync.RWMutex` 保护）
- 如果 `MONGO_ENABLED=true`，变更同时写入 MongoDB（`persist*Unsafe` 方法）
- 启动时通过 `bootstrap.go` 从 MongoDB 加载数据到内存
- 方法名含 `Unsafe` 后缀表示调用者已持有锁

### SSE 实时推送

Task 和 Conversation 都支持 SSE 流：
- `GET /conversations/:id/stream` — Conversation 变更推送
- `GET /tasks/:id/stream` — Task + Events 快照推送
- 通过 `store/streams.go` 的 `Subscribe`/`Publish` + channel 实现

### 前端结构

- API 客户端：`src/api/client.ts` 基于 ky，自动附加 JWT，401 自动跳转登录
- 状态管理：TanStack Query（服务端）+ Zustand（客户端 auth/theme）
- 路由：react-router-dom v7
- UI：shadcn/ui (Radix) + Tailwind CSS v4

## 关键设计决策

- **Todo 是最小执行单元**：Task 包含 Todo 列表，Task 状态由 Todos 自动聚合
- **一个 Conversation 最多一个 Task**：PM 创建 Task 后 Conversation 自动变为 resolved
- **PM 门禁**：PM 不在线时禁止用户发送消息，返回 `PM_AGENT_OFFLINE`
- **幂等性**：webhook 消息通过 `processedMessages` map 实现幂等（基于 `message_id`）
- **MVP 使用 TRUST_MODE=open**：不校验签名和信任关系

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.25, Gin, MongoDB (mongo-driver v2), zap (日志) |
| 通信网络 | ClawSynapse (clawsynapsed + NATS) |
| 前端 | React 19, TypeScript, Vite 7, shadcn/ui (Radix UI), Tailwind CSS v4 |
| 状态管理 | TanStack Query (服务端状态), Zustand (客户端状态) |
| HTTP 客户端 | ky |
| 基础设施 | Docker + docker-compose (MongoDB, NATS, clawsynapsed) |

## API 响应格式

```json
// 成功（单个资源）
{ "data": { ... } }

// 成功（列表）
{ "data": { "items": [...] }, "meta": { "count": N } }

// 错误
{ "error": { "code": "ERROR_CODE", "message": "...", "details": {} } }
```

## 业务消息类型（type 字段）

Agent → TrustMesh (webhook):
- `conversation.reply` — PM 回复用户对话
- `task.create` — PM 创建任务
- `todo.progress` / `todo.complete` / `todo.fail` — 执行 Agent 回传结果

TrustMesh → Agent (clawsynapse publish):
- `conversation.message` — 转发用户需求给 PM
- `task.created` / `task.updated` — 任务状态通知给 PM
- `todo.assigned` / `todo.updated` — Todo 状态通知给执行 Agent

## Task 状态聚合规则

```
存在 todo failed      → task.failed
全部 todos done       → task.done
存在 todo in_progress → task.in_progress
全部 todos pending    → task.pending
```

注意：failed 优先级最高，判断顺序影响结果。

## 配置

后端配置全部通过环境变量，参考 `backend/.env`。关键变量：
- `MONGO_ENABLED` — 是否启用 MongoDB 持久化（默认 true）
- `CLAWSYNAPSE_API_URL` — clawsynapsed Local API 地址
- `CLAWSYNAPSE_NODE_ID` — 本节点 ID
- `CLAWSYNAPSE_PEER_SYNC_INTERVAL` — Agent 在线状态轮询间隔

## 设计文档

详细设计文档在 `docs/` 目录下，包括数据模型、API 设计、消息协议、Agent 工作流、前端设计等。
