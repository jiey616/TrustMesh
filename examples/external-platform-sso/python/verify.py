"""外部平台 SSO 校验示例（Python）。

与后端 external_token.go 的 claims / 算法 / 密钥一致。

安装依赖： pip install pyjwt
安全提示：client_secret / client_id 放环境变量或密钥库，不要入库。
"""

import os
import urllib.parse
import jwt  # PyJWT

ISSUER = "trustmesh"


def verify(token_str: str, client_secret: str, client_id: str) -> dict:
    """校验 TrustMesh 签发的 SSO token，返回解码后的 claims。

    校验失败会抛出 jwt 相关异常（InvalidTokenError / ExpiredSignatureError /
    InvalidAudienceError / InvalidIssuerError / InvalidSignatureError 等）。
    """
    if not client_secret:
        raise ValueError("client_secret not configured")
    if not client_id:
        raise ValueError("client_id not configured")

    # PyJWT 在 decode 时自动校验签名、exp、iss、aud。
    return jwt.decode(
        token_str,
        client_secret,
        algorithms=["HS256"],   # 1) 强制算法，拒绝 none 等
        issuer=ISSUER,          # 2) 校验 iss
        audience=client_id,     # 3) 校验 aud == 本平台 client_id
        options={"require": ["exp"]},  # 4) 要求 exp 存在
    )


def extract_token(raw_url: str) -> str:
    """从跳转 URL 中取 ?token=。"""
    qs = urllib.parse.urlparse(raw_url).query
    params = urllib.parse.parse_qs(qs)
    token = params.get("token")
    if not token:
        raise ValueError("missing ?token= in launch url")
    return token[0]


if __name__ == "__main__":
    client_secret = os.environ["TM_CLIENT_SECRET"]
    client_id = os.environ["TM_CLIENT_ID"]
    launch_url = os.environ["TM_LAUNCH_URL"]  # 含 ?token=

    token_str = extract_token(launch_url)
    claims = verify(token_str, client_secret, client_id)

    # 5) 防重放（推荐）：claims["jti"] 每次唯一，记入一次性消费集合，重复提交拒绝。
    # 6) 账号映射：优先 email，其次 sub(==user_id)。
    subject = claims.get("sub") or claims.get("user_id")
    print("SSO login:", {
        "user_id": subject,
        "email": claims.get("email"),
        "name": claims.get("name"),
        "scope": claims.get("scope"),
        "jti": claims.get("jti"),
        "project_id": claims.get("project_id"),
        "task_id": claims.get("task_id"),
    })
