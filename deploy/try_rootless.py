import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=120):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    PATH = "export PATH=/home/cloudUser/.local/bin:$PATH"
    XDG = "export XDG_RUNTIME_DIR=/run/user/1000"
    DOCKER_HOST = "export DOCKER_HOST=unix:///run/user/1000/docker.sock"

    # Kill old dockerd
    run("pkill -f rootlesskit 2>/dev/null; pkill -f dockerd 2>/dev/null")
    time.sleep(2)

    # Start rootless docker using rootlesskit directly
    print("\n==> Starting rootless Docker with rootlesskit...")
    stdin, stdout, stderr = ssh.exec_command(
        f"mkdir -p /run/user/1000 && "
        f"{PATH} && {XDG} && "
        f"nohup rootlesskit --net=slirp4netns --mtu=65520 --disable-host-loopback "
        f"--port-driver=builtin "
        f"--state-dir=/home/cloudUser/.local/share/docker-rootless "
        f"/home/cloudUser/.local/bin/dockerd "
        f"--data-root /home/cloudUser/.local/share/docker "
        f"--pidfile /home/cloudUser/.local/share/docker-rootless/docker.pid "
        f"> /tmp/dockerd.log 2>&1 &",
        timeout=10
    )
    
    time.sleep(10)

    # Check log
    out, err = run("tail -20 /tmp/dockerd.log 2>/dev/null")
    print(out[:600])

    # Test
    print("\n==> Testing Docker...")
    out, err = run(
        f"{PATH} && {XDG} && {DOCKER_HOST} && docker info 2>&1 | head -8",
        timeout=15
    )
    print(out[:500])

    if "Server Version" in out:
        print("\n✅ Docker is running! Ready for deployment.")
    else:
        print("\n❌ Docker still not ready. This CentOS 7 needs kernel config changes.")
        print("   Try: sudo sh -c 'echo 28633 > /proc/sys/kernel/user/max_user_namespaces'")
        print("   Then: sudo sh -c 'echo 1 > /proc/sys/kernel/unprivileged_userns_clone'")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
