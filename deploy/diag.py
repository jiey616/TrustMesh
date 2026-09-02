import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    transport = ssh.get_transport()

    def run(cmd, timeout=10):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    out = run("sudo docker info 2>&1 | head -20", 10)
    print(f"Docker info:\n{out}")

    out = run("sudo docker images -q 2>&1 | wc -l", 10)
    print(f"\nImage count: {out}")

    out = run("sudo docker ps -a --format '{{.Names}}' 2>&1 || echo 'empty'", 10)
    print(f"Containers: {out}")

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
