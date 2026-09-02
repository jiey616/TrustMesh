import paramiko, sys, time

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

    time.sleep(3)

    ec, out, err = run("sudo docker ps", timeout=15)
    print(f"docker ps: {out[:100]}")

    ec, out, err = run(
        "cd /opt/trustmesh-test && sudo TAG=test docker compose up -d 2>&1",
        timeout=60
    )
    print(f"compose up:\n{out[:300]}")

    time.sleep(10)

    ec, out, err = run("cd /opt/trustmesh-test && sudo docker compose ps 2>&1")
    print(f"\nServices:\n{out}")

    ec, out, err = run("curl -s -o /dev/null -w 'Backend: HTTP %{http_code}\n' http://127.0.0.1:8080/healthz && curl -s -o /dev/null -w 'Frontend: HTTP %{http_code}' http://127.0.0.1:3000/")
    print(f"\nHealth:\n{out}")

    ec, out, err = run("sudo systemctl is-enabled docker && sudo systemctl is-active docker")
    print(f"\nSystem: {out}")

except Exception as e:
    print(f"❌ Error: {e}", file=sys.exit)
finally:
    ssh.close()
