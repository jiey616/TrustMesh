# 生产部署 T1.x 批次 执行方案（PROD 175.27.135.91）

> 目标：把 `main`（HEAD `d20a85b`）部署到生产，补齐实测落后的一整个 T1.x 批次。
> 状态：✅ **已执行并验证通过**（2026-09-14 23:52–23:58，镜像 tag `20260914-234544`）。
> 基线实测时间：2026-09-14 23:40–23:50。

## 1. 范围

**backend**（重建镜像）：补齐 T1.1 / T1.6 / T1.9 / T1.10 全部缺失能力 + 本次策展（`SetWorkflowTemplateCurated`）。
**frontend-v2**（重建镜像）：T1.9 全局模板库 UI、「沉淀为模板」按钮、策展 UI（星标/精选/只看精选）。

不部署：`clawsynapse`（`test` 镜像 12 天未变，无本次改动）、旧 `frontend`（nginx 已不路由，事实下线）、`film-mcp-*`（无关，勿动）。

## 2. 生产现状（实测）

| 容器 | 镜像 | 状态 |
|------|------|------|
| trustmesh-backend | `trustmesh/backend:20260912-172408` | Up 2 天，healthy，启动 2026-09-12T09:26:10Z |
| trustmesh-frontend-v2 | `trustmesh/frontend-v2:20260911-173900` | Up 3 天，healthy |
| trustmesh-frontend（旧） | `trustmesh/frontend:20260911-124317` | Up 3 天，nginx 不路由 |
| trustmesh-clawsynapse | `trustmesh/clawsynapse:test` | Up 3 天 |
| trustmesh-qdrant | `qdrant/qdrant:latest` | ⚠️ **unhealthy** |
| trustmesh-mongo / nats | — | Up 5 周 |

**符号比对（生产二进制 `/usr/local/bin/trustmesh-server`）**

| 符号 | 结果 | 含义 |
|------|------|------|
| `FlushPersistAll` | **25（在）** | T0.8 有界停机落盘已有 → 停机安全 |
| `agentCanWriteTaskUnsafe` | 2（在） | P0 跨用户同租户回写修复已在 |
| `DistillWorkflowTemplateFromTask` | 0（缺） | T1.9 一键沉淀 |
| `SetWorkflowTemplateCurated` | 0（缺） | 本次策展 |
| `judgeDeliverableQuality` | 0（缺） | T1.10 |
| `strictDispatchOrgGate` | 0（缺） | T1.1 |
| `strictProtocolSchemaGate` | 0（缺） | T1.6 |

**数据面**：tasks 42（done 12 / canceled 25 / failed 4 / **awaiting_review 1**）→ **非终态仅 1 个**。
**门禁 env**：三个 `TRUSTMESH_STRICT_*` **全部未设置** → 默认 OFF。

## 3. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| 重启丢内存态 | 低 | T0.8 `FlushPersistAll` 已在生产，停机有界落盘；非终态任务仅 1 个 |
| 在途任务中断 | 低 | 仅 1 个 `awaiting_review`；重启后自 Mongo 重载 |
| JWT 失效、用户需重登 | 中 | 3 个用户；提前告知，重启后验证登录 |
| 误触 `qdrant`（unhealthy）等依赖 | 高（若误操作） | **全程 `--no-deps`**，绝不重建依赖 |
| 门禁翻转引爆（T1.6 全 422 / T1.10 拒收） | 高（若误设） | 部署后复核 env 仍无 `TRUSTMESH_STRICT_*`；本次不翻任何门禁 |
| 需要回滚 | 低 | 旧镜像均在服务器，`TAG=<旧> docker compose up -d --no-deps <svc>` |

## 4. 执行步骤

前置（无副作用，可先行）
1. 本地构建：`docker build --platform linux/amd64 -t trustmesh/backend:<TS> backend/`
   与 `docker build --platform linux/amd64 -t trustmesh/frontend-v2:<TS> frontend-v2/`
2. `docker save … | gzip` → `scp` 到 `/opt/trustmesh-test/`
3. 服务器 `docker load`

