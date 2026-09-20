# 安卓端自签证书预置（`android-config/`）

企业内部分发专用。解决「Android 系统层不允许 App 绕过证书校验，Android 7+ 也不信任用户手动安装的 CA」这一阻断项。

## 目录内容

| 文件 | 作用 |
|---|---|
| `prod_ca.crt` | 从生产 `175.27.135.91:443` 导出的自签证书（PEM） |
| `network_security_config.xml` | 网络安全配置：目标域名只信任上面这张证书 |
| `apply.mjs` | 一键把上面两个文件套进 Capacitor 生成的安卓工程，并给 `AndroidManifest.xml` 注入属性 |
| `apply_last_run.txt` | `apply.mjs` 最近一次执行的输出（自动生成） |

## 生产证书事实（2026-09-19 实测导出）

| 项 | 值 |
|---|---|
| Subject / Issuer | `CN=175.27.135.91, O=TrustMesh, C=CN`（两者相同 ⇒ **自签**） |
| 证书链长度 | **1**（自签根即叶证书，无中间 CA） |
| Basic Constraints | `CA:TRUE`（critical） |
| Subject Alternative Name | `IP Address:175.27.135.91, DNS:localhost` |
| 有效期 | 2026-09-02 → **2036-08-30** |
| DER 大小 | 883 B |
| SHA-256 指纹 | `440A69DC138A54B0EA9CD259B3AEF802F518A8B971E428017E2E9F90C6BDE8D9` |

因 SAN 已包含 IP，Android 的**主机名校验**对 `https://175.27.135.91` 天然通过，无需额外处理。

## 已完成的验证（证书 material 层，非 Android 层）

用 Node 的 `tls` 模块对生产做了「前/后对照」，证明该证书确实是充分的信任锚：

```
不装证书（默认信任库） → ERROR DEPTH_ZERO_SELF_SIGNED_CERT: self-signed certificate
导入 prod_ca.crt 做锚   → authorized=true  proto=TLSv1.3  ms=40  san="IP:175.27.135.91"
```

这一步只证明**证书文件本身是对的**，不等于证明 Android 的 `network_security_config` 生效 —— 后者需要真机验证，**已于 2026-09-19 完成并通过（A/B 反证，见下方「实测结果」）**。

## 套用步骤

```bash
cd frontend-v2
npx cap add android          # 首次生成原生工程（会创建 android/）
node android-config/apply.mjs # 套用证书与配置（幂等，可重复执行）
```

`apply.mjs` 做三件事：

1. `prod_ca.crt` → `android/app/src/main/res/raw/prod_ca.crt`
2. `network_security_config.xml` → `android/app/src/main/res/xml/network_security_config.xml`
3. 给 `AndroidManifest.xml` 的 `<application>` 注入 `android:networkSecurityConfig="@xml/network_security_config"`（已存在则跳过，不会重复）

## 真机验证步骤

采用 **A/B 对照 + CDP 探针**：不改任何应用源码，通过 Chrome DevTools Protocol 在真机 WebView 里执行一次 `fetch`。
这样验证结果只反映「打包进 APK 的网络安全配置」，不会掺入为测试而写的代码。

前置：`adb devices` 能看到设备；WebView 调试仅在 **debug 构建**开启。

```powershell
# 推荐：一键跑完整轮次（配置 + 重建 + 装机 + 探针 + logcat）
powershell -NoProfile -ExecutionPolicy Bypass -File _ab_cert_round.ps1 -Mode positive
powershell -NoProfile -ExecutionPolicy Bypass -File _ab_cert_round.ps1 -Mode negative

# 或手工分步（注意：gradlew.bat 在本机会静默挂死，见下方说明）
node android-config/apply.mjs               # A：带 domain-config
node android-config/apply.mjs --negative    # B：去掉 domain-config
# 构建用下方「完整构建命令」里的绝对路径 gradle.bat
node _android_cert_probe.cjs --label=positive
node _android_cert_probe.cjs --label=negative --expect-fail
```

每组产物落在 `_android_cert_probe_<label>.json`。

| 组 | 打包的配置 | 期望 `fetch` 结果 |
|---|---|---|
| A（positive） | 含 `domain-config` 指向 `@raw/prod_ca` | `tls_ok=true`，`status=200`（探测 `/healthz`） |
| B（negative） | 仅 `base-config`，无 `domain-config` | `tls_ok=false`，`message` 含 `Trust anchor for certification path not found` |

**通过判据 = A 成功 且 B 失败。** 只跑 A 不算验证——那可能是假的绿色。

### ✅ 实测结果（2026-09-19，Redmi K60 至尊版 / Android 16 / SDK 36）

| 组 | 实测证据 | 判定 |
|---|---|---|
| A（positive） | `Network.responseReceived` = **200 `application/json`**；`securityDetails` = **TLS 1.3 / issuer = subjectName = `175.27.135.91` / SAN = `[localhost, 175.27.135.91]`**；logcat 证书错误 **0 条** | ✅ 通过 |
| B（negative） | 无 `responseReceived`；logcat **`handshake failed; returned -1, SSL error code 1, net_error -202` ×2**（`-202` = `ERR_CERT_AUTHORITY_INVALID`） | ✅ 如预期失败 |

A 组 `issuer == subjectName == 175.27.135.91` 是自签证书的指纹，证明握手用上的正是我们预置的那张 CA；两次 fetch 对应两条 `net_error -202`，排除了单次竞态的偶然性。

### 🔴 两个必踩的坑（不要被它们骗到）

1. **`net::ERR_ABORTED` 是假线索 —— 正、负两组都会出现。**
   探针用 `mode:'no-cors'`，opaque 响应的 body 会被丢弃，Chromium 于是在 `responseReceived` **之后**再补一条 `loadingFailed ERR_ABORTED / canceled:true`。**不能拿它当「失败」判据**（否则 A 组会被误判为失败）。

