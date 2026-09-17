# TrustMesh 权限体系设计方案

日期：2026-09-16
状态：**待用户确认后实施**（本期范围 = P1 固定角色权限矩阵 + P2 企业自定义角色）
前置：多租户骨架已上线（org 归属裁决 + 项目成员白名单），T3.1 双实例灰度观察期并行中。

---

## 0. 决策记录（用户已拍板）

| 决策点 | 结论 |
|---|---|
| 平台管理员职责边界 | **纯平台运维型**：管企业生命周期/全局配置/审计/用量，碰不到企业业务数据 |
| 平台管理入口 | **内嵌现有前端**：platform_admin 登录后多出「平台管理」菜单组 |
| 菜单与权限映射 | **权限点为主 + 企业覆盖**：菜单绑定权限点自动过滤，企业可再缩小 |
| member 收紧 | 现有 member 未使用 agent管理/运维/审批，**直接收紧**，无过渡期 |
| 企业覆盖语义 | **只能缩小**：owner 可隐藏菜单，不能放大任何角色的权限点 |
| 平台管理员产生 | **env 种子账号**（`PLATFORM_ADMIN_EMAILS`），账号集合固定 |
| 平台管理本期功能 | 企业生命周期 + 全局审计日志 + 全局配置 + 用量总览（四项全做） |
| 本期范围 | P1（权限点+矩阵+菜单控制）+ P2（企业自定义角色）一起 |
| agent 业务角色 | pm/developer/reviewer 是工作流分工，**不是权限角色，本方案不碰** |

## 1. 现状诊断

已有基础（保留不动）：
- `Scope{UserID, OrgID, Role, System}` 请求上下文（`internal/store/scope.go`）
- 数据行级裁决：`visibleToScope`（org 归属）+ `projectVisible`（成员白名单）
- agent 回写/指派裁决：`agentCanWriteTaskUnsafe` / `agentCanAssignableToProjectUnsafe`

痛点（实证）：
1. 角色只有 `owner/admin/member` 三档硬编码（`model/organization.go`），无权限点概念；
2. 权限判断散落硬编码（`sc.Role == OrgRoleOwner || sc.Role == OrgRoleAdmin`），新增受保护接口靠自觉；
3. member 与 admin 数据权限几乎等价，三角色实际只差"组织管理"一项；
4. 前端零权限感知：10 个主菜单对所有登录用户一视同仁（`MainLayout.tsx`），越权点了才被 403 弹回。

## 2. 三层权限模型

```
平台层   platform_admin（env 种子账号，非任何企业成员）
         └─ 只能访问 /api/v1/platform/*；调企业 API 一律 403
企业层   owner / admin / member（内置种子角色） + 企业自定义角色
         └─ 数据行级裁决沿用 scope.go，零改动
项目层   项目成员白名单（现有 projectMembers；项目内角色细化后置 P3，不在本期）
```

**正交铁律**：平台权限链与企业权限链互不越界。平台管理员点进"项目"等业务菜单时
只能看到自己个人空间的数据（他是平台运维，不是企业成员）；企业用户调
`/api/v1/platform/*` 一律 403。

## 3. 权限点清单

### 3.1 企业层权限点（`internal/authz/permission.go` 集中定义）

**基础权限**：所有登录用户隐式拥有（不枚举、不参与角色配置）——对应仪表盘/AI办公室/
知识库/会议/外部应用的浏览与日常使用。**例外**：这些域内的管理类操作仍需显式权限点（见下表）。

