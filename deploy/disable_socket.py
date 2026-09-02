import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15, banner_timeout=30)
    print("✅ SSH connected")

    # Disable docker.socket (interferes with docker.service)
    cmd = (
        "sudo systemctl stop docker.socket 2>/dev/null; "
        "sudo systemctl disable docker.socket 2>/dev/null; "
        "sudo systemctl stop docker 2>/dev/null; "
        "sudo pkill -9 dockerd 2>/dev/null || true; "
        "sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null; "
        "sleep 3; "
        "sudo systemctl start docker; "
        "sleep 10; "
        "sudo ls -la /var/run/docker.sock; "
        "sudo docker ps"
    )
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=60)
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    print(out[:500])
    if err: print(f"ERR: {err[:200]}")
    
    if "srw" in out:
        print("\n✅ Docker socket exists! Services starting...")
        stdin, stdout, stderr = ssh.exec_command(
            "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1", timeout=60
        )
        print(stdout.read().decode().strip()[:300])
        time.sleep(10)
        stdin, stdout, stderr = ssh.exec_command(
            "cd /opt/trustmesh-test && sudo docker compose ps 2>&1", timeout=15
        )
        print(f"\n{stdout.read().decode().strip()}")
    else:
        print("\n❌ Docker socket still not available")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
