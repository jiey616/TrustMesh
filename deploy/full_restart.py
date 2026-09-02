import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    print("Connecting...")
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, 
                timeout=30, banner_timeout=60, auth_timeout=30)
    print("✅ SSH connected")

    # Full restart
    cmd = (
        "cd /opt/trustmesh-test && "
        "sudo TAG=test docker compose down --timeout 30 2>&1; "
        "sleep 5; "
        "sudo TAG=test docker compose up -d 2>&1"
    )
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=120)
    print(stdout.read().decode().strip()[:500])

    time.sleep(15)

    stdin, stdout, stderr = ssh.exec_command(
        "cd /opt/trustmesh-test && sudo docker compose ps 2>&1", timeout=15
    )
    print(f"\n== Services ==\n{stdout.read().decode().strip()}")

    stdin, stdout, stderr = ssh.exec_command(
        "curl -s -o /dev/null -w 'Backend: HTTP %{http_code}\n' http://127.0.0.1:8080/healthz "
        "&& curl -s -o /dev/null -w 'Frontend: HTTP %{http_code}' http://127.0.0.1:3000/",
        timeout=15
    )
    print(f"\n== Health ==\n{stdout.read().decode().strip()}")

    # Verify systemd
    stdin, stdout, stderr = ssh.exec_command(
        "sudo systemctl is-enabled docker && sudo systemctl is-active docker && "
        "ls -la /var/run/docker.sock", timeout=15
    )
    print(f"\n== Docker systemd ==\n{stdout.read().decode().strip()}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
