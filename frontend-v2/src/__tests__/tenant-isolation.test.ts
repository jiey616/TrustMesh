import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'

// 以 ?raw 读源码文本（vite/client 已声明该模块类型，无需 node 类型）。
import appSource from '../App.tsx?raw'
import clientSource from '../api/client.ts?raw'
import mainLayoutSource from '../layouts/MainLayout.tsx?raw'
import loginPageSource from '../pages/LoginPage.tsx?raw'
import authStoreSource from '../stores/authStore.ts?raw'

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

describe('「记住上次选中的空间」契约（源码级，T03）', () => {
  it('INV-1：partialize 白名单不含运行时租户态（两 id 不落盘）', () => {
    // 只取 partialize 返回的对象字面量内部（避免把相邻注释里的字段名误当白名单）
    const match = authStoreSource.match(/partialize:\s*\(state\)[^{]*=>\s*\(\{([\s\S]*?)\}\)/)
    expect(match).not.toBeNull()
    const body = match![1]
    expect(body).not.toMatch(/activeOrgId|personalOrgId/)
    expect(body).toMatch(/refreshToken/)
    expect(body).toMatch(/workspaceMemory/)
  })

  it('persist 显式 version:1，且 merge 无条件把两个运行时 id 置 null', () => {
    expect(authStoreSource).toMatch(/version:\s*1/)
    const mergeIdx = authStoreSource.indexOf('merge:')
    expect(mergeIdx).toBeGreaterThan(-1)
    const block = authStoreSource.slice(mergeIdx)
    expect(block).toMatch(/activeOrgId:\s*null/)
    expect(block).toMatch(/personalOrgId:\s*null/)
  })

  it('MainLayout 用统一校准 effect：先 setPersonalOrgId 再 setActiveOrg 再 removeQueries', () => {
    // 记依赖校验纯函数（经 userId 守门 + orgs 校验后才写运行时态）
    expect(mainLayoutSource).toMatch(/resolveWorkspaceTarget\(/)
    // 同一 tick 内先写个人、再写企业、最后才 removeQueries —— 杜绝「先个人头、后企业头」中间窗口
    expect(mainLayoutSource).toMatch(
      /setPersonalOrgId\(nextPersonal\)[\s\S]*?setActiveOrg\(nextActive\)[\s\S]*?qc\.removeQueries\(\)/,
    )
  })

  it('切空间两条分支各写一次记忆（rememberWorkspace，已校验值）', () => {
    expect(count(mainLayoutSource, /rememberWorkspace\(/g)).toBeGreaterThanOrEqual(2)
    expect(mainLayoutSource).toMatch(/rememberWorkspace\('personal'\)/)
    expect(mainLayoutSource).toMatch(/rememberWorkspace\('enterprise',\s*key\)/)
  })

  it('保留回落防御 effect（qc.removeQueries 出现 ≥2 次）', () => {
    expect(count(mainLayoutSource, /qc\.removeQueries\(\)/g)).toBeGreaterThanOrEqual(2)
  })
})

describe('F2 门控契约（源码级，T-G3）', () => {
  it('MainLayout 消费门控信号：读 workspaceCalibrated 并以 setWorkspaceCalibrated(true) 开闸', () => {
    expect(mainLayoutSource).toMatch(/workspaceCalibrated/)
    expect(mainLayoutSource).toMatch(/setWorkspaceCalibrated\(true\)/)
  })

  it('🔴 开闸顺序：removeQueries 之后才 setWorkspaceCalibrated(true)（写反=静默复现 F2）', () => {
    expect(mainLayoutSource).toMatch(/qc\.removeQueries\(\)[\s\S]*?setWorkspaceCalibrated\(true\)/)
  })

  it('INV-3 护栏：useOrganizations 以无参调用，绝不接门控', () => {
    expect(mainLayoutSource).toMatch(/useOrganizations\(\)/)
    expect(mainLayoutSource).not.toMatch(/useOrganizations\(\s*workspaceCalibrated\s*\)/)
  })

  it('查询门控存在：三个 MainLayout 顶层 hook 以 workspaceCalibrated 派生的开关为 enabled', () => {
    // 业务查询门控开关必须由 workspaceCalibrated 派生（叠加「平台管理员不发业务请求」的排除），
    // 不允许绕过校准直接传字面量 true / 裸调用。
    expect(mainLayoutSource).toMatch(/const businessEnabled = workspaceCalibrated && !isPlatformAdmin/)
    expect(mainLayoutSource).toMatch(/useProjects\(businessEnabled\)/)
    expect(mainLayoutSource).toMatch(/useExternalApps\(businessEnabled\)/)
    expect(mainLayoutSource).toMatch(/useUnreadCount\(workspaceCalibrated\)/)
  })

  it('单调性护栏：MainLayout 中绝不出现 setWorkspaceCalibrated(false)（唯一写 false 的是 store 的 setAuth/logout）', () => {
    expect(mainLayoutSource).not.toMatch(/setWorkspaceCalibrated\(false\)/)
  })

  it('渲染门控三处：Outlet 骨架切换 / 最近项目 / AssistantFab', () => {
    expect(mainLayoutSource).toMatch(/workspaceCalibrated \? <Outlet \/> : <WorkspaceCalibratingSkeleton \/>/)
    expect(mainLayoutSource).toMatch(/workspaceCalibrated && recentProjects\.length > 0/)
    expect(mainLayoutSource).toMatch(/workspaceCalibrated && <AssistantFab \/>/)
  })

  it('fail-open 有界：orgs error 与 6s 超时兜底都在', () => {
    expect(mainLayoutSource).toMatch(/orgsError/)
    expect(mainLayoutSource).toMatch(/6000/)
  })
})
