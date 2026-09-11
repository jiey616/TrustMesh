# TrustMesh 跨机迁移 · 数据迁移与 Docker Desktop 部署指南

> 场景：代码已在目标机器就位（本次迁移**只做数据迁移 + 新机器部署**）。
> 数据包由旧机器 `scripts/migrate-backup.sh --skip-code --include-agents` 生成。

---

## 一、数据包内容（旧机器已生成，位于 `TrustMesh/backup/`）

| 文件 | 对应数据 | 说明 |
| --- | --- | --- |
| `trustmesh_trustmesh-mongo-data.tar.gz` | 项目/任务/用户/事件/外部应用（**核心**） | 必须迁移 |
| `trustmesh_trustmesh-files-data.tar.gz` | 用户上传文件 | 必须 |
| `trustmesh_trustmesh-knowledge-data.tar.gz` | 知识库文件 | 必须 |
| `trustmesh_trustmesh-qdrant-data.tar.gz` | 向量库 | 必须 |
| `trustmesh_trustmesh-clawsynapse-data.tar.gz` | 消息网关 identity 密钥 | 必须 |
| `trustmesh_trustmesh-transfer-data.tar.gz` | 传输暂存（TTL 24h） | 可选 |
| `clawsynapse*_*.tar.gz`（多个） | 智能体节点数据（含创作任务） | 建议 |

---

## 二、目标机器操作（Docker Desktop 环境）

### 0. 前置准备
- 安装 **Docker Desktop for Windows**，启动并确认 `docker --version` / `docker compose version` 可用。
- 把整个 `backup/` 目录和打包好的代码目录复制到目标机器，保持目录结构：

```
D:\AiWorkspace\
├── TrustMesh\          ← 你的代码包解压位置
│   ├── backup\         ← 本次数据包
│   ├── docker-compose.yml
│   └── scripts\migrate-restore.sh
└── clawsynapse\        ← 构建依赖（docker-compose 的 build context 指向 ../clawsynapse）
```

> ⚠️ 路径需与旧机器一致（`clawsynapse` 在 `TrustMesh` 同级），否则 `docker compose build` 会失败。
> ⚠️ 确认代码包里包含 `.env`（内含 `JWT_SECRET`），否则后端启动会报错。

### 1. 恢复数据卷（在 TrustMesh 目录执行）

```bash
cd D:\AiWorkspace\TrustMesh
bash scripts/migrate-restore.sh backup --skip-code
```

该命令会：
- 重建全部数据卷并灌入备份数据；
- **自动修正 mongo 卷属主**（uid 999），防止 mongod 启动失败；
- 跳过代码解压（`--skip-code`，代码你已放好）。

> 若只想恢复核心 compose 数据、不恢复智能体节点，可手动删除 `backup/` 下 `clawsynapse*` 开头的 tar 后再执行，或先只执行对应卷。

### 2. 构建并启动核心栈

```bash
cd D:\AiWorkspace\TrustMesh
docker compose up -d --build
docker compose ps            # 全部 healthy 即成功
curl http://localhost:8080/healthz
```

### 3. 访问
- 前端 v1（shadcn）：http://localhost:3000
- 前端 v2（antd）：http://localhost:5174
- 后端 API：http://localhost:8080

---

## 三、智能体节点恢复

`migrate-restore.sh` 只恢复了**数据卷**，不会启动智能体容器。智能体节点（5 个）在新机器按下列方式重建。

### 3.1 前置：导入智能体镜像

```bash
bash scripts/agent-migrate.sh import
```

### 3.2 按节点选择重建方式

| 节点 | 端口 | 数据卷（已备份） | 重建方式 |
| --- | --- | --- | --- |
| clawsynapse | 18081 | clawsynapse_clawsynapse-data + clawsynapse_hermes-data | **compose**（见 3.3） |
| clawsynapse3 | 18083 | clawsynapse3_clawsynapse3-data + clawsynapse3_hermes3-data | **compose**（见 3.3） |
| clawsynapse4 | 18084 | clawsynapse4_clawsynapse4-data + clawsynapse4_hermes4-data | **docker run**（`scripts/agent-recreate.sh`） |
| clawsynapse-default | 18082 | clawsynapse_clawsynapse-default-data + clawsynapse_hermes-default-data | **docker run**（`scripts/agent-recreate.sh`） |
| claw4-dash-proxy | 19119→9119 | 无 | **docker run**（`scripts/agent-recreate.sh`） |

