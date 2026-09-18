# PLAN：桌面端应用安装 / 升级（2026-09-18 定稿）

> 状态：**待用户确认后开工**。本文件是执行期的唯一依据，自包含（可脱离对话上下文使用）。
> 调研过程与实测证据见 `.workbuddy/memory/2026-09-18.md`「桌面端现状盘点」一节。

## 0 一句话

平台管理端新增「桌面发行版」管理（**上传 / 发布 / 回滚**）；桌面端用 **electron-updater** 从自家平台拉更新，**应用内提示 → 用户点一下 → 静默安装 → 重启生效**。

## 1 已确认的决策（含理由，执行期勿再讨论）

| # | 决策 | 理由 |
|---|---|---|
| 1 | 升级形态 = **应用内一键更新**（electron-updater + `latest.yml`） | 用户选定 |
| 2 | 使用者 = 内部十来人；**不买代码签名证书** | 用户选定 |
| 3 | 桌面壳与前端**维持现状**（前端打进 asar） | 改 origin 会丢一次本地登录态、`will-navigate` 白名单要改；解耦另立项目 |
| 4 | 升级**保配置 + 保登录态**，**不强推最低版本** | `userData` 与 localStorage 必须活下来，否则每次升级都要重配信任开关 + 重登 |
| 5 | 「回滚」= **只回滚发布指针**（未升级者拿旧版）；**不做客户端降级** | electron-updater 只认「远程 version > 本地 version」 |
| 6 | 更新检查端点 = **公开只读**（无需登录） | 更新器跑在**独立 session 分区**、不共享登录 cookie；要求登录会让登出很久的机器**永远升不了级** |
| 7 | 发布流程 = **网页上传 + 后台审**；`release.json` 随包上传提供元数据 | 用户 Q4 选"暂不考虑脚本推送鉴权" ⇒ 不做脚本直推；版本号由脚本产出、不手填 |
| 8 | 保留**最近 5 版**，后台可手动删 | 每版 87MB，服务器剩 37G |
| 9 | 更新入口**只在桌面端**渲染（`isDesktopShell()` 判定） | Web 端出现一个永远不能用的按钮只会制造困惑 |
| 10 | 信任策略：只对 `electron-updater` 分区、**只放行烘焙的更新主机**，**不依赖 `trustInsecureTls` 开关** | 否则没开过该开关的用户永远升不了级 |
| 11 | 更新源地址**烘焙固定生产地址**（不跟随用户可配的 serverConfig） | 只有一个生产环境；避免用户填错地址就跑去别处更新 |
| 12 | **砍掉 portable** | 自动更新**只支持 NSIS**，便携版用户会永远停在旧版，且造成"同一份代码两种升级行为"的解释成本 |
| 13 | 更新交互 = 启动后延迟静默检查 + **右下角非模态提示** + 设置页"检查更新"兜底；**频率 = 启动一次** | 内部十来人，最不该被强制重启打断工作 |

## 2 明确不做（防范围蔓延）

- **channel（stable/beta）** —— 预留 `channel` 字段但只用一个值。
- **客户端降级**（版本号变小也算更新）—— 会造成两台机器来回拉锯。
- **强制升级 / 最低版本门禁** —— 需先给服务端加版本字段，留待真有 API 不兼容时再做。
- **灰度发布**（`stagingPercentage`）—— 十来人没有统计意义；字段留着。
- **脚本直推到后端**（含长期 upload token）—— 用户"暂不考虑"。接口按平台管理员 JWT 鉴权实现，将来要加 token 不必改表。
- **代码签名** —— 用户已确认不买。

## 3 关键约束与**已实测**事实（避免重复踩坑）

