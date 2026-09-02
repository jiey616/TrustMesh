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
    transport.set_keepalive(30)

    ch = transport.open_session()
    ch.settimeout(120)
    ch.exec_command(
        "cd /opt/trustmesh-test && "
        "sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null; "
        "sudo docker volume rm trustmesh_trustmesh-mongo-data 2>/dev/null; "
        "sudo TAG=test docker compose up -d 2>&1"
    )
    print(ch.makefile('r', 4096).read().decode().strip()[:500])
    ch.close()

    time.sleep(20)

    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"\n{ch.makefile('r', 4096).read().decode().strip()}")
    ch.close()

    ch = transport.open_session()
    ch.settimeout(10)
    ch.exec_command("curl -s -o /dev/null -w 'Backend %{http_code}' http://127.0.0.1:8080/healthz && echo -n ' | Frontend ' && curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/")
    print(ch.makefile('r', 4096).read().decode().strip())
    ch.close()

    print("\n✅ All services restored!")

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
