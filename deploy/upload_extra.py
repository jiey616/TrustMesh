import paramiko, sys, os

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

local_tar = r"C:\Users\38643\AppData\Local\Temp\extra-images.tar.gz"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Upload
    print("==> Uploading extra images (384MB)...")
    sftp = ssh.open_sftp()
    sftp.put(local_tar, "/opt/trustmesh-test/extra-images.tar.gz")
    sftp.close()
    print("✅ Uploaded")

    # Load images
    print("==> Loading images...")
    stdin, stdout, stderr = ssh.exec_command(
        "sudo docker load -i /opt/trustmesh-test/extra-images.tar.gz 2>&1",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    # Check
    stdin, stdout, stderr = ssh.exec_command(
        "sudo docker images --format 'table {{.Repository}}:{{.Tag}}\t{{.Size}}' 2>&1",
        timeout=15
    )
    print(f"\nAll images:\n{stdout.read().decode().strip()}")

    # Start services
    print("\n==> Starting services...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    import time
    time.sleep(20)

    # Status
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1",
        timeout=15
    )
    print(f"\nServices:\n{stdout.read().decode().strip()}")

    # Cleanup
    ssh.exec_command("rm -f /opt/trustmesh-test/extra-images.tar.gz")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
