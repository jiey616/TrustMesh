import paramiko, sys, time

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

ssh = paramiko.SSHClient()
ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())

def run(cmd, timeout=60):
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode().strip()
    err = stderr.read().decode().strip()
    return exit_code, out, err

try:
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=15)
    print("✅ SSH connected")

    # Check what's in the docker tarball
    print("\n==> Checking docker tarball...")
    ec, out, err = run("ls -la /tmp/docker/ 2>/dev/null || echo 'NO_DIR'; tar tzf /tmp/docker.tgz 2>/dev/null | head -20 || echo 'NO_TGZ'")
    print(out[:500])

    # If dockerd is not in the static tarball, download rootless docker
    ec, out, err = run("ls /tmp/docker/dockerd 2>/dev/null && echo 'HAVE_DOCKERD' || echo 'NEED_ROOTLESS'")
    print(f"\n{out}")

    if "NEED_ROOTLESS" in out:
        print("\n==> Downloading rootless Docker extras...")
        run("curl -fsSL https://download.docker.com/linux/static/stable/x86_64/docker-rootless-extras-27.5.1.tgz -o /tmp/rootless.tgz", timeout=120)
        run("tar xzf /tmp/rootless.tgz -C /tmp/")
        run("cp /tmp/docker-rootless-extras/dockerd-rootless-setuptool.sh /home/cloudUser/.local/bin/")
        run("cp /tmp/docker-rootless-extras/rootlesskit /home/cloudUser/.local/bin/")
        run("cp /tmp/docker-rootless-extras/rootlesskit-docker-proxy /home/cloudUser/.local/bin/")
        run("cp /tmp/docker/dockerd /home/cloudUser/.local/bin/")
        run("cp /tmp/docker/containerd /home/cloudUser/.local/bin/")
        run("cp /tmp/docker/containerd-shim-runc-v2 /home/cloudUser/.local/bin/")
        run("cp /tmp/docker/runc /home/cloudUser/.local/bin/")
        run("chmod +x /home/cloudUser/.local/bin/*")
        print("✅ Rootless Docker binaries installed")

    # Now run rootless setup
    print("\n==> Running rootless Docker setup...")
    ec, out, err = run(
        "export PATH=/home/cloudUser/.local/bin:$PATH && "
        "dockerd-rootless-setuptool.sh install 2>&1",
        timeout=120
    )
    print(out[:1000])
    if err: print(f"ERR: {err[:300]}")

    time.sleep(5)

    # Test Docker
    print("\n==> Testing Docker...")
    ec, out, err = run(
        "export PATH=/home/cloudUser/.local/bin:$PATH && "
        "export DOCKER_HOST=unix:///run/user/1000/docker.sock && "
        "docker info 2>&1 | head -5",
        timeout=15
    )
    print(out[:500])

except Exception as e:
    print(f"❌ Error: {e}", file=sys.stderr)
    sys.exit(1)
finally:
    ssh.close()
