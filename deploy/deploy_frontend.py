import paramiko
import sys, os

# ⚠️ 已停用（2026-09-01 用户确认）：TrustMesh 生产机已切换为 175.27.135.91:62000
#    （cloudUser@/opt/trustmesh-test，见 ssh_helper.py）。36.137.106.15 不再承载
#    TrustMesh 生产部署；那边只剩 TokenFlow LLM 网关（:3030），不是本脚本目标。
#    复用前请改连接参数+远端路径并删除这段护栏。
sys.exit(
    "⛔ 本脚本已停用：生产机不再是 36.137.106.15。\n"
    "   现在生产 = 175.27.135.91:62000 (cloudUser, /opt/trustmesh-test)，见 deploy/ssh_helper.py。\n"
    "   确认后请更新连接参数并删除本护栏。"
)

HOST = "36.137.106.15"
USER = "root"
PASS = "Lh1804@1806"

local_tag = "latest"
local_image = f"trustmesh/frontend:{local_tag}"

print("==> Building frontend image for linux/amd64...")
# Build
cmd = f'docker build --platform linux/amd64 -t {local_image} "D:\\AIWorkspace\\TrustMesh\\frontend"'
ret = os.system(cmd)
if ret != 0:
    print("❌ Build failed")
    sys.exit(1)
print("✅ Build done")

# Save
tar_path = "/tmp/trustmesh-frontend-latest.tar.gz"
cmd = f'docker save {local_image} | gzip > {tar_path}'
ret = os.system(cmd)
if ret != 0:
    print("❌ Save failed")
    sys.exit(1)
print("✅ Image saved")

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, username=USER, password=PASS, timeout=10)
    print("✅ SSH connected")

    # Upload
    sftp = ssh.open_sftp()
    remote_path = "/opt/trustmesh/trustmesh-frontend-latest.tar.gz"
    sftp.put(tar_path, remote_path)
    sftp.close()
    print("✅ Image uploaded")

    # Load and restart
    stdin, stdout, stderr = ssh.exec_command("cd /opt/trustmesh && docker load < trustmesh-frontend-latest.tar.gz && docker compose up -d frontend", timeout=120)
    output = stdout.read().decode()
    print(output[:500])

    # Cleanup
    ssh.exec_command("rm /opt/trustmesh/trustmesh-frontend-latest.tar.gz")

    print("\n✅ Frontend deployed!")
except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
    os.remove(tar_path)
