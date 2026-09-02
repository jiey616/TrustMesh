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

    # 1. Kill containers only (keep volumes)
    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("cd /opt/trustmesh-test && sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null; echo 'containers removed'")
    print(ch.makefile('r', 4096).read().decode().strip())
    ch.close()
    time.sleep(2)

    # 2. Find and remove mongo lock file from volume
    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("sudo find /var/lib/docker/volumes -name mongod.lock 2>/dev/null | head -5")
    lock_files = ch.makefile('r', 4096).read().decode().strip()
    print(f"Lock files: {lock_files}")
    ch.close()

    if lock_files:
        for f in lock_files.split('\n'):
            ch = transport.open_session()
            ch.settimeout(10)
            ch.exec_command(f"sudo rm -f {f}; echo 'removed {f}'")
            print(ch.makefile('r', 4096).read().decode().strip())
            ch.close()

    # 3. Also remove the stale PID file
    ch = transport.open_session()
    ch.settimeout(10)
    ch.exec_command("sudo find /var/lib/docker/volumes -name 'mongod.lock' -delete 2>/dev/null; echo 'done'")
    print(ch.makefile('r', 4096).read().decode().strip())
    ch.close()

    # 4. Start services (images already exist, no pull needed)
    ch = transport.open_session()
    ch.settimeout(60)
    ch.exec_command("cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1")
    print(ch.makefile('r', 4096).read().decode().strip()[:500])
    ch.close()

    time.sleep(15)

    ch = transport.open_session()
    ch.settimeout(15)
    ch.exec_command("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"\n{ch.makefile('r', 4096).read().decode().strip()}")
    ch.close()

    ch = transport.open_session()
    ch.settimeout(10)
    ch.exec_command("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz && echo -n ' | ' && curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/")
    print(f"\nHealth: {ch.makefile('r', 4096).read().decode().strip()}")
    ch.close()

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
