import paramiko
import sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check server info
    stdin, stdout, stderr = ssh.exec_command("cat /etc/os-release | head -3 && echo '---' && uname -m && echo '---' && docker --version 2>/dev/null || echo 'DOCKER_NOT_FOUND' && echo '---' && docker compose version 2>/dev/null || echo 'COMPOSE_NOT_FOUND'")
    print(stdout.read().decode().strip())

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
