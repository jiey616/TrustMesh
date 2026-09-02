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

    # Kill everything
    run("sudo systemctl stop docker 2>/dev/null || true")
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo rm -rf /var/run/docker* /run/docker* /etc/docker/daemon.json 2>/dev/null || true")
    time.sleep(3)

    # Start dockerd directly with debug output
    print("==> Starting dockerd directly...")
    stdin, stdout, stderr = ssh.exec_command(
        "sudo /usr/bin/dockerd --debug > /tmp/dockerd.log 2>&1 &",
        timeout=10
    )
    time.sleep(10)

    # Check log for errors
    ec, out, err = run("tail -30 /tmp/dockerd.log 2>&1")
    print(f"Log:\n{out[:800]}")

    # Check socket
    ec, out, err = run("ls -la /var/run/docker.sock 2>&1")
    print(f"\nSocket: {out[:100]}")

    # Try docker
    ec, out, err = run("sudo docker ps 2>&1")
    print(f"\nDocker ps: {out[:200]}")

    if "Cannot connect" in out:
        print("\n❌ Docker still broken on CentOS 7.")
        print("   This is a known issue with Docker CE on CentOS 7.")
        print("   Alternative: install Docker via the convenience script:")
        print("   curl -fsSL https://get.docker.com | sh")
    else:
        print("\n✅ Docker running! Loading images...")
        run("cd /opt/trustmesh-test && sudo docker load < fast-images.tar.gz 2>&1 | tail -5", timeout=120)
        ec, out, err = run("sudo docker images --format '{{.Repository}}:{{.Tag}}' | head -10")
        print(f"Loaded:\n{out}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
