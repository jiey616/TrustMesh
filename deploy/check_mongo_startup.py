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
    ch.exec_command("sudo docker compose -f /opt/trustmesh-test/docker-compose.yml logs mongo 2>&1 | grep -iE 'error|fatal|exception|killed|warn|exit' | tail -10")
    out = ch.makefile('r', 4096).read().decode().strip()
    print(out[:1000] if out else "(no errors found)")
    ch.close()

    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("sudo docker compose -f /opt/trustmesh-test/docker-compose.yml logs mongo 2>&1 | head -30")
    print(f"\n--- Mongo full startup ---\n{ch.makefile('r', 4096).read().decode().strip()[:1000]}")
    ch.close()

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
