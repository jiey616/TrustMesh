# authStore `logout` 保留 org id 改动 —— 独立评审记录

- **日期**：2026-09-16
- **对象**：`frontend-v2/src/stores/authStore.ts` 工作树里一个**未提交、未评审**的改动
- **评审方式**：独立 QA（严过关 / software-qa-engineer-3）对抗评审 + 主理人独立复核关键指控
- **结论**：🔴 **P1（高）—— 应「回退丢弃」。路由判定 = Engineer（源码缺陷，动作 = revert）。**

---

## 一、被评审的改动

```diff
--- a/frontend-v2/src/stores/authStore.ts
+++ b/frontend-v2/src/stores/authStore.ts
@@ -31,7 +31,9 @@ export const useAuthStore = create<AuthState>()(
       setUser: (user) => set({ user }),
       setTokens: (accessToken, refreshToken) => set({ accessToken, refreshToken }),
       logout: () =>
-        set({ accessToken: null, refreshToken: null, user: null, activeOrgId: null, personalOrgId: null }),
+        // 保留 activeOrgId / personalOrgId：重新登录后恢复上次选中的空间（企业/个人），
+        // 不再强制回到个人空间。若该空间已不在当前账号的租户列表，MainLayout 会回落。
+        set({ accessToken: null, refreshToken: null, user: null }),
       isAuthenticated: () => !!get().refreshToken,
     }),
```

`logout` 不再清空 `activeOrgId` / `personalOrgId`。

---

## 二、评审结论摘要

| # | 问题 | 结论 |
|---|---|---|
| Q1 | 跨账号串租户是否存在 | **存在**。`setAuth`（`authStore.ts:30`）= `set({accessToken, refreshToken, user})`，**不含 org id**；登录链路（`LoginPage.tsx:28-29`、`RegisterPage.tsx:36`）只调 `setAuth`；全仓库 org id 写点只有 `MainLayout.tsx:130/144/192/198`。**没有任何「登录成功 / user 变化 / rehydrate」时重置 org id 的路径。** A 登出后 B 的第一批请求即带 A 的 org id |
| Q2 | 注释称「MainLayout 会回落」 | **证伪**。回落 effect（`MainLayout.tsx:127-133`）条件是 `activeOrgId && orgs && !orgs.some(...)` —— **以 `orgs` 已就绪为前提**；而 `orgs` 来自 `useOrganizations` → `GET /organizations`，它自己也带同一条陈旧 `X-Org-Id` → 401 → `orgs` 永远 `undefined` → 条件短路 → **回落永不触发，水合 effect（`:141-147`）同样死锁**。实际后果不是回落，是会话卡死 |
| Q3 | 后端如何处理「非成员 org 的 `X-Org-Id`」 | **硬拒绝 401**（`middleware/org_scope.go:20-39`，非成员即 `Unauthorized` + `Abort`；既有测试 `org_scope_test.go:105-120` 覆盖）。✅ **不存在静默降级成 user 维度的越权路径** ⇒ 不是数据泄露 |
| Q4 | `personalOrgId` 残留的独立风险 | **确认成立，且是主路径**。A 一直待在个人空间时 `activeOrgId` 本就是 null，`X-Org-Id = activeOrgId ?? personalOrgId` 直接命中残留的 A 个人租户。**任何共用浏览器上「A 登出 → B 登入」都会命中**（多数用户从不切企业空间） |
| Q5 | 是否有持久化 | **有**（`authStore.ts:20-21` `persist(...)`，`:40` `name:'trustmesh-v2:auth'`，`:41-46` `partialize` **白名单含 `activeOrgId` 与 `personalOrgId`**）⇒ 陈旧 org id 会写进 localStorage 并**在整页刷新/冷启动时 rehydrate**，刷新也甩不掉 |
| Q6 | 既有测试是否拦截 | **拦截，且改动使其变红（已实测）**。`authStore.test.ts:42-43` 明确断言 `activeOrgId`/`personalOrgId` 为 null；实跑 `vitest run` → `Tests 1 failed | 20 passed`，失败断言 `expected 'org-enterprise' to be null`（`authStore.test.ts:42`） |
| Q7 | 反向验证 | 把 `logout` 改回清空 → 该红测试转绿，**其它测试无一变红** ⇒ 新行为（保留 org id）**零正向覆盖**，是明确的**契约冲突**而非测试真空 |

