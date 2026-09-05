# TrustMesh 多租户 + 企业管理 实施设计

> 状态：**阶段 0 + 阶段 1 已完成，阶段 2 分批推进中**（阶段 0 commit `5fa4a71`；阶段 1 见 §阶段 1 实况；阶段 2 见 §阶段 2 实况，**2-1 Project / 2-2 Task / 2-3 Agent / 2-4 Knowledge+Template / 2-5a File / 2-5b Comment+Meeting 均已上线**）
> 日期：2026-09-04
> 开工前备份：`deploy/backups/mongodump-trustmesh-20260904-214800.archive.gz`，git tag `pre-multitenant-stage0`
> 阶段 1 备份：`deploy/backups/mongodump-trustmesh-20260904-223127-stage1.archive.gz`，git tag `pre-multitenant-stage1`
> 决策来源：grill-me 会话，六个顶层分支全部闭合
> 代码基线：`backend/internal/store` 为全内存状态机，Mongo 为持久化镜像

---

## 0. 代码事实勘误（🔴 必读，影响工期判断）

grill 阶段基于一个假设：**数据主要落在 Mongo，加统一查询过滤即可隔离**。
**代码核查后该假设不成立**，真实架构如下：

| 项 | grill 假设 | 代码实际 | 影响 |
|---|---|---|---|
| 主状态 | Mongo 查询 | **全内存 map**（`Store` struct 30+ 个 map），`loadMongoState()` 启动时全量载入，Mongo 是持久化镜像 | 🔴 隔离必须在**内存层**做，Mongo 过滤只是镜像同步 |
| 隔离方式 | 统一查询 filter / ORM | **散落在每个 store 函数里的手工 if 判断**，如 `store_project.go:41` `if task.UserID != project.UserID { continue }` | 🔴 不存在"加一个中间件就业务无感"的捷径 |
| 改动面 | 集合加字段 + 中间件 | 散落 `UserID` 引用约 **290 处**（2026-09-04 实测：store 169 / handler 108 / clawsynapse+middleware+app 14），其中 store 内直接 `UserID !=` 比较约 90 处 | 🔴 核心工作量在这里，不是在数据迁移 |
| 已分区数据 | — | `userEvents` / `userNotifications` / `userJoinRequests` / `userKnowledgeDocs` / `userWorkflowTemplates` / `userSubscribers` **已按 userID 分区**（`map[userID]→[]id`） | ✅ 这些数据天然隔离，改分区键即可 |
| 前端请求 | — | `src/api/client.ts` 用 `ky` + `beforeRequest` 钩子统一注入 `Authorization` | ✅ 租户头可**单点注入** |

**结论**：六个分支的**决策不变**，但实施路径从"中间件单点控制"改为"**store 层归属收敛 + 编译期兜底**"（见 §4）。
**唯一的安全网**：Go 是强类型语言，漏改处**必然编译报错**，不可能静默漏权限——这是本方案可控的根本原因。

---

## 0.5 运行环境与执行节点清单（2026-09-04 用户确认 + 容器核查）

| 环境 | 地址 | 状态 | 说明 |
|---|---|---|---|
| **生产** | `175.27.135.91`（`:62000` 入口，NATS `:4222`） | ✅ 在用 | ⚠️ 仓库 `deploy/*.py` 里写作 "Test environment"，**实为生产**，以用户口径为准 |
| 旧生产 | `36.137.106.15` | ❌ **已停用** | 不再使用，无需纳入改造 |
| **SZJT 宿主机** | Debian 13 aarch64（cloudUser） | ✅ 在用 | **是执行节点**，承载山雨系列智能体 |

**执行节点清单**（本地容器，镜像均为 `clawsynapse:v1.0.35`）：

| 容器 | 容器 ID（前 12） | `CLAWSYNAPSE_AGENT_ROLE` | 说明 |
|---|---|---|---|
| `clawsynapse` | `82cd1906359a` | **pm** | PM 节点 |
| `clawsynapse-default` | `e93b4c6576ac` | **pm** | PM 节点 |
| `clawsynapse3` | `fdd8b4d3102e` | executor | 执行者 |
| `clawsynapse4` | `9c0c788b4764` | executor | 执行者 |

**另有一个非执行节点但同样连 NATS**（阶段 3 必须一并处理）：

| 容器 | 镜像 | `NODE_ID` | 说明 |
|---|---|---|---|
| `trustmesh-clawsynapse` | `clawsynapse:local-rt4` | `trustmesh-server` | **平台转发节点**（unhealthy 为既有状态，功能正常） |

**消息总线事实**（全部统一，无分裂）：

- 执行节点：`/root/.clawsynapse/config.yaml` → `natsServers: [nats://175.27.135.91:4222]`
- 转发节点：env `NATS_SERVERS=nats://175.27.135.91:4222`
- 本地 backend：`docker-compose.yml:82` → `nats://175.27.135.91:4222`
- ⚠️ 执行节点**不通过 env 配置 NATS**（env 里只有 LLM provider 与角色），改在**挂载卷配置文件** `/root/.clawsynapse/config.yaml`；改节点配置要动 volume（`clawsynapse_clawsynapse-data`），不是改 compose env
- `220.168.146.21:9414` 仅出现在 `docker-compose.prod.yml` 与 `backend/README.md`，当前**无节点在用**（文档称已不可达）

## 1. 决策基线（grill 已定，不再讨论）

| 分支 | 决策 |
|---|---|
| **A. 租户模型** | Organization 实体 + 个人租户兜底；user ↔ org 多对多；现有账号自动生成个人 org 并成为 Owner |
| **B. 数据归属** | 业务集合冗余 `org_id` + 统一 scope 裁决；**Agent 切 org 归属，`user_id` 降级为 `created_by`** |
| **C. 成员权限** | Owner / Admin / Member 三档（Guest 二期）；**项目级成员制**：不设成员=全员可见，可切私有；私有项目 API 层全过滤（列表/详情/任务/文件/评论/通知），AI Office 3D 与 NATS 总线不动 |
| **D. 会话与加入** | JWT 只带身份，活跃租户走 `X-Org-Id` 头（切企业不重登）；**管理员代建账号** + 临时密码一次性展示 + 首登强制改密 |
| **E. 执行节点** | JoinRequest 审批通过即颁发**绑 org 的 node token**，subject 统一加 org 前缀；NATS 部署拓扑不变，跨企业不可互订阅 |
| **F. 配额计费** | org 留配额字段（节点/成员/项目/存储）+ scope 层简单上限校验；套餐/订单/支付二期 |

---

## 2. 归属矩阵（哪些资源挂 org，哪些保持 user）

| 资源 | 现状 | 改造后归属 | 说明 |
|---|---|---|---|
| User | 全局 | **跨 org 全局身份** | 邮箱唯一，一人可属多企业 |
| Organization | 无 | 新增 | 租户实体 |
| OrgMembership | 无 | 新增 | user↔org + role |
| Project / Task / Workflow | `user_id` | **org_id**（`user_id`→`created_by`） | 企业资产，成员共享 |
| **Agent** | `user_id` | **org_id**（`user_id`→`created_by`） | 企业资产，审批后全员可用 |
| Knowledge / WorkflowTemplate | `user_id` | **org_id** | 企业资产 |
| Meeting / Artifact / ProjectFile | 经 project 间接 | **org_id**（冗余，便于过滤） | 随项目走 |
| 事件流 / 通知 | `userEvents[userID]` | **org 分区**（通知保留 user 已读态） | 见 §4.3 |
| JoinRequest | `userJoinRequests[userID]` | **org 分区** | 节点审批是企业级动作 |
| 用户偏好（主题等） | 前端 localStorage | 保持 user 级 | 与租户无关 |

---

