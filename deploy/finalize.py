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

    # Add backend port mapping to compose
    print("==> Adding backend port mapping...")
    run("cd /opt/trustmesh-test && "
        "sudo sed -i '/TRUSTMESH_EXTERNAL_URL/a\\      - \"8080:8080\"' docker-compose.yml && "
        "sudo sed -i '/TRUSTMESH_EXTERNAL_URL/a\\    ports:' docker-compose.yml")

    # Restart backend
    print("==> Restarting backend with port mapping...")
    run("cd /opt/trustmesh-test && sudo TAG=test docker compose up -d backend", timeout=60)

    import time
    time.sleep(8)

    # Verify
    ec, out, err = run("ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null || echo 'SS_NOT_FOUND'; curl -s http://127.0.0.1:8080/healthz")
    print(f"\nBackend check:\n{out[:200]}")
    
    ec, out, err = run("curl -s http://127.0.0.1:3000/ 2>&1 | head -3")
    print(f"\nFrontend:\n{out[:200]}")

    print("\n✅ Test environment ready!")
    print(f"   http://175.27.135.91:3000  (frontend)")
    print(f"   http://175.27.135.91:8080  (backend API)")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