切换（**需授权**，按服务串行）
4. 备份：`cp docker-compose.yml docker-compose.yml.bak-<TS>`、`cp .env .env.bak-<TS>`；记录当前 tag
5. backend：`docker stop trustmesh-backend` → 轮询至 `exited` → `docker rm trustmesh-backend`
   → `TAG=<TS> docker compose up -d --no-deps backend`
   ⚠️ 必须 `--no-deps`（backend `depends_on: [mongo, qdrant]`，带依赖会牵动 unhealthy 的 qdrant 并尝试解析不存在的镜像）
6. 等 `healthy` + 6~8s → 冒烟：`/healthz`、登录、`GET /workflow-templates` 需鉴权返回 401 而非 404/500
7. frontend-v2：同法 `docker stop/rm` → `TAG=<TS> docker compose up -d --no-deps frontend-v2`
8. 验证前端 bundle 含特征串（`沉淀为模板` / `workflow-templates` / `curate`）

验收
9. 符号复核：`DistillWorkflowTemplateFromTask` / `SetWorkflowTemplateCurated` / `judgeDeliverableQuality` 均 > 0
10. env 复核：仍无 `TRUSTMESH_STRICT_*`
11. 数据复核：`tasks.active` 仍为 1，`awaiting_review` 任务未丢

## 5. 回滚

```
TAG=20260912-172408 docker compose up -d --no-deps backend
TAG=20260911-173900 docker compose up -d --no-deps frontend-v2
```
（旧镜像已在服务器 `docker images` 列表中，无需重新传输。）

## 6. 待确认（已决策）

1. 用户选「**立即完整切换**」→ 已执行第 4–8 步。
2. 未跑 T1.1 迁移（门禁保持 OFF，非必需）。

## 7. 执行结果（2026-09-14 23:52–23:58）

镜像 tag：`20260914-234544`（backend + frontend-v2 各一份，均 `--platform linux/amd64` 本地构建）。

| 步骤 | 结果 |
|------|------|
| 备份 compose / .env | ✅ `docker-compose.yml.bak-20260914-234544` |
| backend `stop -t 90` → exited → `rm` → `up -d --no-deps` | ✅ 约 10s 转 healthy |
| `GET /healthz` | ✅ HTTP 200 |
| 新符号 | ✅ Distill=3 / Curated=3 / judgeDeliverableQuality=1 / dispatchOrgBlocked=2 / requiresProtocolEnvelope=1 |
| 门禁复核 | ✅ `NO_STRICT_GATES_GOOD`（三个 STRICT 仍未设置） |
| frontend-v2 切换 | ✅ healthy，`frontend-v2:20260914-234544` |
| bundle 特征串 | ✅ `workflow-templates` / `沉淀为模板` / `curate` 均在 `index-CvGsEpjU.js` |
| 登录 | ✅ 200（access_token 207 字符） |
| `GET /api/v1/workflow-templates` | ✅ 200，返回真实模板数据 |
| 策展路由探测（不存在的 id） | ✅ 404 = 路由已挂载（非 405/401） |
| 数据完整性 | ✅ tasks 42 / active 1，与部署前**完全一致** |
| 外部 HTTPS（经 nginx） | ✅ root 200 / healthz 200 |

### 关键操作要点（下次复用）

- 🔴 `docker stop -t 90`：**默认 10s 会被 SIGKILL 打断 T0.8 有界落盘**，必须显式给足超时（本次实测 backend 很快 exited，但超时仍需留）。
- 🔴 切换命令必须写全：
  `cd /opt/trustmesh-test && sudo env TAG=<TS> docker compose up -d --no-deps <svc>`
  —— `sudo` 会重置环境变量，所以用 `sudo env TAG=...` 而不是 `TAG=... sudo`；
  `--no-deps` 是硬要求（backend `depends_on: [mongo, qdrant]`，而 qdrant 当前 unhealthy，带依赖会牵动它并尝试解析镜像）。
- 镜像传输用 `docker save` → python gzip → `scp` → `sudo docker load -i`（比 `docker save | ssh docker load` 更可控可查）。
- 远程输出含非 UTF-8 字节时，Python `subprocess.run(..., errors='replace')` 才能回读，否则解码抛异常（本次踩到）。
- 旧镜像全部保留在服务器，**回滚**只需：
  `TAG=20260912-172408 docker compose up -d --no-deps backend`
  `TAG=20260911-173900 docker compose up -d --no-deps frontend-v2`
