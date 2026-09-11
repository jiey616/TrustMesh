# TrustMesh 镜像 Digest Pin 落地方案

> 版本：v1.0 | 日期：2026-08-05 | 状态：待评审
> 背景：借鉴 QM（yc-software/qm）的"不可变 pin"工程哲学——一切可变 tag 都是漂移风险，镜像用 digest 引用、部署清单版本化、回滚按 digest 而非 tag。

---

## 1. 背景与现状问题

### 1.1 当前部署方式

```
本地 Windows 构建（--platform linux/amd64）
  → docker save | gzip → tar.gz
  → paramiko sftp 上传到测试服务器 175.27.135.91
  → docker load
  → TAG=test docker compose up -d --force-recreate backend frontend
```

### 1.2 现状的三个问题

| # | 问题 | 后果 |
|---|---|---|
| P1 | **镜像用可变 tag（`test`/`latest`）** | 同 tag 镜像内容变了，compose 按 tag 判断不重建 → **跑旧二进制**（曾导致会议自动收尾修复"看似没生效"） |
| P2 | **无部署历史/清单** | 无法知道"当前跑的是哪次构建"、无法精确回滚到某次部署 |
| P3 | **回滚靠 re-tag 备份**（test-prev-<ts>） | 备份命名随意、无索引、易被 docker image prune 清掉 |

### 1.3 关键约束（影响方案选型）

调研发现测试服务器的镜像 **`RepoDigests` 为空 `[]`**：
```
[trustmesh/backend:test] -> []
```
镜像都是**本地构建 + docker load**，**没有推到任何 registry**。因此：
- ❌ 无法用 **registry digest**（`trustmesh/backend@sha256:xxx` 需要 registry 中的 RepoDigest）
- ✅ 必须用 **Image ID**（`sha256:c0f37c99...`，镜像内容的 SHA-256 哈希，`docker load` 后不变）

> Image ID 与 RepoDigest 的差异：Image ID 是镜像内容哈希（本地唯一）；RepoDigest 是 push 到 registry 后的内容寻址地址。**本地无 registry 阶段用 Image ID，未来推 registry 后升级为 RepoDigest**（方案兼容，见 §8）。

---

## 2. 方案总览

### 2.1 核心机制

```
构建 → 取 Image ID（内容哈希，不可变）
  → 写部署清单 deploy-manifest.json（服务 → Image ID → 时间 → 源码）
  → 上传清单 + 镜像 tar
  → 服务器用 Image ID 引用启动（compose override: image: sha256:xxx）
  → 部署后校验容器实际 Image 与清单一致
  → 回滚 = 读清单 → 用旧 Image ID 重新 tag/load → compose up
```

### 2.2 三个交付物

| 交付物 | 说明 |
|---|---|
| **部署清单** `deploy-manifest.json` | 每次部署的记录（服务 → Image ID → 构建时间 → 触发者），是回滚的唯一依据 |
| **Digest override** `docker-compose.digest.yml` | 服务器端生成的 compose override，把 `image: trustmesh/backend:test` 覆盖为 `image: sha256:<ImageID>` |
| **回滚脚本** `rollback_test.py` | 读清单 → 选历史部署 → 恢复镜像 → 重新部署 |

---

## 3. 部署清单设计（deploy-manifest.json）

### 3.1 存储位置

- **本地**：`D:\AiWorkspace\TrustMesh\deploy\manifest\deploy-manifest.json`（git 跟踪，作为部署历史库）
- **服务器**：`/opt/trustmesh-test/deploy-manifest.json`（部署时同步，供服务器端回滚参考）

### 3.2 清单结构

