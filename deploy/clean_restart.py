import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    print("Connecting...")
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    print("✅ SSH connected\n")

    def run(cmd, timeout=30):
        ch = ssh.get_transport().open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        err = ch.makefile_stderr('r', 4096).read().decode().strip()
        ch.close()
        return out, err

    # 1. Kill stuck containers
    print("1. Killing stuck containers...")
    out, err = run("sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null || true", 15)
    time.sleep(3)

    # 2. Verify Docker works
    out, err = run("sudo docker ps", 10)
    print(f"   docker ps: {out[:50]}")

    # 3. Start services
    print("2. Starting services...")
    out, err = run("cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1", 120)
    print(f"   {out[:300]}")

    time.sleep(15)

    # 4. Check
    out, err = run("cd /opt/trustmesh-test && sudo docker compose ps", 15)
    print(f"\n3. Services:\n{out}")

    out, err = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz", 10)
    print(f"\n4. Backend: HTTP {out}")

    out, err = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/", 10)
    print(f"   Frontend: HTTP {out}")

    # 5. Re-enable docker systemd
    out, err = run("sudo systemctl enable docker 2>&1 && sudo systemctl is-enabled docker", 10)
    print(f"\n5. Docker systemd: {out[:100]}")

    print("\n✅ Done!")

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
    sys.exit(1)
finally:
    ssh.close()
