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

    # Remove daemon.json completely (it's causing issues)
    print("==> Removing problematic daemon.json...")
    run("sudo rm -f /etc/docker/daemon.json")
    run("sudo systemctl daemon-reload")
    
    # Kill all docker processes  
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    time.sleep(3)
    
    # Start docker cleanly
    print("==> Starting Docker cleanly...")
    run("sudo systemctl start docker", timeout=30)
    time.sleep(5)
    
    ec, out, err = run("sudo docker ps", timeout=15)
    if ec == 0:
        print("✅ Docker running!")
        print(out[:200])
    else:
        print("❌ Docker failed to start")
        ec, out, err = run("sudo journalctl -u docker --no-pager -n 20 2>&1")
        print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
