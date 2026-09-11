#!/usr/bin/env bash
#
# TrustMesh 全量迁移 · 恢复脚本
# 在【新机器】上运行。需要已安装 Docker + Compose Plugin。
#
# 用法:
#   bash scripts/migrate-restore.sh /path/to/backup
#   bash scripts/migrate-restore.sh /path/to/backup --skip-start
#
# 选项:
#   --skip-code   跳过解压代码包（已手动放好 TrustMesh / clawsynapse 源码）
#   --skip-start  不自动 docker compose up（恢复完卷就退出）
#
# 恢复内容:
#   - 解压 code-trustmesh.tar.gz / code-clawsynapse.tar.gz 到同级目录
#   - 重建全部数据卷并灌入备份数据（mongo 卷自动修正属主为 uid 999）
#   - 启动 compose 栈
set -euo pipefail

SRC_DIR="${1:-backup}"
SKIP_CODE=0
SKIP_START=0
for arg in "$@"; do
  case "$arg" in
    --skip-code)  SKIP_CODE=1 ;;
    --skip-start) SKIP_START=1 ;;
    --*) echo "❌ 未知参数: $arg"; exit 2 ;;
  esac
done

if [[ ! -d "$SRC_DIR" ]]; then
  echo "❌ 备份目录不存在: $SRC_DIR"; exit 1
fi
SRC_DIR="$(cd "$SRC_DIR" && pwd)"
echo "==> 备份目录: $SRC_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "❌ 未找到 docker，请先安装 Docker Desktop / Docker Engine"; exit 1
fi

PARENT="$(dirname "$SRC_DIR")"

# 1) 解压代码（只在不存在的目标位置解压，避免覆盖/嵌套）
if [[ "$SKIP_CODE" == "0" ]]; then
  if [[ -f "$SRC_DIR/code-trustmesh.tar.gz" && ! -d "$PARENT/TrustMesh" ]]; then
    echo "==> 解压 TrustMesh 代码 -> $PARENT/TrustMesh"
    tar xzf "$SRC_DIR/code-trustmesh.tar.gz" -C "$PARENT"
  fi
  if [[ -f "$SRC_DIR/code-clawsynapse.tar.gz" && ! -d "$PARENT/clawsynapse" ]]; then
    echo "==> 解压 clawsynapse 源码 -> $PARENT/clawsynapse"
    tar xzf "$SRC_DIR/code-clawsynapse.tar.gz" -C "$PARENT"
  fi
fi

# 2) 重建数据卷
restore_volume() {
  local f="$1"
  local vol
  vol="$(basename "$f" .tar.gz)"
  docker volume create "$vol" >/dev/null 2>&1 || echo "⚠️  卷已存在（将覆盖内容）: $vol"
  echo "==> 恢复卷: $vol"
  docker run --rm -v "$vol":/data -v "$f":/backup.tar.gz alpine \
    sh -c "tar xzf /backup.tar.gz -C /data" || { echo "❌ 恢复失败: $f"; exit 1; }
  # MongoDB 容器以 uid 999(mongodb) 运行，修正属主防止启动失败
  if [[ "$vol" == *mongo* ]]; then
    echo "    → 修正 mongo 卷属主为 uid 999"
    docker run --rm -v "$vol":/data alpine chown -R 999:999 /data
  fi
}

for f in "$SRC_DIR"/*.tar.gz; do
  [[ -e "$f" ]] || continue
  case "$(basename "$f")" in
    code-*) continue ;;  # 代码包不按卷处理
  esac
  restore_volume "$f"
done

# 3) 启动 compose 栈
if [[ "$SKIP_START" == "0" ]]; then
  PROJ_DIR=""
  for cand in "$PARENT/TrustMesh" "$(pwd)/TrustMesh" "$(pwd)"; do
    if [[ -f "$cand/docker-compose.yml" || -f "$cand/docker-compose.yaml" ]]; then
      PROJ_DIR="$cand"; break
    fi
  done
  if [[ -z "$PROJ_DIR" ]]; then
    echo "⚠️  未找到 docker-compose.yml，请手动 cd 到项目根后执行 docker compose up -d --build"
    exit 0
  fi
  echo "==> 启动 compose 栈（目录: $PROJ_DIR）..."
  cd "$PROJ_DIR"
  docker compose up -d --build
  echo ""
  echo "======== 恢复完成 ========"
  echo "检查状态: docker compose ps"
  echo "健康检查: curl http://localhost:8080/healthz"
else
  echo "==> 跳过启动（--skip-start），请手动执行: docker compose up -d --build"
fi