| 域 | 权限点 | 说明 | owner | admin | member |
|---|---|---|---|---|---|
| 组织 | `org.settings` | 企业信息/解散/覆盖菜单配置 | ✅ | ❌ | ❌ |
| 组织 | `org.member.mgr` | 邀请/移除/改角色（不可动 owner） | ✅ | ✅ | ❌ |
| 组织 | `org.role.mgr` | 自定义角色管理 | ✅ | ❌ | ❌ |
| 项目 | `project.create` | 新建项目 | ✅ | ✅ | ✅ |
| 项目 | `project.manage` | 归档/配置/成员白名单 | ✅ | ✅ | ❌ |
| 任务 | `task.create` | 建任务 | ✅ | ✅ | ✅ |
| 任务 | `task.dispatch` | 指派 agent / 催办 | ✅ | ✅ | ✅ |
| 工作流 | `workflow.template.mgr` | 模板创建/复制/curate/删除/同步 | ✅ | ✅ | ❌ |
| 数字员工 | `agent.view` | 列表/详情 | ✅ | ✅ | ✅ |
| 数字员工 | `agent.manage` | 增删改/技能/能力配置 | ✅ | ✅ | ❌ |
| 市场 | `market.browse` | 浏览岗位市场 | ✅ | ✅ | ✅ |
| 市场 | `market.install` | 安装角色包 | ✅ | ✅ | ❌ |
| 会议 | `meeting.manage` | 开始/结束会议、删除会议 | ✅ | ✅ | ❌ |
| 知识库 | `knowledge.manage` | 知识条目增删改（浏览为基础权限） | ✅ | ✅ | ❌ |
| 审批 | `join_request.approve` | 数字员工加入审批 | ✅ | ✅ | ❌ |
| 运维 | `ops.view` / `ops.manage` | 运维工单查看/处理 | ✅ | ✅ | ❌ |

> member 收紧是本期唯一改变现有用户行为的点（已确认生产无 member 在用这些功能）。

### 3.2 平台层权限点

| 权限点 | 说明 |
|---|---|
| `platform.org.lifecycle` | 企业列表/详情（元数据）/开通/禁用/恢复 |
| `platform.config.read` / `platform.config.write` | 全局配置查看/修改 |
| `platform.audit.view` | 全局审计日志查询 |
| `platform.usage.view` | 全平台用量总览（企业数/用户数/任务量/存储，不含业务内容） |

## 4. 菜单矩阵（权限点的前端投影）

菜单绑定规则：
- 每个菜单项声明 `perm`（权限点）或 `base`（登录即见）；`base` 菜单不受角色影响
- 「企业管理」菜单绑定规则：`org.settings | org.member.mgr | org.role.mgr` 任一命中即可见
  （admin 有 member.mgr 所以能进，但页内"企业设置"标签页按 `org.settings` 再隐藏）
- 「会议」菜单仍是 base（可旁听列表/详情/发言），但**会议生命周期（创建/开始/结束）**
  统一走 `meeting.manage`（见 §11 v2.2）

| 菜单 | 绑定 | owner | admin | member | 平台管理员 |
|---|---|---|---|---|---|
| 仪表盘 / AI办公室 / 项目 / 工作流 / 会议 / 知识库 / 外部应用 | base | ✅ | ✅ | ✅ | ❌ |
| 数字员工 | `agent.view` | ✅ 管理 | ✅ 管理 | 👁 只读（管理按钮隐藏） | ❌ |
| 市场 | `market.browse` | ✅ 可安装 | ✅ 可安装 | 👁 浏览，安装按钮隐藏 | ❌ |
| 运维工单 | `ops.view` | ✅ | ✅ | ❌ 菜单隐藏 | ❌ |
| 企业管理 | 组织域任一权限点 | ✅ | ✅ | ❌ 菜单隐藏 | ❌ |
| **平台管理**（新增菜单组：企业/全局配置/审计/用量） | `platform.*` | ❌ | ❌ | ❌ | ✅ |

**平台管理员的菜单体验**：登录后**只看到**「平台管理」菜单组 + 用户区（个人信息/退出），
业务菜单全部隐藏。他不是来用业务的，界面不应诱导他点进业务页。（其个人空间数据仍在，
如需以普通用户身份使用业务，需另注册普通账号——权限身份与账号解耦。）

企业覆盖（只能缩小）：`org.menu_overrides: []string`，owner 在企业管理-设置页勾选隐藏菜单。
前端过滤规则：`visible = hasPerm(menu.perm) && !menuOverrides.includes(menu.key)`。
覆盖只影响菜单可见性，**不影响 API 权限**（API 鉴权以权限点为唯一准绳）。

## 5. 自定义角色（P2，本期合并实施）

