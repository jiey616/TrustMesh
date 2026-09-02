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
    print("✅ SSH connected")

    # Check all services in detail
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"Services:\n{out}")

    # Check backend health
    ec, out, err = run("curl -s http://127.0.0.1:8080/healthz 2>&1")
    print(f"\nBackend health: {out[:100]}")

    # Check frontend
    ec, out, err = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/ 2>&1")
    print(f"Frontend HTTP: {out}")

    # Check port mapping
    ec, out, err = run("ss -tlnp | grep -E '8080|3000|18080' 2>&1")
    print(f"\nListening ports:\n{out}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
