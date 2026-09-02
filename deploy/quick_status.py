import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    print("Connecting...")
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    print("✅ SSH connected\n")

    # Quick status
    ch = ssh.get_transport().open_session()
    ch.settimeout(15)
    ch.exec_command("sudo systemctl is-active docker; ls /var/run/docker.sock 2>&1; curl -so /dev/null -w '%{http_code}' http://127.0.0.1:8080/healthz")
    stdout = ch.makefile('r', 4096)
    print(stdout.read().decode().strip()[:300])
    ch.close()

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
    sys.exit(1)
finally:
    ssh.close()