- 新集合 `org_roles`：`{id, org_id, name, permissions[], builtin, created_at, updated_at}`
  - 每个企业创建时种子三个内置角色（owner/admin/member，`builtin=true`）
  - 内置角色权限集**不可修改**（保证三档语义稳定，避免 owner 把 member 改成隐形 admin）；
    需要差异化就新建自定义角色
  - owner 角色不可删改、不可被自定义角色替代（保留转让语义）
  - 自定义角色只能勾选权限点的**子集**（≤ admin 全集），且不允许勾选 `org.role.mgr`
- `org_memberships.role`（string）升级为 `role_id`（引用 org_roles）
  - **兼容期双读**：`role_id` 为空时按旧 `role` 字符串映射内置角色
  - **迁移方式**：服务启动时自动跑批（幂等：只填 `role_id` 为空的 membership，
    按旧 role 字符串映射到本企业对应内置角色种子）；跑批完成打日志，
    核对脚本确认无残留后，后续版本移除双读
- 权限解析链：`membership.role_id → org_roles.permissions[] → authz.Has(rolePerms, perm)`
- 企业管理新增「角色管理」页：角色列表 + 权限点勾选矩阵

## 6. 后端落地

1. **`internal/authz` 包**（纯新增）：
   - 权限点常量、内置角色映射表、`Has(perms []string, perm string) bool`
   - `RequirePerm(perm)` gin 中间件：从 Scope 解析角色权限集，缺失即 403
2. **全量路由映射**：逐个接口标注权限点，替换所有散落的 role 硬编码
   （`handler/organization.go`、`handler/ops.go` 等）；数据裁决层 `scope.go` 零改动
3. **平台管理员**：
   - `PLATFORM_ADMIN_EMAILS` env 种子；登录/`/auth/me` 时打 `is_platform_admin` 标记
   - 平台 API 独立命名空间 `/api/v1/platform/*` + 独立中间件（只认平台标记，不看企业角色）
   - 企业 API 中间件反向拒绝平台管理员（`is_platform_admin && !platform route → 403`）
4. **新集合**：
   - `org_roles`（见 §5）
   - `audit_logs`：`{id, actor_user_id, actor_email, scope("platform"|org_id), action,
     target_type, target_id, detail, ip, created_at}`
     - action 枚举（首版）：`org.create/disable/restore`、`org.member.add/remove/role_change`、
       `org.role.create/update/delete`、`org.menu_override.update`、`project.archive`、
       `agent.delete`、`platform.config.update`、`platform.admin.login`
     - 企业敏感操作同样落审计，但本期审计查询仅平台侧开放（企业侧审计页后置）
     - TTL 策略：保留 180 天（TTL 索引），超期自动清理
   - `platform_settings`：单文档全局配置（默认模型/存储限额/节点参数），本期读写界面都做
5. **用量总览**：`/api/v1/platform/usage` 聚合统计接口（count 级查询，不拉业务数据）

## 7. 前端落地（frontend-v2）

- `/auth/me` 响应扩展：`permissions[]`、`menu_overrides[]`、`is_platform_admin`
- Pinia `permStore`：`hasPerm()` / `visibleMenus()`；登录/切换工作区时刷新
- 菜单过滤：`MainLayout.tsx` 菜单项声明 `perm`，按权限+overrides 过滤
- 路由 `meta.perm` 拦截 + `v-perm` 按钮指令；越权统一 403 页（非白屏）
- 新界面：企业管理-角色管理页（权限点勾选矩阵）；平台管理四页（企业/配置/审计/用量）

## 8. 实施顺序

1. `authz` 包 + 权限点常量 + 内置角色映射（纯新增，零风险）
2. `RequirePerm` 全路由收敛 + member 收紧（唯一行为变更点，已确认无过渡负担）
3. 平台管理员标记 + `/api/v1/platform/*` 四个功能 + 审计切面
4. `org_roles` 集合 + membership role_id 迁移（双读兼容）+ 角色管理 UI
5. 前端 permStore + 菜单过滤 + 按钮控制 + 平台管理界面
6. 回归冒烟：member 收紧路径、双实例下角色变更的权限缓存一致性
   （注意与 T3.1 阶段D观察期叠加——角色/权限变更必须即时生效，不能依赖进程内缓存；
   权限集在请求时从 membership 实时解析，不做跨请求缓存）

