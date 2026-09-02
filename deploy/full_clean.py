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

    def run(cmd, timeout=10):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Full cleanup
    run("sudo pkill -9 dockerd 2>/dev/null || true", 5)
    run("sudo pkill -9 containerd 2>/dev/null || true", 5)
    run("sudo rm -f /var/run/docker.pid /var/run/docker.sock 2>/dev/null || true", 5)
    run("sudo rm -rf /var/run/docker/* /run/docker/* 2>/dev/null || true", 5)
    time.sleep(3)

    # Also check for leftover processes from old docker runs
    run("sudo sh -c 'for pid in $(cat /var/run/docker.pid 2>/dev/null); do kill -9 $pid 2>/dev/null; done'", 5)
    run("sudo rm -f /var/run/docker.pid", 5)

    # Start fresh dockerd
    run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &", 5)
    time.sleep(8)

    out = run("sudo docker ps 2>&1", 8)
    print(f"docker ps: {out[:80]}")

    if "Cannot connect" not in out:
        print("✅ Docker running!")

        # Now start services using docker-compose with COMPOSE_DISABLE_PULL
        ch = transport.open_session()
        ch.settimeout(120)
        ch.exec_command("cd /opt/trustmesh-test && sudo COMPOSE_DISABLE_PULL=true TAG=test docker compose up -d 2>&1")
        time.sleep(5)
        # Read partial output
        ch.shutdown_write()
        out = ""
        while ch.recv_ready():
            out += ch.recv(1024).decode()
        ch.close()
        print(f"compose output: {out[:300]}")

        time.sleep(15)

        out = run("sudo docker ps --format 'table {{.Names}}\t{{.Status}}'", 10)
        print(f"\n{out}")

        out = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz 2>&1", 10)
        print(f"\nBackend: HTTP {out}")
    else:
        print("❌ Docker still not working")
        out = run("tail -10 /tmp/dockerd.log", 8)
        print(out[:300])

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
