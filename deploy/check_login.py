import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=15):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check frontend API URL config
    ec, out, err = run("sudo cat /opt/trustmesh-test/.env | grep -i 'platform\|api\|url\|cors\|external'")
    print(f"ENV:\n{out}")

    # Check backend responds to platform/info directly
    ec, out, err = run("curl -s http://127.0.0.1:8080/api/v1/platform/info 2>&1")
    print(f"\nPlatform API:\n{out[:200]}")

    # Check docker-compose frontend config
    ec, out, err = run("sudo cat /opt/trustmesh-test/docker-compose.yml | grep -A5 'frontend:'")
    print(f"\nFrontend compose:\n{out[:300]}")

    # Also check if the frontend can reach the backend API
    ec, out, err = run("sudo docker exec trustmesh-frontend curl -s http://backend:8080/api/v1/platform/info 2>&1")
    print(f"\nFrontend -> Backend:\n{out[:200]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
