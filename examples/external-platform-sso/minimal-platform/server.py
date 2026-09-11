"""
最小外部平台（用于联调测试 TrustMesh SSO 连接）。

只做一件事：接收 TrustMesh 启动跳转（base_url?token=...），校验 JWT，
页面显示「登录成功 / 失败」及用户上下文。

依赖： pip install pyjwt
配置（在 TrustMesh 创建外部应用后拿到）：
  TM_CLIENT_SECRET  创建应用时下发的 client_secret（一次性，自己保存）
  TM_CLIENT_ID      该应用的 client_id
运行：
  TM_CLIENT_SECRET=xxxx TM_CLIENT_ID=yyyy python server.py
然后在 TrustMesh 把该应用的 base_url 设为 http://localhost:8090/sso
点击「启动」即可跳转到本服务并自动校验登录。
"""

import os
import time
import urllib.parse
import jwt  # PyJWT

ISSUER = "trustmesh"
PORT = int(os.environ.get("TM_PORT", "8090"))

# 一次性 jti 消费集合（演示用：内存 + 简单 TTL）。
# 真实生产环境请用共享存储（如 Redis `SETNX jti 1 EX 300`），
# 避免多实例重复消费、以及 token 泄露后被重放。
_USED_JTI: dict[str, float] = {}


def _consume_jti(jti: str, ttl: int = 300) -> bool:
    """标记 jti 已使用；若已存在（或刚用过）返回 False，拒绝重放。

    每次「启动」后端都会签发一个唯一 jti（见 README 第 2 节），
    用它做一次性消费可在 token 5 分钟有效期内也防住重放攻击。
    """
    now = time.time()
    # 清理过期项，避免内存无限增长
    for k in [k for k, exp in _USED_JTI.items() if exp < now]:
        _USED_JTI.pop(k, None)
    if jti in _USED_JTI:
        return False
    _USED_JTI[jti] = now + ttl
    return True


def verify(token_str: str, client_secret: str, client_id: str) -> dict:
    """校验 token，成功返回 claims，失败抛 jwt 异常。"""
    return jwt.decode(
        token_str,
        client_secret,
        algorithms=["HS256"],
        issuer=ISSUER,
        audience=client_id,
        options={"require": ["exp"]},
    )


_PAGE = """<!doctype html><html lang=zh><meta charset=utf-8>
<h2>最小外部平台 · SSO 联调</h2>
{body}
<hr><small>把本服务地址配置为 TrustMesh 外部应用的 base_url 后，点击「启动」即可触发跳转。</small>
</html>"""


def build_body(token: str | None, client_secret: str, client_id: str) -> str:
    if not token:
        return "<p>等待从 TrustMesh 跳转（带 <code>?token=</code>）…</p>"
    try:
        claims = verify(token, client_secret, client_id)
    except Exception as e:  # noqa: BLE001
        return f"<p style='color:red'>❌ 校验失败：{e}</p>"

    # 防重放：jti 一次性消费（见 _consume_jti 注释）
    jti = claims.get("jti")
    if not jti or not _consume_jti(jti):
        return "<p style='color:red'>❌ 校验失败：jti 已使用（疑似重放攻击）</p>"

    uid = claims.get("sub") or claims.get("user_id")
    rows = "".join(
        f"<li><b>{k}</b>: {v}</li>"
        for k, v in [
            ("user_id", uid),
            ("email", claims.get("email")),
            ("name", claims.get("name")),
            ("scope", claims.get("scope")),
            ("project_id", claims.get("project_id")),
            ("task_id", claims.get("task_id")),
            ("jti", claims.get("jti")),
            ("exp", claims.get("exp")),
        ]
    )
    return (
        "<p style='color:green'>✅ SSO 登录成功</p>"
        f"<ul>{rows}</ul>"
        "<p>→ 这里即可建立本地会话 / 设置 cookie。</p>"
    )


def handler(environ, start_response):
    # 防 token 经 Referer 泄露
    start_response(
        "200 OK",
        [("Content-Type", "text/html; charset=utf-8"), ("Referrer-Policy", "no-referrer")],
    )
    qs = environ.get("QUERY_STRING", "")
    token = urllib.parse.parse_qs(qs).get("token", [None])[0]
    client_secret = os.environ.get("TM_CLIENT_SECRET", "")
    client_id = os.environ.get("TM_CLIENT_ID", "")
    body = _PAGE.format(body=build_body(token, client_secret, client_id))
    return [body.encode("utf-8")]


if __name__ == "__main__":
    from wsgiref.simple_server import make_server

    if not os.environ.get("TM_CLIENT_SECRET") or not os.environ.get("TM_CLIENT_ID"):
        print("⚠️  请先设置环境变量 TM_CLIENT_SECRET 与 TM_CLIENT_ID")
        print("   例: TM_CLIENT_SECRET=xxxx TM_CLIENT_ID=yyyy python server.py")
    print(f"▶ 最小外部平台监听 http://localhost:{PORT}/sso")
    make_server("", PORT, handler).serve_forever()
