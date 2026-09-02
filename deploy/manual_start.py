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
    transport.set_keepalive(30)

    def run(cmd, timeout=15):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Step by step with short timeouts
    print("1. Kill all containers...")
    run("sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null", 10)
    print("2. Clean mongo lock...")
    run("sudo find /var/lib/docker/volumes -name mongod.lock -delete 2>/dev/null", 5)

    # Remove daemon.json to avoid pull issues
    run("sudo rm -f /etc/docker/daemon.json", 5)
    
    time.sleep(2)

    # Use docker run instead of compose to avoid image pull check
    print("3. Starting mongo...")
    mongo = ("sudo docker run -d --name trustmesh-mongo --network trustmesh_default "
             "--restart unless-stopped "
             "-v trustmesh_trustmesh-mongo-data:/data/db "
             "mongodb/mongodb-community-server:7.0-ubuntu2204")
    run(mongo, 30)
    time.sleep(3)

    print("4. Starting qdrant...")
    qdrant = ("sudo docker run -d --name trustmesh-qdrant --network trustmesh_default "
              "--restart unless-stopped "
              "qdrant/qdrant:latest")
    run(qdrant, 30)
    time.sleep(3)

    print("5. Starting backend...")
    backend = ("sudo docker run -d --name trustmesh-backend --network trustmesh_default "
               "--restart unless-stopped --health-cmd 'wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1' "
               "-p 8080:8080 "
               "-e PLATFORM_NAME=TrustMesh "
               "-e MONGO_URI=mongodb://mongo:27017 "
               "-e ALLOW_ALL_CORS=true "
               "-e CLAWSYNAPSE_API_URL=http://clawsynapse:18080 "
               "-e CLAWSYNAPSE_NODE_ID=trustmesh-server "
               "-e CLAWSYNAPSE_WEBHOOK_URL=http://backend:8080/webhook/clawsynapse "
               "-e NATS_SERVERS=nats://220.168.146.21:9414 "
               "-e JWT_SECRET=test-jwt-secret-for-testing-only-1234567890 "
               "-e WRITE_TIMEOUT=0 "
               "-e TRUSTMESH_EXTERNAL_URL=http://175.27.135.91:62000 "
               "trustmesh/backend:test")
    run(backend, 30)
    time.sleep(3)

    print("6. Starting clawsynapse...")
    clawsynapse = ("sudo docker run -d --name trustmesh-clawsynapse --network trustmesh_default "
                   "--restart unless-stopped "
                   "-e NODE_ID=trustmesh-server "
                   "-e NATS_SERVERS=nats://220.168.146.21:9414 "
                   "-e AGENT_ADAPTER=webhook "
                   "-e WEBHOOK_URL=http://backend:8080/webhook/clawsynapse "
                   "trustmesh/clawsynapse:test")
    run(clawsynapse, 30)
    time.sleep(3)

    print("7. Starting frontend...")
    frontend = ("sudo docker run -d --name trustmesh-frontend --network trustmesh_default "
                "--restart unless-stopped -p 3000:80 "
                "trustmesh/frontend:test")
    run(frontend, 30)

    time.sleep(10)

    out = run("sudo docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'", 15)
    print(f"\n{out}")

    out = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz", 10)
    print(f"Backend: HTTP {out}")

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
