import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=15):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Kill ALL docker processes - EVERYTHING
    cmds = [
        "sudo pkill -9 dockerd 2>/dev/null || true",
        "sudo pkill -9 containerd 2>/dev/null || true",
        "sudo pkill -9 containerd-shim 2>/dev/null || true",
    ]
    for cmd in cmds:
        run(cmd)
    time.sleep(3)
    
    # Clean up socket and pid files
    run("sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null || true")
    
    # Make sure no leftover process
    ec, out, err = run("ps aux | grep docker | grep -v grep | wc -l")
    print(f"Remaining docker processes: {out}")

    # Start docker fresh
    print("\n==> Starting Docker fresh...")
    run("sudo systemctl stop docker 2>/dev/null || true")
    time.sleep(2)
    
    # Use simplest config - no daemon.json
    run("sudo rm -f /etc/docker/daemon.json")
    
    ec, out, err = run("sudo systemctl start docker", timeout=30)
    time.sleep(5)
    
    # Check socket
    ec, out, err = run("ls -la /var/run/docker.sock 2>&1")
    print(f"Socket: {out[:100]}")

    if "No such file" in out or "cannot access" in out:
        # Wait a bit more
        time.sleep(5)
        ec, out, err = run("ls -la /var/run/docker.sock 2>&1")
        print(f"Socket (retry): {out[:100]}")
        
        # If still no socket, try direct dockerd
        if "No such file" in out:
            print("systemd not working, trying direct dockerd...")
            run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &")
            time.sleep(8)
    
    # Final test
    ec, out, err = run("sudo docker ps", timeout=15)
    print(f"\nDocker ps: '{out[:200]}'")
    
    if ec == 0:
        print("✅ Docker is running!")
    else:
        ec, out, err = run("tail -20 /tmp/dockerd.log 2>/dev/null || sudo journalctl -u docker --no-pager -n 20")
        print(f"Log:\n{out[:500]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
