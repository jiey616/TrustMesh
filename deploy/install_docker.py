import paramiko, sys

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

    print("\n==> Installing rootless Docker (static binary)...")
    
    # Download Docker static binary
    run("curl -fsSL https://download.docker.com/linux/static/stable/x86_64/docker-27.5.1.tgz -o /tmp/docker.tgz", timeout=120)
    run("tar xzf /tmp/docker.tgz -C /tmp/")
    
    # Install to user local bin
    run("mkdir -p ~/.local/bin")
    run("cp /tmp/docker/docker ~/.local/bin/")
    run("chmod +x ~/.local/bin/docker")
    
    # Add to PATH
    run('echo \'export PATH="$HOME/.local/bin:$PATH"\' >> ~/.bashrc')
    run('export PATH="$HOME/.local/bin:$PATH"')
    
    # Verify
    ec, out, err = run("~/.local/bin/docker --version", timeout=10)
    if ec == 0:
        print(f"✅ Docker binary installed: {out}")
    else:
        print(f"❌ Docker binary failed: {err}")
    
    # Download docker-compose binary
    print("\n==> Installing docker-compose plugin...")
    run("mkdir -p ~/.docker/cli-plugins")
    run("curl -fsSL https://github.com/docker/compose/releases/download/v2.32.4/docker-compose-linux-x86_64 -o ~/.docker/cli-plugins/docker-compose", timeout=120)
    run("chmod +x ~/.docker/cli-plugins/docker-compose")
    
    ec, out, err = run("~/.local/bin/docker compose version", timeout=10)
    if ec == 0:
        print(f"✅ Docker Compose installed: {out}")
    else:
        print(f"❌ Docker Compose failed: {err}")

    # Try starting dockerd in rootless mode
    print("\n==> Setting up rootless Docker...")
    run("~/.local/bin/dockerd-rootless-setuptool.sh install 2>/dev/null || true")
    
    print("\n✅ Setup complete! Testing Docker...")
    ec, out, err = run("~/.local/bin/docker info 2>&1 | head -5", timeout=10)
    print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
