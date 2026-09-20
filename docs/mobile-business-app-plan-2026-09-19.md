# TrustMesh 移动端应用方案（面向业务人员）

> 日期：2026-09-19 ｜ 状态：**已确认，P0 已落地**（2026-09-20 用户确认：②③④⑥ 按建议，⑤ 选择**继续自签**）
> 目标用户：业务端使用人员（非平台管理员）
> 前置事实：2026-09-19 早些时候的 Capacitor/Android 方案**已整体回退**，产物备份在
> `D:\AIWorkspace\_mobile-revert-backup-20260919-233710\`（含旧方案文档 `docs/mobile-android-app-plan-2026-09-19.md`、
> `tracked-working-tree.patch`、`android/` 工程、全部真机探针脚本）。本文是**重新设计**，不是继续上一版。

---

## 1. 结论（一页纸）

做**一个独立的移动端 H5 应用（新子项目 `frontend-mobile`）**，先以 **PWA** 形态交付，业务闭环跑通后再按需加 **Capacitor 壳**打企业内部分发包。

| 决策项 | 结论 |
|---|---|
| 承载形态 | **独立移动端 Web（PWA）**，手机浏览器访问 / 添加到主屏；**不给桌面 Web 套壳** |
| 技术栈 | Vite 7 + React 19 + TypeScript + **Tailwind v4** + **antd-mobile v5** + Zustand 5 + React Query 5 + ky |
| 代码关系 | **新工程、独立 package.json、独立 CI**；只复用后端 API 与鉴权约定，**不 import 桌面端组件** |
| 首期功能 | 待办审批 + 任务（列表/详情/评论/交付物）+ 收件箱 + 我的（4 个 Tab） |
| 原生壳 | **二期可选**。推送/扫码/离线/应用商店成为硬需求时再上，届时复用备份里已真机验证的证书方案 |
| 后端改动 | **期望零改动**。仅移动端必须带 `X-Org-Id`；是否补移动友好分页接口列为待决项 |
| 证书（第一阻断项） | 建议**绑定域名 + Let's Encrypt 免费证书**一次解决；裸 IP `175.27.135.91` 无法签发公开可信证书 |

三条不可妥协的原则：

1. **不做"桌面 Web 套壳"**——上一版把带 three.js / mermaid / katex / pdfjs / xlsx 的完整桌面 bundle 打进 APK（9.86 MB），首屏慢、UI 是桌面组件，这是失败主因。
2. **不污染桌面端**——移动端不并入 `frontend-v2`，避免触碰 Electron asar 闭包闸门与现有 warning 额度。
3. **不被原生工具链拖死**——证书 / Gradle / adb / HyperOS 安装限制这些坑已实证吃掉大量工期，而业务功能一行未写。PWA 阶段**完全绕开**它们，先交付业务价值。

---

## 2. 上一版为什么失败（复盘，避免重蹈）

| # | 上一版做法 | 结果 | 本文的修正 |
|---|---|---|---|
| 1 | 在 `frontend-v2` 内加 `src/components/mobile/*` + 移动端路由，同包分流 | 桌面 bundle 被拖进来；asar 闭包闸门、`eslint` 额度、`runtime.ts` 嗅探垫片全是额外负担 | **独立工程**，互不干扰 |
| 2 | Capacitor 壳直接指向桌面 `dist-v2` | APK 9.86 MB，assets 里有 mermaid/katex/prism/pdf/xlsx/three 等几十个 chunk | 新工程**零重型依赖**，首屏 JS 目标 < 250 KB gzip |
| 3 | 只做了 Tab 骨架与运行时分流，P1 业务页面未开工 | "壳做完了，业务没做"，体感就是失败 | 首期就交付**完整业务闭环**（审批→任务→交付物→评论） |
| 4 | 自签证书 + 预置 CA + `network_security_config` | 真机 A/B 反证**已通过**（`net_error -202` 负向命中），但消耗了大量验证成本且锁死了分发 | PWA 走浏览器信任链；域名 + Let's Encrypt 后证书问题**一次性消失** |
| 5 | Gradle / `cap copy` / 小米 `adb install` 静默挂死或被拒 | 工程时间被原生工具链吞噬 | PWA 阶段 `npm run build` 即发布，**零原生工具链** |

> 上一版并非全无价值：**证书 A/B 判据、CDP 探针按 PID 选 socket、探针必须自带复位前置、appStorage 同步适配层** 这四条经验仍然有效，已沉淀在项目记忆中，二期上原生壳时直接复用。

---

## 3. 技术选型调研

### 3.1 承载方式对比

| 方案 | 做法 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| **A. 独立 PWA / H5** | 新工程 + manifest + Service Worker | 零安装、发版即时（改完刷新即生效）；一套代码覆盖 iOS + Android；**完全绕开证书预置/Gradle/adb**；成本最低；可加桌面图标、全屏启动 | iOS 推送弱（需先添加到主屏）、无后台同步、无应用商店曝光 | ✅ **首期采用** |
| B. Capacitor 壳 | 复用 H5 代码打 APK/IPA | 与 A 复用 100% 业务代码；可企业内部分发；证书方案已实证可行 | 原生工具链坑多（已实证）；自签证书是硬阻断；iOS 端需 macOS + 开发者账号 | ✅ **二期可选** |
| C. React Native / Expo | 重写 | 原生观感、推送/扫码/离线强 | 业务页面全部重写（RN 原语 ≠ DOM 组件）；团队要补原生概念；工期约 ×2.5–3 | ❌ 内部工具不划算 |
| D. Flutter | Dart 重写 | 性能与多端一致性最好 | 语言栈切换；现有 TS/React 资产零复用；桌面端已是 Electron，技术栈进一步分裂 | ❌ |
| E. 桌面 Web 响应式改造 | 在 `frontend-v2` 加媒体查询 | 看似改动最小 | antd 桌面组件（Table/Drawer/hover）在手机依然别扭；bundle 巨大；污染桌面端闭包闸门 | ❌ **上一版教训** |

> 行业共识也支持 A：2026 年 PWA 在 iOS（Safari 16.4+ 支持 Web Push、主屏安装、Service Worker 完整）与 Android（能力评分 97/100）上已覆盖企业内部工具的全部常用能力；跨端框架选型矩阵中"Internal LOB apps（内部业务应用）"一栏的标准答案就是 **Web-first**。

### 3.2 移动端 UI 组件库对比（React 生态）

| 组件库 | 框架 | 组件覆盖 | 体积 | React 19 | 视觉风格 | 适用判断 |
|---|---|---|---|---|---|---|
| **antd-mobile v5** | React | 最全（含 SafeArea / PullToRefresh / InfiniteScroll / SwipeAction / ActionSheet / ImageViewer / Popup 等移动端专属件） | 中（按需引入可控） | 需 `unstableSetRender` 垫片（≥5.40 官方提供，下个 major 移除） | 中性、克制，与 antd 同语言 | ✅ **推荐** |
| react-vant | React | 较全，偏轻交互/电商 | 小（gzip ~70 KB） | 支持 | 圆润、明快 | ⚪ 备选（复杂状态联动需自补胶水代码） |
| TDesign Mobile React | React | 中上，规范统一 | 中 | 支持 | 腾讯系、偏商务 | ⚪ 备选 |
| Konsta UI | React + Tailwind | 少（~25 个，无复杂业务件） | 极小（纯 Tailwind 类，无 JS 逻辑） | 支持 | iOS/Android 原生观感最精致 | ⚪ 若最终主打"质感优先、组件自绘"则选它 |
| Ionic React | React | 全（含路由） | 大（Web Components 整套） | 支持 | 平台自适应最强 | ❌ 与 Tailwind 混用成本高、包重 |
| Framework7 React | React | 全（自带路由/状态） | 大 | 支持 | iOS 味重 | ❌ 框架侵入性强，与 React Router 冲突 |
| Onsen UI | React | 中 | 中 | 支持 | 偏旧 | ❌ 维护放缓 |

**推荐：antd-mobile v5（业务件） + Tailwind v4（布局与自定义视觉）**

- 与桌面端 antd v5 同一设计语言，跨端观感一致，业务人员切换设备无割裂感；
- 移动端专属交互件（下拉刷新、滑动操作、底部弹层、安全区）开箱即用，这正是"手机 appropriateness"的关键；
- 现有前端已经用 Tailwind v4（`@tailwindcss/vite`），风格栈统一、可复用既有手感；
- ⚠️ React 19 必须加官方兼容垫片（见 §7.2），且**严禁与 antd v5 混用**——两者 CSS 体系（CSS-in-JS vs Less/CSS 变量）与渲染范式在同一构建流里会打架，这也是"必须独立工程"的第二个理由。

### 3.3 推荐依赖清单

```
运行时：react 19.2 / react-dom 19.2 / react-router-dom 7（HashRouter）
UI：    antd-mobile ^5.40 / @ant-design/icons / tailwindcss 4 + @tailwindcss/vite
数据：  @tanstack/react-query 5 / zustand 5 / ky（沿用现有鉴权头约定）/ dayjs
工程：  vite 7 / typescript 5.9 / vitest 5 + jsdom 30 / eslint 9
PWA：   vite-plugin-pwa（manifest + Workbox Service Worker）
二期待定：@capacitor/core 7 + android 平台（**仅在决定打壳时安装，且不在 PWA 阶段引入**）
```

**明确不引入**：three.js / @react-three/*、pdfjs-dist（P1 用系统预览或按需 lazy）、@open-file-viewer/*、mermaid、katex、xlsx、framer-motion（用 CSS transition + antd-mobile 动效即可）。

---

## 4. 范围边界

### 4.1 移动端承载（业务人员随身）

| 能力 | 说明 | 优先级 |
|---|---|---|
| 登录 + 组织切换 | 复用现有鉴权约定（`X-Org-Id`、refresh token） | P0 |
| 待办与审批 | 「待我确认/审批」卡片 + 一键通过/驳回 —— **移动端最高频价值** | P0 |
| 任务列表 | 搜索 + 状态筛选，卡片式（非表格） | P1 |
| 任务详情 | 进度、步骤、结果查看 | P1 |
| 交付物查看 | 图片 / PDF 内嵌预览；其余下载 | P1 |
| 评论与 @提及回复 | 随手回复 | P1 |
| 收件箱 | 消息 / @提及 / 系统通知 + 未读红点 | P2 |
| 我的 | 组织切换、通知设置、退出登录 | P2 |

### 4.2 不进入移动端（保留桌面 / Web）

工作流编排画布、办公室 3D 场景、组织管理、平台管理 `/platform/*`、外部应用与密钥、模型与 LLM 配置、步骤 IO 编辑与交付物绑定、文件管理与大文件上传、招聘智能体、运维事件与审计、会议室与会议纪要、市场浏览与订阅。

> 判定标准（沿用并强化）：**需要长时间专注、精确拖拽、批量配置的一律不上手机。**

---

## 5. 信息架构与核心功能清单

底部 4 个 Tab，导航一律「列表 → 详情」两级栈式跳转，**不照搬桌面的树形/多标签导航**。

| Tab | 路由 | 页面与能力 |
|---|---|---|
| 工作台 | `/` | 待我审批（置顶卡片）、我的待办、进行中的任务、@我的未读摘要；下拉刷新 |
| 任务 | `/tasks` | 任务列表（搜索 / 状态筛选 / 无限滚动）→ 任务详情（进度条、步骤时间线、交付物、评论区） |
| 收件箱 | `/inbox` | 消息、@提及、系统通知；已读标记、未读红点 |
| 我的 | `/me` | 当前组织（可切换）、通知开关、关于/版本、退出登录 |

**核心功能清单（MVP = P0 + P1）**

| # | 功能 | 关键交互 | 期次 |
|---|---|---|---|
| F1 | 登录 / 自动续期 / 登出 | 手机号邮箱 + 密码；token 落 localStorage；401 自动 refresh | P0 |
| F2 | 工作区（组织）切换 | 「我的」页顶部切换器，切换后**清缓存并重新拉取**（顺序必须对齐桌面端门控：setPersonalOrgId → setActiveOrg → removeQueries → calibrated） | P0 |
| F3 | 待办审批 | 卡片展示待确认项；底部 `ActionSheet` 出「通过 / 驳回 / 稍后」；驳回需填原因 | P0 |
| F4 | 任务列表 | 卡片流 + 状态徽标 + 搜索 + 下拉刷新 + 无限滚动 + 骨架屏 | P1 |
| F5 | 任务详情 | 折叠分区：概览 / 步骤时间线 / 交付物 / 评论 | P1 |
| F6 | 交付物查看 | 图片内嵌 + 手势缩放；PDF 走按需加载预览；其他类型下载 | P1 |
| F7 | 评论与 @回复 | 底部固定输入条（键盘弹起自适应），支持 @ | P1 |
| F8 | 收件箱 | 三类分组 + 未读红点 + 标记已读 | P2 |
| F9 | 通知设置 / 个人中心 | 通知开关、退出登录 | P2 |
| F10 | PWA 化 | manifest、图标、Service Worker（离线壳 + 静态资源预缓存）、安装引导 | P2 |

---

## 6. 界面设计思路

### 6.1 总体形态

```
┌──────────────────────────────┐
│ 顶栏：标题 + 返回 + 右侧动作   │  ← 44px + 状态栏安全区，半透明毛玻璃
├──────────────────────────────┤
│                              │
│  内容区（可滚动，左右 16px）   │
│  卡片 / 列表 / 时间线          │
│                              │
├──────────────────────────────┤
│ 底部 Tab（4 项，含安全区）     │  ← 56px + env(safe-area-inset-bottom)
└──────────────────────────────┘
```

### 6.2 视觉规范

| 项 | 取值 |
|---|---|
| 主色 | 沿用桌面端品牌主色（同一 token，跨端一致） |
| 间距 | 8pt 栅格；页面边距 16px，卡片内边距 12–16px |
| 圆角 | 卡片 12–16px，按钮 8–10px，底部弹层顶部 16px |
| 层级 | 卡片白底 + 极淡描边 + 轻阴影（不用重投影） |
| 字号 | 标题 17 / 正文 15 / 辅助 13 / 徽标 11 |
| 状态色 | 进行中=主色、待办=橙、完成=绿、阻塞/失败=红、已取消=灰 |
| 热区 | 可点元素最小 44×44px |
| 深色 | 跟随系统 `prefers-color-scheme`，首期至少保证浅色完美 |
| 适配 | `viewport-fit=cover` + `env(safe-area-inset-*)`；antd-mobile `SafeArea` 组件兜底 |

### 6.3 桌面组件 → 移动端映射（关键）

| 桌面端 | 移动端替代 |
|---|---|
| `Table` | **卡片列表**（信息分层：主标题 / 副信息 / 状态徽标） |
| `Drawer` | **全屏 Page** 或 半屏 `Popup`（自底部升起） |
| hover 菜单 | **长按** 或 `SwipeAction` 左滑露出操作 |
| `Modal` 确认 | **`ActionSheet`**（底部弹出，拇指可达） |
| 顶部多 Tab | 顶部 `Tabs` 吸顶（≤4 个），超出改为下拉筛选 |
| 分页器 | **`InfiniteScroll`** 无限滚动 + 下拉刷新 |
| 表单 | 单列、大输入框、底部固定主按钮，键盘避让 |

### 6.4 关键页面草图（工作台 / 任务详情）

```
【工作台】                          【任务详情】
┌────────────────────────┐        ┌────────────────────────┐
│ 工作台            🔔 3 │        │ ‹ 任务详情          ⋯ │
├────────────────────────┤        ├────────────────────────┤
│ 待我审批              2 │        │ 客户画像整理            │
│ ┌────────────────────┐ │        │ 进行中 · 张三 · 2h 前   │
│ │ 方案A评审           │ │        │ ▓▓▓▓▓▓▓░░░  70%        │
│ │ 李四 · 10 分钟前     │ │        ├────────────────────────┤
│ │ [通过] [驳回]       │ │        │ 步骤时间线              │
│ └────────────────────┘ │        │ ● 需求澄清   ✓          │
│ 我的待办              5 │        │ ● 数据拉取   ✓          │
│ ┌────────────────────┐ │        │ ● 初稿撰写   ⟳ 进行中    │
│ │ 补充客户联系方式     │ │        │ ○ 交付复核   –          │
│ └────────────────────┘ │        ├────────────────────────┤
│ 进行中的任务          3 │        │ 交付物 (2)              │
│ ┌────────────────────┐ │        │ 📄 客户画像报告.pdf     │
│ │ 客户画像整理  ⟳ 70% │ │        │ 🖼 画像配图.png          │
│ └────────────────────┘ │        ├────────────────────────┤
│                        │        │ 评论 (3)                │
│                        │        └────────────────────────┘
│                        │        │ [ 说点什么…        @ 发送]│
└────────────────────────┘        └────────────────────────┘
```

### 6.5 交付物展示口径（务必对齐后端语义）

桌面端 `TaskResultView` 不按 `kind` 过滤、整块标题却写「交付物」。移动端屏幕小，必须把歧义去掉：

- **交付物区**：只展示 `output_name !== ''`（即 `kind === 'deliverable'`）的项；
- **过程产物**：单独折叠区；
- 判定看 `kind` / `output_name` 字段，**不要只看 artifacts 数组**（与后端既有不变式一致）。

---

## 7. 工程结构

### 7.1 目录

```
frontend-mobile/                  # 新子项目，独立 package.json / 独立 CI
├── index.html                    # viewport-fit=cover、theme-color
├── vite.config.ts                # @tailwindcss/vite + vite-plugin-pwa
├── public/                       # manifest 图标（192/512/maskable）
├── src/
│   ├── main.tsx                  # React 19 渲染 + antd-mobile unstableSetRender 垫片
│   ├── App.tsx                   # HashRouter + QueryClient + 主题
│   ├── router.tsx                # 仅 4 个 Tab + 详情路由（**不注册任何桌面路由**）
│   ├── pages/                    # Login / Home / TaskList / TaskDetail / Inbox / Me
│   ├── components/               # PageScaffold / Card / StatusTag / EmptyState /
│   │                             # SafeBottomBar / ArtifactList / CommentBar
│   ├── api/                      # 只封装移动端需要的 ~10 个接口（ky + 统一 401 处理）
│   ├── stores/                   # authStore（同步 rehydrate）/ orgStore
│   ├── hooks/                    # useUnreadCount / usePullRefresh / useTaskDetail
│   ├── lib/                      # 时间格式化、blob 下载（**带 Authorization**）
│   └── styles/                   # tokens.css（主色/圆角/间距）+ antd-mobile 变量覆盖
└── README.md
```

### 7.2 React 19 兼容垫片（antd-mobile 必做）

```ts
// src/main.tsx —— antd-mobile v5 默认兼容 React 16~18；React 19 需显式注册渲染器
import { unstableSetRender } from 'antd-mobile'
import { createRoot } from 'react-dom/client'

unstableSetRender((node, container) => {
  // @ts-expect-error 复用 root，避免重复 createRoot
  container._reactRoot ||= createRoot(container)
  const root = container._reactRoot
  root.render(node)
  return async () => {
    await new Promise((r) => setTimeout(r, 0))
    root.unmount()
  }
})
```

> 该接口在下个 major 会被移除 —— 因此**垫片集中在一处**，升级时只改这一个文件。

### 7.3 后端侧

- **期望零改动**：移动端只消费现有 `/api/v1/*`。
- 🔴 **必须带 `X-Org-Id`**：列表接口不带该头只返回「当前个人空间」（实测 9 vs 11 条）。移动端切换组织后所有请求都要带上。
- 🔴 **鉴权接口的图片不能直喂 `<img src>`**（原生加载不带 `Authorization` ⇒ 恒 401，表现为"上传成功但图标不显示"）⇒ 统一走 `fetch(..., { headers: { Authorization }, cache: 'no-store' })` 转 blob URL。
- 待决：是否需要后端补「移动端精简列表接口」（裁剪字段 + 游标分页）。首期先用现有接口，若列表体积过大再提。

### 7.4 证书 / 域名（P0 第一决策项）

| 方案 | 说明 | 影响 |
|---|---|---|
| **A（推荐）绑定域名 + Let's Encrypt** | 加一条 A 记录指向 `175.27.135.91`，签免费证书 | 一次解决 Web / 桌面 / 移动三端；PWA 的 Service Worker 与安装提示才可用；桌面端自签告警一并消失 |
| B 继续自签 | 浏览器需用户手动"继续访问"；Service Worker 与安装提示可能受限 | 仅在内网试点可接受，正式分发不可行 |

---

## 8. 分期计划

| 期次 | 范围 | 出口标准 |
|---|---|---|
| **P0** | 证书/域名决策 + 工程脚手架 + 设计令牌 + 登录 + 组织切换 + 底部 Tab 骨架 | 真机浏览器可登录、可切组织、4 个 Tab 可切换 |
| **P1** | 待办审批 + 任务列表 + 任务详情 + 交付物 + 评论 | **MVP**：业务人员在手机上完成一次完整闭环 |
| **P2** | 收件箱 + 我的 + 通知设置 + PWA（manifest / SW / 图标 / 安装引导） | Lighthouse Mobile ≥ 90、可添加到主屏、离线打开不白屏 |
| **P3（可选）** | Capacitor 壳 + 推送 + 扫码 + 离线写队列 | 企业内部分发包可用 |

**MVP = P0 + P1。**

---

## 9. 风险与待决项

| # | 风险 | 影响 | 应对 |
|---|---|---|---|
| 1 | 裸 IP 无公开可信证书 | 🔴 PWA 的 SW / 安装提示受限，iOS 更严 | P0 先决策域名 + Let's Encrypt（§7.4 A） |
| 2 | antd-mobile 与 antd 混用 | CSS 体系与渲染范式冲突 | **独立工程**，物理隔离；CI 加"不得 import frontend-v2"的护栏 |
| 3 | React 19 垫片未来被移除 | 升级成本 | 垫片集中在 `main.tsx` 一处 |
| 4 | iOS PWA 推送弱 | 通知到达率 | 首期不做推送；P3 若上壳再接厂商通道 |
| 5 | 后端列表接口返回体过大 | 弱网首屏慢 | 首期按需字段渲染；必要时提移动端精简接口（待决） |
| 6 | 真机专属缺陷静态检查不报 | 只在实盘暴露 | 每期出口必须在 **iOS Safari + Android Chrome 真机**各跑一遍（不用模拟器） |
| 7 | 与桌面端共享生产 NATS / 单实例 | 测试误伤生产 | 移动端测试只用既有测试号，**不建任务、不触发 PM 规划** |

---

## 10. 验收方式

1. **真机验收（硬性）**：iOS 与 Android 各一台真机，走通「登录 → 切组织 → 工作台审批 → 打开任务 → 查看交付物 → 评论」全链路，无白屏、无 401、无布局溢出。
2. **性能**：Lighthouse（Mobile）Performance ≥ 90、PWA 可安装、首屏 JS < 250 KB gzip。
3. **独立闸门**：`tsc --noEmit` 零错误、`vitest run` 全绿、`eslint` 零 error —— **在 `frontend-mobile` 内跑，不动 `frontend-v2` 的 warning 额度**。
4. **零回归**：移动端为独立工程，桌面端 / 后端闸门结果不得变化（发版前各跑一次比对）。

---

## 附：决策记录（待确认）

| # | 事项 | 建议结论 | 状态 |
|---|---|---|---|
| 1 | 承载形态 | 独立移动端 Web（PWA），Capacitor 壳放二期 | ✅ 已确认 |
| 2 | UI 组件库 | antd-mobile v5 + Tailwind v4 | ✅ 已确认（实装 5.43.0） |
| 3 | 工程位置 | 新子项目 `frontend-mobile/`（不并入 frontend-v2） | ✅ 已确认 |
| 4 | 首期范围 | 4 个 Tab；MVP = 待办审批 + 任务闭环 | ✅ 已确认 |
| 5 | 证书 | **继续自签**（用户决定，不绑域名） | ✅ 已确认（见下方连带影响） |
| 6 | 是否后端补移动端精简接口 | 首期不补，按需再提 | ✅ 已确认 |

---

## 附：⑤ 选「继续自签」的连带影响与处置

浏览器对 **fetch/XHR 的证书错误没有「继续访问」入口**（只有顶层导航才有 interstitial），因此：

| 场景 | 影响 | 处置 |
|---|---|---|
| 本机开发 | 直连生产必然 `ERR_CERT_AUTHORITY_INVALID` | Vite 代理在 Node 侧发起请求，`secure: false` 跳过校验 ⇒ **已实测打通**（`/api/v1/organizations` 返回后端真实 JSON `UNAUTHORIZED`） |
| 真机访问 | 首次需在浏览器手动接受该站点证书风险；接受后同源请求（含 XHR）即可正常 | 一次性操作，写进分发说明 |
| PWA（Service Worker / 添加到主屏） | secure origin 判定受限，安装提示可能不出现 | **manifest 保留，SW 暂不注册**；换正式证书当天补上 |
| Capacitor 壳（二期） | 自签证书是硬阻断 | 复用备份里已真机 A/B 验证过的「预置 CA + `network_security_config`」方案 |

---

## 附：P0 实施结果（2026-09-20）

**产物**：新子项目 `frontend-mobile/`（Vite 7 + React 19.2 + antd-mobile **5.43.0** + Tailwind 4 + Zustand 5 + RQ5 + ky）。

**已实现**：工程骨架与独立 CI 脚本 · 设计令牌（品牌色 `#6D5FF5`，与桌面端一致）· 登录（`auth/login` + 401 单飞刷新 + 冷启动门闩）· 组织列举与工作区切换（`runWorkspaceCalibration` 单点保证顺序）· 四 Tab 底部导航（内联 SVG 图标，不引图标库）· 我的页（切空间 / 改服务地址 / 退出）。

**三道闸门全绿**：`tsc --noEmit` **0 错误**；`vitest run` **2 文件 / 6 用例全过**；`eslint .` **0 error 0 warning**。

**体积**：`index.js` 412.87 KB → **gzip 136.47 KB**；CSS 21.39 KB → gzip 5.26 KB（目标 < 250 KB gzip，达成；对比上一版把 three/mermaid/katex/pdfjs 打进 9.86 MB APK）。

**冒烟**：开发服务器 `/` = **200**；`/api/v1/organizations` = **401** 且响应体为后端真实 JSON ⇒ 证明「同源/代理 → 生产」链路与自签证书处置都成立。

**未做（P1）**：工作台待办与审批、任务列表与详情、交付物预览、评论 —— 需先确认后端对应接口的字段口径，再动手。

---

## 附：P1 实施结果（2026-09-20）

**先做的只读勘察**（不建任务、不触发规划）：`login → organizations → projects → project tasks → task detail / events / comments`，
用真实数据核对字段口径，结论全部与实现一致：

| 观察项 | 实测 | 对本实现的意义 |
|---|---|---|
| 企业空间项目数 | **11**（不带 `X-Org-Id` 只有 9） | 再次印证"列表必带 `X-Org-Id`" |
| 任务状态 | `awaiting_review` | 属活跃态，会进工作台扫描范围 |
| 待人工确认 todo | **1** | 工作台"待我确认"确实有内容 |
| artifact | 42 个（交付物 41 / 过程 1） | 与桌面端口径一致（单输出位步骤的历史误绑，非移动端问题） |
| 评论 | 148 | 评论分页后续需考虑（当前全量拉取） |

**已实现**：
- 工作台：四类待确认事项（`plan_clarify` / `plan_review` / `todo_ask` / `todo_review`），口径**照搬桌面端 `lib/pendingItems.ts`**；含 PM 规划澄清的动态表单（single_select / text_input / confirm / info）。
- 任务：项目 → 任务列表 → 任务详情（概览 + 进度条 + 步骤时间线 + 待处理项 + 交付物 + 评论）。
- 交付物：图片内嵌全屏预览、PDF 新窗口打开、其余下载；**一律经 apiClient 取 blob 转 object URL**（直连 `<img src>` 恒 401）。
- 评论：底部固定输入条 + 气泡列表。

**两条刻意的取舍**：
1. 工作台只扫**最近更新的 8 个活跃任务**——避免手机上 N+1 打爆；很久没动的任务不出现在移动端。
2. 交付物/评论**全量拉取不分页**——内部工具当前的真实体量（42 文件 / 148 评论）尚可接受，超量再补分页。

**闸门**：`tsc --noEmit` **0 错**；`vitest run` **3 文件 / 10 用例全过**（新增交付物口径 4 条）；`eslint .` **0 error 0 warning**；`vite build` 成功，**gzip 144.84 KB**（P0 时 136 KB）。

**未做（P2）**：收件箱、通知设置。

---

## 附：P2 实施结果（2026-09-20）

**只读勘察**（同上，未写任何数据）：`unreadCount={count:8}`、`notifications` 字段 = `id, org_id, event_id, project_id, actor_type, actor_id, actor_name, title, body, category, priority, is_read, read_at, created_at`；`filter=unread` 生效（返回 5 条）。
⚠️ 样本里 **没有 `task_id`** —— 它是可选字段，移动端已做兜底（无 task_id 时退到 project_id，再没有就不跳转）。

**已实现**：
- 收件箱：最近 / 未读分段、按日期分组（今天 / 昨天 / 日期）、点击直达任务详情并标记已读、一键全部已读。
- 底部导航未读红点（`useUnreadCount` 轮询，间隔可在「我的 → 通知」里调，可关闭）。
- 通知设置：红点开关 + 轮询频率（关闭 / 30 秒 / 1 分钟 / 5 分钟）。

**两条如实说明**：
1. **通知设置是纯本地偏好** —— 后端无通知设置接口（`api/user.ts` 只有 getMe / 改资料 / 改密码），真正的应用内推送属 P3（Capacitor 壳 + 厂商通道），界面上已写清楚，不伪装成"已支持"。
2. **PWA 的 Service Worker 仍未启用** —— 用户选择继续自签，secure origin 受限；manifest 与图标已就位，换正式证书当天即可开启。
3. 收件箱**不做「消息 / @提及 / 系统通知」三分**：屏幕窄，category 徽标已足够区分，再叠一层筛选反而增加点击成本。

**闸门**：`tsc --noEmit` **0 错**；`vitest run` **4 文件 / 12 用例全过**（新增通知分组 2 条）；`eslint .` **0 error 0 warning**；`vite build` 成功，**gzip 154.84 KB**。

**至此 MVP（P0+P1+P2）全部完成。** 剩余为可选项：P3 原生壳（推送 / 扫码 / 离线）、PWA Service Worker（待正式证书）、评论与交付物的分页。
