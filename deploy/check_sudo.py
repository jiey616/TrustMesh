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

    # Check dockerd log
    ec, out, err = run("tail -40 /tmp/dockerd.log 2>/dev/null || echo 'NO_LOG'")
    print("==> dockerd.log:")
    print(out[:600] if out else "NO_LOG")

    # Check if we can sudo
    print("\n==> Checking sudo access...")
    cmds = [
        'echo "test" | sudo -S id 2>&1 || echo "NO_SUDO"',
    ]
    for cmd in cmds:
        ec, out, err = run(cmd)
        print(f"$ {cmd}")
        print(out[:200])

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
