import paramiko, sys

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=15):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Test API through nginx proxy (frontend port)
    ec, out, err = run("curl -s http://127.0.0.1:3000/api/v1/platform/info 2>&1")
    print(f"Through nginx proxy:\n{out[:200]}")

    # Check if PlatformName component exists in JS bundle
    ec, out, err = run('sudo docker exec trustmesh-frontend sh -c "grep -c \\"displayName\\|platformName\\|TrustMesh\\" /usr/share/nginx/html/assets/index-*.js" 2>&1')
    print(f"\nTrustMesh in bundle: {out[:100]}")

    # Check the HTML title
    ec, out, err = run('curl -s http://127.0.0.1:3000/ 2>&1 | grep -i title')
    print(f"\nPage title:\n{out[:200]}")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
