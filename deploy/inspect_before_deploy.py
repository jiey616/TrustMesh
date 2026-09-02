"""部署前只读勘察（第四轮）：找 frontend-v2 的真实部署位置。"""
from ssh_helper import connect, run

ssh = connect()
try:
    print("=== 1. /opt 下所有目录 ===")
    run(ssh, "ls -la /opt")

    print("\n=== 2. 家目录 ===")
    run(ssh, "ls -la ~ | head -30")

    print("\n=== 3. 全盘找 frontend-v2（限深度） ===")
    run(ssh, "find /opt /home /srv /data -maxdepth 3 -name 'frontend-v2*' 2>/dev/null | head -20 || true", timeout=90)

    print("\n=== 4. 全盘找 compose 文件 ===")
    run(ssh, "find /opt /home /srv /data -maxdepth 3 -name 'docker-compose*.yml' 2>/dev/null | head -20 || true", timeout=90)

    print("\n=== 5. 监听端口（看 5173/5174 在哪） ===")
    run(ssh, "ss -lntp 2>/dev/null | head -30 || netstat -lntp 2>/dev/null | head -30")
finally:
    ssh.close()
