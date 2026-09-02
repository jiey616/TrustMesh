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

    # Check compose status
    print("\n==> Checking services...")
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1", timeout=15)
    print(out[:500])
    if err: print(f"ERR: {err[:200]}")

    # Check docker info
    ec, out, err = run("sudo docker info 2>&1 | head -10", timeout=15)
    print(f"\nDocker info: {out[:300]}")

    # Check port availability
    ec, out, err = run("ss -tlnp | grep -E '8080|3000|27017|6333|18080'", timeout=10)
    print(f"\nPorts:")
    print(out[:300] if out else "(none)")

    # Try starting again with explicit sudo
    print("\n==> Starting services with sudo...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=120
    )
    print(out[:500])
    if err: print(f"ERR: {err[:200]}")

    time.sleep(10)

    # Check again
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1", timeout=15)
    print(f"\nServices:")
    print(out[:500])

    # Check logs of failed services
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose logs --tail=20 backend 2>&1", timeout=15)
    print(f"\nBackend log:")
    print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
