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

    ch = ssh.get_transport().open_session()
    ch.settimeout(180)
    ch.exec_command(
        "cd /opt/trustmesh-test && "
        "sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null; "
        "sudo TAG=test docker compose up -d 2>&1"
    )
    out = ch.makefile('r', 4096).read().decode().strip()
    print(out[:500])
    ch.close()

    time.sleep(15)

    ch = ssh.get_transport().open_session()
    ch.settimeout(15)
    ch.exec_command("cd /opt/trustmesh-test && sudo docker compose ps")
    out = ch.makefile('r', 4096).read().decode().strip()
    print(f"\nServices:\n{out}")
    ch.close()

    ch = ssh.get_transport().open_session()
    ch.settimeout(10)
    ch.exec_command("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz && echo -n ' | ' && curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/")
    out = ch.makefile('r', 4096).read().decode().strip()
    print(f"\nHealth: Backend HTTP {out}")
    ch.close()

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
