# -*- coding: utf-8 -*-
"""生产环境全量更新 + HTTPS 启用一体化脚本（175.27.135.91）。

流程：
  1. 本地 save 五镜像（backend/frontend/frontend-v2/clawsynapse + nginx:1.27-alpine）→ gzip
  2. SFTP 上传镜像包 + nginx 配置 + 自签证书
  3. 生产：备份 test → test-prev-<ts> tag（可回滚）→ load 新镜像
  4. 注入 nginx-proxy 服务到 compose（幂等，带备份）
  5. .env 更新 TRUSTMESH_EXTERNAL_URL=https://175.27.135.91
  6. up -d 四服务 + nginx-proxy，本机自测 https

用法：python update_prod_https.py [--skip-build-check]
"""
import os
import sys
import time
import tempfile
import paramiko

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from ssh_helper import connect, run, upload, DOCKER_ENV  # noqa: E402

ROOT = r"D:\AIWorkspace\TrustMesh"
REMOTE_ROOT = "/opt/trustmesh-test"
TAG = "test"
IMAGES = [
    f"trustmesh/backend:{TAG}",
    f"trustmesh/frontend:{TAG}",
    f"trustmesh/frontend-v2:{TAG}",
    f"trustmesh/clawsynapse:{TAG}",
    "nginx:1.27-alpine",
]
SERVICES = ["backend", "frontend", "frontend-v2", "clawsynapse"]

NGINX_CONF = os.path.join(ROOT, "deploy", "nginx", "nginx-https.conf")
CERT_DIR = os.path.join(ROOT, "deploy", "nginx", "certs")

TS = time.strftime("%Y%m%d-%H%M%S")

NGINX_SERVICE_BLOCK = """
  nginx-proxy:
    image: nginx:1.27-alpine
    container_name: trustmesh-nginx-proxy
    restart: unless-stopped
    depends_on:
      - frontend-v2
      - backend
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx-https.conf:/etc/nginx/conf.d/default.conf:ro
      - ./nginx/certs:/etc/nginx/certs:ro
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- https://127.0.0.1/healthz --no-check-certificate >/dev/null || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 6
      start_period: 5s
"""


def run_local(cmd, timeout=None):
    print(f"  $ {cmd[:100]}")
    ret = os.system(cmd)
    if ret != 0:
        print(f"  ❌ Failed (exit={ret})")
        sys.exit(1)


