import paramiko, sys

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
    print("✅ SSH connected\n")

    # 1. Docker system info
    ec, out, err = run("sudo docker info 2>&1 | grep -E 'Server Version|Storage|Logging|Cgroup|containers|Images|Memory'")
    print("== Docker Info ==")
    print(out[:300])

    # 2. All services status
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print("\n== Services ==")
    print(out)

    # 3. Resource usage
    ec, out, err = run("free -h | head -2 && echo '---' && df -h / | tail -1")
    print("\n== Resources ==")
    print(out)

    # 4. Backend health
    ec, out, err = run("curl -s -o /dev/null -w 'Backend health: HTTP %{http_code}' http://127.0.0.1:8080/healthz 2>&1")
    print(f"\n== Health Checks ==")
    print(out)

    # 5. Frontend
    ec, out, err = run("curl -s -o /dev/null -w 'Frontend: HTTP %{http_code}' http://127.0.0.1:3000/ 2>&1")
    print(out)

    # 6. Platform API
    ec, out, err = run("curl -s http://127.0.0.1:3000/api/v1/platform/info 2>&1")
    print(f"\nPlatform API: {out[:100]}")

    # 7. ClawSynapse health
    ec, out, err = run("curl -s -o /dev/null -w 'ClawSynapse: HTTP %{http_code}' http://127.0.0.1:18080/v1/health 2>&1")
    print(out)

    # 8. Service logs (recent errors)
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose logs --tail=10 backend 2>&1 | grep -iE 'error|warn|panic|fail' | tail -5 || echo '(no errors)')")
    print(f"\n== Backend Errors (recent) ==\n{out}")

    # 9. Docker disk usage
    ec, out, err = run("sudo docker system df 2>&1")
    print(f"\n== Docker Disk ==\n{out}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
