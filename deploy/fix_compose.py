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

    # Check compose file
    ec, out, err = run("sudo grep -n 'ports\|8080|3000' /opt/trustmesh-test/docker-compose.yml 2>&1")
    print(f"Compose ports section:\n{out}")

    # Fix: manually add ports to backend service
    print("\n==> Fixing compose file for proper port mapping...")
    
    # Download, fix locally, upload
    local_compose = r"D:\AIWorkspace\TrustMesh\docker-compose.prod.yml"
    with open(local_compose, 'r') as f:
        content = f.read()
    
    # Add ports to backend
    old = "    healthcheck:"
    new = """    ports:
      - "8080:8080"
    healthcheck:"""
    content = content.replace(old, new, 1)
    
    # Also fix MongoDB version
    content = content.replace(
        "mongodb/mongodb-community-server:8.2-ubuntu2204",
        "mongodb/mongodb-community-server:7.0-ubuntu2204"
    )
    
    # Upload
    sftp = ssh.open_sftp()
    with sftp.open('/opt/trustmesh-test/docker-compose.yml', 'w') as f:
        f.write(content)
    sftp.close()
    print("✅ docker-compose.yml updated")

    # Restart
    print("\n==> Restarting all services...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=120
    )
    print(out[:500])

    time.sleep(15)

    # Final check
    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"\nServices:\n{out}")

    ec, out, err = run("ss -tlnp 2>/dev/null | grep -E '8080|3000' 2>&1")
    print(f"\nPorts:\n{out[:200]}")
    
    ec, out, err = run("curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz 2>&1")
    print(f"\nBackend health: {out}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
