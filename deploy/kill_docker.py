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

    def run(cmd, timeout=8):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Force kill dockerd
    run("sudo pkill -9 dockerd 2>/dev/null || true", 5)
    run("sudo pkill -9 containerd 2>/dev/null || true", 5)
    time.sleep(3)
    run("sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null || true", 5)
    print("✅ docker killed")

    # Start fresh
    time.sleep(2)
    run("sudo rm -f /etc/docker/daemon.json", 5)
    run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &", 5)
    time.sleep(10)

    out = run("sudo docker ps", 8)
    print(f"docker ps: {out[:80]}")

    # If that doesn't work, try systemctl one more time
    if "Cannot connect" in out:
        print("Docker still not ready, checking log...")
        out = run("tail -10 /tmp/dockerd.log", 8)
        print(out[:300])
    else:
        print("✅ Docker running!")

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
