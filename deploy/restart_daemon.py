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

    def run(cmd, timeout=10):
        ch = transport.open_session()
        ch.settimeout(timeout)
        ch.exec_command(cmd)
        out = ch.makefile('r', 4096).read().decode().strip()
        ch.close()
        return out

    # Minimal commands - just restart docker daemon
    print("Restarting Docker daemon...")
    run("sudo systemctl restart docker", 30)
    time.sleep(8)

    print("Testing docker...")
    out = run("sudo docker ps 2>&1", 10)
    print(f"docker ps: {out[:100]}")
    
    if "Cannot connect" in out:
        out = run("sudo docker ps 2>&1", 10)
        print(f"retry: {out[:100]}")
        
    if "Cannot connect" not in out:
        print("✅ Docker functioning!")
        out = run("sudo docker rm -f $(sudo docker ps -aq) 2>/dev/null", 10)
        print(f"Remove: {out[:100]}")
        
        time.sleep(2)
        
        # Now start with compose but skip image pull
        ch = transport.open_session()
        ch.settimeout(120)
        ch.exec_command("cd /opt/trustmesh-test && sudo rm -f /etc/docker/daemon.json && sudo systemctl restart docker && sleep 5 && sudo COMPOSE_DISABLE_PULL=true TAG=test docker compose up -d 2>&1", timeout=120)
        out = ch.makefile('r', 4096).read().decode().strip()
        print(out[:500])
        ch.close()

except Exception as e:
    print(f"❌ {type(e).__name__}: {e}")
finally:
    ssh.close()