### ⚠️ 已作废 + 更正（2026-09-16 晚，本节原结论是错的）

> **更正说明**：本节原先断言「401 → 无界 refresh+重放循环」，并标注为「主理人已独立核对源码」。**该结论是错的** —— 由工程师在实现阶段实测证伪、经主理人复核确认：
>
> `client.ts:82` 的 `return ky(request)` 用的是文件顶部 **`import ky from 'ky'`（`client.ts:1`）的裸默认实例**，**不是** `apiClient`。自定义 `beforeRequest`/`afterResponse` hooks 只挂在 `apiClient`（`client.ts:49`）上，因此**重放不会重跑 hooks**；ky 默认 retry 也不含 401。实测：持续 401 下 `refresh=1 / 业务请求=2`，**有界**。
>
> **主理人的错误来源**：只看到 `return ky(request)` 就推断「重放 → 再次进 afterResponse → 递归」，未确认 `ky` 究竟是哪个实例。这与本次 T2.5 的教训同源 —— **看代码推断出的结论，必须用运行验证兜底**。
>
> **正确的缺陷描述**是下面两条（均已实测）：
> 1. **任何 401（含「租户非成员」）都会触发一次无谓 refresh**（org 场景实测 `refresh=1`，理想应为 0）。注意后端两种 401 的 `error.code` **完全相同**（都是 `UNAUTHORIZED`，`transport/response.go:65-67` 写死），仅 message 不同 —— 故前端**无法**用 code 区分。
> 2. **重放后的终态 401 由裸默认 ky 抛 `HTTPError`，没有 `.code`**；而全站调用方统一按 `err instanceof ApiRequestError` 分支取 `err.message` → 用户只看到通用「请求失败」，**真实原因丢失**。这是本项真正的用户可见缺陷。
>
> **对风险评级的影响**：**P1 结论不变，但机制要改** —— 不是「401 风暴」，而是「B 的每条请求都 401 ⇒ `orgs` 恒加载不出 ⇒ 回落/水合死锁 ⇒ B 的会话卡死」。
>
> **注意**：当前的有界性是**偶然的** —— 它依赖 `client.ts:82` 恰好用裸 `ky`。任何人把该行改成 `apiClient(request)` 就会**瞬间变成真正的无界循环**。修复已要求把这一性质转为**结构性保证**并加注释防止后人踩。
>
> **本节以下为已作废的原分析，仅作留档。**

`client.ts:69-82`：

```ts
if (response.status === 401 && !request.url.includes('auth/refresh')) {
  ... refreshAccessToken() ...
  const { accessToken } = useAuthStore.getState()
  request.headers.set('Authorization', `Bearer ${accessToken}`)
  return ky(request)          // ← 重放
}
```

- 该分支**不校验 `error.code`**（不区分「非成员」与「token 过期」），**也没有重试计数器**；
- 重放会再次走过 `beforeRequest`（`client.ts:61-63`），把**同一条陈旧 `X-Org-Id` 设回去** → 再次 401 → 再次 refresh + 重放 → **发散**；
- ky 自身的 retry 不含 401，故循环完全由该钩子产生。

**观测后果**：B 一登录即把后端刷成 401/refresh 风暴，`orgs` 永不加载，界面卡死，只能手清 localStorage。

既有 `client.test.ts:103-126` 只覆盖「401 一次后变 200」，**未覆盖持续 401**，故无测试拦截此循环。

---

## 三、风险定级

