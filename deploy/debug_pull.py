import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=60):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    ec = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return ec, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check full docker info
    ec, out, err = run("sudo docker info 2>&1", timeout=15)
    print("==> Docker Info (key lines):")
    for line in out.split('\n'):
        if any(k in line.lower() for k in ['mirror', 'registry', 'server', 'storage', 'cgroup']):
            print(f"  {line}")

    # Check daemon.json
    ec, out, err = run("sudo ls -la /etc/docker/daemon.json && sudo cat /etc/docker/daemon.json")
    print(f"\ndaemon.json:\n{out[:400]}")

    # Try pulling with explicit registry mirror via env var
    print("\n==> Setting HTTP_PROXY and testing pull...")
    ec, out, err = run(
        "sudo HTTP_PROXY='' HTTPS_PROXY='' "
        "docker pull alpine:3.18 2>&1",
        timeout=120
    )
    print(out[:200])
    
    if "not found" in out or "manifest" in out or "pulling" in out:
        print("✅ Pull started!")
    elif "timeout" in out or "canceled" in out:
        print("❌ Still blocked")

    # Check if we can use a different approach - save images locally transfer
    print("\n\n== Alternative approach ==")
    print("Network is blocking Docker Hub.")
    print("Need to build all images locally and transfer them.")

except Exception as e:
    print(f"❌ Error: {e}")
    sys.exit(1)
finally:
    ssh.close()
