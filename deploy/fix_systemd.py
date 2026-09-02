import paramiko, sys

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

    cmds = [
        "sudo systemctl status docker --no-pager 2>&1 | head -15",
        "echo '---'",
        "sudo systemctl status docker.socket --no-pager 2>&1 | head -10",
        "echo '---'",
        "ls -la /var/run/docker.sock 2>&1",
        "echo '---'",
        "ps aux | grep dockerd | grep -v grep",
        "echo '---'",
        "sudo journalctl -u docker --no-pager -n 10 2>&1",
    ]
    for cmd in cmds:
        ec, out, err = run(cmd)
        print(f"\n$ {cmd[:60]}")
        print(out[:300])
        if err and '---' not in cmd: print(err[:100])

    # Try direct dockerd instead
    print("\n\n==> Killing systemd dockerd and starting directly...")
    run("sudo systemctl stop docker 2>/dev/null || true")
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null || true")
    import time
    time.sleep(3)
    run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &")
    time.sleep(8)
    
    ec, out, err = run("sudo docker ps", timeout=15)
    print(f"Direct dockerd: {out[:100]}")

    if "Cannot connect" not in out:
        ec, out, err = run(
            "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
            timeout=60
        )
        print(f"Services: {out[:200]}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
