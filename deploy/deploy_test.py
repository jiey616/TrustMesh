import paramiko, sys, os, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ROOT = r"D:\AIWorkspace\TrustMesh"
TAG = "test"
IMAGES = [
    ("backend", f"{ROOT}\\backend"),
    ("frontend", f"{ROOT}\\frontend"),
]

def run_local(cmd, timeout=300):
    print(f"  {cmd[:80]}...")
    ret = os.system(cmd)
    if ret != 0:
        print(f"  ❌ Failed (exit={ret})")
        sys.exit(1)
    return ret

print("==> Building images for linux/amd64...")
for name, path in IMAGES:
    run_local(f'docker build --platform linux/amd64 -t trustmesh/{name}:{TAG} "{path}"')

# Save all images (no gzip: Windows has no gzip CLI; tar is fine for ~90MB)
tar_path = "/tmp/trustmesh-test-images.tar"
print(f"\n==> Saving images to {tar_path}...")
run_local(
    f'docker save -o {tar_path} trustmesh/backend:{TAG} trustmesh/frontend:{TAG}'
)
print("✅ Images saved")

# Upload
ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("\n✅ SSH connected")

    # Upload images
    print("\n==> Uploading images...")
    sftp = ssh.open_sftp()
    sftp.put(tar_path, "/opt/trustmesh-test/trustmesh-test-images.tar")
    sftp.close()
    print("✅ Images uploaded")

    # Load and start
    print("\n==> Loading images and starting services...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && "
        "docker load < trustmesh-test-images.tar && "
        "TAG=test docker compose up -d --force-recreate backend frontend",
        timeout=300
    )
    out = stdout.read().decode().strip()
    print(out[:500])

    # Cleanup
    ssh.exec_command("rm -f /opt/trustmesh-test/trustmesh-test-images.tar")

    time.sleep(10)

    # Check services
    print("\n==> Checking services...")
    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && docker compose ps",
        timeout=15
    )
    print(stdout.read().decode().strip())

    print("\n✅ Deployment complete!")
    print(f"   Test environment: http://175.27.135.91:62000")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
    os.remove(tar_path)