2. **Android WebView 拿不到 `ERR_CERT_*`，证书错误码只能从 logcat 取。**
   证书错误走宿主 `onReceivedSslError`，Capacitor 的 WebViewClient 默认 `handler.cancel()`，渲染器只看到「请求被取消」；`Security.securityStateChanged` 在失败场景**也不发**（实测 `security: []`）。所以上表 B 组期望的 `Trust anchor for certification path not found` **在 CDP 里取不到**。正确做法：

   ```powershell
   adb logcat -c                                     # 必须先清，否则会捞到旧记录
   node _android_cert_probe.cjs --label=negative --expect-fail
   adb logcat -d | Select-String "net_error|handshake failed|SslError|cert_verify"
   ```

### ⚠️ 装机坑（小米 HyperOS）

`adb install` 会报 `INSTALL_FAILED_USER_RESTRICTED: Install canceled by user`，**并且会把已装的旧包一并清掉**（`pm path` 变空 ⇒ 探针报 `Activity class ... does not exist`，看着像代码问题其实只是没装上）。可靠通路：

```powershell
adb push <apk> /data/local/tmp/tm.apk
adb shell pm install -r -t /data/local/tmp/tm.apk   # 无需手机点确认
adb shell pm path com.trustmesh.mobile              # 每轮装机后必须复核
```

### 一键轮次脚本

`_ab_cert_round.ps1 -Mode positive|negative`：配置 → 重建 → 装机 → 探针 → logcat 全自动，日志落 `_ab_<mode>.txt`。

### 探针为什么用 `/healthz`

实测（2026-09-19，经 nginx 网关）：

| 路径 | 状态 |
|---|---|
| `/healthz` | **200**（24 B JSON） |
| `/api/v1/healthz` | 404（不存在，别用） |
| `/api/v1/organizations` | 401（需鉴权，不适合做连通性探针） |

关键点：**探针只看「TLS 是否握手成功」**。TLS 失败时 `fetch` 直接抛异常；TLS 成功则任何状态码（含 401/404）都会正常返回。所以 200 是干净判据，非 200 也仍能证明 TLS 通了。

### 完整构建命令（本机实测可用）

```powershell
$env:JAVA_HOME    = "C:\Users\38643\.jdks\jbr-21.0.11"                      # 必须 JDK 21
$env:ANDROID_HOME = "$env:LOCALAPPDATA\Android\Sdk"
Set-Location frontend-v2
npm run mobile:build          # tsc + vite build -> dist-android（base=./）
npx cap add android           # 仅首次
node android-config/apply.mjs
Set-Location android; & "C:\Users\38643\.gradle\wrapper\dists\gradle-8.14.3-all\cbf6zifq8xavouihta8md72jo\gradle-8.14.3\bin\gradle.bat" --init-script "D:\AIWorkspace\TrustMesh\_gradle_cn_mirror.init.gradle" assembleDebug
```

⚠️ **不要用 Android Studio 自带的 JBR**（本机实测为 **JDK 25**），对 AGP 8.13 过新；用 `C:\Users\38643\.jdks\jbr-21.0.11`。

🔴 **`gradlew.bat` 在本机会静默挂死，不要用**：`~/.gradle/wrapper/dists/gradle-8.14.3-all/` 下有两个哈希目录 —— `cbf6zifq8xavouihta8md72jo`（完整解压）与 `10utluxaxniiv4wxiphsi49nj`（只有 `.zip.part` 还在长）。哈希由 `distributionUrl` 推导 ⇒ 旧目录是**当年用镜像源**下的，而 Capacitor 模板写的是官方 `services.gradle.org` ⇒ 缓存未命中、回退官方源龟速下载 ⇒ **wrapper JVM 起来了但 Gradle daemon 永不启动、`android/.gradle` 不生成、CPU≈0**。判断「是否真在编译」看有没有**几百 MB 的 daemon JVM**。

## 环境清单（本机已确认，2026-09-19）

| 组件 | 路径 / 状态 |
|---|---|
| Android SDK | `C:\Users\38643\AppData\Local\Android\Sdk` |
| platforms | `android-34`、`android-36` |
| build-tools | `35.0.0`、`36.0.0` |
| platform-tools | `adb` 1.0.41（37.0.1） |
| emulator | 已安装（`emulator/emulator.exe`） |
| licenses | `android-sdk-license` 已接受 |
| JDK | ⚠️ Android Studio 自带 JBR = **JDK 25**（对 AGP 8.13 过新，**不要用**）；应使用 `C:\Users\38643\.jdks\jbr-21.0.11`（JDK 21） |
| Gradle / AGP | Gradle **8.14.3**、AGP **8.13.0**（均已在 `~/.gradle` 缓存中） |
| ⚠️ 缺口 | `cmdline-tools` / `tools` / `ndk` **缺失**（不影响 Gradle 构建） |
| 测试设备 | ✅ Redmi K60 至尊版（`23116PN5BC` / codename `shennong`），**Android 16 (SDK 36)**，arm64-v8a，adb 已授权 |

## ⚠️ 维护注意

- 服务器一旦**重新签发证书**，`prod_ca.crt` 与 SHA-256 指纹都会变，必须同步更新本目录，否则 App 直接连不上。
- 未来面向**外部用户公开分发**时，此方案不适用：必须换成正式域名 + 受信任证书，并把 `network_security_config.xml` 中该 domain 的信任锚改回 `src="system"`。
- 该 domain 下**刻意不保留 `system`** 信任锚（true pinning），避免被公共 CA 中间人；这是有意的耦合，改动前请确认。
