import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=120):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # 1. Copy all docker binaries
    print("\n==> Copying docker binaries...")
    run("cp /tmp/docker/* /home/cloudUser/.local/bin/ 2>/dev/null")
    print("✅ Done")

    # 2. Download rootless extras
    print("\n==> Downloading rootless extras...")
    ec, out, err = run(
        "curl -fsSL https://download.docker.com/linux/static/stable/x86_64/docker-rootless-extras-27.5.1.tgz -o /tmp/rootless.tgz "
        "&& tar xzf /tmp/rootless.tgz -C /tmp/ "
        "&& cp /tmp/docker-rootless-extras/* /home/cloudUser/.local/bin/ "
        "&& chmod +x /home/cloudUser/.local/bin/*",
        timeout=120
    )
    if ec != 0:
        print(f"❌ Download failed: {err[:200]}")
        sys.exit(1)
    print("✅ Rootless extras installed")

    # 3. Set up kernel parameters (needed for rootless)
    print("\n==> Setting kernel parameters...")
    run("sudo sh -c 'echo 1 > /proc/sys/kernel/unprivileged_userns_clone' 2>/dev/null || echo 'Need root for this step'")
    
    # Try with docker context instead of rootless setup
    # Actually, let's try a simpler approach: run dockerd manually
    print("\n==> Starting dockerd manually as rootless...")
    run("kill -9 $(cat /home/cloudUser/.local/run/docker.pid 2>/dev/null) 2>/dev/null || true")
    time.sleep(2)

    # Start dockerd with rootless mode
    PATH_EXPORT = "export PATH=/home/cloudUser/.local/bin:$PATH"
    stdin, stdout, stderr = ssh.exec_command(
        f"cd && {PATH_EXPORT} && "
        f"dockerd-rootless-setuptool.sh install 2>&1 || "
        f"echo 'SETUP_FAILED'",
        timeout=60
    )
    out = stdout.read().decode().strip()
    print(out[:500])
    
    # If setup script fails, try manual dockerd
    if "SETUP_FAILED" in out or not out:
        print("\n   Trying manual dockerd start...")
        stdin, stdout, stderr = ssh.exec_command(
            f"cd && {PATH_EXPORT} && "
            f"dockerd --rootless --data-root /home/cloudUser/.local/share/docker "
            f"--pidfile /home/cloudUser/.local/run/docker.pid > /tmp/dockerd.log 2>&1 &",
            timeout=10
        )

    time.sleep(8)

    # 5. Check if Dockerd is running
    print("\n==> Checking Docker...")
    ec, out, err = run(
        f"{PATH_EXPORT} && "
        f"export DOCKER_HOST=unix:///run/user/1000/docker.sock && "
        f"docker info 2>&1 | head -8",
        timeout=15
    )
    print(out[:500])

    # Check log if failed
    if "Cannot connect" in out:
        ec, out, err = run("tail -30 /tmp/dockerd.log 2>/dev/null")
        print(f"\n==> dockerd log:")
        print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
