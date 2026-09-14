#!/usr/bin/env bash
set -euo pipefail

#
# TrustMesh 部署脚本（首次部署 + 升级更新通用）
#
# 用法:
#   ./deploy/deploy.sh <user@host> [选项]
#
# 选项:
#   -t, --tag <tag>        镜像 tag（默认: git short hash）
#   -d, --dir <path>       远程部署目录（默认: /opt/trustmesh）
#   --setup                首次部署：传输 compose 文件并提示配置 .env
#   --compose-only         仅更新 compose 文件，不构建镜像
#

usage() {
  echo "用法: $0 <user@host> [选项]"
  echo ""
  echo "选项:"
  echo "  -t, --tag <tag>        镜像 tag（默认: git short hash）"
  echo "  -d, --dir <path>       远程部署目录（默认: /opt/trustmesh）"
  echo "  --setup                首次部署：传输 compose 文件并提示配置 .env"
  echo "  --compose-only         仅更新 compose 文件，不构建镜像"
  exit 1
}

# 解析参数
REMOTE_HOST=""
TAG=""
REMOTE_DIR="/opt/trustmesh"
SETUP=false
COMPOSE_ONLY=false

while [[ $# -gt 0 ]]; do
  case $1 in
    -t|--tag)       TAG="$2"; shift 2 ;;
    -d|--dir)       REMOTE_DIR="$2"; shift 2 ;;
    --setup)        SETUP=true; shift ;;
    --compose-only) COMPOSE_ONLY=true; shift ;;
    -h|--help)      usage ;;
    -*)             echo "未知选项: $1"; usage ;;
    *)
      if [[ -z "$REMOTE_HOST" ]]; then
        REMOTE_HOST="$1"; shift
      else
        echo "多余参数: $1"; usage
      fi
      ;;
  esac
done

[[ -z "$REMOTE_HOST" ]] && usage
[[ -z "$TAG" ]] && TAG="latest"

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGES_FILE="trustmesh-images-${TAG}.tar.gz"

# --setup: 首次部署，初始化远程目录
if $SETUP; then
  echo "==> 首次部署: 初始化远程目录 ${REMOTE_DIR}"
  ssh "${REMOTE_HOST}" "mkdir -p ${REMOTE_DIR}"
  scp "${PROJECT_ROOT}/docker-compose.prod.yml" "${REMOTE_HOST}:${REMOTE_DIR}/docker-compose.yml"
  echo ""
  echo "compose 文件已传输。请在远程服务器创建 .env 文件:"
  echo "  ssh ${REMOTE_HOST}"
  echo "  cd ${REMOTE_DIR}"
  echo "  cp .env.example .env   # 或手动创建"
  echo "  vim .env               # 配置 JWT_SECRET 等环境变量"
  echo ""
  echo "配置完成后，再次运行（不带 --setup）进行部署:"
  echo "  $0 ${REMOTE_HOST} -t ${TAG}"
  exit 0
fi

# --compose-only: 仅更新 compose 文件
if $COMPOSE_ONLY; then
  echo "==> 更新 compose 文件"
  scp "${PROJECT_ROOT}/docker-compose.prod.yml" "${REMOTE_HOST}:${REMOTE_DIR}/docker-compose.yml"
  ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && docker compose up -d"
  echo "==> compose 文件已更新并重启服务"
  exit 0
fi

# 常规部署/升级: 构建 → 传输 → 加载 → 启动
echo "==> 构建镜像 (tag: ${TAG})"

docker build --platform linux/amd64 -t "trustmesh/backend:${TAG}"     "${PROJECT_ROOT}/backend"
docker build --platform linux/amd64 -t "trustmesh/frontend:${TAG}"    "${PROJECT_ROOT}/frontend"
docker build --platform linux/amd64 -t "trustmesh/clawsynapse:${TAG}" \
  --build-arg CLAWSYNAPSE_REF="${CLAWSYNAPSE_REF:-main}" \
  --build-arg CACHE_BUST="$(date +%s)" \
  "${PROJECT_ROOT}/deploy/clawsynapse"

echo "==> 导出镜像到 ${IMAGES_FILE}"

docker save \
  "trustmesh/backend:${TAG}" \
  "trustmesh/frontend:${TAG}" \
  "trustmesh/clawsynapse:${TAG}" \
  | gzip > "/tmp/${IMAGES_FILE}"

echo "==> 传输镜像到 ${REMOTE_HOST}:${REMOTE_DIR}/"

rsync -avz --progress --partial "/tmp/${IMAGES_FILE}" "${REMOTE_HOST}:${REMOTE_DIR}/${IMAGES_FILE}"

echo "==> 在远程服务器加载镜像并启动服务"

ssh "${REMOTE_HOST}" bash -s <<EOF
  set -euo pipefail
  cd ${REMOTE_DIR}

  echo "--- 加载镜像"
  docker load < ${IMAGES_FILE}

  echo "--- 更新服务"
  TAG=${TAG} docker compose up -d

  echo "--- 清理镜像包"
  rm -f ${IMAGES_FILE}

  echo "--- 清理旧镜像"
  docker image prune -f

  echo "--- 服务状态"
  docker compose ps
EOF

rm -f "/tmp/${IMAGES_FILE}"

echo "==> 部署完成! (tag: ${TAG})"
