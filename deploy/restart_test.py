import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=60):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Stop everything
    print("\n==> 1. Cleaning up Docker...")
    run("cd /opt/trustmesh-test && sudo TAG=test docker compose down 2>/dev/null || true")
    run("sudo systemctl stop docker 2>/dev/null || true")
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo pkill -9 containerd 2>/dev/null || true")
    time.sleep(3)
    run("sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null || true")
    time.sleep(2)

    # Remove problematic daemon.json (the one we had caused crashes before)
    run("sudo rm -f /etc/docker/daemon.json")

    # Start Docker fresh
    print("==> 2. Starting Docker daemon...")
    run("sudo nohup /usr/bin/dockerd --registry-mirror=https://docker.m.daocloud.io --registry-mirror=https://docker.xuanyuan.me --registry-mirror=https://docker.1ms.run > /tmp/dockerd.log 2>&1 &")
    time.sleep(10)

    # Verify
    ec, out, err = run("sudo docker ps", timeout=15)
    if "Cannot connect" in out:
        print(f"❌ Docker start failed, checking log...")
        run("tail -20 /tmp/dockerd.log")
    else:
        print("✅ Docker daemon running")

    # Start services
    print("\n==> 3. Starting services...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=120
    )
    print(out[:500])

    time.sleep(15)

    # Final check
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"\n== Services ==\n{out}")

    ec, out, err = run("curl -s -o /dev/null -w 'Backend: HTTP %{http_code}\nFrontend: HTTP ' http://127.0.0.1:8080/healthz && curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/ 2>&1")
    print(f"\n== Health ==\n{out}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
