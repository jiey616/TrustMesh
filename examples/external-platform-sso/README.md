# 外部平台 SSO 校验参考实现

TrustMesh 通过签发一个**短时效 HS256 JWT** 让外部平台在「新标签页」模式下发起来并自动登录。
本目录给出**外部平台侧**如何校验该 token 并完成账号映射的参考代码（Go / Node / Python）。

> 后端签发逻辑见 `backend/internal/auth/external_token.go` 与 `backend/internal/handler/external_app.go`，
> 本目录代码与其字段、算法、密钥完全一致。

## 1. Token 是怎么来的

1. 用户在 TrustMesh 里「连接」一个外部平台，后端生成一对 `client_id` / `client_secret`。
   `client_secret` **仅在一次创建响应里明文返回**，之后不再存储展示 —— 把它配置进外部平台的 SSO 校验端。
2. 用户点击「启动」外部平台，后端调用 `IssueExternalToken` 用 `client_secret` 签名一个 JWT，
   并把 `launch_url = base_url?token=xxx` 返回给前端。
3. 前端在新标签页打开该 URL，外部平台从 `?token=` 读取 token 并校验。

## 2. JWT 结构与声明（claims）

| Claim | 来源 | 说明 |
| --- | --- | --- |
| `iss` | 固定 `"trustmesh"` | 签发方，平台必须校验 |
| `aud` | 应用 `client_id` | 平台必须校验其等于自己的 `client_id`，否则拒绝 |
| `sub` | 用户 ID | 登录主体 |
| `user_id` | 用户 ID | 与 `sub` 相同，便于非标准库读取 |
| `email` | 用户邮箱（可能为空） | 账号映射候选键 |
| `name` | 用户昵称（可能为空） | 展示用 |
| `scope` | 应用 scopes | 权限范围 |
| `project_id` | 可选 | 来源项目上下文 |
| `task_id` | 可选 | 来源任务上下文 |
| `iat` / `exp` | 时间戳 | 默认 TTL = 5 分钟（`EXTERNAL_APP_TOKEN_TTL`） |
| `jti` | 随机 nonce | **每次启动唯一**，可用于一次性 / 防重放 |

算法：**HS256**，密钥 = `client_secret`（对称共享密钥）。

## 3. 平台侧必须做的校验（硬性）

1. 从 `?token=` 取出 token（注意设置 `Referrer-Policy: no-referrer`，避免 token 进 Referer 泄露）。
2. **算法固定 HS256**，拒绝 `alg=none` 及其它算法（库默认会拒绝 none，但仍要显式限定）。
3. 用共享 `client_secret` 校验签名。
4. 校验 `iss == "trustmesh"`。
5. 校验 `aud` 包含本平台的 `client_id`。
6. 校验 `exp` 未过期（库会自动做，但建议显式感知）。
7. （推荐）用 `jti` 做一次性消费：把 jti 记入已用集合，重复提交直接拒绝，防重放 / 防 token 泄露后被复用。
8. 校验通过后，用 `sub` / `email` 在本地查找或按需创建账号，建立 SSO 会话。

## 4. 目录

- `go/verify.go` —— Go（golang-jwt/v5），与后端同库同源，最贴近参考实现
- `node/verify.js` —— Node.js（jsonwebtoken）
- `python/verify.py` —— Python（PyJWT）

每个示例都暴露一个 `verify(token, clientSecret, clientID)` 函数，返回解析后的 claims 或抛错。

- `minimal-platform/` —— 单文件可跑的最小外部平台（WSGI），用于端到端联调：
  - `server.py` 接收 `base_url?token=`，校验 JWT 后渲染登录页（含 jti 一次性防重放）
  - `selftest.py` 自签同名格式 token 跑通 4 条路径（合法 / 错密钥 / 错 aud / 过期）
  - `realtest.py` 对接**已部署后端**：登录 → 调 launch 接口拿真 token → 验证连接

## 5. TrustMesh 侧如何发起（外部平台参考）

外部平台本身**不调用**任何 TrustMesh 接口，它只被动接收跳转 URL。发起动作由
TrustMesh 前端完成，流程如下（供联调时对照）：

1. 前端 `POST /api/v1/external-apps/:id/launch`（需登录 Bearer Token），
   可选体 `{"project_id":"","task_id":""}`。
2. 后端用该应用 `client_secret` 签发 JWT，返回：

   ```json
   { "code": 0, "data": { "launch_url": "https://your-app.example.com/sso?token=xxxx",
                          "expires_in": 300 } }
   ```

   > ⚠️ 后端所有响应都包在 `data` 层（如 `data.external_apps`、`data.launch_url`、
   > `data.access_token`、`data.user.id`），联调取字段时别漏了这层。
3. 前端在新标签页打开 `launch_url`，外部平台从 `?token=` 读取并校验（第 3 节）。

## 6. 安全边界（实测结论）

- **token 经 URL 传递**：务必全程 HTTPS；外部平台设置 `Referrer-Policy: no-referrer`
  （`server.py` 已设置），并且**不要在任何日志里记录完整 launch_url / token**。
- **TTL 仅 5 分钟** + **jti 每次唯一**：光靠 exp 不够，必须配合第 3 节第 7 条的
  jti 一次性消费来防重放（`server.py` 已演示内存版，生产用共享存储）。
- **launch 接口不校验应用归属**：当前后端允许任意已登录用户启动任意 `enabled`
  应用（不做创建者鉴权）。对外部平台而言这不影响校验（token 仍只用该应用自己的
  `client_secret` 验签），但 TrustMesh 运营侧应知晓该宽松策略。
- `client_secret` 仅在创建时一次性下发，之后后端不再展示；一旦疑似泄露，应在
  TrustMesh 重新生成。
