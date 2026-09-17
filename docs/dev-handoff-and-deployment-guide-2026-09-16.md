# TrustMesh 开发进度、环境指南与智能体交接规范

> **文档版本**：v3.0（2026-09-16 权威交接版）  
> **适用对象**：后续接手开发的 AI 智能体（Agent）及研发工程师  
> **核心原则**：严格遵守开发铁律、先方案后执行、生产唯一活动环境、禁止本地触发线上共享任务。

---

## 目录
1. [项目全景与最新开发进度](#1-项目全景与最新开发进度)
2. [环境信息与拓扑架构](#2-环境信息与拓扑架构)
3. [本地开发与编译指令（无本地Go工具链）](#3-本地开发与编译指令无本地go工具链)
4. [生产部署与镜像更新标准操作程序 (SOP)](#4-生产部署与镜像更新标准操作程序-sop)
5. [架构设计与开发核心铁律 (必读)](#5-架构设计与开发核心铁律-必读)
6. [后续待开发任务卡与交接清单](#6-后续待开发任务卡与交接清单)

---

## 1. 项目全景与最新开发进度

### 1.1 阶段交付全景状态

| 阶段 | 阶段名称 | 状态 | 核心交付内容 / 生产证据 |
|---|---|---|---|
| **阶段 0** | **紧急止血 (T0.0–T0.12)** | ✅ **100% 已上生产** | 测试前置 DoD、隔离收尾、错误处理吞错修复（7处 `ShouldBindJSON`）、消息双读、计数器修复、对账器自愈拉起未推进 pending 任务、30s 有界持久化关机屏障。 |
| **阶段 1** | **隔离加固与单轨收敛 (T1.0–T1.10)** | ✅ **已上生产** (T1.10暂缓) | 旧前端归档（`archive/frontend-legacy`）、前端单轨化（React19+antd v5）、多租户第二租户真实冷启动全流程打通、会议写透与会话态恢复、CI 门禁强化、工作流模板沉淀上线。T1.10 媒体规格暂缓待产品确认。 |
| **阶段 2** | **Mongo 权威与状态外置 (T2.1–T2.6)** | ✅ **四域权威已交付** | **T2.1 会议域**、**T2.2 任务/待办域**、**T2.3 项目域**、**T2.3b 项目文件域**已全部实现 Mongo 权威并上生产；**T2.6 幂等键**已接入会议消息（5m TTL）。**T2.5 细粒度锁调用点已安全回退**（见 1.2）。 |
| **阶段 3** | **弹性与水平扩展 (T3.1)** | ✅ **100% 已上生产 (双实例)** | **T3.1 无状态 Backend + 双实例水平扩展已全量上线**：分布式 Mongo Leader 选举租约（`leader_leases`）、跨实例 SSE 广播 Outbox（`user_events`，延迟 0.286s）、Nginx 容器 DNS 别名轮询分发、故障 30s 自动接管已完成生产灰度并稳定运行。 |
| **阶段 4** | **商业化收口** | ⏳ **规划中** | 货币化配额强制（G4已记已知缺口）、质量看板（卡 T1.10）。岗位市场暂缓。 |

### 1.2 生产镜像与最新回滚锚
- **后端 (Backend)**：`trustmesh/backend:20260917-081102`（commit `7a42e7b`：外部应用三级作用域（global/org/personal）+ 平台命名空间 `/api/v1/platform/external-apps`）。
  - *双实例拓扑*：`trustmesh-backend` (主/8080端口映射) + `trustmesh-backend-2` (副本/DNS别名轮询)，本轮按「先 backend-2 后 backend」滚动替换。
  - *回滚锚点*：`20260916-221319`（双实例用户态分歧修复）/ `20260916-214416`（平台用户管理首版）/ `20260916-203600`（三层权限体系）。
- **前端 (Frontend-v2)**：`trustmesh/frontend-v2:20260917-081102`（commit `7a42e7b`：外部应用页面改三级标签页、平台侧新增「外部应用」菜单与 `/platform/external-apps` 页面）。
  - *回滚锚点*：`20260917-072506`（「组织」文案 + 个人信息页收纳）/ `20260916-214416`（平台用户管理 + 全局菜单基线）。

### 1.2.1 双实例用户态分歧缺陷（2026-09-16 已修复并上生产）
- **现象**：实例 A 重置密码/禁用账号后，命中实例 B 的请求仍用旧内存态校验 —— 同一密码 10 次登录 6 成功 4 次 `INVALID_CREDENTIALS`；被禁用账号仍能拿到 access token。
- **二次伤害**：用户写路径是整文档 `ReplaceOne`，实例 B 随后的任意用户写会把实例 A 刚写的 `disabled=true` 覆盖丢失（实测 Mongo 里禁用标记消失）。
- **修复**：`FindUserByEmailAuthoritative` / `FindUserByIDAuthoritative`（Mongo 优先，缺失/不可用时回落内存，刻意不回填内存）用于登录与 refresh；`UpdateUserPassword` / `SetUserDisabled` 改 `persistUserFieldsUnsafe`（`$set`/`$unset`）字段级落库，免疫跨实例整文档覆盖。
- **生产验收证据（20260916-221319）**：新密码跨实例登录 20/20 成功（修复前 6/10）；禁用后两实例登录与 refresh 均 403 `USER_DISABLED`；「A 禁用 → B 重置密码」后 Mongo 仍 `disabled=true` 且登录仍 403。
- **仍存留的同类风险**：`FindUserByEmail/ID` 等普通读路径仍只读进程内存（读侧陈旧窗口为既定取舍）；`SeedPlatformAdmins` 等仍走整文档 `persistUserUnsafe`，跨实例并发种子同步理论上仍可互相覆盖。

### 1.3 关键技术决策备忘：T2.5 细粒度锁回退（HEAD 7396a2c）
- **决策**：回退四域调用点迁移，store 运行时完全恢复全局锁基线 `4bcff06`；**保留锁基础设施**（`store_lock.go`、`Aggregate` 枚举、升序取锁、死锁压测及「一张 map 必须一把锁」永久护栏）。
- **根因铁律**：**同一张 map 的守护锁只能有一把，细粒度锁绝不能增量/分批迁移访问者集合**。只要有 1 个函数仍持全局 `s.mu` 访问同张 map，互斥即失效并引发 `-race` 数据竞争崩溃。后续若重启 T2.5，必须 44 函数/11 文件原子性一次性整批迁移。

---

## 2. 环境信息与拓扑架构

### 2.1 生产服务器访问凭据
- **主机 IP**：`175.27.135.91`
- **SSH 端口**：`62000`（⚠️ 端口 22 已关闭/过滤）
- **登录用户**：`cloudUser`（禁止用 root/ubuntu/jiey，不支持密码登录）
- **私钥路径**：`D:/AIWorkspace/公司/vic_key.pem`
- **连接命令**：
  ```bash
  ssh -i "D:/AIWorkspace/公司/vic_key.pem" -p 62000 -o StrictHostKeyChecking=no cloudUser@175.27.135.91
  ```
- **生产目录**：`/opt/trustmesh-test/`（docker-compose 编排目录）。执行 docker 命令需 `sudo`。
- **特别注意**：`film-mcp-*` 容器为外部服务，切勿随意停止；服务器上 curl 命令必须带 `--noproxy '*'`。

### 2.2 🔴 共享 NATS 铁律（方案 A）
- **现状**：本地开发环境与生产环境共用生产 NATS 消息总线（`nats://175.27.135.91:4222`）。
- **红线禁令**：
  1. **严禁在本地开发环境创建任务、触发 PM 规划节点或重跑历史任务**。本地一旦触发，会被生产上的 Agent 抢单消费导致生产脏数据。
  2. **本地运行的 4 个 ClawSynapse 容器是生产核心执行节点**：
     - `clawnsynapse`（总控节点 `n1-9881a46a`）
     - `clawnsynapse3`（导演节点 `n1-0b7fb13b`）
     - `clawnsynapse4`（编剧节点 `n1-3945d737`）
     - `clawnsynapse-default`
     **这 4 个本地 Docker 容器严禁停止！**
  3. **远程 SZJT 节点**：资产(102)、视频(104)、分镜(103) 由远程执行。
  4. **Trust 信任链**：已注册 agent 切勿清空 `trust.json`（会导致 `AGENT_NODE_ID_EXISTS` 僵尸态）；修改生产 `trust.json` 后必须 `chown 100:101 && chmod 600`。

---

## 3. 本地开发与编译指令（无本地Go工具链）

Windows 开发宿主机上**未安装原生 Go 工具链**，所有后端编译、测试与构建均通过容器运行。

### 3.1 后端编译与类型检查
在项目根目录运行 Docker 容器进行编译：
```bash
# 标准极速构建检查（无 gcc）
"/c/Program Files/Docker/Docker/resources/bin/docker.exe" run --rm \
  -v "D:/AIWorkspace/TrustMesh:/repo" \
  -v trustmesh-gomod:/go/pkg/mod \
  -v trustmesh-gocache:/root/.cache/go-build \
  -w /repo/backend \
  -e CGO_ENABLED=0 -e GOPROXY=off -e GOFLAGS=-mod=mod \
  golang:1.25.1-alpine sh -c "go build ./..."
```

### 3.2 单元测试与竞态检查 (-race)
涉及 `-race` 检测时，需使用具备 gcc 的标准 golang 镜像：
```bash
"/c/Program Files/Docker/Docker/resources/bin/docker.exe" run --rm \
  -v "D:/AIWorkspace/TrustMesh:/repo" \
  -v trustmesh-gomod:/go/pkg/mod \
  -v trustmesh-gocache:/root/.cache/go-build \
  -w /repo/backend \
  -e CGO_ENABLED=1 \
  golang:1.25.1 sh -c "go test -race -v ./internal/store/..."
```

### 3.3 活库隔离测试配方 (Live Mongo Test Recipe)
`*_mongo_authority_test.go` 需要真实的 Mongo 实例（未指定 URI 会直接 `t.Skip`）。**严禁连接生产 Mongo！** 使用本地独立临时容器测试：
```bash
# 1. 创建独立测试网络与容器
docker network create tm-live
docker run -d --name tm-live-mongo --network tm-live mongo:7

# 2. 跑权威测试（必须确认 SKIP=0）
docker run --rm --network tm-live \
  -v "D:/AIWorkspace/TrustMesh:/repo" \
  -v trustmesh-gomod:/go/pkg/mod \
  -v trustmesh-gocache:/root/.cache/go-build \
  -w /repo/backend \
  -e CGO_ENABLED=1 \
  -e TRUSTMESH_TEST_MONGO_URI=mongodb://tm-live-mongo:27017 \
  -e TRUSTMESH_TEST_MONGO_DB=trustmesh_test \
  golang:1.25.1 sh -c "go test -race -v ./internal/store/... -run TestMongoAuthority"

# 3. 清理临时环境
docker rm -f tm-live-mongo && docker network rm tm-live
```

### 3.4 前端开发与检查 (frontend-v2)
单轨目录 `frontend-v2/`（React 19 + antd v5，纯内联 style）：
```bash
cd frontend-v2

# 类型检查
npx tsc -p tsconfig.app.json --noEmit

# 代码风格检查 (当前允许最多 4 个 warnings)
npm run lint -- --max-warnings=4

# 单元测试
npx vitest run
```

---

## 4. 生产部署与镜像更新标准操作程序 (SOP)

为防止构建环境污染，生产部署必须执行 **源码干净导出（git archive）→ 容器构建 → 保存压缩 → 上传加载 → 优雅平滑重启**。

### 4.1 后端部署步骤 (Backend SOP)

```bash
# 1. 导出当前 commit 源码到独立临时目录（避免未提交脏文件进入镜像）
mkdir -p _build_backend
git archive HEAD:backend | tar -x -C _build_backend

# 2. 构建 linux/amd64 生产镜像
TAG=$(date +"%Y%m%d-%H%M%S")
docker build --platform linux/amd64 -t trustmesh/backend:$TAG _build_backend

# 3. 校验镜像特征符号（确保代码确实打入）
# 必须能 grep 到核心函数名，例如: dispatchOrgBlocked / judgeDeliverableQuality

# 4. 保存并压缩镜像
docker save trustmesh/backend:$TAG | gzip > backend-$TAG.tar.gz

# 5. 上传至生产服务器
scp -P 62000 -i "D:/AIWorkspace/公司/vic_key.pem" backend-$TAG.tar.gz cloudUser@175.27.135.91:/opt/trustmesh-test/

# 6. 生产端加载镜像并优雅重启
ssh -i "D:/AIWorkspace/公司/vic_key.pem" -p 62000 cloudUser@175.27.135.91 << 'EOF'
  cd /opt/trustmesh-test
  sudo docker load -i backend-*.tar.gz
  sudo docker stop -t 90 backend
  sudo env TAG=<TAG_VALUE> docker compose up -d --no-deps backend
  rm -f backend-*.tar.gz
EOF
```

### 4.2 前端部署步骤 (Frontend-v2 SOP)
```bash
# 1. 导出前端源码
mkdir -p _build_frontend
git archive HEAD:frontend-v2 | tar -x -C _build_frontend

# 2. 构建并推送部署
TAG=$(date +"%Y%m%d-%H%M%S")
docker build --platform linux/amd64 -t trustmesh/frontend-v2:$TAG _build_frontend
docker save trustmesh/frontend-v2:$TAG | gzip > fv2-$TAG.tar.gz
scp -P 62000 -i "D:/AIWorkspace/公司/vic_key.pem" fv2-$TAG.tar.gz cloudUser@175.27.135.91:/opt/trustmesh-test/

# 3. 生产端加载并重启
ssh -i "D:/AIWorkspace/公司/vic_key.pem" -p 62000 cloudUser@175.27.135.91 << 'EOF'
  cd /opt/trustmesh-test
  sudo docker load -i fv2-*.tar.gz
  sudo docker stop -t 30 frontend-v2
  sudo env TAG=<TAG_VALUE> docker compose up -d --no-deps frontend-v2
  rm -f fv2-*.tar.gz
EOF
```

---

## 5. 架构设计与开发核心铁律 (必读)

在修改 TrustMesh 代码时，必须严格遵守以下设计契约：

1. **内存状态机与 Mongo 镜像双写模型**：
   - Store 层以内存 Map 为核心状态，Mongo 充当持久化镜像与权威源。
   - 写路径标准范式：`mutateXxxUnsafe`（持锁内存变更，具备快照回滚能力） $
ightarrow$ `applyXxxVersionedReplaceLocked`（乐观锁带版本号 Upsert，处理 E11000 自愈与 409 冲突）。
2. **多租户权限裁决**：
   - 无租户上下文降级为 `user` 个人维度；有租户上下文且资源回填 `org_id` 走组织级权限。
   - 写路径上的 `org_id` 必须使用 `resolveOwnerOrgUnsafe(sc)`。
3. **Typed-Nil 接口反模式（重大教训）**：
   - Go 中 `func foo() error` 如果 `return s.applyXxx(...)`（其中底层返回 `*transport.AppError`），成功时 typed nil 装入 `error` 接口会**判定为非 nil**，导致调用方误判失败甚至 panic。
   - **正确做法**：底层错误必须先显式判断 `err != nil` 再返回。
4. **Gofmt 与代码格式化陷阱**：
   - CI 严格执行 `gofmt -l .` 检查（非空即失败）。
   - 注释中如果有编号列表（1. 2. 3.），Go 1.19+ 的 gofmt 会自动重排缩进。每次改动 Go 文件后**必须执行 gofmt 格式化**，严防纯注释改动导致 CI 变红。
5. **Git 与环境特殊性**：
   - Windows Bash 环境下禁止使用管道过滤 git 输出（本地 shim 缺失可能导致静默吞字符）。
   - 禁止 `git add -A`，必须按文件路径精确暂存并核对 `git diff --cached --numstat`。
   - `frontend-v2/src/stores/authStore.ts` 包含未提交的登出租户保留逻辑改动，用户要求单独评审，**切勿顺手提交或回退**。

---

## 6. 后续待开发任务卡与交接清单

智能体接手后续开发时，请按如下优先顺序推进：

### 任务 1：#27「记住上次选中的工作空间」F2 无头窗口门控（当前第一优先级）
- **背景**：已完成 WorkspaceMemory 核心库、authStore 改造、持久化 v0 迁移及 INV-1/INV-3 不变量落地；QA 验证发现冷启动挂载期存在短暂无头（无 `X-Org-Id`）请求窗口。
- **目标**：实现 `calibrated` 状态门控与骨架屏保护，防止在工作空间校准完成前对个人空间产生误写，消除无头闪烁与写污染。
- **详见设计**：`docs/remember-last-workspace-f2-gate-design-2026-09-16.md`。

### 任务 2：#28 后端租户非成员独立错误码上线与客户端契约对齐
- **目标**：`middleware/org_scope.go` 针对租户非成员返回独立 code `NOT_A_MEMBER`（401 HTTP 状态），前端 `client.ts` 拦截后实现 0 次无谓 refresh 刷新。需安排后续后端发布。

### 任务 3：T1.10 媒体规格与 T4.3 质量看板
- **目标**：根据产品侧最终给出的视频/音频/图片交付规格，在任务验收节点增加结构化校验器与质量指标上报。

### 任务 4：T05 细粒度锁全量原子性重构（待拍板）
- **前置硬约束**：必须 44 函数/11 文件一次性原子整批迁移，禁止增量迁移单张 map 访问者；必须保持 `TestQAT25_Invariant3_SameMapMustHaveOneProtectingLock` 恒绿。
- **详见决策**：`docs/t2.5-rollback-decision-2026-09-16.md`。

---

> 💡 **智能体接手确认提示**：若开始执行具体代码任务，请务必遵循 **“先分析根因 $
ightarrow$ 输出决策与实施方案 $
ightarrow$ 经确认后方可修改代码”** 的协作模式。
