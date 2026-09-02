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

    ec, out, err = run("sudo systemctl is-active docker")
    print(f"systemctl: {out}")

    ec, out, err = run("sudo ls -la /var/run/docker.sock 2>&1")
    print(f"Socket: {out[:100]}")

    ec, out, err = run("sudo docker ps -a 2>&1")
    print(f"\nContainers:\n{out[:300]}")

    ec, out, err = run("sudo docker images 2>&1 | head -10")
    print(f"\nImages:\n{out[:400]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
