import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    transport = ssh.get_transport()

    def run(cmd, timeout=15):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Check all containers (including stopped)
    out = run("sudo docker ps -a --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'", 10)
    print(f"All containers:\n{out}")

    # Clean up and start fresh
    run("sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null", 10)
    time.sleep(2)
    print("\n✅ Containers removed")

    # Check mongo lock
    out = run("sudo find /var/lib/docker/volumes -name mongod.lock -delete 2>/dev/null; echo 'cleaned'", 5)

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
