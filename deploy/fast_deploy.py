import paramiko, sys, os

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

# Tag existing images
print("==> Tagging existing images...")
os.system("docker tag trustmesh-clawsynapse:latest trustmesh/clawsynapse:test")
os.system("docker tag trustmesh/backend:test trustmesh/backend:test")
os.system("docker tag trustmesh/frontend:test trustmesh/frontend:test")
print("✅ Tags ready")

# Save only needed images (skip pulling)
IMAGES = [
    "trustmesh/backend:test",
    "trustmesh/frontend:test",
    "trustmesh/clawsynapse:test",
    "mongodb/mongodb-community-server:7.0-ubuntu2204",
    "qdrant/qdrant:latest",
]

tar_path = "/tmp/trustmesh-fast-deploy.tar.gz"
print(f"\n==> Saving images to {tar_path}...")
os.system(f'docker save {" ".join(IMAGES)} | gzip > {tar_path}')
size_mb = os.path.getsize(tar_path) / 1024 / 1024
print(f"✅ Archive size: {size_mb:.0f} MB")

# Upload
ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("\n✅ SSH connected, uploading...")
    sftp = ssh.open_sftp()
    sftp.put(tar_path, "/opt/trustmesh-test/fast-images.tar.gz")
    sftp.close()
    print("✅ Uploaded")

    # Fix Docker and load
    stdin, stdout, stderr = ssh.exec_command(
        "sudo rm -f /etc/docker/daemon.json && "
        "sudo systemctl stop docker 2>/dev/null; "
        "sudo pkill -9 dockerd 2>/dev/null; "
        "sudo rm -rf /var/run/docker* /run/docker*; "
        "sleep 2; "
        "sudo systemctl start docker; "
        "sleep 10; "
        "echo 'DOCKER_READY'; "
        "sudo docker ps",
        timeout=60
    )
    out = stdout.read().decode().strip()
    print(f"Docker restart: {out[:200]}")

    # Load images
    print("\n==> Loading images...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo docker load < fast-images.tar.gz 2>&1 | tail -10",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    # Update compose to use MongoDB 7.0
    print("\n==> Updating compose to use MongoDB 7.0...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && "
        "sudo sed -i 's|mongodb/mongodb-community-server:8.2-ubuntu2204|mongodb/mongodb-community-server:7.0-ubuntu2204|' docker-compose.yml && "
        "echo 'COMPOSE_UPDATED'",
        timeout=15
    )
    print(stdout.read().decode().strip())

    # Start services
    print("\n==> Starting all services...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:800])

    import time
    time.sleep(15)

    # Check
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1",
        timeout=15
    )
    print(f"\nServices:\n{stdout.read().decode().strip()}")

    # Cleanup
    ssh.exec_command("rm -f /opt/trustmesh-test/fast-images.tar.gz")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
    os.remove(tar_path)
