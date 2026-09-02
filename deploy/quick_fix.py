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

    # 1. Kill containers
    run("sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null", 10)
    print("✅ Containers removed")

    # 2. Delete mongo lock files via volume path
    run("sudo sh -c 'find /var/lib/docker/volumes -name mongod.lock -delete'", 10)
    print("✅ Mongo lock cleaned")

    # 3. Start with --pull missing (don't pull if local exists)
    run("cd /opt/trustmesh-test && sudo TAG=test docker compose up -d --pull missing 2>&1", 60)
    print("✅ compose up done")

    time.sleep(15)

    # 4. Check
    out = run("cd /opt/trustmesh-test && sudo docker compose ps", 15)
    print(f"\n{out}")

    out = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz", 10)
    print(f"Backend: HTTP {out}")

    out = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/", 10)
    print(f"Frontend: HTTP {out}")

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
