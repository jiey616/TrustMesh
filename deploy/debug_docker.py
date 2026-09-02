import paramiko, sys, time

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

    PATH = "export PATH=/home/cloudUser/.local/bin:$PATH"

    # 1. Check prerequisites for rootless Docker
    print("\n==> Checking prerequisites...")
    cmds = [
        "which fuse2fs fuse3 2>/dev/null; ls /dev/fuse 2>/dev/null; echo '---'",
        "cat /proc/sys/kernel/unprivileged_userns_clone 2>/dev/null || echo 'not set'; cat /sys/kernel/security/apparmor/profiles 2>/dev/null | head -1 || echo 'no apparmor'",
        "id -u",
    ]
    for cmd in cmds:
        ec, out, err = run(cmd)
        print(f"$ {cmd}")
        if out: print(out[:200])
        if err: print(f"ERR: {err[:200]}")

    # 2. Check dockerd log
    ec, out, err = run("tail -50 /tmp/dockerd.log 2>/dev/null || echo 'NO_LOG'")
    print(f"\n==> dockerd.log:")
    print(out[:500])

    # 3. Try starting with rootlessport disabled
    print("\n==> Starting dockerd rootless (with rootlesskit)...")
    run("pkill -f dockerd 2>/dev/null || true")
    time.sleep(2)
    
    # Start dockerd using rootlesskit
    stdin, stdout, stderr = ssh.exec_command(
        f"cd && {PATH} && dockerd-rootless-setuptool.sh install 2>&1",
        timeout=60
    )
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    print(out[:800])
    if err: print(f"ERR: {err[:300]}")
    
    time.sleep(3)

    # 4. Check status
    ec, out, err = run(f"{PATH} && docker info 2>&1 | head -10", timeout=15)
    print(f"\n==> Docker info:")
    print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