## 3. 数据模型

```go
// internal/model/organization.go

type Organization struct {
    ID        string    `json:"id" bson:"_id"`
    Name      string    `json:"name" bson:"name"`
    Slug      string    `json:"slug" bson:"slug"`         // 唯一，预留域名/URL 隔离
    Kind      string    `json:"kind" bson:"kind"`         // personal | enterprise
    OwnerID   string    `json:"owner_id" bson:"owner_id"` // 唯一 Owner，可转让
    Quota     OrgQuota  `json:"quota" bson:"quota"`
    CreatedAt time.Time `json:"created_at" bson:"created_at"`
    UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// 配额占位（分支 F：只留字段 + 上限校验，不做计费）
type OrgQuota struct {
    MaxMembers int   `json:"max_members" bson:"max_members"` // -1 = 不限
    MaxNodes   int   `json:"max_nodes"   bson:"max_nodes"`
    MaxProjects int  `json:"max_projects" bson:"max_projects"`
    MaxStorageBytes int64 `json:"max_storage_bytes" bson:"max_storage_bytes"`
}

// user ↔ org 多对多 + 角色（分支 C：三档）
type OrgMembership struct {
    ID        string    `json:"id" bson:"_id"`
    OrgID     string    `json:"org_id"  bson:"org_id"`
    UserID    string    `json:"user_id" bson:"user_id"`
    Role      string    `json:"role"    bson:"role"` // owner | admin | member
    JoinedAt  time.Time `json:"joined_at" bson:"joined_at"`
}

// 项目级成员（分支 C：默认全员，设置后转为私有白名单）
type ProjectMember struct {
    ID        string    `json:"id" bson:"_id"`
    ProjectID string    `json:"project_id" bson:"project_id"`
    UserID    string    `json:"user_id"    bson:"user_id"`
    Role      string    `json:"role"       bson:"role"` // viewer | editor
    CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// ⚠️ 原设计的 NodeToken 实体 —— 已取消（2026-09-04 核查后）
// 节点侧**已有成熟的身份与信任体系**，再造一套是重复轮子：
//   · 节点身份：/root/.clawsynapse/identity.key + identity.pub（密钥对）
//   · 信任配对：/root/.clawsynapse/trust.json，trustMode: tofu / trustAutoApprove: false
//     结构 { trusted:[{nodeId, atMs}], pending:[{requestId, from, to, nodeId, ...}] }
//   · 平台侧已用 node_id + trust_request_id 关联（model.JoinRequest 本就带这两个字段）
//   · subject 前缀体系已存在：config.yaml 的 deliverablePrefixes: [chat, task, todo, meeting]
//
// 分支 E 因此改为「**给现有绑定关系补 org 维度**」，不新增凭证实体：

// model/agent.go 扩字段（其余 model 同构加 OrgID）
type Agent struct {
    // ...既有字段不变
    OrgID  string `json:"org_id"  bson:"org_id"`  // 主归属（分支 B）
    NodeID string `json:"node_id" bson:"node_id"` // 既有：节点身份 = trust.json 里的 nodeId
}
// 凭证变更审计 → 二期再建 node_credential_audit 集合；本期不新增实体。
```

**存量资源统一加两个字段**（`org_id` 新增 + `user_id` 语义降级）：

```go
// 以 Project 为例，其余集合同构
type Project struct {
    ID          string `json:"id" bson:"_id"`
    OrgID       string `json:"org_id"  bson:"org_id"`   // 新增：主归属/过滤键
    UserID      string `json:"user_id" bson:"user_id"`  // 保留：语义降级为 created_by
    // ...其余字段不变
}
```

---

## 4. 隔离机制改造：从"散落 if"到"统一归属裁决"

### 4.1 现状代码模式（问题所在）

```go
// store_project.go —— 隔离靠手工 if，每个函数各写一遍
for _, id := range ids {
    task, ok := s.tasks[id]
    if !ok || task.UserID != project.UserID {   // ← 散落判断，372 处之一
        continue
    }
    // ...
}
```

### 4.2 目标：收敛为 scope 裁决（可脚本化替换）

```go
// internal/store/scope.go —— 新增
type Scope struct {
    UserID string
    OrgID  string
    Role   string // owner | admin | member
}

// org 资产裁决（项目/任务/Agent/知识库/模板/会议/文件）
func ownedByOrg(sc Scope, ownerOrgID string) bool {
    return sc.OrgID != "" && ownerOrgID == sc.OrgID
}

// user 资产裁决（个人已读态、用户偏好等跨 org 数据）
func ownedByUser(sc Scope, ownerUserID string) bool {
    return sc.UserID != "" && ownerUserID == sc.UserID
}

// 项目可见性（分支 C：不设成员 = 全员可见）
func (s *Store) projectVisible(sc Scope, p *model.Project) bool {
    if !ownedByOrg(sc, p.OrgID) {
        return false
    }
    members := s.projectMembers[p.ID]
    if len(members) == 0 {
        return true // 未设成员 → 全员可见
    }
    for _, m := range members {
        if m.UserID == sc.UserID {
            return true
        }
    }
    return false
}
```

**替换手法**：把 store 函数签名 `userID string` → `sc Scope`，把 `x.UserID != userID` → `!ownedByOrg(sc, x.OrgID)`。
**为什么安全**：Go 强类型，任何漏改都会**编译期报错**，不会静默漏权限。配合 §7 越权测试双保险。

### 4.3 内存分区改造（已按 userID 分区的 map）

| map | 现状 | 改造后 | 理由 |
|---|---|---|---|
| `userEvents` | `map[userID][]*Event` | `orgEvents map[orgID][]*Event` | 项目活动是 org 级，成员都该看到 |
| `userNotifications` | `map[userID][]id` | **保留 user 分区**（已读态是个人维度），**发布源加 org 过滤** | 未读状态属个人 |
| `userKnowledgeDocs` | `map[userID][]id` | `orgKnowledgeDocs map[orgID][]id` | 组织知识资产 |
| `userWorkflowTemplates` | `map[userID][]id` | `orgWorkflowTemplates map[orgID][]id` | 组织模板资产 |
| `userJoinRequests` | `map[userID][]id` | `orgJoinRequests map[orgID][]id` | 节点审批是 org 级动作 |
| `userSubscribers` | `map[userID]map[chan]` | `orgSubscribers map[orgID]map[chan]` | SSE 推 org 内事件 |
| `agents` / `projects` / `tasks` / `agentChats` / `meetings` / `externalApps` | 全局 `map[id]` | **不改 key 结构**，靠 §4.2 scope 裁决过滤 | ⚠️ 改 key 会影响 ID 引用、文件存储路径、artifact 绑定，风险远大于收益 |

### 4.4 请求侧链路（新增）

```
前端 client.ts (beforeRequest 钩子)
  → 注入 X-Org-Id: <活跃 org>
     ↓
middleware.OrgScope（新增，位于 RequireAuth 之后）
  → 查 OrgMembership 校验 user 属该 org → 注入 Scope{UserID, OrgID, Role}
     ↓
handler：sc := middleware.Scope(c) → 传 store 各函数
```

前端单点注入（`src/api/client.ts` 第 26-31 行处）：

```ts
beforeRequest: [
  (request) => {
    const { accessToken, activeOrgId } = useAuthStore.getState()
    if (accessToken) request.headers.set('Authorization', `Bearer ${accessToken}`)
    if (activeOrgId) request.headers.set('X-Org-Id', activeOrgId)   // ← 新增一行
  },
],
```

---

## 5. 分阶段实施计划

> 遵循项目惯例：设计文档 → 阶段 0（无依赖）→ 阶段 1（读）→ 阶段 2（写）→ 前端，分阶段验证。

