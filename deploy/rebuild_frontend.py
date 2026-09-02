import paramiko, sys, os, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

print("==> Building frontend (no cache)...")
os.system(
    'docker build --no-cache --platform linux/amd64 '
    '-t trustmesh/frontend:test '
    '"D:\\AIWorkspace\\TrustMesh\\frontend"'
)

print("==> Building backend (no cache)...")
os.system(
    'docker build --no-cache --platform linux/amd64 '
    '-t trustmesh/backend:test '
    '"D:\\AIWorkspace\\TrustMesh\\backend"'
)

tar_path = r"C:\Users\38643\AppData\Local\Temp\fresh-images.tar.gz"
print(f"\n==> Saving images...")
os.system(f'docker save trustmesh/frontend:test trustmesh/backend:test | gzip > {tar_path}')

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("\n✅ SSH connected, uploading...")
    sftp = ssh.open_sftp()
    sftp.put(tar_path, "/opt/trustmesh-test/fresh-images.tar.gz")
    sftp.close()
    print("✅ Uploaded")

    print("==> Loading and restarting...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && "
        "sudo docker load < fresh-images.tar.gz 2>&1 && "
        "sudo TAG=test docker compose up -d backend frontend 2>&1",
        timeout=120
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    time.sleep(10)

    # Verify
    stdin, stdout, stderr = ssh.exec_command(
        "curl -s http://127.0.0.1:3000/api/v1/platform/info 2>&1",
        timeout=15
    )
    print(f"\nAPI: {stdout.read().decode().strip()}")

    # Cleanup
    stdin, stdout, stderr = ssh.exec_command(
        "sudo docker exec trustmesh-frontend sh -c 'grep -o \"font-bold.tracking-wide.*<\" /usr/share/nginx/html/assets/index-*.js' 2>&1 | head -3"
    )
    print(f"PlatformName in bundle:\n{stdout.read().decode().strip()[:200]}")

    ssh.exec_command("rm -f /opt/trustmesh-test/fresh-images.tar.gz")

    print("\n✅ Redeployed! Ctrl+Shift+R refresh the login page.")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
    os.remove(tar_path)