```json
{
  "schemaVersion": 1,
  "environment": "test",
  "deployments": [
    {
      "id": "20260805-152030",
      "ts": "2026-08-05T15:20:30+08:00",
      "trigger": "deploy_test.py",
      "sourceCommit": "e58d442b",           // TrustMesh 仓库当前 commit
      "services": {
        "backend": {
          "tag": "test",
          "imageId": "sha256:c0f37c9939b113d2c91e74049073c516c6b4f49346002bcd6b87e48817c11ae6",
          "backupTag": "test-prev-20260805-152030",
          "sizeBytes": 37748736
        },
        "frontend": {
          "tag": "test",
          "imageId": "sha256:ff1d782b94a3c558c3fcd3768f0cc9e2a9f5b2a13f6153741224a6ac1af4471d",
          "backupTag": "test-prev-20260805-152030",
          "sizeBytes": 52219084
        },
        "clawsynapse": {
          "tag": "test",
          "imageId": "sha256:319305c635dd7d249e912ee1abd5e3e9b8265a7e76564b2a54250ba3469ba393",
          "backupTag": "test-prev-20260805-152030",
          "sizeBytes": 6459228160
        }
      },
      "health": {
        "backend": "ok",
        "frontend": "ok",
        "clawsynapse": "healthy"
      }
    }
  ]
}
```

### 3.3 清单更新规则

- **追加不覆盖**：每次部署 append 一条 `deployments[]`，保留全历史
- **保留上限**：本地保留最近 20 条（超出归档到 `deploy/manifest/archive/`）
- **字段必填**：`id`/`ts`/`services[*].imageId`/`services[*].backupTag` 缺失视为清单损坏

---

## 4. 实施步骤（分阶段）

### 阶段 A：构建脚本加"取 digest + 备份"

**改动 `deploy/deploy_test.py`**（或新增 `deploy/build_images.py`）：

```python
import json, subprocess, hashlib, time

def get_image_id(name_tag):
    """取镜像 Image ID（内容哈希）"""
    r = subprocess.run(
        ["docker", "inspect", name_tag, "--format", "{{.Id}}"],
        capture_output=True, text=True, check=True)
    return r.stdout.strip()  # sha256:xxx

def backup_image(name_tag, backup_tag):
    """构建前先备份当前 test tag 到 test-prev-<ts>（防覆盖）"""
    subprocess.run(["docker", "tag", name_tag, backup_tag],
                   capture_output=True, check=False)  # 首次部署无旧镜像，忽略失败

# 构建（与现状相同）
docker build --platform linux/amd64 -t trustmesh/backend:test ./backend
docker build --platform linux/amd64 -t trustmesh/frontend:test ./frontend

# 取 Image ID + 备份
deploy_ts = time.strftime("%Y%m%d-%H%M%S")
manifest_entry = {"id": deploy_ts, "ts": ..., "services": {}}
for name in ["backend", "frontend", "clawsynapse"]:
    tag = f"trustmesh/{name}:test"
    backup_tag = f"trustmesh/{name}:test-prev-{deploy_ts}"
    backup_image(tag, backup_tag)           # 先备份（新镜像 tag 前）
    # —— 注意：备份要在构建之后、tag 覆盖之前做。实际顺序见 §4.4 时序
    manifest_entry["services"][name] = {
        "tag": "test",
        "imageId": get_image_id(tag),
        "backupTag": backup_tag,
    }

# 写清单（追加）
append_manifest(manifest_entry)  # 见 deploy/manifest.py
```

> ⚠️ **备份时序**（容易踩坑）：`docker build -t X:test` 会**覆盖** X:test 的旧镜像。要备份旧镜像，必须在 build **之前** `docker tag X:test X:test-prev-<ts>`（旧镜像还在时），否则 build 后旧镜像引用被新镜像替换。见 §4.4 时序图。

### 阶段 B：清单读写库 `deploy/manifest.py`