### 阶段 0 — 建模与骨架（纯新增，零破坏）

| 项 | 内容 |
|---|---|
| **目标** | 新实体落地，现有逻辑**完全不动**，可随时回滚 |
| 改动 | ① 新增 `model/organization.go`（Organization / OrgMembership / ProjectMember / OrgQuota；NodeToken 已取消，见 §3）<br>② 新增 `store/scope.go`（Scope + ownedByOrg / ownedByUser / projectVisible）<br>③ 新增 `store/store_org.go`（org 与 membership CRUD）<br>④ 现有 model 加 `OrgID` 字段（**只加字段，不改任何判断逻辑**）<br>⑤ 新增 `middleware/org_scope.go`（解析 X-Org-Id + 校验 membership）<br>⑥ Mongo 建 `organizations` / `org_memberships` / `project_members` 集合 + 索引 |
| 验收 | 服务正常启动；现有全部功能回归通过；新集合可读写；`tsc`/`go build` 零新增错误 |
| 回滚 | 删除新增文件与字段即可，无数据变更 |
| 风险 | 低（纯增量） |
| **2026-09-04 实况** | ✅ 完成并部署。6 项改动全部落地：新实体（Organization/OrgMembership/ProjectMember/OrgQuota）、`store/scope.go`、`store/store_org.go`（CRUD + Mongo 持久化）、9 个 model 加 `OrgID`（omitempty，不污染现有 API）、`middleware/org_scope.go`、3 个新集合 + 索引。<br>验收实测：backend healthy；/projects /agents /notifications 200；无租户头行为与改造前一致；伪租户头 `X-Org-Id: org_fake123` → 401；新集合 `organizations` `org_memberships` `project_members` 索引就位、0 文档（阶段 0 不回填）；go build/vet/test 全绿（7 包 ok + 5 个新测试）。<br>⚠️ 踩坑：CRLF 文件用脚本插入时，插入内容/锚点必须显式带行尾换行，否则新行会粘到相邻行末尾造成语法错误（本次因此回滚重打 2 轮）。 |

### 阶段 1 — 数据回填与双写

| 项 | 内容 |
|---|---|
| **目标** | 存量数据获得 org 归属，新数据自动带上 |
| 改动 | ① 每个现存 user 生成个人 org（`kind=personal`，该 user 为 Owner）<br>② 回填脚本：`users → org_id`、各集合 `org_id = 该 user 的个人 org`（含 Mongo 与内存加载路径）<br>③ 写入路径改为双写（新建资源时自动填 `org_id`）<br>④ 回填前后**计数校验**：每集合回填条数 == 总条数，抽样 20 条人工核对 |
| 验收 | `db.<coll>.countDocuments({org_id: {$exists: false}})` == 0（users 除外）；重启后内存态 org_id 均在位 |
| 回滚 | 备份 Mongo（执行前 `mongodump`）+ git tag；异常时恢复备份 |
| 风险 | 中（写操作）🔴 **执行前必须停 backend 并等状态落盘**（见项目铁律：stop → 轮询 exited → 改库 → start） |

#### 阶段 1 实况（2026-09-04 完成）

**改动**
- 7 个实体补齐 `org_id`：`Event` / `Notification` / `TaskArtifact` / `AgentChat` / `WorkflowTemplate` / `ExternalApp` / `MeetingMessage`（阶段 0 已加 9 个 → 至此 16 个实体全覆盖）。
- `EnsurePersonalOrg` 拆出 `ensurePersonalOrgUnsafe`（调用方持锁），`CreateUser` 内联调用：**注册即开通个人租户**，失败只 `log.Warn` 不阻断注册（可事后补偿）。
- 新增 `personalOrgOfUnsafe(userID)`（调用方持锁）：解析作者的个人租户，供写入路径双写。
- 写入双写 17 处：projects / tasks（4 处构造点）/ agents / comments / events / notifications / artifacts / project_files（3 处）/ workflow_templates（2 处）/ join_requests / external_apps / meetings / meeting_messages。

**回填结果**（`deploy/org_backfill.js`，先 `APPLY=false` 预演再 `APPLY=true` 落盘）

| 集合 | 条数 | 归属来源 |
|---|---|---|
| events | 4000 | `user_id` |
| notifications | 1709 | `user_id` |
| comments | 1439 | `user_id` |
| artifacts | 281 | `task_id` → task.user_id |
| project_files | 275 | `project_id` → project.user_id |
| tasks | 142 | `user_id` |
| agent_chats | 27 | `user_id` |
| projects / join_requests | 11 / 8 | `user_id` |
| knowledge_chunks / agents | 11 / 7 | `user_id` |
| meeting_messages / meetings | 4 / 2 | `meeting_id` → meeting.org_id / `project_id` |
| knowledge_documents / workflow_templates / external_apps | 1 / 1 / 1 | `user_id` / `created_by` |

8 个 user → 8 个 personal org + 8 条 owner membership；**7,918 篇文档零孤儿**。

**校验**（`deploy/org_verify.js`）：16 个集合覆盖率 `missing=0`；跨集合一致性 4 项 `mismatched=0`；task→org 孤儿 0 → `VERIFY OK`。

**踩坑（🔴 下次必须避开）**
1. 补丁脚本的幂等判定**不能只认 `OrgID:` 开头的行** —— 多行插入块（注释 + 赋值）会被重复写入。`meeting.go` / `store_artifact.go` / `store_workflow_template.go` 因此被插了两遍，靠 `git checkout` 回滚后重跑修复。已改为「插入块首行是否已存在」。
2. 双写插入点**必须自动校验持锁状态**：`CreateWorkflowTemplate`、`CreateExternalApp` 的构造体位于 `s.mu.Lock()` **之前**；`GenerateMeetingMinutesFile` 全程不持锁。脚本内置「向上找最近 `func` → 区间内须有 `s.mu.Lock()` 或函数名含 `Unsafe`」校验，两处被拦截后改为「锁内赋值」与「只从入参派生」。
3. `git add -A` 会把 `.playwright-cli/`、`.workbuddy/` 等日志临时文件一并暂存；本仓库只能按路径精确 `git add`。

### 阶段 2 — 后端归属收敛（**最重**）

| 项 | 内容 |
|---|---|
| **目标** | 把约 290 处散落 `UserID` 引用收敛到 scope 裁决，隔离逻辑单点化 |
| 改动 | ① store 函数签名 `userID string` → `sc Scope`（脚本批量替换 + 编译错误逐个修）<br>② `x.UserID != userID` → `!ownedByOrg(sc, x.OrgID)`<br>③ §4.3 分区 map 改造（events / knowledge / templates / joinRequests / subscribers）<br>④ 项目可见性接入 `projectVisible()`（列表/详情/任务/文件/评论/通知六处）<br>⑤ Agent / JoinRequest 归属切 org，审批链写入 node token |
| 验收 | 两个测试 org 互相不可见对方项目/任务/Agent/知识库；同 org 内成员按角色与项目成员制正常协作；回归全通过 |
| 回滚 | 阶段 2 整体一个 commit 区间，git revert 可回退；数据不变（仅读法变） |
| 风险 | 🔴 **高**：改动面大，必须分批提交（每类资源一个 commit），每批跑一次回归；依赖编译期兜底 + §7 测试 |

#### 阶段 2 实况

**🔴 首要裁决原则（2-1 血泪教训，后续各批必须遵守）**

> **无租户上下文（`X-Org-Id` 为空）时，一律退回 user 维度裁决，与改造前完全一致。**
> 只有「**请求带租户上下文**」**且**「**资源已回填 `org_id`**」两个条件同时成立，才走 org 裁决。

