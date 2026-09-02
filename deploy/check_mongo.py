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

    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("sudo docker compose -f /opt/trustmesh-test/docker-compose.yml logs --tail=20 mongo 2>&1")
    print(ch.makefile('r', 4096).read().decode().strip()[:1000])
    ch.close()

    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("sudo docker compose -f /opt/trustmesh-test/docker-compose.yml ps -a 2>&1")
    print(f"\n--- All containers ---\n{ch.makefile('r', 4096).read().decode().strip()}")
    ch.close()

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
