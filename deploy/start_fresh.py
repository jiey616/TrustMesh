import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    transport = ssh.get_transport()

    def run(cmd, timeout=15):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Create network first
    run("sudo docker network create trustmesh_default 2>/dev/null || true", 5)
    run("sudo docker volume create trustmesh_trustmesh-mongo-data 2>/dev/null || true", 5)
    print("✅ Network and volumes ready")

    # Run each container with specific network
    print("\nStarting mongo...")
    run("sudo docker run -d --name trustmesh-mongo --network trustmesh_default --restart unless-stopped -v trustmesh_trustmesh-mongo-data:/data/db mongodb/mongodb-community-server:7.0-ubuntu2204", 30)
    time.sleep(3)

    print("Starting qdrant...")
    run("sudo docker run -d --name trustmesh-qdrant --network trustmesh_default --restart unless-stopped qdrant/qdrant:latest", 30)
    time.sleep(3)

    print("Starting backend...")
    run("sudo docker run -d --name trustmesh-backend --network trustmesh_default "
        "--restart unless-stopped "
        "-p 8080:8080 "
        "-e PLATFORM_NAME=TrustMesh "
        "-e MONGO_URI=mongodb://mongo:27017 "
        "-e ALLOW_ALL_CORS=true "
        "-e JWT_SECRET=test-jwt-secret-for-testing-only-1234567890 "
        "-e CLAWSYNAPSE_API_URL=http://clawsynapse:18080 "
        "-e CLAWSYNAPSE_NODE_ID=trustmesh-server "
        "-e CLAWSYNAPSE_WEBHOOK_URL=http://backend:8080/webhook/clawsynapse "
        "-e NATS_SERVERS=nats://220.168.146.21:9414 "
        "-e WRITE_TIMEOUT=0 "
        "-e TRUSTMESH_EXTERNAL_URL=http://175.27.135.91:62000 "
        "trustmesh/backend:test", 30)
    time.sleep(3)

    print("Starting clawsynapse...")
    run("sudo docker run -d --name trustmesh-clawsynapse --network trustmesh_default "
        "--restart unless-stopped "
        "-e NODE_ID=trustmesh-server "
        "-e NATS_SERVERS=nats://220.168.146.21:9414 "
        "-e AGENT_ADAPTER=webhook "
        "-e WEBHOOK_URL=http://backend:8080/webhook/clawsynapse "
        "trustmesh/clawsynapse:test", 30)
    time.sleep(3)

    print("Starting frontend...")
    run("sudo docker run -d --name trustmesh-frontend --network trustmesh_default "
        "--restart unless-stopped -p 3000:80 "
        "trustmesh/frontend:test", 30)

    time.sleep(10)

    out = run("sudo docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'", 10)
    print(f"\n{out}")

    out = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz", 10)
    print(f"\nBackend health: HTTP {out}")

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