```go
func visibleToScope(sc Scope, ownerOrgID, ownerUserID string) bool {
    if sc.HasOrg() && ownerOrgID != "" {
        return ownedByOrg(sc, ownerOrgID)
    }
    return ownedByUser(sc, ownerUserID)
}
```

违反这条的代价：阶段 1 回填后资源都带上了 `org_id`，若裁决写成「有 org 就比 org」，**所有不带 `X-Org-Id` 的存量客户端会瞬间看不到自己的全部数据**。2-1 首次实现即踩此坑，6 个既有测试（`TestArchiveProjectBlocksTaskExecutionMutations` / `TestApplyWorkflowSync` / `TestUpdateProjectAssignsWorkflowIDs` / `TestSyncAgentPresenceMarksOfflineAndBusy` / `TestProjectWorkflowRefAndProgress` / `TestCreateTaskInvalidStepRange`）集体报 `NOT_FOUND: project not found`。

这条同时决定了「未回填资源」的兜底语义：**宁可退回 user 维度放行，也不因数据缺失让作者失联** —— 回填是尽力而为，不能假定 100% 命中。

**2-1 Project 归属收敛（已完成并上线）**

| 项 | 内容 |
|---|---|
| 裁决层 | `scope.go` 新增 `visibleToScope()` 通用入口 + `resolveOwnerOrgUnsafe()`（新建资源挂活跃租户，无租户上下文则挂个人租户）；`projectVisible()` 改为三级：无租户头→user 维度 / 有租户头但资源未回填→user 维度兜底 / 否则 org 归属 + 项目成员白名单 |
| 签名收敛 | `store_project.go` 7 个函数 `userID string` → `sc Scope`：`CreateProject` / `ListProjects` / `GetProject` / `UpdateProject` / `ArchiveProject` / `GetProjectPMNode`；`projectForUserUnsafe`→`projectForScopeUnsafe`（4 处）、`pmAgentForUserUnsafe`→`pmAgentForScopeUnsafe` |
| Handler | 新增 `handler/helpers.go: currentScope()`（取 `middleware.Scope(c)`，缺 UserID 直接 401）；`project.go` 5 个 handler 改 `currentScope` |
| 内部路径 | `store_task.go` / `store_planning.go` / `handler/task.go` / `assistant/tools.go`(2) / `clawsynapse/webhook.go`(2) 显式传 `Scope{UserID: ...}`，保持原 user 语义 |
| 测试 | 新增 3 个：`TestProjectScopedVisibility`（无头=改造前行为 / 同租户可见 / 跨租户不可见）、`TestResolveOwnerOrg`、`TestVisibleToScopeFallback`；存量 24 处调用点批量改 `Scope{UserID: userID}` |
| 验证 | `go build` + `go vet` + `go test ./...` 全绿；部署后冒烟：无头 200 列 11 项目 / 伪造 `X-Org-Id` 401 / 真实头 200 列同 11 项目 / 新建项目 `org_id` 正确落个人租户 / 探针已清理 |

**踩坑（🔴 下次必须避开）**

1. **补丁脚本的 func 范围模式会被 Edit 静默失败坑到** —— 对脚本做多行修改后必须 grep 验证落盘，否则「报成功不落盘」会让你对着正确的逻辑调试半天。改脚本一律用 Python 写文件。
2. **测试文件批量替换要注意路径分隔符**：Windows 下 `startswith("backend/internal/store/")` 判定会因反斜杠失效，导致 store 包内测试被误写成 `store.Scope{`（包内无需限定符）。
3. **容器刚启动时的首次 API 读数不可信** —— 内存状态机从 Mongo 灌数据需要时间，`up -d` 后 2 秒查询只返回 1 个项目（实际 11 个）。冒烟测试必须等容器 `healthy` **之后再等状态灌完**，否则会误判成回归。

### 2-2 Task 归属收敛（已完成并上线）

| 项 | 内容 |
|---|---|
| 读路径 | `store_task.go` 6 个函数收 Scope：`ListTasks` / `GetTask` / `ListTaskEvents` / `ListUserEvents` / `ListAgentEvents` / `ListRecentTasks`；`store_planning.go` 6 个：`CreateTaskPlanning` / `CreateTaskPlanningWithFiles` / `AppendTaskMessage` / `ApprovePlan` / `RejectPlan` / `GetTaskPMPublishTarget`；`workflow.go` 的 `CreateTaskByUser`（写路径） |
| 写路径落点 | 新建任务的 `OrgID` 从 `personalOrgOfUnsafe(userID)` 改为 `resolveOwnerOrgUnsafe(sc)` —— **带租户头建的任务挂活跃租户，否则挂个人租户** |
| 节点路径 | `GetTaskByNodeID` 用 agent 自身身份构造 `Scope{UserID, OrgID}`；`FinalizePlanByPMNode` 的 todo 派发校验用**项目归属**构造 `projectScope`（PM 代表项目归属方派活） |
| 内部辅助 | `assistant/tools.go`(3) / `clawsynapse/webhook.go`(2) / handler 内 3 个辅助函数（`createPlanningTask` 除外，见下）显式传 user-only Scope，语义与改造前完全一致 |
| 测试 | 新增 4 个：`TestTaskScopedVisibility` / `TestGetTaskByNodeIDSameOrg` / `TestFinalizePlanByPMNodeAssigneeScope` / `TestCreateTaskOwnerOrg` |

**关键设计决策**

1. **写路径必须吃租户上下文**。`createPlanningTask` / `CreateTaskByUser` 决定新任务的 `org_id`。若沿用 user-only Scope，**在企业租户下建的任务会落到个人租户，之后带租户头反而看不到** —— 这是改造引入的功能缺陷，不是安全问题。因此这两个入口改成吃 `Scope`（`createPlanningTask` 形参 `userID string` → `sc store.Scope`）。
2. **handler 层保留 `userID := sc.UserID`**。大量 handler 里 `userID` 还用于事件写入、日志、payload 构造；全局替换成 `sc` 风险不可控。做法是鉴权行改 `currentScope(c)`，需要作者维度的地方补一行 `userID := sc.UserID`，其余代码一行不动。
3. **`ListUserEvents` 仍是 user 维度**。活动流是「谁触发了什么」，不按租户共享；收 Scope 参数只为调用侧统一。
4. **未回填资源在租户上下文下不对他人放行**。`visibleToScope` 在 `ownerOrgID == ""` 时退回 user 比较，所以他人看不到；只有作者本人仍可见。刻意的安全兜底：**数据缺失时宁可漏，不可泄**。

**踩坑（🔴 下次必须避开）**

1. **单行替换要按「子串」匹配，不能按「整行相等」**。函数签名只给到形参列表结束，整行还带返回类型和 `{`，整行相等永远匹配不上。
2. **替换块的行数不能硬编码**。`AUTH_OLD` 是 4 行我却写了 `body[k:k+3]`，导致鉴权块永远找不到。一律用 `len(parts)`。
3. **先替换再判断会让降级分支失效**：脚本先把首参 `userID` → `sc`，再判断有没有鉴权块，结果没有鉴权块的函数里 `sc` 未定义、且降级分支再也匹配不到 `h.store.X(userID`。修法是按函数名精确回改。
4. **`agentByNodeUnsafe` 走 `s.agentByNode` 索引**，测试 fixture 只写 `s.agents` 会报 `agent not found by node_id`。
5. **尾随换行会 split 出空串**，整行块匹配因此永远失败 —— 匹配前先 `rstrip("\n")`。
6. **容器重启后 JWT 失效**，冒烟脚本必须**每次重新登录**再取 token，不能复用旧 token。

### 2-3 Agent 归属收敛（已完成并上线）

