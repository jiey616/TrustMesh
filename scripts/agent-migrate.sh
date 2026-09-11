#!/usr/bin/env bash
#
# TrustMesh 智能体节点迁移辅助脚本
# ============================================================
# 场景：智能体节点（clawsynapse / clawsynapse3 / clawsynapse4 /
#        clawsynapse-default / claw4-dash-proxy）是独立 compose/手工启动的，
#       镜像用【本地预构建 tag】（新机器没有），数据在独立卷中（已随 backup 导出）。
#
# 用法：
#   旧机器导出镜像:   bash scripts/agent-migrate.sh export
#   新机器导入镜像:   bash scripts/agent-migrate.sh import <镜像包目录>
#   旧机器生成重建:   bash scripts/agent-migrate.sh gen  (→ 输出 agent-recreate.sh)
# ============================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGES_DIR="${ROOT_DIR}/backup/agent-images"

# Git Bash (MSYS) 下把 /d/... 转成 Windows 路径，否则 Windows docker 会收到 \d\...
winpath() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1"
  else
    echo "$1"
  fi
}

# 运行中的智能体容器（按启动顺序：核心 4 节点 + dashboard 代理）
AGENT_CONTAINERS="clawsynapse clawsynapse3 clawsynapse4 clawsynapse-default claw4-dash-proxy"

case "${1:-}" in
  export)
    mkdir -p "$IMAGES_DIR"
    # 收集所需镜像 tag（去重）
    IMAGES=""
    for c in $AGENT_CONTAINERS; do
      img="$(docker inspect -f '{{.Config.Image}}' "$c" 2>/dev/null || true)"
      [[ -n "$img" ]] && IMAGES="$IMAGES $img"
    done
    IMAGES="$(echo $IMAGES | tr ' ' '\n' | sort -u | grep -v '^$')"
    echo "==> 需要导出的镜像:"
    echo "$IMAGES"
    for img in $IMAGES; do
      # tag 可能已被删除（容器早期用该 tag 启动，镜像后来被重打 tag）。
      # 此时尝试用容器真实 ImageID 打回原 tag；若 ImageID 也丢失则跳过并警告。
      if ! docker image inspect "$img" >/dev/null 2>&1; then
        src_c="$(docker ps -a --format '{{.Names}}' | while read n; do
          [ "$(docker inspect -f '{{.Config.Image}}' "$n" 2>/dev/null)" = "$img" ] && echo "$n" && break
        done)"
        real_id="$(docker inspect -f '{{.Image}}' "$src_c" 2>/dev/null || true)"
        if [[ -n "$real_id" ]] && docker image inspect "$real_id" >/dev/null 2>&1; then
          echo "⚠️  tag [$img] 已失效，用真实镜像 $real_id 打回原 tag"
          docker tag "$real_id" "$img"
        else
          echo "❌ 镜像 [$img] 及其镜像层均已丢失，无法导出。"
          echo "   数据卷已随 backup 备份；新机器重建时请用 compose 文件 + registry 或本地重新构建该镜像。"
          continue
        fi
      fi
      fname="$(echo "$img" | tr '/:' '__')"
      echo "==> 导出镜像: $img -> $IMAGES_DIR/$fname.tar"
      docker save -o "$(winpath "$IMAGES_DIR/$fname.tar")" "$img"
    done
    echo "==> 镜像已导出到 $IMAGES_DIR（共 $(echo "$IMAGES" | grep -c .) 个）"
    ;;
  import)
    SRC="${2:-$IMAGES_DIR}"
    [[ -d "$SRC" ]] || { echo "❌ 镜像目录不存在: $SRC"; exit 1; }
    for f in "$SRC"/*.tar; do
      [[ -e "$f" ]] || continue
      echo "==> 导入镜像: $f"
      docker load -i "$(winpath "$f")"
    done
    echo "==> 镜像导入完成"
    ;;
  gen)
    # 从运行中容器反向生成 docker run 重建脚本（精确复刻当前状态）
    OUT="${ROOT_DIR}/scripts/agent-recreate.sh"
    cat > "$OUT" <<'HEADER'
#!/usr/bin/env bash
#
# 由 agent-migrate.sh gen 自动生成 —— 在【新机器】执行，重建智能体节点。
# 前置：数据卷已恢复（migrate-restore.sh）、镜像已导入（agent-migrate.sh import）。
# 用法： bash scripts/agent-recreate.sh
set -euo pipefail

run_existing() { docker ps -a --format '{{.Names}}' | grep -qx "$1"; }

HEADER

    for c in $AGENT_CONTAINERS; do
      if ! docker ps -a --format '{{.Names}}' | grep -qx "$c"; then
        echo "⚠️  容器 $c 不存在，跳过生成"; continue
      fi
      echo "==> 生成 $c 的启动命令"
      # 检查镜像是否可用（tag 可能已失效）
      IMG="$(docker inspect -f '{{.Config.Image}}' "$c" 2>/dev/null || true)"
      IMG_OK=1
      if [[ -z "$IMG" ]] || ! docker image inspect "$IMG" >/dev/null 2>&1; then
        IMG_OK=0
        echo "⚠️  [$c] 镜像 [$IMG] 不可用，重建脚本中将以占位符标注（需在新机器手动替换为可用镜像）"
      fi
      mapfile -t ARGS < <(docker inspect "$c" | python3 "$(winpath "$ROOT_DIR/scripts/agent-inspect.py")" | grep -v '^[[:space:]]*$')
      N="${#ARGS[@]}"
      {
        echo "# ── $c ──"
        echo "#   image: $IMG $([[ $IMG_OK == 1 ]] && echo '(可迁移)' || echo '(⚠️ 镜像已丢失，需替换)')"
        echo "if ! run_existing $c; then"
        echo "docker run -d --name $c \\"
        for ((i=0; i<N; i++)); do
          line="${ARGS[$i]}"
          if (( i == N-1 )); then
            if [[ "$IMG_OK" == "1" ]]; then
              echo "  $line"        # 最后一行（镜像）不加续行符
            else
              # 镜像不可用：替换为占位符，让用户在新机器编辑
              echo "  __MISSING_IMAGE_PLACEHOLDER__"
            fi
          else
            echo "  $line \\"
          fi
        done
        echo "fi"
        echo ""
      } >> "$OUT"
    done

    # claw4-dash-proxy 依赖 clawsynapse4 网络，放最后单独提示
    cat >> "$OUT" <<'FOOTER'
echo ""
echo "==== 全部智能体节点已启动 ===="
echo "检查: docker ps | grep clawsynapse"
echo "网络: 如容器间互访异常，先确保 clawsynapse4_default 网络存在:"
echo "  docker network create clawsynapse4_default"
FOOTER
    chmod +x "$OUT"
    echo "==> 重建脚本已生成: $OUT"
    echo "    拷贝到新机器后执行: bash scripts/agent-recreate.sh"
    ;;
  *)
    echo "用法: bash scripts/agent-migrate.sh {export|import|gen}"
    echo "  export   旧机器: 导出智能体镜像到 backup/agent-images/"
    echo "  import   新机器: 从 backup/agent-images/ 导入镜像"
    echo "  gen      旧机器: 生成 agent-recreate.sh（docker run 重建脚本）"
    exit 1
    ;;
esac
