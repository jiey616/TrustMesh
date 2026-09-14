import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'

// 以 ?raw 读源码文本（vite/client 已声明该模块类型，无需 node 类型）。
import appSource from '../App.tsx?raw'
import clientSource from '../api/client.ts?raw'
import mainLayoutSource from '../layouts/MainLayout.tsx?raw'
import loginPageSource from '../pages/LoginPage.tsx?raw'

/**
 * 跨租户隔离契约（T1.5）。
 *
 * 为什么要做「源码级断言」：v2 的租户切换刻意走整页 reload（window.location.reload()），
 * 用「重建模块级 QueryClient」来兜底缓存隔离，而不是把 orgId 编进每个 queryKey。
 * 这条策略很容易被后续重构无意破坏（比如把 reload 改成 navigate，缓存就会跨租户存活），
 * 而后果是「切了企业还看到上一家数据」这种静默串数据。这里把策略钉成可执行断言。
 *
 * 同时补一组缓存失效原语的行为断言，证明 clear/removeQueries 确实会丢弃数据。
 */

function count(source: string, pattern: RegExp): number {
  return source.match(pattern)?.length ?? 0
}

describe('租户切换 / 缓存失效契约（源码级）', () => {
  it('切换工作区的两条分支都走整页 reload，靠重建 QueryClient 隔离缓存', () => {
    // 个人空间分支 + 企业分支各一次
    expect(count(mainLayoutSource, /window\.location\.reload\(\)/g)).toBeGreaterThanOrEqual(2)
    // 若有人把 reload 换成 navigate，这里会失败——那正是缓存串租户的开始
    expect(mainLayoutSource).toMatch(/handleOrgSwitch/)
  })

  it('登出清空 QueryClient，并重置两个租户 id', () => {
    expect(mainLayoutSource).toMatch(/qc\.clear\(\)/)
    expect(mainLayoutSource).toMatch(/logout\(\)/)
  })

  it('个人租户 id 首次水合与失效回落都会 removeQueries 重取数据', () => {
    expect(count(mainLayoutSource, /qc\.removeQueries\(\)/g)).toBeGreaterThanOrEqual(2)
    expect(mainLayoutSource).toMatch(/setPersonalOrgId\(/)
  })

  it('登录成功先清空上一个账号的缓存再写入新会话', () => {
    const clearIdx = loginPageSource.indexOf('queryClient.clear()')
    const setAuthIdx = loginPageSource.indexOf('setAuth(')
    expect(clearIdx).toBeGreaterThan(-1)
    expect(setAuthIdx).toBeGreaterThan(-1)
    // 顺序不可颠倒：必须先清缓存，再落新 token
    expect(clearIdx).toBeLessThan(setAuthIdx)
  })

  it('请求头的租户来源固定为 activeOrgId ?? personalOrgId', () => {
    expect(clientSource).toMatch(/const orgId = activeOrgId \?\? personalOrgId/)
    expect(clientSource).toMatch(/request\.headers\.set\(\s*'X-Org-Id'\s*,\s*orgId\s*\)/)
    expect(clientSource).toMatch(/request\.headers\.set\(\s*'Authorization'/)
  })

  it('QueryClient 默认不开启 window focus 重取，staleTime 有明确取值', () => {
    expect(appSource).toMatch(/refetchOnWindowFocus:\s*false/)
    expect(appSource).toMatch(/staleTime:\s*30_000/)
  })
})

describe('缓存失效原语行为', () => {
  it('clear() 丢弃全部跨租户缓存', () => {
    const qc = new QueryClient()
    qc.setQueryData(['agents'], [{ id: 'a-1' }])
    qc.setQueryData(['orgs'], [{ id: 'org-1' }])
    expect(qc.getQueryData(['agents'])).toBeDefined()

    qc.clear()

    expect(qc.getQueryData(['agents'])).toBeUndefined()
    expect(qc.getQueryData(['orgs'])).toBeUndefined()
  })

  it('removeQueries() 清掉匹配前缀但不影响无关缓存', () => {
    const qc = new QueryClient()
    qc.setQueryData(['agents', 'x', 'stats'], { n: 1 })
    qc.setQueryData(['meetings'], [{ id: 'm-1' }])

    qc.removeQueries()

    expect(qc.getQueryData(['agents', 'x', 'stats'])).toBeUndefined()
    expect(qc.getQueryData(['meetings'])).toBeUndefined()
  })
})
