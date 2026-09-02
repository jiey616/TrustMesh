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
    transport.set_keepalive(30)

    def run(cmd, timeout=15):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        err = ch.makefile_stderr('r', 4096).read().decode().strip()
        ch.close()
        return out, err

    # Remove problematic daemon.json
    run("sudo rm -f /etc/docker/daemon.json", 5)
    print("✅ daemon.json removed")

    # Kill existing containers
    run("sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null", 10)
    print("✅ Old containers removed")

    # Clean mongo lock
    run("sudo find /var/lib/docker/volumes -name mongod.lock -delete 2>/dev/null", 5)

    # Disable docker pull check by using COMPOSE_DISABLE_PULL
    out, err = run("cd /opt/trustmesh-test && sudo COMPOSE_DISABLE_PULL=true TAG=test docker compose up -d 2>&1", 60)
    print(out[:400])
    if err: print(f"ERR: {err[:200]}")

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
