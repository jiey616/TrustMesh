#!/usr/bin/env bash
#
# TrustMesh 全量迁移 · 备份脚本
# 在【旧机器】的 TrustMesh 项目根目录运行。
#
# 用法:
#   bash scripts/migrate-backup.sh
#   bash scripts/migrate-backup.sh --output /d/backup-trustmesh --include-agents
#
# 选项:
#   --output DIR        备份输出目录（默认 ./backup）
#   --include-agents    额外导出智能体节点卷（clawsynapse*/hermes*/roundtable*，
#                       含正在进行的创作任务数据）
#   --skip-stop         跳过 docker compose stop（数据一致性自行保证）
#   --skip-code         不打包代码目录（只导数据卷）
#
# 产物:
#   <output>/<卷名>.tar.gz        每个 Docker 数据卷一个压缩包
#   <output>/code-trustmesh.tar.gz     TrustMesh 项目代码（排除 node_modules/.git/dist）
#   <output>/code-clawsynapse.tar.gz   clawsynapse 源码（若存在 ../clawsynapse）
set -euo pipefail

OUT_DIR="backup"
INCLUDE_AGENTS=0
SKIP_STOP=0
SKIP_CODE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output)   OUT_DIR="$2"; shift 2 ;;
    --include-agents) INCLUDE_AGENTS=1; shift ;;
    --skip-stop)      SKIP_STOP=1;    shift ;;
    --skip-code)      SKIP_CODE=1;    shift ;;
    *) echo "❌ 未知参数: $1"; echo "   用法: bash scripts/migrate-backup.sh [--output DIR] [--include-agents] [--skip-stop] [--skip-code]"; exit 2 ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"

echo "==> 备份目录: $OUT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "❌ 未找到 docker，请先安装 Docker Desktop / Docker Engine"; exit 1
fi

# 0) 探测 compose 数据卷（动态获取，避免写死前缀）
#    注意: compose 配置里的卷名是短名（如 trustmesh-files-data），
#    实际 Docker 卷名 = <项目名>_<短名>（如 trustmesh_trustmesh-files-data）。
PROJECT="$(docker compose config --format json 2>/dev/null \
  | grep -m1 '"name"' | sed -E 's/.*"name": *"([^"]+)".*/\1/' || true)"
PROJECT="${PROJECT:-trustmesh}"
SHORT_VOLUMES="$(docker compose config --volumes 2>/dev/null || true)"
COMPOSE_VOLUMES=""
for s in $SHORT_VOLUMES; do
  COMPOSE_VOLUMES="$COMPOSE_VOLUMES ${PROJECT}_${s}"
done
if [[ -z "$COMPOSE_VOLUMES" ]]; then
  echo "⚠️  docker compose config 未返回卷（项目可能从未启动过），回退到默认卷名列表"
  COMPOSE_VOLUMES="trustmesh_trustmesh-mongo-data trustmesh_trustmesh-files-data trustmesh_trustmesh-knowledge-data trustmesh_trustmesh-qdrant-data trustmesh_trustmesh-clawsynapse-data trustmesh_trustmesh-transfer-data"
fi
echo "==> 项目名: $PROJECT"

# 1) 停栈（保证 mongo / qdrant 落盘一致）
if [[ "$SKIP_STOP" == "0" ]]; then
  echo "==> 停止 compose 栈以保证数据一致性 ..."
  docker compose stop || echo "⚠️  停栈未完全成功，继续导出"
else
  echo "==> 跳过停栈（--skip-stop）"
fi

# 2) 导出单个卷
backup_volume() {
  local vol="$1"
  if ! docker volume inspect "$vol" >/dev/null 2>&1; then
    echo "⚠️  卷不存在，跳过: $vol"; return
  fi
  echo "==> 导出卷: $vol"
  docker run --rm -v "$vol":/data -v "$OUT_DIR":/backup alpine \
    tar czf "/backup/${vol}.tar.gz" -C /data . || { echo "❌ 导出失败: $vol"; exit 1; }
}

echo "==> 导出 compose 数据卷"
for v in $COMPOSE_VOLUMES; do backup_volume "$v"; done

# 3) 可选：智能体节点卷（clawsynapse4/hermes 等，含创作产物）
#    只导出「当前运行中」的智能体容器所挂载的卷，避免把已停用的旧节点也打包。
if [[ "$INCLUDE_AGENTS" == "1" ]]; then
  echo "==> 导出运行中的智能体节点卷"
  AGENT_VOLUMES=""
  for c in $(docker ps --format '{{.Names}}' \
    | grep -E '^(clawsynapse|claw4|claw[0-9]?|roundtable)' || true); do
    vols="$(docker inspect -f '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}} {{end}}{{end}}' "$c" 2>/dev/null)"
    AGENT_VOLUMES="$AGENT_VOLUMES $vols"
  done
  for v in $AGENT_VOLUMES; do backup_volume "$v"; done
fi

# 4) 打包代码（TrustMesh + 同级 clawsynapse 源码）
if [[ "$SKIP_CODE" == "0" ]]; then
  PARENT="$(dirname "$ROOT_DIR")"
  if [[ -f "$OUT_DIR/code-trustmesh.tar.gz" ]]; then
    echo "⚠️  已存在 code-trustmesh.tar.gz，跳过打包（先删除可重新打包）"
  else
    echo "==> 打包 TrustMesh 代码 ..."
    tar czf "$OUT_DIR/code-trustmesh.tar.gz" \
      --exclude='.git' \
      --exclude='backup' \
      --exclude='frontend/node_modules' --exclude='frontend/dist' \
      --exclude='frontend-v2/node_modules' --exclude='frontend-v2/dist' \
      --exclude='frontend-v3/node_modules' --exclude='frontend-v3/dist' \
      -C "$PARENT" "$(basename "$ROOT_DIR")"
  fi
  if [[ -d "$PARENT/clawsynapse" && ! -f "$OUT_DIR/code-clawsynapse.tar.gz" ]]; then
    echo "==> 打包 clawsynapse 源码 ..."
    tar czf "$OUT_DIR/code-clawsynapse.tar.gz" --exclude='.git' -C "$PARENT" clawsynapse
  fi
fi

# 5) 结果清单
echo ""
echo "======== 备份完成 ========"
ls -lh "$OUT_DIR"/*.tar.gz 2>/dev/null | awk '{print "  " $NF "  (" $5 ")"}'
echo ""
echo "将整个目录 [$OUT_DIR] 拷贝到目标机器后，在目标机器运行:"
echo "  bash scripts/migrate-restore.sh <备份目录路径> [--skip-code] [--skip-start]"