| 项 | 内容 |
|---|---|
| 签名收敛 | `store_agent.go` 8 个函数 `userID string` → `sc Scope`：`CreateAgent`（写路径）/ `ListAgents` / `GetAgent` / `UpdateAgent` / `DeleteAgent` / `GetAgentStats` / `GetAgentInsights` / `ListAgentTasks`；`store_agent_chat.go` 6 个公开函数 + `agentForUserUnsafe` |
| 写路径落点 | `CreateAgent` 的 `OrgID` 改 `resolveOwnerOrgUnsafe(sc)` —— **带租户头建的 agent 挂活跃租户，否则挂个人租户** |
| Handler | `handler/agent.go` 11 处 + `handler/agent_chat.go` 6 处鉴权行 `currentUserID(c)` → `currentScope(c)` |
| 补漏（action_items） | `workflow.go` 的 `ListActionItems`（`task.UserID != userID` → `visibleToScope`）+ `ConvertActionItems`（**里层解析 executor agent 按 user 维度找**，企业租户下成员 A 无法把待办派给成员 B 建的 agent；且新建 task 仍落 `personalOrgOfUnsafe`）。连同 `handler/action_items.go` 3 处调用点一并收敛 |
| 中间件修复 | 🔴 `middleware.Scope()` 在只跑了 `RequireAuth` 而没跑 `OrgScope` 时**退回 `store.Scope{UserID: UserID(c)}`**，与「未带 `X-Org-Id` 的存量客户端」语义一致。此前返回零值 Scope，导致 12 个直接 `c.Set("user_id", ...)` 的 handler 测试集体 401 |
| 测试 | 新增 3 个：`TestAgentScopedVisibility`（无头=改造前行为 / 同租户可见 / 跨租户不可见）、`TestCreateAgentOwnerOrg`（带租户头挂活跃租户 / 无头挂个人租户）、`TestAgentChatIsUserScoped`（会话归属按 user） |

**关键设计决策**

1. **实体吃 Scope，会话按 user**。同租户成员共享 agent 可见性（org 成员可与租户内的 agent 对话），但 **`activeAgentChatKey` 仍是 `(userID, agentID)` 分区** —— 会话是「某人 ↔ 某 agent」的一对一私人对话，不做租户共享。因此 `GetActiveAgentChat` / `ListAgentChatSessions` / `GetAgentChatByID` 内部一律用 `sc.UserID`，只有 `agentForUserUnsafe`（agent 可见性）走 `visibleToScope`。
2. **`isValidRole` 只接受 `pm/developer/reviewer/custom`**，没有 `executor`。写 agent 相关测试时别踩这个坑。

**踩坑（🔴 下次必须避开）**

1. 🔴 **`middleware.Scope()` 不能返回零值**。handler 一旦用 `currentScope(c)`，任何漏挂 `OrgScope` 中间件的路径（尤其是直接 `c.Set("user_id", ...)` 的测试）都会拿到 `UserID == ""` 而 401。修复方式是让 `Scope()` 在拿不到租户上下文时退回 `store.Scope{UserID: UserID(c)}` —— 一处修复覆盖全部 12 个失败测试，也防御未来漏挂中间件的路径。
2. **单参数调用会漏网**。批量改调用点时，正则若写死 `\(userID,` 就匹配不到 `ListAgents(userID)` 这种首参后紧跟 `)` 的形式，必须补 `PATTERN_SINGLE`。
3. **Go map 遍历序随机**。冒烟对比「无头 vs 真实头」的返回集时，首条元素不同是**正常的**（不是回归）；要按**集合差集**比对，不要按首条或顺序。实测 action-items 两边均 7 条、差集为 0。

### 2-4 Knowledge + WorkflowTemplate 归属收敛（已完成并上线）

| 项 | 内容 |
|---|---|
| 签名收敛 | `store_knowledge.go` 7 个函数收 Scope：`CreateKnowledgeDocument`（写路径）/ `GetKnowledgeDocument` / `ListKnowledgeDocuments` / `UpdateKnowledgeDocument` / `DeleteKnowledgeDocument` / `SearchKnowledgeChunks` / `ValidateProjectOwnership`；`store_workflow_template.go` 10 个：CRUD + `CopyWorkflowTemplate` / `InheritWorkflowTemplate` / `ComputeWorkflowSyncDiff` / `ApplyWorkflowSync` / `DetachWorkflowFromTemplate` |
| 写路径落点 | `CreateWorkflowTemplate` / `CopyWorkflowTemplate` 的 `OrgID` 改 `resolveOwnerOrgUnsafe(sc)` |
| Handler | `knowledge.go` 8 处 + `workflow_template.go` 10 处 + `project_file.go` 1 处鉴权行改 `currentScope`；`project.go` 的 `InheritWorkflowTemplate` 调用点 |
| 内部路径 | `assistant/tools.go`(1) / `clawsynapse/webhook.go`(2) 显式传 user-only Scope，语义与改造前一致 |
| 测试 | 新增 4 个：`TestKnowledgeScopedVisibility` / `TestCreateKnowledgeDocOwnerOrg` / `TestWorkflowTemplateScopedVisibility` / `TestValidateProjectOwnershipScoped` |

#### 🔴 补掉阶段 1 的两处双写漏网

1. **`CreateKnowledgeDocument` 从未设置 `doc.OrgID`**。集合里现有的 `org_id` 全是阶段 1 回填脚本写的（记录创建于 8 月，早于回填）。不补的话，2-4 收敛后新建的知识库文档 `org_id` 恒为空 → `visibleToScope` 永远退回 user 维度 → **企业租户下建的文档对同租户其他成员不可见**（与 2-2 写路径同类的功能缺陷）。
2. **`knowledge.Processor` 构造 chunk 时漏了 `OrgID`**。chunk 从 `doc.UserID` 派生但没有 `doc.OrgID`，而文本检索的 Mongo 过滤是 `{"user_id": ...}` → 已改为租户感知后，缺 `org_id` 的 chunk 会检索不到。

#### 关键设计决策

1. **List 走「双路径」**。`userKnowledgeDocs` / `userWorkflowTemplates` 是 **user 分区 map**（`map[userID]→[]id`）。若只改单条裁决，会出现「Get 能打开同租户成员的资源、List 却列不出来」的行为不一致。做法：
   - **无租户上下文** → 走分区索引，**零改动、零回归、零额外开销**
   - **带租户上下文** → 全量扫描 + `visibleToScope` 裁决（并按原有顺序排序：模板新→旧，文档旧→新）
   这与阶段 2-7 处理 Event/Notification 分区 map 的思路一致。
2. **检索过滤租户感知**。`SearchKnowledgeChunks` 的 Mongo 过滤与 `vectorSearch` 的 Qdrant 过滤，均为「带租户上下文按 `org_id`，否则按 `user_id`」。
3. 🔴 **store 层信任传入的 Scope，成员合法性由 `middleware.OrgScope` 保证**。`ownedByOrg` 只比较 `sc.OrgID == ownerOrgID`，不重复校验成员关系 —— 非成员带伪租户头在中间件就被 401 拦掉了。写测试时**必须**用「真实成员 + 目标租户」的组合，构造 `Scope{UserID: 非成员, OrgID: 某租户}` 会得到无意义的结论（作者本人的未回填资源仍可见）。

**踩坑（🔴 下次必须避开）**

1. **CRLF 文件的多行模式必须带 `\r\n`**。`store_workflow_template.go` 是纯 CRLF，脚本里写 `\n` 的单行/多行模式全部匹配失败（报 SKIPPED），而**不含换行的单行模式反而能命中** —— 这种「部分成功」最容易漏。脚本输出一定要逐条核对 `ok/SKIPPED`。
2. **handler 加 `userID := sc.UserID` 会造成大量 `declared and not used`**。正确做法是**按函数体静态分析**删除未使用的声明（去掉声明行后正则搜 `\buserID\b`），而不是编译报错一行行删。本次 knowledge.go 删 6 处、workflow_template.go 删 10 处。
3. **内部辅助函数的 `userID string` 形参要一并改**。`textSearch` / `vectorSearch` 不是 gin handler，持有的是裸 `userID`；改成 `sc store.Scope` 后检索才可能租户感知。

