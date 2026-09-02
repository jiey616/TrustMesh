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
    
    # Check all images
    ec, out, err = run("sudo docker images 2>&1")
    print(f"Images:\n{out}")

    # Load remaining images from the uploaded file
    print("\n==> Loading full image set...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo docker load < fast-images.tar.gz 2>&1",
        timeout=300
    )
    print(out[:500])

    # Check images again
    print("\n==> Available images:")
    ec, out, err = run("sudo docker images --format 'table {{.Repository}}:{{.Tag}}\t{{.Size}}' 2>&1")
    print(out)

    # Start services
    print("\n==> Starting services...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=300
    )
    print(out[:500])

    time.sleep(15)

    # Check status
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1",
        timeout=15
    )
    print(f"\nServices:\n{out[:500]}")

    # Check health
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo docker compose logs --tail=5 backend mongo frontend 2>&1",
        timeout=15
    )
    print(f"\nRecent logs:\n{out[:500]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
