import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    print("✅ SSH connected")

    transport = ssh.get_transport()
    transport.set_keepalive(30)

    # Start services in background
    ch = transport.open_session()
    ch.exec_command("cd /opt/trustmesh-test && nohup sudo TAG=test docker compose up -d > /tmp/compose.log 2>&1 &")
    ch.close()

    time.sleep(20)

    # Check result
    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("cat /tmp/compose.log 2>/dev/null; echo '---'; sudo docker compose -f /opt/trustmesh-test/docker-compose.yml ps 2>&1 | head -10")
    out = ch.makefile('r', 4096).read().decode().strip()
    print(out[:500])
    ch.close()

    ch = transport.open_session()
    ch.settimeout(10)
    ch.exec_command("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz 2>&1")
    out = ch.makefile('r', 4096).read().decode().strip()
    print(f"\nBackend health: HTTP {out}")
    ch.close()

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
