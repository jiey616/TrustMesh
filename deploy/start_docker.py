import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=60):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    DOCKER = "/home/cloudUser/.local/bin/docker"
    PATH = f"export PATH=/home/cloudUser/.local/bin:$PATH && export DOCKER_HOST=unix:///run/user/1000/docker.sock"

    # 1. Fix docker-compose permissions
    run("chmod +x /home/cloudUser/.docker/cli-plugins/docker-compose")
    print("✅ docker-compose permission fixed")

    # 2. Install rootless docker dependencies
    run("yum install -y fuse3 fuse3-libs 2>/dev/null || true")

    # 3. Start dockerd in rootless mode
    print("\n==> Starting rootless Docker daemon...")
    run("dockerd-rootless-setuptool.sh install 2>/dev/null || true")
    ec, out, err = run(f"{PATH} && {DOCKER} info 2>&1 | head -3", timeout=30)
    if "Server Version" in out:
        print("✅ Docker daemon is running!")
        print(out[:300])
    else:
        # Try starting manually
        print(f"   Attempting manual start...")
        stdin, stdout, stderr = ssh.exec_command(
            f"nohup {DOCKER}d --rootless > /tmp/dockerd.log 2>&1 &",
            timeout=10
        )
        import time
        time.sleep(5)
        
        ec, out, err = run(f"{PATH} && {DOCKER} info 2>&1 | head -5", timeout=15)
        print(out[:500] if out else "(empty)")
        if err: print(f"ERR: {err[:300]}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
