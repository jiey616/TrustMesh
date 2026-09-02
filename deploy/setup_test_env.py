import paramiko
import sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=60):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    print("\n==> 1. Installing Docker...")
    run("sudo yum install -y yum-utils device-mapper-persistent-data lvm2")
    run("sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo")
    ec, out, err = run("sudo yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin", timeout=120)
    if ec != 0:
        print(f"❌ Docker install failed: {err[:300]}")
        sys.exit(1)
    print("✅ Docker installed")

    print("\n==> 2. Starting Docker...")
    run("sudo systemctl start docker")
    run("sudo systemctl enable docker")
    ec, out, _ = run("docker --version")
    print(f"   {out}")

    print("\n==> 3. Adding cloudUser to docker group...")
    run("sudo usermod -aG docker cloudUser")
    print("✅ User added to docker group")

    print("\n==> 4. Verifying docker compose...")
    ec, out, _ = run("docker compose version")
    print(f"   {out}")

    print("\n✅ Docker setup complete! Now deploy the application.")
    print("⚠️  Need to log out and back in for docker group to take effect.")
    print("   Run: newgrp docker")
    print("   Or just continue with sudo where needed.")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
