"""部署「外部平台挂载」能力到测试机 175.27.135.91。

流程（沿用仓库既有部署范式：本地构建 → docker save → SFTP → docker load）：
  1. 备份远端 docker-compose.yml
  2. 上传镜像 tar.gz
  3. docker load
  4. 若 compose 里没有 frontend-v2 服务，则注入（nginx 反代依赖同网络的 backend:8080）
  5. force-recreate backend + frontend-v2
  6. 健康检查

前置：本机构建好 trustmesh/backend:test 与 trustmesh/frontend-v2:test。
"""
import sys
import time

from ssh_helper import connect, run, upload, REMOTE_ROOT

TAG = "test"
LOCAL_TAR = r"C:\Users\38643\AppData\Local\Temp\trustmesh-extapp.tar.gz"
REMOTE_TAR = f"{REMOTE_ROOT}/trustmesh-extapp.tar.gz"

FRONTEND_V2_BLOCK = """
  frontend-v2:
    image: trustmesh/frontend-v2:${TAG:-test}
    container_name: trustmesh-frontend-v2
    restart: unless-stopped
    depends_on:
      - backend
    ports:
      - "5174:80"
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1/ >/dev/null || exit 1"]
      interval: 10s
      timeout: 3s
      retries: 6
      start_period: 5s

volumes:"""


def ensure_frontend_v2_service(ssh):
    """远端 compose 里注入 frontend-v2 服务（幂等）。"""
    remote_compose = f"{REMOTE_ROOT}/docker-compose.yml"
    sftp = ssh.open_sftp()
    local_copy = r"C:\Users\38643\AppData\Local\Temp\remote-docker-compose.yml"
    sftp.get(remote_compose, local_copy)
    with open(local_copy, "r", encoding="utf-8") as f:
        content = f.read()
    sftp.close()

    if "frontend-v2:" in content:
        print("[skip] compose 已包含 frontend-v2 服务")
        return

    if "\nvolumes:" not in content:
        print("❌ compose 里找不到 volumes: 锚点，中止（避免写坏配置）")
        sys.exit(1)

    new_content = content.replace("\nvolumes:", FRONTEND_V2_BLOCK, 1)
    with open(local_copy, "w", encoding="utf-8") as f:
        f.write(new_content)

    upload(ssh, local_copy, remote_compose)
    print("[ok] compose 已注入 frontend-v2 服务")


def main():
    ssh = connect()
    try:
        print("=== 1. 备份 compose ===")
        ts = time.strftime("%Y%m%d-%H%M%S")
        run(ssh, f"cp {REMOTE_ROOT}/docker-compose.yml {REMOTE_ROOT}/docker-compose.yml.bak-{ts} && ls -la {REMOTE_ROOT}/docker-compose.yml.bak-{ts}")

        print("\n=== 2. 上传镜像 ===")
        upload(ssh, LOCAL_TAR, REMOTE_TAR)
        run(ssh, f"ls -lh {REMOTE_TAR}")

        print("\n=== 3. docker load ===")
        run(ssh, f"export PATH=$HOME/.local/bin:$PATH; docker load -i {REMOTE_TAR}", timeout=600)

        print("\n=== 4. 注入 frontend-v2 服务 ===")
        ensure_frontend_v2_service(ssh)

        print("\n=== 5. 重建容器 ===")
        run(
            ssh,
            f"export PATH=$HOME/.local/bin:$PATH; cd {REMOTE_ROOT} && "
            f"TAG={TAG} docker compose up -d --force-recreate backend frontend-v2",
            timeout=600,
        )

        print("\n=== 6. 状态 ===")
        run(ssh, "export PATH=$HOME/.local/bin:$PATH; docker ps -a --format 'table {{.Names}}\\t{{.Image}}\\t{{.Status}}' | head -12", timeout=120)
    finally:
        ssh.close()


if __name__ == "__main__":
    main()
