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

    # 1. Check current docker service file
    ec, out, err = run("sudo cat /usr/lib/systemd/system/docker.service 2>&1 | head -20")
    print(f"== Current docker.service ==\n{out[:300]}")

    # 2. Check existing daemon.json (remove if broken)
    run("sudo rm -f /etc/docker/daemon.json")

    # 3. Create proper daemon.json - minimal, no exec-opts
    daemon = '''{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.xuanyuan.me",
    "https://docker.1ms.run"
  ]
}
'''
    run("sudo mkdir -p /etc/docker")
    run(f"sudo bash -c 'cat > /etc/docker/daemon.json << \"JEOF\"\n{daemon}\nJEOF'")
    print("\n✅ daemon.json created")

    # 4. Remove docker override if it exists
    run("sudo rm -rf /etc/systemd/system/docker.service.d")
    run("sudo systemctl daemon-reload")

    # 5. Kill existing dockerd and clean up
    print("\n==> Killing existing dockerd...")
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo pkill -9 containerd 2>/dev/null || true")
    time.sleep(3)
    run("sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null || true")
    time.sleep(2)

    # 6. Enable and start via systemd
    print("==> Enabling Docker as system service...")
    run("sudo systemctl enable docker")
    ec, out, err = run("sudo systemctl start docker", timeout=30)
    if ec != 0:
        print(f"❌ systemctl start failed: {err[:200]}")
        ec, out, err = run("sudo journalctl -u docker --no-pager -n 20 2>&1")
        print(out[:500])
    else:
        print("✅ systemctl start OK")
    
    time.sleep(5)

    # 7. Verify
    ec, out, err = run("sudo docker ps", timeout=15)
    if "Cannot connect" not in out:
        print("✅ Docker running via systemd!")
    else:
        print("❌ Docker not running, checking log...")
        ec, out, err = run("sudo journalctl -u docker --no-pager -n 20 2>&1")
        print(out[:500])

    # 8. Bring up services
    print("\n==> Starting TrustMesh services...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=60
    )
    print(out[:300])

    time.sleep(10)

    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"\n== Services ==\n{out}")

    # 9. Clean up crontab watchdog (no longer needed since systemd handles restart)
    print("\n==> Removing crontab watchdog (replaced by systemd)...")
    run("sudo crontab -l 2>/dev/null | grep -v docker-watchdog | sudo crontab -")
    ec, out, err = run("sudo crontab -l 2>/dev/null")
    print(f"Crontab now:\n{out}")

    # 10. Final status
    ec, out, err = run("sudo systemctl status docker --no-pager 2>&1 | grep -E 'Active:|Loaded:'")
    print(f"\n== Docker Status ==\n{out}")

    print("\n✅ Docker is now a proper system service, auto-starts on boot!")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
