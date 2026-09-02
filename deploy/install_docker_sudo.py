import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=120):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Verify sudo
    ec, out, err = run("echo 'test' | sudo -S id 2>&1")
    print(f"sudo test: {out[:100]}")
    if ec != 0:
        print("❌ Still no sudo. Please verify cloudUser has sudo access.")
        sys.exit(1)
    print("✅ sudo confirmed!")

    # Kill existing dockerd processes first
    run("sudo pkill -f dockerd 2>/dev/null || true")
    run("sudo pkill -f rootlesskit 2>/dev/null || true")
    time.sleep(2)

    # 1. Install Docker
    print("\n==> 1. Installing Docker...")
    run("sudo yum install -y yum-utils device-mapper-persistent-data lvm2", timeout=60)
    print("   yum-utils installed")
    
    ec, out, err = run("sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo", timeout=30)
    print("   repo added")
    
    ec, out, err = run("sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin", timeout=180)
    if ec != 0:
        print(f"❌ Docker install failed: {err[:300]}")
        sys.exit(1)
    print("✅ Docker CE installed")

    # 2. Start Docker
    print("\n==> 2. Starting Docker...")
    run("sudo systemctl start docker")
    run("sudo systemctl enable docker")
    time.sleep(3)
    
    ec, out, err = run("docker --version")
    print(f"   {out}")

    # 3. Add user to docker group
    print("\n==> 3. Adding cloudUser to docker group...")
    run("sudo usermod -aG docker cloudUser")
    print("✅ Done")

    # 4. Test with newgrp
    print("\n==> 4. Testing Docker...")
    ec, out, err = run("sudo -u cloudUser docker ps 2>&1", timeout=15)
    print(out[:200])

    ec, out, err = run("sudo -u cloudUser docker compose version 2>&1", timeout=15)
    print(out[:200])

    print("\n✅ Docker installation complete!")
    print("Run 'newgrp docker' or re-login for group change to take effect.")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
