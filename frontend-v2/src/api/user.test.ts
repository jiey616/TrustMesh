import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { updateProfile } from '@/api/user'
import type { User } from '@/types'

/**
 * `PATCH /users/me` 的响应信封契约回归测试（P0 护栏）。
 *
 * 后端返回 `{"data":{"user":{...}}}` —— `data` 里**嵌了一层 `user`**
 * （`handler/user.go` 用 `gin.H{"user": user}`，与 `GET /users/me` 的
 * `{user, permissions, menu_overrides, is_platform_admin}` 信封保持一致）。
 *
 * 🔴 历史缺陷：`updateProfile` 曾把类型写成 `ApiResponse<User>` 并在调用点直接
 * `onSuccess(res.data)`，于是 `authStore.user` 被写成 `{user:{...}}` ⇒
 * `user.id` / `user.name` 全为 `undefined` ⇒
 *   ① 工作区校准 `resolveWorkspaceTarget(…, user.id, …)` 因 `userId` 为空恒返回 null，
 *      切企业空间后一刷新就被踢回个人空间（切换失效）；
 *   ② 侧边栏 / 资料页姓名显示不出来（改名「没生效」）；
 *   ③ 脏对象经 `partialize` 落盘、`merge` 不做形状校验 ⇒ 跨刷新存活，必须登出重登。
 *
 * 本测试真跑 ky（stub 全局 fetch），而非源码级文本断言 —— 只有行为级断言才能
 * 挡住「类型写错但恰好能编译且不报错」这类静默污染。
 */

const USER: User = {
  id: 'u-1',
  email: 'u1@example.com',
  name: '改后的名字',
  created_at: '2026-01-01T00:00:00.000Z',
  updated_at: '2026-01-02T00:00:00.000Z',
}

interface Captured {
  method: string
  url: string
  body: string | null
}

let captured: Captured[]
let responder: () => Response

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

beforeEach(() => {
  captured = []
  // 忠实复刻后端信封：data 下再包一层 user
  responder = () => jsonResponse({ data: { user: USER } })
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input as string, init)
    captured.push({
      method: req.method,
      url: req.url,
      body: await req.clone().text().catch(() => null),
    })
    return responder()
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('updateProfile 响应信封契约（P0 回归）', () => {
  it('解包 data.user：解析结果是 User 本身，绝不把信封层交出去', async () => {
    const updated = await updateProfile({ name: '改后的名字' })

    // 必须等于真实 User
    expect(updated).toEqual(USER)
    expect(updated.id).toBe('u-1')
    expect(updated.name).toBe('改后的名字')

    // 🔴 防退化核心断言：信封层不得泄漏到调用方
    //    （旧实现返回 { user: {...} }，会让 user.id 变 undefined）
    expect(updated).not.toHaveProperty('user')
    expect(updated).not.toHaveProperty('data')
  })

  it('解析出的 id 是可直接用于工作区校准的非空字符串', async () => {
    const updated = await updateProfile({ name: '改后的名字' })
    // resolveWorkspaceTarget 的守门依据就是 user.id；为空即导致切空间整体失效
    expect(typeof updated.id).toBe('string')
    expect(updated.id.length).toBeGreaterThan(0)
  })

  it('请求打到 PATCH /users/me 且体为 { name }', async () => {
    await updateProfile({ name: '改后的名字' })
    expect(captured).toHaveLength(1)
    expect(captured[0].method).toBe('PATCH')
    expect(captured[0].url).toContain('/users/me')
    expect(JSON.parse(captured[0].body ?? '{}')).toEqual({ name: '改后的名字' })
  })
})
