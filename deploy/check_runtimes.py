import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=30):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check for container runtimes
    print("\n==> Checking available runtimes...")
    cmds = [
        "which podman docker nerdctl 2>/dev/null",
        "ls /var/run/docker.sock 2>/dev/null && echo 'DOCKER_SOCK_EXISTS' || echo 'NO_DOCKER_SOCK'",
        "which python3 python 2>/dev/null",
        "free -h | head -2",
        "df -h / | tail -1",
    ]
    for cmd in cmds:
        out, err = run(cmd)
        print(f"$ {cmd}")
        print(out[:300] if out else "(empty)")
        if err: print(f"ERR: {err[:200]}")

    # Check if we can install podman without sudo (unlikely but worth checking)
    print("\n==> Checking disk space, CPU, memory...")
    out, err = run("nproc && free -m | grep Mem && df -h / | tail -1")
    print(out)

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
