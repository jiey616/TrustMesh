import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=30):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check Docker service status
    ec, out, err = run("sudo systemctl status docker 2>&1 | head -20")
    print(out[:500])

    # Check log
    ec, out, err = run("sudo journalctl -u docker --no-pager -n 20 2>&1")
    print(f"\nJournalctl:")
    print(out[:500])

    # Remove the bad override
    print("\n==> Removing bad override config...")
    run("sudo rm -f /etc/systemd/system/docker.service.d/override.conf")
    run("sudo systemctl daemon-reload")

    # Start Docker again
    print("\n==> Starting Docker...")
    run("sudo systemctl start docker")
    time.sleep(5)

    ec, out, err = run("sudo docker ps 2>&1")
    print(f"\nDocker status: {out[:200]}")

    if "Cannot connect" in out:
        print("❌ Docker still broken, trying dockerd directly...")
        run("sudo nohup dockerd --registry-mirror=https://docker.m.daocloud.io --registry-mirror=https://docker.xuanyuan.me --registry-mirror=https://docker.1ms.run > /tmp/dockerd.log 2>&1 &")
        time.sleep(10)
        ec, out, err = run("sudo docker ps 2>&1")
        print(f"After manual start: {out[:200]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
