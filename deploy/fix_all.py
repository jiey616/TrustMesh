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

    # Check what PIDs exist
    ec, out, err = run("sudo ls -la /var/run/docker.pid 2>&1; sudo cat /var/run/docker.pid 2>&1")
    print(f"PID file: {out[:100]}")

    # Check if that PID exists
    ec, out, err = run("ps aux | grep 6647 | grep -v grep")
    print(f"Process 6647: {out[:100]}")
    
    # Check containerd
    ec, out, err = run("sudo ls -la /var/run/docker/containerd/containerd.pid 2>&1")
    print(f"Containerd pid: {out[:100]}")

    # Remove ALL stale files
    print("\n==> Cleaning up all stale files...")
    run("sudo rm -rf /var/run/docker* /run/docker* /var/run/docker/ 2>/dev/null || true")
    run("sudo rm -f /var/run/docker.pid /var/run/docker.sock 2>/dev/null || true")
    run("sudo mkdir -p /var/run/docker 2>/dev/null || true")
    
    # Verify clean
    ec, out, err = run("sudo find /var/run -name 'docker*' -type s 2>/dev/null; sudo ls -la /var/run/docker.pid 2>&1")
    print(f"After cleanup: {out[:200]}")

    # Start Docker
    print("\n==> Starting Docker...")
    run("sudo systemctl stop docker 2>/dev/null || true")
    time.sleep(2)
    ec, out, err = run("sudo systemctl start docker", timeout=30)
    time.sleep(8)

    # Test
    ec, out, err = run("sudo docker ps", timeout=15)
    print(f"\nDocker ps: '{out[:200]}'")
    
    if "CONTAINER" in out:
        print("✅ Docker is running!")
        
        # Now configure mirrors
        print("\n==> Configuring registry mirrors...")
        daemon = '''{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.xuanyuan.me",
    "https://docker.1ms.run"
  ]
}
'''
        run(f"sudo bash -c 'cat > /etc/docker/daemon.json << \"JEOF\"\n{daemon}\nJEOF'")
        run("sudo systemctl restart docker")
        time.sleep(8)
        
        ec, out, err = run("sudo docker info 2>&1 | grep -A5 'Registry Mirrors'", timeout=15)
        print(f"Mirrors:\n{out[:500]}")
        
        # Test pull
        print("\n==> Testing pull with mirrors...")
        stdin, stdout, stderr = ssh.exec_command(
            "sudo docker pull alpine:3.18 2>&1",
            timeout=180
        )
        pull_out = stdout.read().decode().strip()
        print(f"Pull result:\n{pull_out[:300]}")

    elif "Cannot connect" in out:
        ec, out, err = run("sudo journalctl -u docker --no-pager -n 15 2>&1")
        print(f"Logs:\n{out[:500]}")
    else:
        print(f"Empty response (Docker might be working)")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
