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

    # Check daemon.json
    ec, out, err = run("sudo cat /etc/docker/daemon.json")
    print(f"daemon.json: {out[:300]}")
    
    # Check docker info
    ec, out, err = run("sudo docker info 2>&1 | grep -A5 Mirrors", timeout=15)
    print(f"Registry Mirrors: {out[:300]}")
    
    # Check DNS
    ec, out, err = run("cat /etc/resolv.conf")
    print(f"DNS: {out[:200]}")
    
    # Test network
    ec, out, err = run("curl -sI --connect-timeout 5 https://docker.m.daocloud.io 2>&1 | head -3 || echo 'FAIL'")
    print(f"DaoCloud mirror: {out[:100]}")
    
    ec, out, err = run("curl -sI --connect-timeout 5 https://docker.xuanyuan.me 2>&1 | head -3 || echo 'FAIL'")
    print(f"Xuanyuan mirror: {out[:100]}")
    
    ec, out, err = run("curl -sI --connect-timeout 5 https://registry-1.docker.io/v2/ 2>&1 | head -3 || echo 'FAIL'")
    print(f"Docker Hub: {out[:100]}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
