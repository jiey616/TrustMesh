import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15, banner_timeout=30)
    print("✅ SSH connected")

    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo docker compose ps && echo '---' "
        "&& sudo systemctl status docker --no-pager 2>&1 | grep Active "
        "&& echo '---' "
        "&& curl -s -o /dev/null -w 'Backend: HTTP %{http_code}\n' http://127.0.0.1:8080/healthz "
        "&& curl -s -o /dev/null -w 'Frontend: HTTP %{http_code}' http://127.0.0.1:3000/",
        timeout=30
    )
    print(stdout.read().decode().strip())

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
