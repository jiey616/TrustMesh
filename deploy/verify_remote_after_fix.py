"""只读：确认远端测试机已换上修复后的前端产物。"""
from ssh_helper import connect, run

ssh = connect()
try:
    print("=== 容器状态 ===")
    run(ssh, "export PATH=$HOME/.local/bin:$PATH; docker ps --format 'table {{.Names}}\\t{{.Status}}' | head -4", timeout=60)

    print("\n=== 健康 ===")
    run(ssh, "curl -s -o /dev/null -w 'backend  => %{http_code}\\n' http://127.0.0.1:8080/healthz", timeout=30)
    run(ssh, "curl -s -o /dev/null -w 'v2       => %{http_code}\\n' http://127.0.0.1:5174/", timeout=30)

    print("\n=== 线上 index.html 引用的 bundle ===")
    run(ssh, "curl -s http://127.0.0.1:5174/ | grep -o 'assets/index-[A-Za-z0-9_-]*\\.js'", timeout=30)

    print("\n=== 产物是否含挂载点配置 UI ===")
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; docker exec trustmesh-frontend-v2 sh -c "
        "'grep -rl \"挂载位置\" /usr/share/nginx/html/assets/ | head -3'",
        timeout=60,
    )

    print("\n=== 是否还残留旧 bundle ===")
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; docker exec trustmesh-frontend-v2 sh -c "
        "'ls /usr/share/nginx/html/assets/ | grep -c \"index-CrKduxrD\" || echo 0'",
        timeout=60,
    )
finally:
    ssh.close()
