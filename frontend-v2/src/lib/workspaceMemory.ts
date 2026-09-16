import type { WorkspaceKind, WorkspaceMemory } from '@/types'

/**
 * 工作区记忆纯函数模块（"记住上次选中的空间"）。
 *
 * 设计要点（见 docs/remember-last-workspace-design-2026-09-16.md §3.3(a)）：
 *   1. 「记忆」与「运行时权威租户态」彻底分离——本模块只处理记忆，绝不写运行时态。
 *   2. 一切函数必须 **total（绝不抛）**：任何损坏输入 → null / 空降级，绝不弹窗、绝不白屏。
 *   3. 记忆的采用必须经两道守门：`userId` 匹配（R3）+ 企业 org 必须 ∈ 本账号 orgs（R5）。
 *
 * 本模块为纯函数、零副作用、零依赖（仅类型），便于在无 @testing-library 的环境下单测。
 */

/** 解析所需的最小租户视图：只需 `id` 与 `kind`（与 `OrgView` 结构兼容，避免耦合上层类型）。 */
export type WorkspaceOrgRef = { id: string; kind: WorkspaceKind }

/**
 * 依「记忆 + 当前用户 + 本账号租户列表」解析出**应恢复的企业 org id**。
 *
 * 返回 `null` 表示「无有效企业记忆」→ 调用方应回落个人空间（`activeOrgId = null`）。
 * 绝不返回未校验的值：任何不满足守门的输入一律返回 `null`。
 *
 * @param memory 持久化的记忆（可能为空 / 结构损坏 → 已被 parseWorkspaceMemory 归一为 null）
 * @param currentUserId 当前登录用户 id（未登录 → null）
 * @param orgs 本账号全部租户（个人 + 企业）；未就绪 → null/undefined
 * @returns 校验通过的企业 org id，或 null（回落个人空间）
 */
export function resolveWorkspaceTarget(
  memory: WorkspaceMemory | null | undefined,
  currentUserId: string | null | undefined,
  orgs: ReadonlyArray<WorkspaceOrgRef> | null | undefined,
): string | null {
  // 无记忆 / 未登录 → 无可恢复目标
  if (!memory || !currentUserId) return null
  // R3：绝不读取他人记忆（按 userId 严格隔离）
  if (memory.userId !== currentUserId) return null
  // 记忆为个人空间 → 明确回落 null（个人空间）
  if (memory.kind !== 'enterprise') return null
  if (!memory.orgId) return null
  // 租户列表未就绪 → 无法校验，保守回落
  if (!orgs) return null
  // R5：企业 org 必须 ∈ 本账号 orgs 且确为企业租户，否则视为记忆失效
  return orgs.some((o) => o.id === memory.orgId && o.kind === 'enterprise') ? memory.orgId : null
}

/**
 * 结构校验并归一化持久化的记忆。
 *
 * 任何非对象 / 字段缺失 / 类型错误 / kind 非法 → `null`（视为无记忆）。绝不抛。
 *
 * @param raw 来自持久化的原始值（unknown）
 * @returns 合法记忆（已裁剪为规范形状），或 null
 */
export function parseWorkspaceMemory(raw: unknown): WorkspaceMemory | null {
  if (!raw || typeof raw !== 'object') return null
  const m = raw as Record<string, unknown>
  if (typeof m.userId !== 'string' || !m.userId) return null
  if (m.kind !== 'personal' && m.kind !== 'enterprise') return null
  if (m.kind === 'enterprise') {
    if (typeof m.orgId !== 'string' || !m.orgId) return null
    return { userId: m.userId, kind: 'enterprise', orgId: m.orgId }
  }
  // personal：裁剪掉可能残留的 orgId（个人空间不需要 org id）
  return { userId: m.userId, kind: 'personal' }
}

/** v1 持久化 schema：**绝不**包含 `activeOrgId` / `personalOrgId`。 */
export interface PersistedAuthV1 {
  refreshToken: string | null
  user: unknown
  workspaceMemory: WorkspaceMemory | null
}

/**
 * 存量数据迁移：v0（隐式）→ v1（显式）。
 *
 * 职责有二：
 *   1. **结构性剥离**旧运行时字段：返回体不含 `activeOrgId` / `personalOrgId`。
 *   2. **合成记忆**：若旧数据含 `activeOrgId`（优先）或 `personalOrgId`，且能取到 `user.id`，
 *      则合成一条按 userId 维度的 `workspaceMemory`，让存量用户升级后保留一次「上次空间」体验。
 *      （合成结果仍需经 `resolveWorkspaceTarget` 的 userId 守门 + orgs 校验，故安全。）
 *
 * **必须 total（R10）**：任何输入（含 JSON 已损坏、字段为 getter 抛错等）都不抛，
 * 退化为全空 → 个人空间。
 *
 * @param persisted zustand persist 反序列化后的 **inner state**（未知形状）
 * @param _version 旧版本号（结构性剥离与合成与本值无关，仅保留签名以对齐 zustand migrate）
 */
export function migratePersistedAuth(persisted: unknown, _version: number): PersistedAuthV1 {
  // _version 仅用于对齐 zustand migrate 的 (state, version) 契约；结构性剥离与记忆合成
  // 对所有旧版本一致。此处显式引用以满足 no-unused-vars（本仓库 eslint 未配置下划线前缀豁免）。
  void _version
  try {
    const s = (persisted ?? {}) as Record<string, unknown>
    const user = (s.user ?? null) as { id?: unknown } | null
    const refreshToken = typeof s.refreshToken === 'string' ? s.refreshToken : null

    let workspaceMemory: WorkspaceMemory | null = null
    const uid = user && typeof user === 'object' && typeof user.id === 'string' ? user.id : null
    if (uid) {
      if (typeof s.activeOrgId === 'string' && s.activeOrgId) {
        workspaceMemory = { userId: uid, kind: 'enterprise', orgId: s.activeOrgId }
      } else if (typeof s.personalOrgId === 'string' && s.personalOrgId) {
        workspaceMemory = { userId: uid, kind: 'personal' }
      }
    }

    // 关键：返回体【不含】activeOrgId / personalOrgId —— 结构性剥离。
    return { refreshToken, user, workspaceMemory }
  } catch {
    // 任何异常（含访问抛错属性）→ 全空降级，绝不向上抛。
    return { refreshToken: null, user: null, workspaceMemory: null }
  }
}
