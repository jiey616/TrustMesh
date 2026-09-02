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

    # Check what's blocking
    ec, out, err = run("ls -la /var/run/docker.sock /var/run/docker.pid 2>&1")
    print(out[:200])
    
    ec, out, err = run("ps aux | grep docker | grep -v grep")
    print(f"Processes:\n{out[:300]}")

    # Kill everything
    print("\n==> Force cleanup...")
    run("sudo pkill -9 dockerd 2>/dev/null || true")
    run("sudo pkill -9 containerd 2>/dev/null || true")
    time.sleep(2)
    run("sudo rm -f /var/run/docker.sock /var/run/docker.pid 2>/dev/null || true")
    run("sudo rm -rf /var/run/docker 2>/dev/null || true")
    
    # Restore daemon.json
    daemon = """{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.xuanyuan.me",
    "https://docker.1ms.run"
  ],
  "exec-opts": ["native.cgroupdriver=systemd"],
  "log-driver": "json-file",
  "log-opts": {"max-size": "10m", "max-file": "3"}
}
"""
    run(f"sudo bash -c 'cat > /etc/docker/daemon.json << \"JEOF\"\n{daemon}\nJEOF'")
    print("✅ daemon.json restored")

    # Start Docker properly
    print("\n==> Starting Docker daemon...")
    run("sudo systemctl daemon-reload")
    run("sudo systemctl enable docker")
    ec, out, err = run("sudo systemctl start docker", timeout=30)
    if ec != 0:
        print(f"systemctl start failed: {err[:200]}")
        # Try direct
        run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &")
        time.sleep(8)
    
    time.sleep(5)

    # Test
    ec, out, err = run("sudo docker ps", timeout=15)
    print(f"\nDocker ps: {out[:200]}")
    
    if "Cannot connect" in out:
        ec, out, err = run("sudo journalctl -u docker --no-pager -n 15 2>&1")
        print(f"Logs:\n{out[:500]}")
    else:
        print("✅ Docker is running!")
        
        # Check mirrors
        ec, out, err = run("sudo docker info 2>&1 | grep -i -A5 mirror")
        print(f"Mirrors:\n{out[:500]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
