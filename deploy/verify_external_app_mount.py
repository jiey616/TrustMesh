"""部署后验证：健康检查 + 新代码是否上线 + 外部平台 API 新字段。

只读检查 + 创建一个用于人工验收的示例平台（测试环境，可删）。
"""
import json
import time

from ssh_helper import connect, run, REMOTE_ROOT

TEST_USER = "test001@163.com"
TEST_PASS = "qwer1234"

ssh = connect()
try:
    print("=== 1. 容器状态 ===")
    run(ssh, "export PATH=$HOME/.local/bin:$PATH; docker ps -a --format 'table {{.Names}}\\t{{.Image}}\\t{{.Status}}' | head -6", timeout=60)

    print("\n=== 2. 后端健康 ===")
    run(ssh, "curl -s -o /dev/null -w 'backend /healthz => %{http_code}\\n' http://127.0.0.1:8080/healthz", timeout=30)

    print("\n=== 3. V2 健康 ===")
    run(ssh, "curl -s -o /dev/null -w 'frontend-v2 / => %{http_code}\\n' http://127.0.0.1:5174/", timeout=30)

    print("\n=== 4. V2 产物是否含新代码（挂载位置 / 外部平台外壳页） ===")
    # 新代码在懒加载 chunk 中，直接扫整个 assets 目录
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; "
        "docker exec trustmesh-frontend-v2 sh -c "
        "'grep -rl \"挂载位置\" /usr/share/nginx/html/assets/ 2>/dev/null | head -5'",
        timeout=60,
    )
    run(
        ssh,
        "export PATH=$HOME/.local/bin:$PATH; "
        "docker exec trustmesh-frontend-v2 sh -c "
        "'grep -rho \"sandbox=\\\\\"allow-scripts[^\"]*\" /usr/share/nginx/html/assets/ 2>/dev/null | head -2'",
        timeout=60,
    )

    print("\n=== 5. 后端新字段（未登录态应 401） ===")
    run(ssh, "curl -s -o /dev/null -w 'GET /api/v1/external-apps (no auth) => %{http_code}\\n' http://127.0.0.1:8080/api/v1/external-apps", timeout=30)

    print("\n=== 6. 登录测试账号 ===")
    code, out, err = run(
        ssh,
        f"curl -s -X POST http://127.0.0.1:8080/api/v1/auth/login "
        f"-H 'Content-Type: application/json' "
        f"-d '{{\"email\":\"{TEST_USER}\",\"password\":\"{TEST_PASS}\"}}'",
        timeout=60,
        quiet=True,
    )
    print(out[:600])
    token = ""
    try:
        payload = json.loads(out)
        token = (
            payload.get("data", {}).get("access_token")
            or payload.get("data", {}).get("token")
            or payload.get("access_token")
            or ""
        )
    except Exception as exc:  # noqa: BLE001
        print(f"解析登录响应失败: {exc}")
    print(f"token 获取: {'成功' if token else '失败'}")

    if token:
        print("\n=== 7. 外部平台列表（应含 placement/visibility/icon_url/sort_order） ===")
        code, out, err = run(
            ssh,
            f"curl -s http://127.0.0.1:8080/api/v1/external-apps -H 'Authorization: Bearer {token}'",
            timeout=60,
            quiet=True,
        )
        print(out[:1200])

        print("\n=== 8. 创建示例平台（挂载 sidebar + project_tab，公共可见） ===")
        body = json.dumps(
            {
                "name": "示例外部平台（挂载测试）",
                "base_url": "https://example.com/sso/entry",
                "client_id": "demo-mount-test",
                "sso_type": "trustmesh_jwt",
                "frame_mode": "newtab",
                "scopes": "read",
                "placement": "sidebar,project_tab",
                "visibility": "public",
                "icon_url": "",
                "sort_order": 10,
            },
            ensure_ascii=False,
        )
        # 单引号内不能有单引号，这里用 base64 传递避免转义地狱
        import base64
        b64 = base64.b64encode(body.encode("utf-8")).decode()
        code, out, err = run(
            ssh,
            f"echo {b64} | base64 -d > /tmp/ext_app_body.json && "
            f"curl -s -X POST http://127.0.0.1:8080/api/v1/external-apps "
            f"-H 'Authorization: Bearer {token}' -H 'Content-Type: application/json' "
            f"--data @/tmp/ext_app_body.json",
            timeout=60,
            quiet=True,
        )
        print(out[:900])

        print("\n=== 9. 再次列出（确认挂载字段落库） ===")
        code, out, err = run(
            ssh,
            f"curl -s http://127.0.0.1:8080/api/v1/external-apps -H 'Authorization: Bearer {token}'",
            timeout=60,
            quiet=True,
        )
        print(out[:1500])
finally:
    ssh.close()
