import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=30):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check user permissions
    cmds = [
        "whoami",
        "id",
        "ls -la /opt/ 2>/dev/null",
        "which curl wget yum apt-get 2>/dev/null",
        "ls -la $HOME",
        "cat /etc/sudoers 2>/dev/null | head -5 || echo 'NO_SUDO_ACCESS'",
    ]
    for cmd in cmds:
        ec, out, err = run(cmd)
        print(f"\n$ {cmd}")
        print(out[:500] if out else "(empty)")
        if err: print(f"ERR: {err[:200]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