**后续批次排期**

| 批次 | 范围 | 状态 |
|---|---|---|
| 2-1 | Project（7 函数 + handler + 裁决层） | ✅ 已上线 |
| 2-2 | Task 归属收敛 | ✅ 已上线 |
| 2-3 | Agent 归属收敛 | ✅ 已上线 |
| 2-4 | Knowledge + WorkflowTemplate | ✅ 已上线 |
### 2-5a File 归属收敛（已完成并上线）

| 项 | 内容 |
|---|---|
| 签名收敛 | `store_project_file.go` 11 个函数收 Scope：`SaveProjectFile`（写路径）/ `CreateFolder` / `RenameProjectFile` / `MoveProjectFile` / `BatchDeleteProjectFiles` / `ListProjectFiles` / `GetProjectFile` / `DeleteProjectFile` / `GetProjectFileTree` / `BrowseProjectFiles` / `ListArtifactGroups`；`store_artifact.go` 的 `BindArtifactOutput` |
| 写路径落点 | `SaveProjectFile` 补 `pf.OrgID = s.resolveOwnerOrgUnsafe(sc)` |
| 项目归属 | 11 处 `project.UserID != userID` 统一改 `!s.projectVisible(sc, project)`，继承 2-1 的**项目成员白名单**语义 |
| Handler | `project_file.go` 11 处调用点 + `task.go` 的 `BindTodoOutput` 鉴权行改 `currentScope` |
| 内部路径 | `store/meeting.go` 落会议纪要时显式传 `Scope{UserID: ownerID}` |
| 测试 | 新增 3 个：`TestProjectFileScopedVisibility` / `TestSaveProjectFileOwnerOrg` / `TestBindArtifactOutputScoped` |

#### 🔴 补掉阶段 1 的一处双写漏网

**`SaveProjectFile` 从未设置 `pf.OrgID`**。与 2-2 `CreateTask`、2-4 `CreateKnowledgeDocument` 同类：不补的话，企业租户下上传的文件 `org_id` 恒为空，`visibleToScope` 永远退回 user 维度，**对同租户其他成员不可见**。

#### 关键设计决策

1. **项目文件的归属继承项目，不独立裁决**。文件自身只有 `UploadedBy`，归属判断一律走 `pf.ProjectID` → 项目 → `projectVisible(sc, project)`。这样**项目成员白名单**自动生效，与文件是谁上传的无关 —— 与「项目内资源共享」的产品语义一致。
2. **`GetProjectFileByID` / `GetProjectFileByTransferID` 保持无 Scope**。这两个供 agent 下载端点使用，agent 凭 download token 鉴权，不经过用户会话。

**踩坑（🔴 下次必须避开）**

1. **`projectVisible` 是 `*Store` 的方法，替换时漏了接收者**。批量把 `project.UserID != userID` 换成 `!projectVisible(sc, project)` 后，编译报 11 处 `undefined: projectVisible`。批量替换**凡是调方法的地方一律带 `s.`**，改完立刻 `go build`。
2. **`GetProjectFileTree` 的拒绝路径返回空树而非 nil**。写断言 `got != nil` 会误判成放行 —— 正确断言是 `got != nil && (len(got.Uploads) != 0 || len(got.Tasks) != 0)`。
3. **冒烟选项目不能只按「有 project_files 记录」挑**。本次首次冒烟选中的项目 Mongo 里有 106 条记录，但容器磁盘上 `uploads/` 目录根本不存在（历史遗留），详情路径必然 404，与改造无关却会污染结论。**挑项目时要 Mongo 记录与容器磁盘双向核对**。

**后续批次排期**

| 批次 | 范围 | 状态 |
|---|---|---|
| 2-1 | Project（7 函数 + handler + 裁决层） | ✅ 已上线 |
| 2-2 | Task 归属收敛 | ✅ 已上线 |
| 2-3 | Agent 归属收敛 | ✅ 已上线 |
| 2-4 | Knowledge + WorkflowTemplate | ✅ 已上线 |
| 2-5a | File（project_file + artifact 绑定） | ✅ 已上线 |
### 2-5b Comment + Meeting 归属收敛（已完成并上线）

| 项 | 内容 |
|---|---|
| 签名收敛 | `workflow.go` 2 个：`AddTaskComment`（写路径）/ `ListTaskComments`；`meeting.go` 10 个：`CreateMeeting` / `GetMeeting` / `ListMeetings` / `ListMeetingsByStatus` / `UpdateMeetingStatus` / `AddMeetingMessage` / `ListMeetingMessages` / `UpdateMeetingSummary` / `UpdateMeetingMinutes` / `AddMeetingTodo` / `GenerateMeetingMinutesFile` |
| 写路径落点 | `CreateMeeting` 的 `OrgID` 改 `resolveOwnerOrgUnsafe(sc)`；`addCommentUnsafe` 的 `OrgID` 改继承 `task.OrgID` |
| Handler | `meeting.go` 8 处鉴权行 + 12 处调用点；`task.go` 评论 2 处；`project_file.go` 纪要恢复路径 |
| 内部路径 | `clawsynapse/webhook.go` 30 处 + `timeout_monitor.go` 4 处 + `meeting.go` 4 处改用 `SystemScope()` |
| 测试 | 新增 4 个：`TestMeetingScopedVisibility` / `TestSystemScopeBypassesMeetingOwnership` / `TestCreateMeetingOwnerOrg` / `TestTaskCommentScopedAndOrgInheritance` |

#### 🔴 修掉一个改造前就存在的越权漏洞

`GetMeeting(_ string, ...)` / `ListMeetings(_ string, ...)` / `ListMeetingsByStatus` /
`ListMeetingMessages` / `UpdateMeetingSummary` / `UpdateMeetingMinutes` /
`AddMeetingTodo` 七个函数的 userID 形参**被写成 `_`（完全丢弃）**，HTTP 请求
不做任何归属校验 —— 任何登录用户凭 meetingID 就能读写任意会议、发消息、加待办。
`AddMeetingMessage` 干脆没有 userID 形参。内部路径（agent webhook 20+ 处）则
一律显式传 `""` 绕过。

#### 🔴 补掉阶段 1 的两处双写漏网

1. **`CreateMeeting` 恒挂作者个人租户**。企业租户下建的会议 `org_id` 是个人
   租户，同租户成员带租户头看不到（与 2-2 / 2-4 / 2-5a 同类）。
2. **`addCommentUnsafe` 的 `OrgID` 从 `personalOrgOfUnsafe(task.UserID)` 派生**。
   评论是任务的附属资源，归属必须跟随任务，不能跟作者 —— 否则企业租户下
   评论挂个人租户、与所属任务的 org 不一致。

#### 关键设计决策：`SystemScope()` 显式系统旁路

恢复校验后，内部路径（agent webhook 30 处、timeout_monitor 4 处）必须保持
零回归。这里**不能**靠「UserID 为空」蒙混放行 —— 那是「忘了传 userID」和
「故意放行」分不清，等于给越权留后门。做法：

```go
type Scope struct {
    UserID string
    OrgID  string
    Role   string
    // System 标记内部系统路径（agent webhook / timeout_monitor / 后台定时器），
    // 不做归属裁决直接放行。🔴 HTTP 路径永远拿不到 System Scope。
    System bool
}

func SystemScope() Scope { return Scope{System: true} }
```

