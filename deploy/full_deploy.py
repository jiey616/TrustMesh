import paramiko, sys, os, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

IMAGES = [
    "mongodb/mongodb-community-server:8.2-ubuntu2204",
    "qdrant/qdrant:latest",
]

TAG = "test"
LOCAL_IMAGES = [
    f"trustmesh/backend:{TAG}",
    f"trustmesh/frontend:{TAG}",
]

def run_local(cmd, timeout=600):
    print(f"  Running: {cmd[:100]}...")
    ret = os.system(cmd)
    if ret != 0:
        print(f"  ⚠️  Failed (exit={ret})")
    return ret

print("==> Pulling dependencies locally...")
for img in IMAGES:
    run_local(f"docker pull {img}")

# Build ClawSynapse image
print("\n==> Building ClawSynapse image...")
CLAWSYNAPSE_REF = "main"
CACHE_BUST = str(int(time.time()))
run_local(
    f'docker build --platform linux/amd64 '
    f'--build-arg CLAWSYNAPSE_REF={CLAWSYNAPSE_REF} '
    f'--build-arg CACHE_BUST={CACHE_BUST} '
    f'-t trustmesh/clawsynapse:{TAG} '
    f'"D:\\AIWorkspace\\TrustMesh\\deploy\\clawsynapse"'
)

ALL_IMAGES = IMAGES + LOCAL_IMAGES + [f"trustmesh/clawsynapse:{TAG}"]

# Save all images
tar_path = "/tmp/trustmesh-all-images.tar.gz"
print(f"\n==> Saving all images ({len(ALL_IMAGES)} images) to {tar_path}...")
run_local(f'docker save {" ".join(ALL_IMAGES)} | gzip > {tar_path}')

size_mb = os.path.getsize(tar_path) / 1024 / 1024
print(f"✅ Archive size: {size_mb:.0f} MB")

# Upload
ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("\n✅ SSH connected, uploading...")
    sftp = ssh.open_sftp()
    sftp.put(tar_path, "/opt/trustmesh-test/all-images.tar.gz")
    sftp.close()
    print("✅ Uploaded")

    # Fix Docker first (remove daemon.json that breaks it)
    stdin, stdout, stderr = ssh.exec_command(
        "sudo rm -f /etc/docker/daemon.json && "
        "sudo systemctl stop docker 2>/dev/null; "
        "sudo pkill -9 dockerd 2>/dev/null; "
        "sudo rm -rf /var/run/docker* /run/docker*; "
        "sudo systemctl start docker; "
        "sleep 10; "
        "docker ps",
        timeout=60
    )
    out = stdout.read().decode().strip()
    print(f"Docker restart: {out[:100]}")

    # Load images
    print("\n==> Loading images (this may take a while)...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && "
        "sudo docker load < all-images.tar.gz 2>&1 | tail -10",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    # Start services
    print("\n==> Starting services...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    time.sleep(15)

    # Check services
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1",
        timeout=15
    )
    print(f"\nServices:")
    print(stdout.read().decode().strip())

    # Cleanup
    ssh.exec_command("rm -f /opt/trustmesh-test/all-images.tar.gz")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
    os.remove(tar_path)
