"""端到端自测：不依赖真实后端，用与 backend/internal/auth/external_token.go
完全相同的 claim 结构签发测试 token，启动最小外部平台服务并验证校验逻辑。

运行： python selftest.py
"""

import threading
import time
import urllib.parse
import urllib.request
import jwt

import server  # 复用 minimal-platform/server.py 的 verify()

ISSUER = "trustmesh"
SECRET = "test-secret-0000000000000000000000"  # 模拟外部应用下发的 client_secret
CLIENT_ID = "test-client-id"
PORT = 8099


def issue_token(secret=SECRET, aud=CLIENT_ID, ttl=300, exp=None, jti="nonce-1"):
    """复刻后端 IssueExternalToken 的 claim 结构。"""
    now = int(time.time())
    payload = {
        "user_id": "u_123",
        "email": "alice@trustmesh.dev",
        "name": "Alice",
        "scope": "read write",
        "project_id": "p_1",
        "task_id": "t_1",
        "iss": ISSUER,
        "sub": "u_123",
        "aud": [aud],
        "iat": now,
        "exp": exp if exp is not None else now + ttl,
        "jti": jti,
    }
    return jwt.encode(payload, secret, algorithm="HS256")


def start_server():
    from wsgiref.simple_server import make_server

    srv = make_server("", PORT, server.handler)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    return srv


def get(token: str | None) -> str:
    qs = ""
    if token is not None:
        qs = "?" + urllib.parse.urlencode({"token": token})
    with urllib.request.urlopen(f"http://localhost:{PORT}/sso{qs}", timeout=5) as r:
        return r.read().decode("utf-8")


def check(name, haystack, expect_success):
    ok = ("登录成功" in haystack) if expect_success else ("校验失败" in haystack)
    status = "PASS" if ok else "FAIL"
    print(f"[{status}] {name}")
    return ok


def main():
    server.os.environ["TM_CLIENT_SECRET"] = SECRET
    server.os.environ["TM_CLIENT_ID"] = CLIENT_ID
    start_server()
    time.sleep(0.3)

    results = []
    # 1) 无 token -> 等待页（不算校验，仅渲染）
    print("[INFO] 无 token 页面:", "等待" in get(None))
    # 2) 合法 token -> 成功
    results.append(check("合法 token 应登录成功", get(issue_token()), True))
    # 3) 错误密钥 -> 失败
    results.append(check("错误 client_secret 应拒绝", get(issue_token(secret="wrong")), False))
    # 4) 错误 aud -> 失败
    results.append(check("错误 client_id(aud) 应拒绝", get(issue_token(aud="other")), False))
    # 5) 过期 token -> 失败
    results.append(check("过期 token 应拒绝", get(issue_token(exp=int(time.time()) - 10)), False))

    passed = sum(results)
    print(f"\n结果: {passed}/{len(results)} 通过")
    if passed != len(results):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