## 9. 风险与回滚

| 风险 | 缓解 |
|---|---|
| member 收紧误伤 | 已确认生产无 member 使用受影响功能；保留 `PERM_LEGACY_MEMBER=1` env 开关可临时回退旧行为 |
| role_id 迁移遗漏 | 兼容期双读兜底；迁移后跑核对脚本（membership 无 role_id 的条数 = 0）才移除 |
| 平台管理员误配 | env 种子账号集合固定，不可在界面授予/撤销（本期）；后续如需界面授权再开 |
| 权限点漏标路由 | 中间件默认拒绝原则：未标注权限点的 authed 路由在测试模式 panic，逼开发显式声明 |
| T3.1 观察期叠加 | 权限改动不碰 SSE/leader/outbox 代码面；上线错峰，先观察期收尾再部署本方案 |

## 10. 不做清单（本期明确排除）

- 项目内角色（项目负责人/成员/观察员）→ P3，挂在现有 projectMembers 上扩展
- 平台管理员界面授权/多平台管理员管理 → 后续（本期 env 固定）
- 字段级权限（如薪资字段仅 HR 可见）→ 无业务需求，不预埋
- 企业审计日志对企业 owner 开放 → 本期审计仅平台侧可见
- 内置角色权限集的可编辑化 → 刻意锁定，保证 owner/admin/member 三档语义稳定
- 平台管理员复用业务界面 → 平台管理员只见平台管理菜单组，业务需求另注册普通账号

## 11. 修订记录

- 2026-09-16 v2：补权限点与菜单闭合（base 基础权限概念；workflow/meeting/knowledge
  管理权限点）；明确「企业管理」菜单绑定规则与平台管理员菜单体验；内置角色权限集锁定；
  补审计 action 枚举与 TTL；membership 迁移改为启动时自动幂等跑批。
- 2026-09-16 v2.1（实施决策，两处语义收敛）：
  1. **平台管理员的业务 API 门禁 = 种子模式下严格分离**。`PLATFORM_ADMIN_EMAILS` 已配置时，
     平台管理员调业务 API 一律 403（`authz.RequireBusinessAccount`），
     账号/会话类白名单前缀除外：`/api/v1/users`、`/api/v1/organizations`、
     `/api/v1/notifications`、`/api/v1/events`、`/api/v1/llm-config`
     （前四个是外壳能力，最后一个是平台配置页的连通性测试/模型列表共用入口）。
     **未配置 env 的环境保持历史行为**（首个用户自动提升，不做反向拒绝）——
     否则会把存量环境里自动提升的管理员锁在自己的业务之外。
     平台默认 LLM 配置（`/api/v1/platform/llm-config` 三条）从 `authed` 组收敛进平台命名空间，
     企业角色自此不可及。
  2. **禁用企业 = 禁用即拦截**。新增 `org.status`（`active`/`disabled`，默认 active）；
     被禁用企业的成员带该企业 `X-Org-Id` 的请求返回 403 `ORG_DISABLED`
     （拦截点在 `middleware.OrgScope`，校验成员资格之后）；个人空间不受影响，恢复即时生效。
- 2026-09-16 v2.2（实施决策，会议生命周期收敛）：
  **「创建会议」也走 `meeting.manage`**（`POST /api/v1/projects/:projectId/meetings`）。
  原设计里创建属基础权限、开始/结束属 `meeting.manage`，会出现「member 能建会却开不了会」
  的僵尸会议（会议室还会因自动开始打一次必然 403 的 `start`）。故会议生命周期
  （创建/开始/结束）统一为一个权限点；**只读旁听（列表/详情/发言）仍对所有人开放**。
  前端：`MeetingListPage` 的建会入口按权限点隐藏；`MeetingRoomPage` 的自动开始额外要求
  权限视图**已就绪**（fail-closed）——`hasPerm` 未就绪时 fail-open 为 true，
  直接用会让无权限者一进会议室就吃一次 403 并弹出误导性报错。
