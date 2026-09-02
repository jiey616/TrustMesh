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

    # Check process
    ec, out, err = run("ps aux | grep dockerd | grep -v grep")
    print(f"Dockerd process:\n{out[:300]}")

    # Check journalctl for latest failures
    ec, out, err = run("sudo journalctl -u docker --no-pager -n 30 2>&1")
    print(f"\nJournalctl (last 30):\n{out[:1000]}")

    # Full restart
    print("\n==> Full Docker restart...")
    run("sudo systemctl stop docker")
    time.sleep(3)
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    time.sleep(2)
    run("sudo systemctl start docker")
    time.sleep(8)
    
    ec, out, err = run("sudo docker ps", timeout=15)
    print(f"\nAfter restart - docker ps: '{out[:100]}' (exit={ec})")
    
    if ec != 0:
        ec, out, err = run("sudo journalctl -u docker --no-pager -n 15 2>&1")
        print(f"Logs:\n{out[:500]}")
    else:
        print("✅ Docker running!")
        # Try a pull test
        print("\n==> Testing pull (with long timeout)...")
        stdin, stdout, stderr = ssh.exec_command(
            "sudo docker pull alpine:3.18 2>&1 | tail -5",
            timeout=180
        )
        out = stdout.read().decode().strip()
        print(out[:200])

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
