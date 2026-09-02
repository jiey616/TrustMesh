import paramiko, sys

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

    # Check tar file
    ec, out, err = run("ls -la /opt/trustmesh-test/fast-images.tar.gz 2>&1")
    print(f"Tar file: {out[:100]}")

    # Check what's in it
    ec, out, err = run("tar tzf /opt/trustmesh-test/fast-images.tar.gz 2>&1 | head -20")
    print(f"\nTar contents:\n{out[:500]}")

    # Load with verbose
    print("\n==> Loading with verbose...")
    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo docker load -i fast-images.tar.gz 2>&1",
        timeout=120
    )
    print(out[:500])

    # Check images
    ec, out, err = run("sudo docker images --format 'table {{.Repository}}:{{.Tag}}' 2>&1")
    print(f"\nImages after load:\n{out}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