**P1（高）** —— 触发前提是「共用浏览器上 A 登出 → B 登入」（含 A 处于个人空间的默认态）；后果是 B 的请求携带 A 的租户 id → 后端 401 → `orgs` 恒加载不出 → 回落/水合死锁 → **B 的会话卡死**（不是 401 风暴，见第二节更正块）。

**不构成 P0**：后端 `OrgScope` 硬拒绝，**无静默越权 / 数据错配**（Q3 已证）。

---

## 四、该改动是否解决了真实问题

**无证据。** 多租户方案文档（`docs/multi-tenant-enterprise-plan.md §4.4`）只要求注入 `X-Org-Id = activeOrgId` 并持久化 `activeOrgId`，**没有任何「重登后恢复上次空间」的需求**。改动来源不明，属顺手改动。

---

## 五、建议

1. **首选（应做）**：`authStore.ts:33-36` 回退为
   `set({ accessToken: null, refreshToken: null, user: null, activeOrgId: null, personalOrgId: null })`。
2. 若「记住上次空间」确是真需求：**必须按用户维度隔离**（如 `trustmesh-v2:auth:<userId>`），且使用时**先用当前账号的 org 列表校验后再采信**；同时 `setAuth` 里显式重置两 id。**这是新功能，不是一行改动。**
3. **独立缺陷（本改动暴露、但与本改动无关，单独立项）**：`client.ts:69` 的 401 分支存在两条真实缺陷 ——（a）任何 401 都会白刷一次 refresh，包括「租户非成员」这种与 token 无关的 401；（b）重放后的终态 401 由裸默认 ky 抛出无 `.code` 的 `HTTPError`，调用方丢失真实原因。修法：判据统一为 `code === 'UNAUTHORIZED'`，用 `WeakSet<Request>` + 裸 `ky(..., {throwHttpErrors:false})` 把「至多 refresh+重放一次」做成**结构性保证**（现状的有界性是偶然的：若有人把 `client.ts:82` 改成 `apiClient(request)` 即变成真正的无界循环，故须加注释防退化），非 ok 时抛携带真实 `code`/`message` 的 `ApiRequestError`。**已立项，见任务 #26。**
   - 附带：**不要**用字符串匹配 message 来区分两种 401 —— 前端会耦合后端文案、后端改措辞即静默失效。正确解法是让后端为该场景返回**独立错误码**（见任务 #28），本轮前端不依赖它。

---

## 六、原始改动留档（若日后需恢复）

见 `tmp/authstore-logout-orgid-2026-09-16.patch`（可直接 `git apply`）。改动本身仅此一处、共 4 行。

---

## 七、后续处置（2026-09-16 终态）

甲方已拍板：**保留「记住上次选中的空间」这一需求，但不接受上述实现，重设计为真功能。**

| 项 | 处置 |
|---|---|
| `authStore.ts` 那个顺手改动 | **已被取代**。由任务 #27 的 T02（commit `811ff3f`）重写为最终版本：`logout` 恢复清空两个运行时 org id（回归既有契约，`authStore.test.ts` 那条红测试已转绿），「记住」改由独立、**按 userId 守门**的 `workspaceMemory` 记忆层承担 |
| `client.ts` 401 保真缺陷 | **已修**（任务 #26，commit `a9f102c`）：判据只认 `code`、`WeakSet` + `retry.shouldRetry` + 裸 ky 三重上限、非 ok 抛携带真实 code 的 `ApiRequestError` |
| 后端「租户非成员」错误契约 | **新立项**（任务 #28）：为该场景给独立错误码（HTTP 状态维持 401），使前端不必靠文案区分 |
| 设计文档 | `docs/remember-last-workspace-design-2026-09-16.md`（三条代码级不变量 INV-1/2/3） |

**本评审中最值钱的一条负面结论**：本次会话两次「看代码推断出的结论」都被运行结果推翻（另一次是 T2.5 复核）。**推断必须有运行验证兜底**。