1. 🔴 **electron-updater 走 `electron.net.request`（Chromium 栈），不是 Node https**，且挂在**独立分区** `session.fromPartition('electron-updater')` 上（`electronHttpExecutor.js` 的 `NET_SESSION_NAME`）。
2. 🔴 **`app.on('certificate-error')` 对该请求完全不触发**（实测日志一次都没打印）⇒ 现有 `trustInsecureTls` 开关对更新通道**无效**。**正解**：`session.fromPartition('electron-updater').setCertificateVerifyProc((req, cb) => …)`，可运行时设置。四分位实测：无处理 ❌ / 仅 apphook ❌ / `cb(0)` ✅ 200 / `cb(-3)` ❌。
3. 🔴 **`win.verifyUpdateCodeSignature` 必须设 `false`**（默认 `true`；它会把构建期 `publisherName` 写进 `app-update.yml`，拿下载包的 Authenticode 主体比对，不匹配即拒绝安装）。官方原文：*"Disable this only if your updates are not Authenticode-signed."* 未签名 ⇒ 只剩这一条路。**代价：更新通道真伪性只剩 HTTPS + sha512，属知情接受的取舍。**
4. Node 的 `NODE_EXTRA_CA_CERTS` **只在进程启动时读取**，在 `main.cjs` 里设**无效**（实测：外部注入 200 / 进程内设置失败）。且对本场景本就无关（走 Chromium 栈）。
5. **只有 NSIS 可自动更新**，portable 不行（⇒ 决策 12）。
6. **更新器不共享登录 cookie**（独立分区）⇒ 端点必须公开只读（⇒ 决策 6）。
7. `latest.yml` 的 `version` **必须等于** `package.json` 的 version，否则**静默查不到更新**。
8. electron-updater **不支持回滚**；坏版本只能向上修复。
9. 基建已就绪，**无需新增**：nginx 已有 `client_max_body_size 512m`；backend 有 named volume `trustmesh-files-data → /var/lib/trustmesh-files`（**能活过 `docker rm` + `up -d`**）；`/api/` 已代理到 backend。
10. ⚠️ **0.1.0 无法自更新**：当前构建**没有 `publish` 配置 ⇒ 镜像内没有 `app-update.yml`**，也没有任何更新代码。⇒ **`0.2.0` 必须让用户手动装一次**，从 `0.2.0` 起才能自动升级。这是发布节奏的硬约束，必须写进给用户的操作说明。

## 4 后端工作项

- **权限点**：`authz/permission.go` 加 `PermPlatformDesktopRelease = "platform.desktop.release"`，并加进 `PlatformPermissions()`。前端 `src/lib/perms.ts` 同步加一项。
- **审计**：`model/audit.go` 加 `AuditActionDesktopReleaseUpload/ Publish / Rollback / Delete` 与 `AuditTargetDesktopRelease = "desktop_release"`。
- **数据模型** `internal/model/desktop_release.go`：`ID`、`Version`(semver)、`Channel`(预留)、`FileName`、`Size`、`Sha512`、`Notes`、`Status`(`draft`|`published`|`archived`)、`PublishedAt`、`UploadedBy`、`CreatedAt`。集合 `desktop_releases`。
- **文件存储**：复用 `project.NewLocalFileStorage(filepath.Join(cfg.FilesStoragePath, "desktop-releases"))`，`projectID` 传 `"desktop-releases"`、`fileID` 传 version。**不新写存储层**。
- **端点**（平台侧，逐一挂 `authz.RequirePlatformPerm(authz.PermPlatformDesktopRelease, platformAdminChecker)`）：
  - `GET  /api/v1/platform/desktop-releases` 列表
  - `POST /api/v1/platform/desktop-releases` 上传（multipart：exe + `release.json` [+ `latest.yml`、`.blockmap`]）
  - `POST /api/v1/platform/desktop-releases/:id/publish` 发布（幂等：发布前把其它 `published` 置 `archived`）
  - `POST /api/v1/platform/desktop-releases/:id/rollback` 回滚（把指针指回旧版；语义 = 只影响尚未升级者）
  - `DELETE /api/v1/platform/desktop-releases/:id` 删除（**禁止删除当前 published**；同时删文件）
- **公开 feed**（`plat` 组之外，**无鉴权**）：
  - `GET /api/v1/desktop/releases/feed/latest.yml` —— **按当前 published 记录动态生成**，`files[].url` 写该 id 的下载地址、`sha512` 用库里的值
  - `GET /api/v1/desktop/releases/feed/:filename` —— 安装包/`.blockmap` 字节流；⚠️ **必须支持 HTTP Range**（差分下载依赖它），需实测确认
- **清理**：每次发布后检查版本数 > 5，删除最旧的 `archived`（记录 + 文件）。

## 5 前端（Web 管理端）工作项

- 新页 `src/pages/platform/DesktopReleasesPage.tsx`（参照 `external-apps` 那套）；
  路由 + `layouts/MainLayout.tsx` 菜单（`/platform/desktop-releases`，图标 `WindowsOutlined`）。
- `src/api/platformAdmin.ts` 增 5 个函数；`src/types/index.ts` 增 `PlatformDesktopReleaseView`。
- 上传：单请求 multipart（87MB，nginx 已 512m），**必须有进度条 + 失败重试**；上传时一并选 `release.json`，前端先校验 `version` 与文件名一致再提交。
- 列表列：版本 / 状态 / 大小 / sha512 前 12 位 / 上传人 / 时间 / 操作（发布·回滚·删除）。
- 发布与回滚二次确认（说明"回滚只影响尚未升级的客户端"）。

## 6 桌面端工作项

