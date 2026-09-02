import paramiko, sys, time

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

    # Validate JSON
    ec, out, err = run("sudo python3 -m json.tool /etc/docker/daemon.json")
    if ec != 0:
        print(f"❌ Invalid JSON: {err[:200]}")
    else:
        print("✅ daemon.json is valid JSON")

    # Force restart Docker
    print("\n==> Full Docker restart...")
    run("sudo systemctl daemon-reload")
    run("sudo systemctl stop docker")
    time.sleep(3)
    # Ensure no leftover processes
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo pkill -9 containerd 2>/dev/null || true")
    time.sleep(2)
    run("sudo systemctl start docker")
    time.sleep(5)

    ec, out, err = run("sudo docker info 2>&1 | grep -A5 Mirrors", timeout=15)
    print(f"Registry Mirrors: {out[:500]}")

    if "Mirrors" in out and "https://docker.m.daocloud.io" in out:
        print("✅ Mirrors configured!")
    else:
        print("⚠️  Mirrors not showing in docker info, trying alternative approach...")
        # Try environment variable approach
        run("sudo mkdir -p /etc/systemd/system/docker.service.d")
        override = """[Service]
Environment="DOCKER_OPTS=--registry-mirror=https://docker.m.daocloud.io --registry-mirror=https://docker.xuanyuan.me --registry-mirror=https://docker.1ms.run"
"""
        run(f"cat > /tmp/override.conf << 'OVEOF'\n{override}OVEOF")
        run("sudo cp /tmp/override.conf /etc/systemd/system/docker.service.d/override.conf")
        run("sudo systemctl daemon-reload")
        run("sudo systemctl restart docker")
        time.sleep(5)
        ec, out, err = run("sudo docker info 2>&1 | grep -A5 'Registry Mirrors'", timeout=15)
        print(f"After override - Mirrors: {out[:500]}")

    # Test pull
    print("\n==> Testing pull...")
    ec, out, err = run("sudo docker pull alpine:3.18 2>&1", timeout=120)
    if ec == 0:
        print("✅ Pull successful!")
    else:
        print(f"❌ Pull failed: {err[:200]}")

    # Try starting services
    print("\n==> Starting services...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=600
    )
    print(out[:1000])
    
    time.sleep(15)
    
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1",
        timeout=15
    )
    print(f"\nServices:")
    print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
