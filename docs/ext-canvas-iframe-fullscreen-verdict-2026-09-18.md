# 外部应用「无限画布（画宗）」iframe 全屏能力 —— 实测判决（2026-09-18）

> 复现脚手架：`_fs_test/`（Electron 35.7.5 / Chromium；父页 `file://`，子页 `https://127.0.0.1:9443`，跨源）。
> 关联：`docs/ext-canvas-iframe-embedding-verdict-2026-09-18.md`（能否嵌进来的那道坎）。

## 1 结论

**可以实现。** 而且分两条互不依赖的路线，**其中一条不需要画宗配合**。

| 路线 | 谁发起 | 现状 |
|---|---|---|
| **A** 外部应用自己的全屏按钮生效 | 画宗页面内部调 `requestFullscreen()` | 权限**已就绪**（`ExternalAppFrame.tsx` 已有 `allow="...fullscreen"`），但取决于画宗是否真有此按钮 |
| **B** TrustMesh 工具栏加「全屏」 | TrustMesh 父页对**自己的容器节点**调 `requestFullscreen()` | ✅ **已实现**，零外部依赖、零权限依赖 |

## 2 权限门槛矩阵（唯一变量 = iframe 属性）

父页 `file://`，子页同源 https 自签但**跨源**于父页；逐个在子框架内调用 `document.documentElement.requestFullscreen()`。

| 用例 | iframe 属性 | 结果 |
|---|---|---|
| V1 | `sandbox="allow-scripts allow-same-origin …"` + `allow="…; fullscreen"`（**= 现状**） | ✅ `OK`，`fullscreenElement = HTML` |
| V2 | V1 再补 `allowfullscreen` 属性 | ✅ `OK` |
| V3 | 仅 `allow="…; fullscreen"`（无 sandbox） | ✅ `OK` |
| V4 | 仅 `sandbox`（**无** `allow`） | ❌ `TypeError: Disallowed by permissions policy` |

⇒ 真正的开关是 **Permissions Policy（`allow="fullscreen"`）**：
- **`sandbox` 不阻断全屏**（V1 通过）——不要为了全屏去摘 `sandbox`，那会白白降低隔离性。
- `allowfullscreen` 属性在已有 `allow="fullscreen"` 时**冗余**（V1 ≡ V2）。

## 3 TrustMesh 侧自行加按钮（路线 B）

父页对 iframe 元素调用：`document.querySelector('#v1').requestFullscreen()`

- 实测 ✅ `OK`，`document.fullscreenElement.tagName === "IFRAME"` ⇒ 画宗**整个界面**铺满屏幕。
- **Electron 窗口会真正进入系统级全屏**：

| 时刻 | `isFullScreen` | `isSimpleFullScreen` | bounds |
|---|---|---|---|
| 初始 | false | false | 1101×801 @(169,56) |
| 全屏中 | **true** | **true** | 1441×961 @(0,0) |
| 退出后 | false | false | 1101×801 @(169,56) |

⇒ **不需要**在 `frontend-v2/electron/main.cjs` 里补 `enter-html-full-screen` → `win.setFullScreen(true)` 的接线，Electron 已自动跟随（`enter-html-full-screen`/`leave-html-full-screen` 事件照常可用于更新 UI 状态）。任务栏是否被盖住取决于系统全屏语义。

### 3.1 全屏「哪个节点」——实现选型（重要）

父页有两种可选目标，差别在**是否需要跨域权限**：

| 目标 | 视觉 | 权限依赖 | 退出入口 |
|---|---|---|---|
| iframe 元素 | 只剩外部应用，最大画布 | 父页持有该元素，**不需要**站点授权 | 只有 Esc |
| **容器 div（已采用）** | 工具栏保留在顶部 | 同上，**不需要** | 有可见的「退出全屏」按钮 |

**采用容器**：容器是本页自己的节点，`requestFullscreen()` 只走父页自身，**连 `allow="fullscreen"` 都不依赖**（该属性只影响 iframe 内部自己发起的全屏请求，即路线 A）。这带来两个好处：

1. **零外部依赖** —— 即使画宗完全不配合，全屏也能用；
2. **有可见的退出入口** —— 全屏元素在 top layer，`z-index` 无法在其上叠加任何自家 UI，所以若只全屏 iframe 就只能靠 Esc；全屏容器则把工具栏一起带进全屏。

代价是全屏时顶部多一条工具栏（约 32px）。若日后要极致沉浸，可改为只全屏 iframe 并接受「只能 Esc 退出」。

## 4 硬约束（会咬人的地方）

- 🔴 **必须在用户手势的处理函数里同步调用**。放到 `await` / `setTimeout` / `fetch().then()` / `postMessage` 回调之后都会失败，报 `NotAllowedError: Permission denied`（Chrome 文案：`API can only be initiated by a user gesture`）。Chrome / Firefox / Safari 行为一致。有效手势：`click`、`dblclick`、`pointerup`、`submit` 等；`mousemove` / `mouseenter` 不算。
- ⚠️ **本判决的一个方法论边界**：Electron 的 `webContents.executeJavaScript` 等价于 DevTools 控制台——**执行会被当作用户手势**。因此"无手势也成功"这类结果在注入式脚本里测不出来（本次一度得出"无需手势"的假象，已由文档查证推翻）。§2 的权限矩阵不受此影响：四组用例在**同一注入上下文**下横向对比，唯一变量是属性，结论有效。

## 5 实施记录（路线 B）

