import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=30):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Kill existing dockerd
    print("\n==> Cleaning up old Docker processes...")
    run("sudo kill -9 $(cat /var/run/docker.pid 2>/dev/null) 2>/dev/null || true")
    run("sudo kill -9 $(cat /var/run/docker/containerd/containerd.pid 2>/dev/null) 2>/dev/null || true")
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo pkill -9 containerd 2>/dev/null || true")
    run("sudo rm -f /var/run/docker.pid /var/run/docker/containerd/containerd.pid 2>/dev/null || true")
    time.sleep(3)

    # Start Docker via systemd
    print("\n==> Starting Docker via systemd...")
    ec, out, err = run("sudo systemctl start docker 2>&1", timeout=30)
    if ec != 0:
        print(f"systemctl failed: {err[:200]}")
        run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &")
        time.sleep(6)
    else:
        print("systemctl start OK")
        time.sleep(3)

    # Test Docker
    print("\n==> Testing Docker...")
    ec, out, err = run("sudo docker ps", timeout=15)
    print(out[:200])
    if ec == 0 and len(out) > 0:
        print("✅ Docker is running!")
    else:
        ec, out, err = run("tail -15 /tmp/dockerd.log 2>/dev/null")
        print(f"Log: {out[:300]}")

    # Test compose
    ec, out, err = run("sudo docker compose version", timeout=15)
    print(f"Compose: {out[:200]}")

    # Set up deploy dir
    print("\n==> Setting up deployment directory...")
    run("sudo rm -rf /opt/trustmesh-test")
    run("sudo mkdir -p /opt/trustmesh-test")
    run("sudo chown cloudUser:cloudUser /opt/trustmesh-test")
    print("✅ /opt/trustmesh-test ready")

    # Generate .env
    env = """PLATFORM_NAME=TrustMesh
PORT=8080
LOG_LEVEL=debug
ALLOW_ALL_CORS=true
JWT_SECRET=test-jwt-secret-for-testing-only-1234567890
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=168h
MONGO_ENABLED=true
MONGO_URI=mongodb://mongo:27017
MONGO_DATABASE=trustmesh
MONGO_TIMEOUT=5s
CLAWSYNAPSE_API_URL=http://clawsynapse:18080
CLAWSYNAPSE_TIMEOUT=3s
CLAWSYNAPSE_PEER_SYNC_INTERVAL=10s
CLAWSYNAPSE_NODE_ID=trustmesh-server
CLAWSYNAPSE_LOCAL_API_ADDR=0.0.0.0:18080
CLAWSYNAPSE_TRUST_MODE=open
CLAWSYNAPSE_AGENT_ADAPTER=webhook
CLAWSYNAPSE_WEBHOOK_URL=http://backend:8080/webhook/clawsynapse
CLAWSYNAPSE_DATA_DIR=/var/lib/clawsynapse
CLAWSYNAPSE_LOG_LEVEL=info
CLAWSYNAPSE_LOG_FORMAT=json
NATS_SERVERS=nats://220.168.146.21:9414
DELIVERABLE_PREFIXES=chat,task,todo,conversation
TRANSFER_DIR=/var/lib/trustmesh-transfers
TRANSFER_TTL=24h
TRUSTMESH_EXTERNAL_URL=http://175.27.135.91:62000
DOWNLOAD_TOKEN_TTL=24h
WRITE_TIMEOUT=0
"""
    run(f"cat > /opt/trustmesh-test/.env << 'ENVEOF'\n{env}ENVEOF")
    print("✅ .env created")

    # Get docker-compose.yml
    print("\n==> Copying docker-compose.yml...")
    import os
    local_compose = r"D:\AIWorkspace\TrustMesh\docker-compose.prod.yml"
    sftp = ssh.open_sftp()
    sftp.put(local_compose, "/opt/trustmesh-test/docker-compose.yml")
    sftp.close()
    print("✅ docker-compose.yml uploaded")

    print("\n" + "="*50)
    print("✅ Docker installed and configured!")
    print("Ready to deploy TrustMesh.")
    print("Next step: build and push Docker images, then:")
    print("  cd /opt/trustmesh-test && docker compose up -d")
    print("="*50)

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