`visibleToScope` / `projectVisible` / `meetingVisible` 一律在开头 `if sc.System
{ return true }`。**零值 `Scope{}` 不是系统旁路** —— 这条断言已写进
`TestSystemScopeBypassesMeetingOwnership`，是这个安全属性的回归守卫。

配套：`meetingVisible(sc, m)` 优先走项目裁决（继承 2-1 的项目成员白名单），
项目不在内存（懒加载边界）时退回会议自身的 `OrgID` / `CreatorID`。

**踩坑（🔴 下次必须避开）**

1. **`meetingVisible` 由调用者持锁**（与 `projectVisible` 同一约定）。`GetMeeting`
   原本是 `RLock` → `RUnlock` → 返回，裁决必须挪进锁内；懒加载分支同理。
   搞反会 data race，加锁则会死锁（Go 的 `sync.RWMutex` 不可重入）。
2. **Go 编译器默认最多报 10 个错误**。`webhook_transfer_test.go` 实际有 6 处
   `ListTaskComments` 调用点，编译器只报了前 4 处 —— 按报错数写 `expect` 会漏。
   批量替换前先用 `grep -c` 数真实总数。
3. **`GenerateMeetingMinutesFile` 的 `ownerID := userID` 兜底逻辑要按 Scope 语义
   重写**：HTTP 路径用请求者身份做真实校验，只有 `sc.System` 才兜底到项目
   owner / 会议创建者。原代码无锁读 `s.projects`（既有 race），顺手用 RLock 包住。

**后续批次排期**

| 批次 | 范围 | 状态 |
|---|---|---|
| 2-1 | Project（7 函数 + handler + 裁决层） | ✅ 已上线 |
| 2-2 | Task 归属收敛 | ✅ 已上线 |
| 2-3 | Agent 归属收敛 | ✅ 已上线 |
| 2-4 | Knowledge + WorkflowTemplate | ✅ 已上线 |
| 2-5a | File（project_file + artifact 绑定） | ✅ 已上线 |
| 2-5b | Comment + Meeting | ✅ 已上线 |
| 2-6 | JoinRequest + ExternalApp | ✅ 已上线 |
| 2-7 | Event + Notification 分区 map（§4.3） | ✅ 已上线 |



#### 阶段 2-6 JoinRequest + ExternalApp（commit 见 git log，已上线）

#### store 层（11 个函数收 Scope）
- `store_join_request.go` 5 个：`ListJoinRequests` / `GetJoinRequest` /
  `ApproveJoinRequest`（写路径）/ `RejectJoinRequest` / `PendingJoinRequestCount`
- `store_external_app.go` 6 个：`CreateExternalApp`（写路径）/ `ListExternalApps` /
  `GetExternalApp` / `UpdateExternalApp` / `DeleteExternalApp` /
  `GetExternalAppForLaunch` / `RecordExternalAppLaunch`

#### 归属语义（与前面批次不同，需注意）
- **JoinRequest 的 userID 是邀请人，不是审批人**。申请由 agent 内部路径
  （`trust_sync.go` → `CreateJoinRequest`）创建，无租户上下文，恒挂邀请人个人租户。
- 「仅同租户」收敛后：同企业租户的其他成员**看不到**邀请人个人租户里的申请 ——
  「仅同租户」语义天然满足；跨成员共享邀请待阶段 3 节点 org 绑定后再开。
- `joinRequestVisible` 增加**邀请人兜底**：`jr.UserID == sc.UserID` 恒可见。
  否则邀请人切到企业租户审批时按 org 比对看不到自己的邀请（org 是个人租户），
  形成功能死锁。自己的数据对自己可见不构成越权。
- ExternalApp 保持既有 `canAccessExternalApp` 可见性模型：public 对所有人可见，
  private 走 `visibleToScope`（org 比对优先，作者兜底）。

#### 补阶段 1 双写漏网（累计第 6 处）
- **`ApproveJoinRequest` 审批创建/恢复 agent 的构造体从未设 `OrgID`** ——
  企业租户下审批出的 agent 对同租户其他成员不可见。新建与恢复归档两条路径
  都补 `s.resolveOwnerOrgUnsafe(sc)`；`jr.OrgID` 同步按审批上下文重解析。

#### handler 层
- `join_request.go`：List / Approve（GetJoinRequest + ApproveJoinRequest）/
  Reject 共 5 处调用点 + 2 处鉴权行；GetInvitePrompt 保留 userID（提示词需要）。
- `external_app.go`：6 个函数全部改 `sc := middleware.Scope(c)`；Launch 里
  `FindUserByID(sc.UserID)` 保留 user 维度（SSO token 签发身份）。

#### 测试（3 个新增）
`TestJoinRequestScopedVisibility` / `TestApproveJoinRequestOwnerOrg` /
`TestExternalAppScopedVisibility`。零值 Scope 非系统旁路断言沿用 2-5b 守卫。

#### 部署冒烟
- JoinRequest 列表 × 3 种租户头：无头 200 / 真实头 200 / 伪头 401，集合差集 0。
- ExternalApp 列表 + 详情 × 3 种租户头：同上，集合差集 0；创建→详情→删除闭环。
- 零 5xx 零 error。

### 阶段 2-7 Event + Notification（commit 见 git log，已上线）—— 阶段 2 收官

#### 改动面
- Event 域已在 2-2 收敛（`ListTaskEvents` / `ListUserEvents` / `ListAgentEvents`），
  本批无需改动。
- `notification.go` 4 个函数收 Scope：`ListNotifications` / `UnreadNotificationCount` /
  `MarkNotificationRead` / `MarkAllNotificationsRead`。
- handler `notification.go` 4 处鉴权行 + 4 处调用点。

#### 归属语义：通知是 user 维度 feed
- 通知天然发就发给某个人（`userNotifications` 分区 map 按 userID 索引），
  **不按租户共享、不因租户上下文改变归属** —— 与 2-3 会话、2-2 ListUserEvents 同款语义。
- 分区 map 保留（无租户头零开销；有租户头也不需要全量扫描 —— 通知不存在
  「同租户成员可见」场景）。收 Scope 参数只为调用侧统一，`MarkNotificationRead`
  原有的 `n.UserID != userID` 校验就是归属校验。
- 写路径（`addEventUnsafe` / `maybeCreateNotificationUnsafe`）是 agent 内部路径，
  `OrgID` 挂目标用户个人租户 —— 与 JoinRequest 同款，阶段 3 节点 org 绑定后收口。

#### 测试（1 个新增）
`TestNotificationUserScopedFeed`：本人无头/带头都可见、他人不可见、
跨用户标记已读被拒、本人标记成功、全部已读幂等。

#### 部署冒烟
通知列表 + 未读数 × 3 种租户头：无头 200 / 真实头 200 / 伪头 401；
无头与真实头集合差集 0（50=50 条），未读数一致（20=20）；零 5xx 零 error。

#### 🎉 阶段 2 全部完成
2-1 Project / 2-2 Task / 2-3 Agent / 2-4 Knowledge+Template / 2-5a File /
2-5b Comment+Meeting / 2-6 JoinRequest+ExternalApp / 2-7 Event+Notification
共 8 批全部上线，累计 25 个新测试。剩余：阶段 3（节点 org 绑定，跨仓库）、
阶段 4（前端双轨）、阶段 5（越权测试）。

## 阶段 3 — 节点 org 绑定（✅ 已完成，2026-09-05 方向修订：协议零改动）

> **方向修订（用户拍板）**：放弃 subject 插 org 段方案，改为「org 绑定进现有字段」——
> 节点只订阅自己的 inbox（`clawsynapse.msg.<nodeID>.inbox`，service.go:76 点对点投递），
> 隔离由「点对点投递 + backend 只派发给 org 内 agent 的节点 + 业务层 scope 裁决」保证，
> 与原方案强度等价。**clawsynapse 仓库零改动、不发镜像**（R3 高风险消除）。
> 未来若需 org 级广播（会议室全员/公告），用 node_id 内嵌 org 前缀 + 通配订阅，协议依然零改。