```python
"""部署清单读写：append / list / get / rollback-info"""
import json, os

MANIFEST_PATH = os.path.join(os.path.dirname(__file__), "manifest", "deploy-manifest.json")

def load():
    if not os.path.exists(MANIFEST_PATH):
        return {"schemaVersion": 1, "environment": "test", "deployments": []}
    with open(MANIFEST_PATH, encoding="utf-8") as f:
        return json.load(f)

def append(entry):
    data = load()
    data["deployments"].append(entry)
    # 保留最近 20 条
    data["deployments"] = data["deployments"][-20:]
    os.makedirs(os.path.dirname(MANIFEST_PATH), exist_ok=True)
    with open(MANIFEST_PATH, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

def list_deployments(limit=10):
    data = load()
    return data["deployments"][-limit:][::-1]

def get_by_id(deploy_id):
    data = load()
    for d in data["deployments"]:
        if d["id"] == deploy_id:
            return d
    return None
```

### 阶段 C：服务器端 digest 应用脚本

**新增 `deploy/apply_digest_test.py`**：上传镜像后，在服务器端生成 override 并启动。

```python
# 服务器端命令（paramiko exec_command）：
def apply_digest(ssh, manifest_entry):
    services = manifest_entry["services"]
    # 生成 docker-compose.digest.yml（image 用 Image ID）
    lines = ["services:"]
    for name, svc in services.items():
        lines.append(f"  {name}:")
        lines.append(f"    image: {svc['imageId']}")
    digest_yml = "\n".join(lines) + "\n"

    # 写入服务器
    sftp = ssh.open_sftp()
    with sftp.open("/opt/trustmesh-test/docker-compose.digest.yml", "w") as f:
        f.write(digest_yml)
    sftp.close()

    # 用 override 启动（digest override 优先级高于主 compose）
    cmd = (
        "cd /opt/trustmesh-test && "
        "docker load < trustmesh-test-images.tar.gz && "
        "docker compose -f docker-compose.yml -f docker-compose.digest.yml "
        "up -d --force-recreate backend frontend"
    )
    # —— 注意：clawsynapse 镜像未改时不必 force-recreate，按 digest 判断即可
```

> **Digest override 的威力**：compose 加载两个文件时，后文件（digest）的 `image` 字段**覆盖**前文件（主 compose）的 `image: trustmesh/backend:test`。容器运行时的镜像引用变成 `sha256:c0f37c...`——**tag 是否变化不再影响重建判断**，只有 digest 变化才触发重建，从根上解决"tag 不变但跑旧二进制"。

### 阶段 D：部署后校验

```python
def verify_deploy(ssh, manifest_entry):
    """校验服务器容器实际镜像与清单一致"""
    for name, svc in manifest_entry["services"].items():
        _, stdout, _ = ssh.exec_command(
            f'docker inspect trustmesh-{name} --format "{{{{.Image}}}}"')
        actual = stdout.read().decode().strip()
        if actual != svc["imageId"]:
            raise RuntimeError(f"{name} 镜像不匹配: 期望 {svc['imageId']} 实际 {actual}")
    print("✅ 全部容器镜像与清单一致")
```

### 阶段 E：回滚脚本 `deploy/rollback_test.py`

```python
"""回滚到指定部署：读清单 → 恢复镜像 → 重新部署"""
import sys
from manifest import list_deployments, get_by_id

def rollback(ssh, deploy_id):
    entry = get_by_id(deploy_id)
    if not entry:
        print(f"❌ 未找到部署 {deploy_id}，可用：")
        for d in list_deployments():
            print(f"  {d['id']}  {d['ts']}  {d['sourceCommit']}")
        sys.exit(1)

    # 1. 从 backupTag 恢复（本地构建机上）
    for name, svc in entry["services"].items():
        backup_tag = svc["backupTag"]
        # docker tag trustmesh/backend:test-prev-xxx trustmesh/backend:test
        subprocess.run(["docker", "tag", backup_tag, f"trustmesh/{name}:test"], check=True)
        # 或直接从 Image ID tag（不依赖 backupTag 命名）
        subprocess.run(["docker", "tag", svc["imageId"], f"trustmesh/{name}:test"], check=True)

    # 2. 重新走 save → upload → load → apply_digest（同部署流程）
    deploy(entry["services"])   # 复用阶段 A/C/D 逻辑

    print(f"✅ 已回滚到 {deploy_id}")
```

