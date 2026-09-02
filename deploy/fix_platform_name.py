import paramiko
import sys

# ⚠️ 已停用（2026-09-01 用户确认）：TrustMesh 生产机已切换为 175.27.135.91:62000
#    （cloudUser@/opt/trustmesh-test，见 ssh_helper.py）。36.137.106.15 不再承载
#    TrustMesh 生产部署。确认要复用本脚本前，请先把下面的连接参数改到新生产机
#    并删掉这段护栏。
sys.exit(
    "⛔ 本脚本已停用：生产机不再是 36.137.106.15。\n"
    "   现在生产 = 175.27.135.91:62000 (cloudUser, /opt/trustmesh-test)，见 deploy/ssh_helper.py。\n"
    "   确认后请更新连接参数并删除本护栏。"
)

HOST = "36.137.106.15"
USER = "root"
PASS = "Lh1804@1806"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, username=USER, password=PASS, timeout=10)
    print("✅ SSH connected")

    # Upload updated docker-compose.prod.yml
    local_path = r"D:\AIWorkspace\TrustMesh\docker-compose.prod.yml"
    remote_path = "/opt/trustmesh/docker-compose.yml"
    
    sftp = ssh.open_sftp()
    sftp.put(local_path, remote_path)
    sftp.close()
    print("✅ docker-compose.yml uploaded")

    # Restart backend
    stdin, stdout, stderr = ssh.exec_command("cd /opt/trustmesh && docker compose up -d backend", timeout=60)
    print(stdout.read().decode().strip())

    # Verify
    stdin, stdout, stderr = ssh.exec_command("docker exec trustmesh-backend sh -c 'echo PLATFORM_NAME=$PLATFORM_NAME'")
    print(f"Container PLATFORM_NAME: {stdout.read().decode().strip()}")

    print("\n✅ Done!")
except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
