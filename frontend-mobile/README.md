# frontend-mobile（面向业务人员的移动端）

独立子项目，**不复用 `frontend-v2` 的任何组件与依赖**，只消费同一套后端 `/api/v1/*`。
方案与选型依据见 `docs/mobile-business-app-plan-2026-09-19.md`。

## 技术栈

Vite 7 + React 19 + TypeScript + Tailwind 4 + antd-mobile 5 + Zustand 5 + React Query 5 + ky

## 命令

```bash
npm install
npm run dev      # 5175，/api 由 Vite 代理到生产（开发期跳过自签证书校验）
npm run build    # tsc --noEmit + vite build → dist-mobile/
npm run test     # vitest
npm run lint     # eslint
```

## 三条不可回退的约定

1. **不 import `frontend-v2` 的任何东西** —— antd（CSS-in-JS）与 antd-mobile（CSS 变量）在同一构建流里会互相破坏。
2. **React 19 垫片只在 `src/main.tsx` 一处** —— antd-mobile v5 默认兼容 16~18，靠 `unstableSetRender` 适配，该接口下个 major 会移除。
3. **工作区切换顺序由 `runWorkspaceCalibration` 单点保证** —— `personal → active → 清缓存 → 放开渲染`，写反会静默复发。

## 服务地址优先级

手动配置（我的 → 服务地址） > `VITE_API_BASE_URL` > 同源 `/api/v1/`。
生产把产物挂在后端**同域** nginx 下即可零配置；跨源部署时须由后端加 CORS 头。

## 已知限制（自签证书带来的）

生产是裸 IP 自签证书，浏览器对 fetch 的证书错误没有「继续访问」入口，因此：
- 真机首次访问需在浏览器里先手动接受该站点的证书风险；
- PWA 的 Service Worker 与「添加到主屏」**暂未启用**，换正式证书后再开（manifest 已就位）。

## 当前进度

- **P0 已完成**：工程骨架、登录、组织/工作区切换、四 Tab 导航、设计令牌。
- **P1 已完成**：工作台待办/审批（四类待确认事项）、项目 → 任务列表 → 任务详情（步骤时间线、交付物、评论）、交付物预览。
- **P2 已完成**：收件箱（最近/未读、按日期分组、点击直达任务并标记已读、全部已读）、底部导航未读红点、通知设置（红点开关 + 未读轮询频率）。

> ⚠️ **通知设置是纯本地偏好**：后端目前没有通知设置接口（`api/user.ts` 只有 getMe / 改资料 / 改密码），
> 所以这里只控制前端轮询与红点；真正的应用内推送要等 P3（Capacitor 壳 + 厂商通道）。
> 同理，PWA 的 Service Worker 因自签证书未启用（manifest 已就位）。

## 待确认事项口径（照搬桌面端，不另造）

- **四类待确认**：`plan_review`（任务 `status==='review'`）、`plan_clarify`（planning 中最近一条未应答的 pm_agent ui_blocks）、`todo_ask`（事件 `todo_ask_received` 且 `metadata.answer==null`）、`todo_review`（`todo.review_status==='pending_approval'`）；排序 `plan_clarify > plan_review > todo_ask > todo_review`。
- **交付物判定**：`output_name != ""` 或 `kind==='deliverable'` ⇒ 交付物，其余归过程产物（`kind` 为空的历史数据走正向判定，不得反向排除）。
- **工作台只扫最近更新的 8 个活跃任务**（避免手机上打爆 N+1）；`X-Org-Id` 必带，否则列表只返回个人空间。

## 实测校验（只读探针，真实数据）

企业空间 `projects=11`、任务状态 `awaiting_review`、`pendingApprovalTodos=1`、`artifacts=42`（交付物 41 / 过程 1）、`comments=148` —— 字段口径与上述实现一致。
