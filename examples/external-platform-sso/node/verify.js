// 外部平台 SSO 校验示例（Node.js）。
// 与后端 external_token.go 的 claims / 算法 / 密钥一致。
//
// 安装依赖： npm i jsonwebtoken
//
// 安全提示：把 client_secret / client_id 放在环境变量或密钥管理里，不要入库。

const jwt = require("jsonwebtoken");

const ISSUER = "trustmesh";

/**
 * 校验 TrustMesh 签发的 SSO token。
 * @param {string} tokenStr     ?token= 的值
 * @param {string} clientSecret 创建外部应用时下发的 client_secret
 * @param {string} clientID     本平台自己的 client_id（必须等于 token 的 aud）
 * @returns {object} 解码后的 claims
 * @throws 校验失败时抛错
 */
function verify(tokenStr, clientSecret, clientID) {
  if (!clientSecret) throw new Error("client_secret not configured");
  if (!clientID) throw new Error("client_id not configured");

  return jwt.verify(tokenStr, clientSecret, {
    // 1) 强制算法，杜绝 alg=none 等降级攻击
    algorithms: ["HS256"],
    // 2) 校验 iss
    issuer: ISSUER,
    // 3) 校验 aud 包含本平台 client_id
    audience: clientID,
    // 4) 要求 exp 存在并校验
    requireExpirationTime: true,
  });
  // jsonwebtoken 默认会拒绝过期 token，并在上述 option 不通过时抛错。
}

/** 从跳转 URL 中取 ?token= */
function extractToken(rawUrl) {
  const u = new URL(rawUrl);
  const t = u.searchParams.get("token");
  if (!t) throw new Error("missing ?token= in launch url");
  return t;
}

// ---- 使用示例 ----
if (require.main === module) {
  const clientSecret = process.env.TM_CLIENT_SECRET;
  const clientID = process.env.TM_CLIENT_ID;
  const launchURL = process.env.TM_LAUNCH_URL; // 含 ?token=

  const tokenStr = extractToken(launchURL);
  const claims = verify(tokenStr, clientSecret, clientID);

  // 5) 防重放（推荐）：claims.jti 每次唯一，记入一次性消费集合，重复提交拒绝。
  // 6) 账号映射：优先 email，其次 sub(==user_id)。
  const subject = claims.sub || claims.user_id;
  console.log("SSO login:", {
    user_id: subject,
    email: claims.email,
    name: claims.name,
    scope: claims.scope,
    jti: claims.jti,
    project_id: claims.project_id,
    task_id: claims.task_id,
  });
}

module.exports = { verify, extractToken, ISSUER };