> 说明：`clawsynapse`(18081) 与 `clawsynapse3`(18083) 的旧镜像 tag（`clawsynapse:local` / `local-rt`）在本机已丢失无法导出，但其 compose 文件指向的镜像（`clawsynapse:local-rt4` / `clawsynapse:latest`）均已导出可用，且 compose 卷名与备份卷完全一致 → 用 compose 重建即可自动接回数据。

### 3.3 clawsynapse / clawsynapse3 用 compose 重建

```bash
# 在 D:\AiWorkspace\clawsynapse 目录（代码已打包同级）

# 重建 clawsynapse(18081)：镜像取 .env 的 CLAWSYNAPSE_IMAGE=clawsynapse:local-rt4
docker compose up -d

# 重建 clawsynapse3(18083)：镜像取 claw4.yml 的 clawsynapse:latest，项目名须带 -p clawsynapse3
docker compose -p clawsynapse3 -f docker-compose.claw4.yml up -d
```

⚠️ 要点：
- **项目名必须与旧机器一致**（`-p clawsynapse3`），否则 compose 会生成新前缀的卷，无法接回备份数据（会变成全新空卷）。
- 若 `.env` 未随代码打包，需在新机器重建（从 `.env.example` 复制并填 API Key / CLAWSYNAPSE_* 配置，特别是 `HERMES_CUSTOM_PROVIDER_*` 与 `HERMES_GATEWAY_KEY`）。
- `claw4.yml` 的 `environment` 引用 `${HERMES_*}` 等变量，需保证 `.env` 就位。

### 3.4 clawsynapse4 / clawsynapse-default / claw4-dash-proxy 用 docker run 重建

```bash
# 镜像已导入后，直接执行生成好的重建脚本（端口/卷/网络/环境变量已精确复刻）
bash scripts/agent-recreate.sh
```

⚠️ 要点：
- 该脚本由 `agent-migrate.sh gen` 从运行中容器反向生成，**保留 API Key 等敏感环境变量明文**，请勿提交到 git。
- 若镜像 tag 失效，脚本中对应位置会标注 `__MISSING_IMAGE_PLACEHOLDER__`，需手动替换为可用镜像名（本机 `clawsynapse:local-rt4` / `clawsynapse:latest` 均已导出）。
- `claw4-dash-proxy` 依赖 `clawsynapse4_default` 网络，若提示网络不存在先执行 `docker network create clawsynapse4_default`。

### 3.5 验证

```bash
docker ps | grep clawsynapse        # 5 个节点全部 Up
curl http://localhost:18084/v1/health   # 各节点健康检查
# 平台「任务详情」能看到智能体正常承接、历史数据完整
```

---

## 四、注意事项
1. **`.env` 的 `JWT_SECRET` 必须与旧机器一致**，否则所有已登录用户 token 失效。
2. **`NATS_SERVERS`** 指向外部 NATS（旧机器为 `nats://175.27.135.91:4222`），新机器只需网络可达，无需迁移 NATS 本身。
3. **端口占用**：新机器若 3000/5174/8080/18080/6333/27017 已被占用，改 `docker-compose.yml` 的 `ports` 或 `.env`。
4. 首次启动 `--build` 需要拉取/构建镜像，耗时取决于网络；后端 `build` 依赖 Go module 缓存，已通过 `trustmesh-gomod` 卷加速。
5. 迁移完成后旧机器可先保留备份包，待新机器验证无误再清理。
6. `clawsynapse` 源码目录下 `.env` 含 API Key / CLAWSYNAPSE_* 配置，**必须随代码一起迁移**（用户打包代码时应包含）。
