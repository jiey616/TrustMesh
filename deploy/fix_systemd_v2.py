import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15, banner_timeout=30)
    print("✅ SSH connected")

    # Simple check
    stdin, stdout, stderr = ssh.exec_command("sudo systemctl is-active docker", timeout=10)
    status = stdout.read().decode().strip()
    print(f"Docker systemd: {status}")

    # If not running via systemd, kill it and restart
    if status != "active":
        print("Docker not active. Restarting...")
        ssh.exec_command("sudo systemctl start docker", timeout=30)
        time.sleep(8)
        stdin, stdout, stderr = ssh.exec_command("sudo docker ps", timeout=15)
        result = stdout.read().decode().strip()
        print(f"docker ps: {result[:100] if result else '(empty)'}")
    else:
        # Docker is active, check socket
        stdin, stdout, stderr = ssh.exec_command("ls -la /var/run/docker.sock 2>&1", timeout=10)
        sock = stdout.read().decode().strip()
        print(f"Socket: {sock}")

        if "No such file" in sock:
            print("Socket missing! Restarting Docker...")
            ssh.exec_command("sudo systemctl restart docker", timeout=30)
            time.sleep(8)

        stdin, stdout, stderr = ssh.exec_command("sudo docker ps", timeout=15)
        result = stdout.read().decode().strip()
        print(f"docker ps: {result[:100] if result else '(empty)'}")

        if "Cannot connect" not in result and result:
            stdin, stdout, stderr = ssh.exec_command(
                "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1", timeout=60
            )
            up = stdout.read().decode().strip()
            print(f"compose up: {up[:200]}")

            time.sleep(10)
            stdin, stdout, stderr = ssh.exec_command(
                "cd /opt/trustmesh-test && sudo docker compose ps 2>&1", timeout=15
            )
            print(f"\nServices:\n{stdout.read().decode().strip()}")
        else:
            # Try direct dockerd
            print("Systemd Docker not working. Switching to direct dockerd...")
            ssh.exec_command("sudo systemctl stop docker 2>/dev/null || true", timeout=10)
            ssh.exec_command("sudo pkill -9 dockerd 2>/dev/null || true", timeout=10)
            ssh.exec_command("sudo rm -rf /var/run/docker* /run/docker* 2>/dev/null || true", timeout=10)
            time.sleep(3)
            ssh.exec_command("sudo nohup /usr/bin/dockerd > /tmp/dockerd.log 2>&1 &", timeout=10)
            time.sleep(8)
            stdin, stdout, stderr = ssh.exec_command("sudo docker ps", timeout=15)
            result = stdout.read().decode().strip()
            print(f"Direct dockerd: {result[:100] if result else '(empty)'}")

            if "Cannot connect" not in result:
                stdin, stdout, stderr = ssh.exec_command(
                    "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1", timeout=60
                )
                up = stdout.read().decode().strip()
                print(f"compose up: {up[:200]}")
                time.sleep(10)
                stdin, stdout, stderr = ssh.exec_command(
                    "cd /opt/trustmesh-test && sudo docker compose ps 2>&1", timeout=15
                )
                print(f"\nServices:\n{stdout.read().decode().strip()}")

    # Final health check
    stdin, stdout, stderr = ssh.exec_command(
        "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz 2>&1", timeout=10
    )
    health = stdout.read().decode().strip()
    print(f"\nBackend health: HTTP {health}")

except paramiko.AuthenticationException:
    print("❌ SSH authentication failed")
    sys.exit(1)
except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