| 项 | 内容 |
|---|---|
| **改动** | 平台侧 only：审批带企业头 → `agent.org_id` 挂企业（2-6 已实现 `resolveOwnerOrgUnsafe(sc)`） |
| **闭环实测** | ① 新节点容器（v1.0.35 现有镜像）auth challenge → trust request → 平台 sync 出 JoinRequest<br>② A 带企业头 `POST /agents/join-requests/:id/approve` → **agent.org_id = 企业 org** ✅<br>③ `PATCH /agents/:id {"role":"pm"}`（trust reason role 需在白名单 pm/developer/reviewer/custom 内，否则降级 custom）<br>④ **企业租户建项目 201（409 解除）** ✅<br>⑤ B(member) 带头读项目 200 / 无头 404 / 带头建 meeting 201 / 建 task 201 ✅ |
| **踩坑** | ① 节点 NODE_ID 是自动生成 `n1-<hash>`，env NODE_ID 不影响 nodeID<br>② 节点容器内无 curl/wget，trust request 用宿主机 curl 打映射端口<br>③ trust request 的 reason 是**内嵌 JSON 的字符串**不是对象<br>④ trust request 前必须先 `POST /v1/auth/challenge` 完成 auth 握手<br>⑤ 已互信的 peer 不能重复发 trust request（`trust.already_trusted`）<br>⑥ 转发节点实际 nodeID 是 `n1-cb979d6a…`（env NODE_ID=trustmesh-server 未生效） |
| **遗留可选** | ① 存量 agent 批量绑 org 的迁移脚本（管理 API 逐个 PATCH 也可）<br>② SZJT 远程节点加入企业的审批（走同一流程，SSH 恢复后即可）<br>③ org 级广播（二期） |

### 阶段 4 — 前端

| 项 | 内容 |
|---|---|
| **目标** | 租户切换器、企业管理、成员与权限 UI |
| 改动 | ① `client.ts` 注入 `X-Org-Id`（一行）<br>② authStore 增加 `activeOrgId` + 持久化<br>③ 侧边栏底部新增**企业切换器**（与现有主题/收起图标行并列，复用 `.tm-iconbtn` 样式或加下拉）<br>④ 企业管理页：成员管理（代建账号/改角色/停用）、项目成员设置、配额展示、节点审批<br>⑤ 权限驱动 UI（按 role 显隐管理入口） |
| 验收 | 切换企业后数据完全隔离；Member 看不到管理入口；切企业不重登 |
| 回滚 | 前端独立部署，回滚镜像即可 |
| 风险 | 低-中（frontend / frontend-v2 **双轨需同步**，别忘了旧前端 3000 端口那份） |

### 阶段 5 — 越权专项测试与灰度

| 项 | 内容 |
|---|---|
| **目标** | 用攻击视角验证隔离无洞 |
| 内容 | 见 §7 测试清单 |
| 验收 | 全部用例通过，无跨 org 数据泄露 |
| 回滚 | — |
| 风险 | — |

---

## 6. 迁移脚本清单

| 脚本 | 用途 | 备注 |
|---|---|---|
| `deploy/org_backfill.js`（已落地） | 存量 user → 个人 org；各集合回填 `org_id` | `mongosh trustmesh --eval "var APPLY=false;" org_backfill.js` 预演，`APPLY=true` 落盘；幂等，重跑安全 |
| `deploy/org_verify.js`（已落地） | 回填校验：覆盖计数 + 跨集合一致性 + 孤儿归属 | 校验不通过立即停止后续阶段 |
| `deploy/.tmp/patch_stage1_models.py`（已落地） | 7 个实体补 `org_id` 字段 | 按文件主导换行符回写，插完校验无 MIXED |
| `deploy/.tmp/patch_stage1_store.py`（已落地） | `EnsurePersonalOrg` 拆 unsafe + `CreateUser` 接线 | 持锁路径必须调 unsafe 版，否则死锁 |
| `deploy/.tmp/patch_stage1_dualwrite.py`（已落地） | 17 处写入点双写 `org_id` | 内置持锁校验，未通过直接报错 |
| `deploy/migrate_agent_org_bind.py`（新增） | 存量 Agent 回填 org 绑定（复用 node_id ↔ org，不新发凭证） | 阶段 3 用 |
| `deploy/.tmp/tokenize_styles.py` 等 | 前端治理脚本（已有） | 与本次无关，勿混淆 |

---

## 7. 越权测试清单（阶段 5 必跑）

| # | 用例 | 期望 |
|---|---|---|
| 1 | A org 用户直接请求 B org 项目 ID | 404 / 403，不泄露存在性 |
| 2 | A org 用户请求 B org 任务 / 文件 / 评论 / 通知 | 全部拒绝 |
| 3 | A org 用户把 `X-Org-Id` 伪造成 B org | 401（membership 校验失败） |
| 4 | 私有项目非成员访问（列表/详情/任务/文件） | 全部拒绝，且列表不出现 |
| 5 | 私有项目成员访问 | 正常 |
| 6 | Member 尝试成员管理 / 节点审批 / 删项目 | 403 |
| 7 | Admin 尝试转让 Owner / 删 org | 403（仅 Owner） |
| 8 | A org 节点订阅 B org NATS subject | 拒绝 |
| 9 | 旧版 node token 在兼容期后访问 | 拒绝，提示重新审批 |
| 10 | 切企业后旧标签页带上旧 org 请求 | 数据与新 org 一致，不串 |
| 11 | 个人 org 用户（存量账号）行为 | 与改造前完全一致 |
| 12 | 代建账号首登 | 强制改密，临时密码失效 |

---

## 8. 风险登记册

| # | 风险 | 等级 | 缓解 |
|---|---|---|---|
| R1 | 🔴 内存态隔离散落约 290 处（store 90 处直接比较），改漏即越权 | 高 | Go 编译期兜底 + §7 越权测试 + 分批提交 |
| R2 | 🔴 阶段 1 回填是写操作，失败会污染数据 | 高 | 执行前 `mongodump` + git tag + 停 backend 等落盘 + 回填后校验脚本 |
| R3 | 🔴 阶段 3 **跨仓库**（ClawSynapse 镜像须同步发版）+ **跨环境**（本地 4 节点 + SZJT 远程节点） | 高 | 兼容期双订阅；镜像可回滚 `v1.0.35`；**先在 ClawSynapse 仓库落地并发布镜像，再动平台侧**；SZJT 需可用 SSH 通道；节点配置在 volume 不在 env，改法与常规不同 |
| R4 | 前端双轨（3000 / 5174）需同步改造 | 中 | 阶段 4 明确双轨清单，勿漏旧前端 |
| R5 | 个人 org 用户行为回归（存量账号体验不变） | 中 | 用例 11 专项验证 |
| R6 | 配额只占位不做计费，客户可能误以为有限额执行 | 低 | 阶段 4 UI 上标注为"预留" |

---

## 9. 本期明确不做

- Guest 只读角色（二期）
- 套餐 / 订单 / 支付 / 用量账单（二期，仅留配额字段）
- 邮箱邀请（无邮件服务；本期为管理员代建账号）
- 库级物理隔离（内存态为主状态，物理隔离收益低、成本高）
- NATS 事件发布侧按订阅者过滤（出口已被 API 层过滤覆盖）
- AI Office 3D 的可见性投影（不涉项目内容，保持 org 级视图）
- SSO / 企业微信钉钉集成（二期）