> **回滚的两个数据源**：
> - `backupTag`（`test-prev-<ts>`）：如果备份 tag 还在本地，直接用它
> - `imageId`（sha256:...）：**只要镜像还在本地（未被 prune），直接 `docker tag <imageId> X:test` 即可**，比 backupTag 更可靠（不依赖命名约定）
> - 若镜像已被 `docker image prune` 清掉：需要从 tar 归档恢复（见 §5 归档策略）

---

## 4.4 部署时序图（含备份时机）

```
本地构建机                             测试服务器 175.27.135.91
─────────                             ─────────────────────
1. docker tag X:test X:test-prev-<ts>   ← 先备份旧镜像（build 前）
2. docker build -t X:test ./X           ← 覆盖 test tag
3. 取 Image ID: docker inspect X:test
4. 写 deploy-manifest.json（append）
5. docker save 新镜像 | gzip
6. sftp 上传 tar.gz + manifest
                                       7. docker load < tar.gz
                                       8. 生成 docker-compose.digest.yml
                                          （image: sha256:<ImageID>）
                                       9. compose -f 主 -f digest up -d
                                      10. 校验 docker inspect 容器 Image
11. 部署结果回写清单 health 字段
```

---

## 5. 镜像保留与归档策略

| 策略 | 说明 |
|---|---|
| **本地备份 tag** | 每次部署前 `X:test → X:test-prev-<ts>`，保留最近 3 次（旧的可删，因为清单里有 imageId，只要镜像没被 prune 就能恢复） |
| **tar 归档** | 每次部署的 tar.gz 保留在 `deploy/archive/<ts>-images.tar.gz`（gitignore），作为最后的兜底（镜像被 prune 后可从 tar 恢复） |
| **服务器侧** | 服务器 `docker image prune` 只清 dangling（无 tag 引用）镜像——digest 引用后旧镜像会变 dangling，**prune 会清掉它们**。建议服务器禁用自动 prune 或保留最近 N 个 |

> ⚠️ 注意：应用 digest pin 后，**旧镜像（被 digest 替换的）会变成 dangling 镜像**，`docker image prune -a` 会删掉它们——这会破坏"按 imageId 回滚"的能力。解决方案：**回滚需要镜像在本地（构建机）或归档 tar**，服务器不承担镜像库角色。

---

## 6. 对现有流程的改动清单

| 文件 | 改动 |
|---|---|
| `deploy/deploy_test.py` | 构建前备份旧镜像；构建后取 Image ID；写清单；上传清单；服务器端改用 digest override 启动；部署后校验 |
| `deploy/manifest.py` | **新增**：清单读写库 |
| `deploy/apply_digest_test.py` | **新增**：服务器端 digest 应用 + 校验（或并入 deploy_test.py） |
| `deploy/rollback_test.py` | **新增**：回滚脚本（`--list` 列出历史，`<id>` 回滚） |
| `deploy/manifest/deploy-manifest.json` | **新增**：部署历史库（git 跟踪） |
| `deploy/archive/` | **新增**：tar 归档目录（gitignore） |
| 测试服务器 `/opt/trustmesh-test/docker-compose.digest.yml` | 部署时自动生成（不入 git） |
| `docker-compose.yml`（本地） | 不改（digest 是部署时动态注入的 override，源码 compose 保持可读） |

---

## 7. 验证方案

### 7.1 单测（本地）

```bash
# manifest 库测试
python -m pytest deploy/tests/test_manifest.py
# 覆盖：append / list / get_by_id / 损坏清单容错 / 超 20 条归档

# Image ID 获取测试
python deploy/tests/test_image_id.py
# 覆盖：真实镜像取 ID、tag 不存在报错
```

### 7.2 集成验证（部署一次 + 回滚一次）

