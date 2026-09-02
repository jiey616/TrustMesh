import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=15):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    cmds = [
        "sudo ls -la /var/run/docker.sock",
        "sudo docker ps 2>&1; echo 'EXIT:'$?",
        "groups",
        "sudo systemctl is-active docker",
    ]
    for cmd in cmds:
        ec, out, err = run(cmd)
        print(f"\n$ {cmd}")
        print(out[:200])
        if err: print(f"ERR: {err[:100]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
