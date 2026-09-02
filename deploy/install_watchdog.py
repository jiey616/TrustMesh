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

    # Create restart script
    restart_script = """#!/bin/bash
# Auto-restart Docker and TrustMesh services
if ! docker ps >/dev/null 2>&1; then
    echo "$(date): Docker is down, restarting..."
    rm -rf /var/run/docker* /run/docker* 2>/dev/null
    nohup /usr/bin/dockerd --registry-mirror=https://docker.m.daocloud.io --registry-mirror=https://docker.xuanyuan.me --registry-mirror=https://docker.1ms.run > /tmp/dockerd.log 2>&1 &
    sleep 10
    cd /opt/trustmesh-test && TAG=test docker compose up -d 2>&1
    echo "$(date): Services restarted"
fi
"""
    run("sudo bash -c 'cat > /opt/trustmesh-test/docker-watchdog.sh' << 'SCRIPTEOF'\n" + restart_script + "\nSCRIPTEOF")
    run("sudo chmod +x /opt/trustmesh-test/docker-watchdog.sh")

    # Add crontab for root to run every 2 minutes
    run("sudo crontab -l 2>/dev/null | grep -v docker-watchdog | sudo crontab -")
    run("(sudo crontab -l 2>/dev/null; echo '*/2 * * * * /opt/trustmesh-test/docker-watchdog.sh') | sudo crontab -")

    ec, out, err = run("sudo crontab -l 2>/dev/null")
    print(f"Crontab:\n{out}")

    print("\n✅ Watchdog installed! Docker will auto-restart within 2 minutes if it crashes.")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
