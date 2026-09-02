import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=30)
    transport = ssh.get_transport()

    def run(cmd, timeout=8):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Check log
    out = run("tail -20 /tmp/dockerd.log 2>/dev/null", 8)
    print(f"dockerd log:\n{out[:500]}")

    # Also check for core dumps
    out = run("ls /tmp/core* 2>/dev/null; dmesg 2>/dev/null | tail -3 || true", 8)
    print(f"\nSystem:\n{out[:200]}")

except Exception as e:
    print(f"❌ {e}")
finally:
    ssh.close()