def main():
    # Windows 安全临时路径（避免 Git Bash /tmp 与 Windows %TMP% 歧义）
    tar_path = os.path.join(tempfile.gettempdir(), "tm-prod-update.tar.gz")
    gz_path = None  # 兼容保留：当前直接 save+gzip 到 tar_path

    # ── 1. 本地打包 ──
    print("==> [1/6] 打包五镜像 ...")
    run_local(f'docker save {" ".join(IMAGES)} | gzip > "{tar_path}"')
    size_mb = os.path.getsize(tar_path) / 1024 / 1024
    print(f"  ✅ 包大小: {size_mb:.0f} MB")

    ssh = connect()
    try:
        # ── 2. 上传 ──
        print("\n==> [2/6] 上传镜像包 + nginx 材料 ...")
        sftp = ssh.open_sftp()
        sftp.put(tar_path, f"{REMOTE_ROOT}/prod-update.tar.gz")
        sftp.close()
        upload(ssh, NGINX_CONF, f"{REMOTE_ROOT}/nginx/nginx-https.conf")
        upload(ssh, os.path.join(CERT_DIR, "fullchain.pem"), f"{REMOTE_ROOT}/nginx/certs/fullchain.pem")
        upload(ssh, os.path.join(CERT_DIR, "privkey.pem"), f"{REMOTE_ROOT}/nginx/certs/privkey.pem")
        print("  ✅ 上传完成")

        # ── 3. 备份 tag + load ──
        print("\n==> [3/6] 备份旧镜像 tag（回滚点）+ 加载新镜像 ...")
        for img in [f"trustmesh/{s}" for s in SERVICES]:
            run(ssh, f"{DOCKER_ENV}docker tag {img}:{TAG} {img}:{TAG}-prev-{TS} 2>/dev/null || true", quiet=True)
        run(ssh, f"cd {REMOTE_ROOT} && {DOCKER_ENV}docker load < prod-update.tar.gz 2>&1 | tail -6")

        # ── 4. 注入 nginx-proxy 服务（幂等）──
        print("\n==> [4/6] 注入 nginx-proxy 服务 ...")
        code, out, _ = run(ssh, f"grep -c 'nginx-proxy:' {REMOTE_ROOT}/docker-compose.yml || true", quiet=True)
        if out.strip() == "0":
            run(ssh, f"cp {REMOTE_ROOT}/docker-compose.yml {REMOTE_ROOT}/docker-compose.yml.bak-{TS}", quiet=True)
            # 读回 compose，在 volumes: 段前插入服务块
            sftp = ssh.open_sftp()
            with sftp.open(f"{REMOTE_ROOT}/docker-compose.yml", "r") as f:
                compose = f.read().decode()
            marker = "\nvolumes:"
            assert marker in compose, "compose 中找不到 volumes: 段"
            compose = compose.replace(marker, NGINX_SERVICE_BLOCK + "\nvolumes:", 1)
            with sftp.open(f"{REMOTE_ROOT}/docker-compose.yml", "w") as f:
                f.write(compose)
            sftp.close()
            print("  ✅ 已注入（原文件备份 docker-compose.yml.bak-%s）" % TS)
        else:
            print("  ℹ️ nginx-proxy 已存在，跳过注入")

        # ── 5. .env 更新外部 URL ──
        print("\n==> [5/6] 更新 TRUSTMESH_EXTERNAL_URL ...")
        run(ssh, f"cp {REMOTE_ROOT}/.env {REMOTE_ROOT}/.env.bak-{TS}", quiet=True)
        run(ssh,
            f"sed -i 's|^TRUSTMESH_EXTERNAL_URL=.*|TRUSTMESH_EXTERNAL_URL=https://175.27.135.91|' {REMOTE_ROOT}/.env",
            quiet=True)
        run(ssh, f"grep TRUSTMESH_EXTERNAL_URL {REMOTE_ROOT}/.env")

        # ── 6. 更新服务 + 启动 nginx-proxy + 自测 ──
        print("\n==> [6/6] 更新四服务 + 启动 nginx-proxy ...")
        run(ssh, f"cd {REMOTE_ROOT} && {DOCKER_ENV}TAG={TAG} docker compose up -d --force-recreate {' '.join(SERVICES)} 2>&1 | tail -6", timeout=300)
        run(ssh, f"cd {REMOTE_ROOT} && {DOCKER_ENV}TAG={TAG} docker compose up -d nginx-proxy 2>&1 | tail -3", timeout=120)
        time.sleep(12)

        print("\n==> 服务状态：")
        run(ssh, f"cd {REMOTE_ROOT} && {DOCKER_ENV}docker compose ps --format 'table {{{{.Name}}}}\\t{{{{.Status}}}}'")

        print("\n==> HTTPS 本机自测（-k 忽略自签告警）：")
        run(ssh, f"curl -sk https://127.0.0.1/ -o /dev/null -w 'https / → %{{http_code}}\\n'")
        run(ssh, f"curl -sk https://127.0.0.1/healthz -w ' → healthz\\n'")
        run(ssh, f"curl -s  http://127.0.0.1/  -o /dev/null -w 'http  / → %{{http_code}} (期望301)\\n'")

        run(ssh, f"rm -f {REMOTE_ROOT}/prod-update.tar.gz", quiet=True)
        print(f"\n✅ 部署完成。回滚命令：docker tag trustmesh/<svc>:{TAG}-prev-{TS} trustmesh/<svc>:{TAG} && TAG={TAG} docker compose up -d --force-recreate <svc>")
    finally:
        ssh.close()
        for p in (tar_path, gz_path):
            if p and os.path.exists(p):
                os.remove(p)


if __name__ == "__main__":
    main()