已落地于 `frontend-v2/src/components/external/ExternalAppFrame.tsx`（+63/−5）：

1. 工具栏加「全屏 / 退出全屏」按钮，`onClick` 里**同步**调 `stageRef.current.requestFullscreen()` —— 前面没有任何 `await`。
2. `stageRef` 绑在**容器 div**（包住工具栏 + 提示 + iframe），并对容器加 `background: var(--surface)`，避免全屏时出现 UA 默认底色的突兀观感；全屏时补 `padding: 10`。
3. 监听 `document` 的 `fullscreenchange`，以 `isElementFullscreen(stageRef.current, document.fullscreenElement)` 判定状态 ⇒ **按 Esc、或外部应用自身退出全屏时按钮状态能同步回来**。注意这里**不能**直接写 `document.fullscreenElement === stageRef.current`，原因见 §7。
  3b. 另用**回调 ref** 在容器真正 attach 时再同步一次：首次渲染时凭证未就绪，组件走的是 `<Spin>` 分支，容器根本没挂上去，只靠挂载期 effect 会漏判（§7）。
4. 请求被拒时用 `App.useApp()` 的 `message.error` 提示（遵循本仓约定，**不用** antd 静态 `message`）。注意全屏元素在 top layer，普通浮层盖不上去，所以该提示只在**未进入全屏**时可见——这正是需要它的时候。
5. 全屏时隐藏那条「可能空白」的蓝色提示条，避免在沉浸态占位。
6. `allow="clipboard-read; clipboard-write; fullscreen"` 与 `sandbox` **原样未动**（全屏不依赖它们；保留是为了支持路线 A）。

门禁：`tsc -p tsconfig.app.json --noEmit` 0 错误；`eslint . --max-warnings=4` 0 错误 4 警告（4 条均为他处历史告警，本次改动零告警）；`vitest run` 127/127 通过。

> 备选（未采用）：纯 CSS 伪全屏（`position:fixed; inset:0`）不触发 Fullscreen API、不看手势脸色，但盖不住系统任务栏，也会被自家浮层遮挡。

## 6 前置依赖（重要）

以上都成立，**但桌面端目前 iframe 根本渲染不出来**——`file://` 顶页 + 画宗 `frame-ancestors *` 导致 `ERR_BLOCKED_BY_RESPONSE`（见 `ext-canvas-iframe-embedding-verdict-2026-09-18.md`）。**全屏是「嵌进来」之后才有意义的能力**，排期上应放在 CSP 那道坎之后。

## 7 回归与修复：首次进入显示「退出全屏」（2026-09-18 上线后）

**症状**（用户上报）：第一次进入外部应用，按钮显示的是「退出全屏」；应当默认是「全屏」，只有全屏态才是「退出全屏」。

**先排除的假设**：按钮文案的三元式 `isFullscreen ? '退出全屏' : '全屏'` **一直是对的**，没有写反。图标同理。所以这不是文案问题。

**真因：`null === null` 陷阱。**

组件在拿到 `launch_url` 之前返回的是 `<Spin>` 分支（`!launchUrl` 提前 return），**容器 `<div ref={stageRef}>` 尚未渲染**。于是挂载期 effect 跑 `onChange()` 时：

```
document.fullscreenElement === stageRef.current
        null                 ===       null          → true   ❌
```

两边同为 `null`，判等成立 ⇒ `isFullscreen` 被置为 `true` ⇒ 首次进入就显示「退出全屏」，并连带把 `padding` 设成 10、把提示条藏掉。按钮文案没错，错的是**这个初始判定**。

**修复**（提交 `b2d50a3`，改动 3 个文件）：

1. 抽出纯函数 `src/lib/fullscreen.ts`：

   ```ts
   export function isElementFullscreen(container, fullscreenElement) {
     return container !== null && fullscreenElement === container
   }
   ```

   **先把左侧的 `null` 挡掉**再做判等。组件三处判定（状态同步、按钮切换、`toggleFullscreen`）统一走它，避免逻辑分叉。

2. 改用**回调 ref**（`attachStage`）：`mount` effect 只跑一次，而那一刻容器还不存在（Spin 分支），单靠它永远只能得到「容器为 `null`」这一个结论；在节点真正 attach 时再判定，状态才与 DOM 一致。（该 ref 还顺带覆盖了「组件卸载重挂、浏览器仍在全屏」的边缘场景。）

**测试**：`src/lib/fullscreen.test.ts`，8 条用例。本仓 `vitest.config.ts` 明确不引入 `@testing-library`（测不变量而非像素），故采取「纯函数单测 + 源码级契约」组合：

- 首条用例直接复现 `null === null` 场景；
- 另含契约断言，禁止再把裸比较写回组件；
- ⚠️ 源码文本用 Vite 的 `?raw` 导入而非 `node:fs`——`tsconfig.app.json` 的 `types` 只有 `vite/client`，**没有 node 类型**，用 `node:fs` / `process` 会让 `tsc` 直接报 TS2307/TS2591。

**变异验证**（证明该护栏不是恒真断言）：临时把守卫去掉（`return fullscreenElement === container`）后重跑，恰好那一条用例变红 `expected true to be false`，其余 7 条仍绿；恢复后 135/135 全绿。

**门禁**：`tsc` 0 错误；`eslint --max-warnings=4` 0 错误 4 警告（全为他处历史告警，本次零新增）；`vitest run` 135/135（原 127 + 新增 8）。
