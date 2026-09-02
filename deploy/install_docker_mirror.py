import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=180):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Try Aliyun mirror for Docker
    print("\n==> 1. Installing Docker via Aliyun mirror...")
    
    # Remove existing docker repo if any
    run("sudo rm -f /etc/yum.repos.d/docker-ce.repo")
    
    # Add Aliyun Docker CE repo
    cmds = [
        "sudo yum install -y yum-utils device-mapper-persistent-data lvm2",
        "sudo yum-config-manager --add-repo http://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo",
        "sudo yum makecache fast",
        "sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin",
    ]
    
    for cmd in cmds:
        print(f"   Running: {cmd[:50]}...")
        ec, out, err = run(cmd)
        if ec != 0:
            print(f"   ⚠️  Exit: {ec}, err: {err[:200]}")
        else:
            print(f"   ✅ OK")
        time.sleep(1)

    # 2. Check docker binary
    ec, out, err = run("which docker")
    if ec == 0:
        print(f"✅ Docker installed at: {out}")
    else:
        print("❌ Docker not found, trying direct binary install...")
        # Fallback: copy the statically linked docker we already downloaded
        run("sudo cp /home/cloudUser/.local/bin/docker /usr/bin/docker")
        run("sudo cp /home/cloudUser/.local/bin/containerd /usr/bin/containerd")
        run("sudo cp /home/cloudUser/.local/bin/containerd-shim-runc-v2 /usr/bin/containerd-shim-runc-v2")
        run("sudo cp /home/cloudUser/.local/bin/runc /usr/bin/runc")
        run("sudo cp /home/cloudUser/.docker/cli-plugins/docker-compose /usr/libexec/docker/cli-plugins/docker-compose 2>/dev/null || true")
        
        # Install docker-compose plugin
        run("sudo mkdir -p /usr/libexec/docker/cli-plugins")
        run("sudo curl -fsSL https://github.com/docker/compose/releases/download/v2.32.4/docker-compose-linux-x86_64 -o /usr/libexec/docker/cli-plugins/docker-compose", timeout=120)
        run("sudo chmod +x /usr/libexec/docker/cli-plugins/docker-compose")

    # 3. Start Docker
    print("\n==> 2. Starting Docker daemon...")
    ec, out, err = run("sudo dockerd > /tmp/dockerd.log 2>&1 &", timeout=10)
    time.sleep(5)
    
    # Try systemd first
    ec, out, err = run("sudo systemctl start docker 2>&1", timeout=15)
    if ec != 0:
        # Try direct dockerd
        run("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &")
        time.sleep(5)
    
    time.sleep(3)

    # 4. Test
    print("\n==> 3. Testing Docker...")
    ec, out, err = run("sudo docker ps", timeout=15)
    if ec == 0:
        print("✅ Docker is running!")
        print(out[:200])
    else:
        # Check log
        ec, out, err = run("tail -10 /tmp/dockerd.log 2>/dev/null")
        print(f"❌ Docker not running. Log:")
        print(out[:500])

    # Add user
    print("\n==> 4. Adding user to docker group...")
    run("sudo usermod -aG docker cloudUser")
    print("✅ Done")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
