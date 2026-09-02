import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=30):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    PATH = "export PATH=/home/cloudUser/.local/bin:$PATH && export DOCKER_HOST=unix:///run/user/1000/docker.sock"

    # 1. Test Docker
    ec, out, err = run(f"{PATH} && docker ps", timeout=15)
    if ec == 0:
        print("✅ Docker daemon is running!")
        print(out[:500])
    else:
        print(f"❌ Docker daemon not ready: {err[:200]}")
        sys.exit(1)

    # 2. Check NATS connectivity
    ec, out, err = run("timeout 5 bash -c 'echo | nc -w3 220.168.146.21 9414' 2>&1 || echo 'UNREACHABLE'", timeout=10)
    print(f"\nNATS reachable: {'UNREACHABLE' not in out and 'Connection refused' not in err}")

    # 3. Create deploy directory
    run("mkdir -p /home/cloudUser/trustmesh-test")
    print("✅ /home/cloudUser/trustmesh-test created")

    # 4. Generate test .env
    env_content = """PORT=8080
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
    cmd = f"cat > /home/cloudUser/trustmesh-test/.env << 'ENVEOF'\n{env_content}ENVEOF"
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=10)
    err = stderr.read().decode().strip()
    if err: print(f"  .env write err: {err}")
    print("✅ .env created")

    # 5. Show what's needed next
    print("\n" + "="*60)
    print("Docker is running. Next steps needed:")
    print("1. Copy docker-compose.yml to the server")
    print("2. Docker image needs to be built and pushed")
    print("3. docker compose up -d")
    print("="*60)

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
