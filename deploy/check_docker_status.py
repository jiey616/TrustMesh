import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check docker status
    cmds = [
        "ls -la ~/.local/bin/docker 2>/dev/null && echo 'DOCKER_BIN_EXISTS' || echo 'NO_DOCKER'",
        "ls -la ~/.docker/cli-plugins/docker-compose 2>/dev/null && echo 'COMPOSE_EXISTS' || echo 'NO_COMPOSE'",
        "ps aux | grep docker | grep -v grep | head -3",
    ]
    for cmd in cmds:
        stdin, stdout, stderr = ssh.exec_command(cmd, timeout=10)
        out = stdout.read().decode().strip()
        err = stderr.read().decode().strip()
        print(f"\n$ {cmd}")
        if out: print(out[:300])
        if err: print(f"ERR: {err[:200]}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
