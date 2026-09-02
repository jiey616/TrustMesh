"""生产机 SSH 辅助封装（175.27.135.91:62000，/opt/trustmesh-test）。

⚠️ 2026-09-01 用户确认：这台才是 TrustMesh 生产机。
   旧的 36.137.106.15 已作废，不要再连（那里只有 TokenFlow LLM 网关）。

用法：
    from ssh_helper import connect, run, upload
    c = connect()
    print(run(c, "docker ps --format '{{.Names}}\\t{{.Status}}'"))
"""
import os
import paramiko

HOST = "175.27.135.91"
PORT = 62000
USER = "cloudUser"
PASS = "uW7s-5zix.A3N5_"

REMOTE_ROOT = "/opt/trustmesh-test"

# docker CLI 不在默认 PATH 里；是**系统守护进程**，走默认 /var/run/docker.sock，
# 严禁设 DOCKER_HOST=unix://$XDG_RUNTIME_DIR/docker.sock（rootless socket 不存在）。
DOCKER_ENV = "export PATH=$HOME/.local/bin:$PATH && "


def connect():
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect(HOST, port=PORT, username=USER, password=PASS, timeout=20)
    return ssh


def run(ssh, cmd, timeout=120, quiet=False):
    """执行远程命令，返回 (exit_code, stdout, stderr)。默认打印。"""
    if not quiet:
        print(f"$ {cmd}")
    stdin, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode(errors="replace").strip()
    err = stderr.read().decode(errors="replace").strip()
    code = stdout.channel.recv_exit_status()
    if not quiet:
        if out:
            print(out)
        if err:
            print(f"[stderr] {err}")
        print(f"[exit {code}]")
    return code, out, err


def upload(ssh, local_path, remote_path):
    """上传单个文件，远端父目录不存在时自动创建。"""
    sftp = ssh.open_sftp()
    try:
        remote_dir = os.path.dirname(remote_path).replace("\\", "/")
        try:
            sftp.stat(remote_dir)
        except IOError:
            mkdir_p(sftp, remote_dir)
        sftp.put(local_path, remote_path)
    finally:
        sftp.close()
    print(f"↑ {local_path} -> {remote_path}")


def mkdir_p(sftp, remote_dir):
    parts = [p for p in remote_dir.split("/") if p]
    cur = ""
    for p in parts:
        cur += "/" + p
        try:
            sftp.stat(cur)
        except IOError:
            sftp.mkdir(cur)