- **`package.json` 的 `build`**：
  - `win.target` 去掉 `portable`，只留 `nsis`
  - `win.artifactName`: `TrustMesh-Setup-${version}.exe`（现名 `TrustMesh Setup 0.1.0.exe` 含空格，URL 里要转义）
  - `win.verifyUpdateCodeSignature: false`
  - `win.signtoolOptions`：不配（不签名）
  - `publish: { provider: "generic", url: "https://175.27.135.91/api/v1/desktop/releases/feed" }`
  - `nsis.deleteAppDataOnUninstall: false`（显式写死，保证 `%APPDATA%\TrustMesh` 不被卸载清掉）
- **`electron/main.cjs`**：
  - `app.whenReady` 里对 `session.fromPartition('electron-updater')` 调 `setCertificateVerifyProc`，**只在 `request.hostname` 等于烘焙的更新主机时 `cb(0)`，其余 `cb(-3)`**
  - 接 updater 生命周期（`checking-for-update` / `update-available` / `download-progress` / `update-downloaded` / `error`），状态经 IPC 推给渲染进程
  - ⚠️ **不要在 `app.whenReady` 之前调 `checkForUpdates`**（IPC 未就绪会丢事件）
  - ⚠️ 不调 `setFeedURL`（地址由 `app-update.yml` 烘焙，见决策 11）
- **`electron/preload.cjs`**：`window.desktop` 增 `updateCheck()` / `updateDownload()` / `updateInstall()` / `getUpdateState()` / `onUpdateState(cb)`（`tm:update-*` 通道）。**注意：改了 preload 就必须发新桌面包。**
- **渲染进程**：`src/components/settings/DesktopUpdateCard.tsx`（`isDesktopShell()` 守卫，挂进 `pages/ProfilePage.tsx`，与 `ServerConfigCard` 并列）；角落提示用 antd `notification`，按钮为「重启更新 / 稍后」——**非模态，绝不打断工作**。
- **`_deploy_desktop.py`**（比照 `_deploy_fv2.py` 风格）：
  `npm version patch` → `desktop:pack` → 计算 exe 的 sha512 → 产出 `release.json{version,file,size,sha512,notes}` → 打印**待上传清单**到 stdout（不做网络请求）。

## 7 验收清单

**后端**：`go test ./...`；权限测试确认企业 owner 调 `/platform/desktop-releases` → **403**；`go build`；gofmt 干净。
**前端**：`tsc -p tsconfig.app.json --noEmit` 零错误；`eslint . --max-warnings=4`（不得新增第 5 条）；`vitest run` 全绿。
**端到端（桌面真机）**：
1. 手动装 `0.1.0` → 无法自更新（预期，见约束 10）。
2. 手动装 `0.2.0` → 后台发布 `0.3.0` → 重启应用 → **出现非模态提示** → 点「重启更新」→ 装好后重启 → **版本变 `0.3.0`**。
3. **升级后检查**：`%APPDATA%\TrustMesh\trustmesh-desktop.json` 仍在（`trustInsecureTls` 未丢）；**登录态仍在**（不需要重登）。
4. **回滚**：后台把指针指回 `0.2.0` → **未升级的机器**查到 `0.2.0`；**已升到 `0.3.0` 的机器不受影响**（预期行为）。
5. **公开可读**：未登录状态（清掉分区 cookie）仍能拉 `latest.yml` → 200。
6. Range 请求实测：`curl -r 0-99 .../feed/<file>` → **206**（差分下载前提）。

## 8 风险

- **未签名的 SmartScreen**：首次手动安装会出现「未知发布者」，需在操作说明里教用户点「更多信息 → 仍要运行」。这是不签名的必然代价。
- **差分下载**：electron-updater 依赖 `.blockmap` + 服务端 **Range** 支持；若 Range 不可用则退化为每次全量 87MB（内部网络可接受）。**需在实现时实测**。
- **磁盘**：5 版 × 87MB ≈ 435MB，服务器剩 37G，安全。
- **版本号纪律**：`latest.yml` 的 version 与 `package.json` 必须一致 ⇒ 一律用 `_deploy_desktop.py` 自增，禁止手改。
- **改 preload/main 必须发新版**：`0.2.0` 之后任何主进程改动都要求用户升级，注意不要频繁改。

## 9 执行顺序

1. 后端（模型 + 权限 + 审计 + 5 个平台端点 + 2 个公开端点 + 清理）→ 单测
2. 前端管理页（列表 / 上传 / 发布 / 回滚）
3. `package.json` build 配置 + `main.cjs` + `preload.cjs` + 桌面 UI + `_deploy_desktop.py`
4. 出 `0.2.0` 包 → **手动装一台做基线**
5. 后台发布 `0.2.1` → 走完验收清单 2–6
