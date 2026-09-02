import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=120):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Configure Docker mirror registry
    print("\n==> Configuring Docker mirror (registry mirror)...")
    docker_config = """{
  "registry-mirrors": [
    "https://docker.1ms.run",
    "https://docker.xuanyuan.me",
    "https://hub.uuuadc.top",
    "https://docker.1panel.live"
  ],
  "exec-opts": ["native.cgroupdriver=systemd"],
  "log-driver": "json-file",
  "log-opts": {"max-size": "10m", "max-file": "3"}
}
"""
    run("sudo mkdir -p /etc/docker")
    run(f"cat > /tmp/daemon.json << 'JSONEOF'\n{docker_config}JSONEOF")
    run("sudo cp /tmp/daemon.json /etc/docker/daemon.json")
    print("✅ Mirror configured")

    # Restart Docker
    print("\n==> Restarting Docker...")
    run("sudo systemctl restart docker", timeout=30)
    time.sleep(5)
    
    ec, out, err = run("sudo docker ps", timeout=15)
    if ec == 0:
        print("✅ Docker restarted!")
    else:
        print(f"❌ Restart failed: {err[:200]}")

    # Test pull with mirror
    print("\n==> Testing pull with mirror...")
    ec, out, err = run("sudo docker pull alpine:3.18 2>&1", timeout=120)
    if ec == 0:
        print("✅ Mirror working! Pulled alpine")
    else:
        print(f"❌ Pull failed: {err[:200]}")

    # Try starting services again
    print("\n==> Starting services...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=300
    )
    print(out[:800])
    
    time.sleep(10)
    
    # Check status
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1",
        timeout=15
    )
    print(f"\nServices:")
    print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
