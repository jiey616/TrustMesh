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

    # Verify Docker works
    ec, out, err = run("sudo docker version 2>&1")
    print(f"\nDocker version: {out[:200]}")

    # Configure mirrors
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
    run("sudo systemctl restart docker", timeout=30)
    time.sleep(8)
    
    # Check mirrors
    ec, out, err = run("sudo docker info 2>&1 | grep -A5 'Registry Mirrors'", timeout=15)
    print(f"Mirrors: {out[:300]}")
    
    # Test pull
    print("\n==> Testing pull with mirrors...")
    stdin, stdout, stderr = ssh.exec_command(
        "sudo docker pull alpine:3.18 2>&1",
        timeout=180
    )
    pull_out = stdout.read().decode().strip()
    print(f"Pull: {pull_out[:300]}")
    
    if "Pulled" in pull_out or "Image is up to date" in pull_out or "Status" in pull_out:
        print("✅ Mirror working!")
    else:
        # Check if Docker is alive
        ec, out, err = run("sudo docker ps", timeout=15)
        print(f"Docker alive: {out[:100]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