```bash
# 1. 正常部署（带 digest）
python deploy/deploy_test.py
# 预期：部署成功，服务器 docker inspect 容器 Image = 清单 imageId

# 2. 模拟"tag 不变内容变"场景
#    修改源码 → 重新 build（不手动 force-recreate）→ 用 digest override up -d
#    预期：容器重建（digest 变了 → compose 识别变化）✅ 核心价值验证

# 3. 回滚测试
python deploy/rollback_test.py --list
python deploy/rollback_test.py <上次部署id>
# 预期：服务器回到上一版镜像，健康检查通过
```

### 7.3 清单一致性巡检

```bash
# 服务器端巡检：容器实际镜像 vs 服务器 deploy-manifest.json 最新条目
python deploy/audit_test.py --check
# 输出：每个服务 actual imageId vs manifest imageId，不一致标红
```

---

## 8. 未来演进：升级到 Registry digest（生产形态）

本地无 registry 阶段用 **Image ID**；接入 registry 后（如腾讯云 TCR / GHCR / 自建 Harbor）升级为 **RepoDigest**：

| 阶段 | pin 方式 | compose 引用 | 前提 |
|---|---|---|---|
| **现在（测试）** | Image ID | `image: sha256:c0f37c...` | 本地构建 + docker load |
| **未来（生产）** | RepoDigest | `image: registry.example.com/trustmesh/backend@sha256:xxx` | 镜像 push 到 registry |

升级要点：
- 构建脚本加 `docker push`，`docker inspect --format '{{index .RepoDigests 0}}'` 取 RepoDigest
- 清单 `imageId` 字段升级为 `digest`（`registry/name@sha256:...`）
- compose override 用 RepoDigest 形式
- **信任链**：`docker buildx build --provenance` 可同时拿到 SBOM/attestation，`cosign verify` 做签名校验（QM 正是这么做的：GHCR + cosign 签名）

---

## 9. 风险与注意

| 风险 | 缓解 |
|---|---|
| **本地镜像被 prune 后无法回滚** | 保留 `deploy/archive/<ts>-images.tar.gz` 兜底；回滚优先用 `imageId`（不依赖命名） |
| **服务器 docker image prune 清 dangling** | 服务器不做自动 prune；或保留最近 N 个（用 `docker image prune --filter "until=24h"`） |
| **digest override 与主 compose 的 image 字段冲突** | compose 文件叠加时后文件覆盖前文件——digest override 在 `-f` 顺序中**放最后**（`-f docker-compose.yml -f docker-compose.digest.yml`） |
| **clawsynapse 大镜像（6.4GB）每次上传慢** | 只有 clawsynapse 源码变化时才重传；`docker save` 用 gzip 压缩；可考虑增量（`docker save` 全量无法增量，改用 registry 推送可去重） |
| **首次部署无旧镜像可备份** | `docker tag` 失败时忽略（`check=False`），清单里 backupTag 标记为 `"none"` |
| **清单与服务器不同步** | 部署时清单随 tar 一起上传；服务器 `audit_test.py` 可巡检 |
| **Windows 路径/编码** | 脚本用 `os.path` 而非硬编码 `D:\...`；paramiko sftp 二进制模式传 tar |

---

## 10. 落地工作量评估

| 阶段 | 内容 | 预估 |
|---|---|---|
| A | 构建脚本加备份 + Image ID 获取 | 0.5h |
| B | manifest.py 清单库 + 单测 | 1h |
| C | 服务器端 digest override 应用 | 0.5h |
| D | 部署后校验 | 0.5h |
| E | rollback_test.py 回滚 | 1h |
| 集成 | 部署 + 回滚 + 巡检验证 | 1h |
| **合计** | | **约 4.5h** |

---

*方案完成：2026-08-05 | 基于 QM（yc-software/qm）"不可变 pin"理念 | 适用：测试环境（本地构建 + docker load）*
