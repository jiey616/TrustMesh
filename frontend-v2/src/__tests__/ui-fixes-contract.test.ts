/// <reference types="node" />
// 契约测试要读源码做断言，因此显式引入 node 类型（原因同 desktop-notification-contract.test.ts）。
import fs from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

// 三个「静默退化」型 UI 缺陷的契约守卫（2026-09-28）。
//
// 🔴 为什么需要它：这三个问题**不报错、不崩、类型与 lint 全绿**，只是窄一点 / 不关框 / 慢一点。
//   没人会写测试去测「宽度对不对」「要不要自动关」，所以只能靠源码契约把形状钉死。
//
//   1) 任务详情「用户需求」比「执行过程」窄 —— 描述块留在 header 的 flex 行里，
//      被右侧 6 个按钮（终止/执行清单/待确认/交付成果/沉淀为模板/关闭）挤扁。
//   2) 人工确认后抽屉不自动关闭 —— 抽屉压根没有「items 处理完 → 关」的逻辑，
//      空态还留着框，用户会以为「没生效、得再点一次」。
//   3) 切换模型报超时、刷新却已成功 —— **前端与后端两侧超时预算都是 10s**，
//      而该操作要经 NATS 穿透到远端节点并重启其 gateway，生产实测单次 9.97s ⇒ 必然误报。

const ROOT = path.resolve(__dirname, '../../..')
const FE = path.join(ROOT, 'frontend-v2')
const BE = path.join(ROOT, 'backend')

function read(p: string) {
  return fs.readFileSync(p, 'utf8').replace(/\r\n/g, '\n')
}

/** 去掉注释后再断言：避免「注释里提过某个值」让断言假通过。 */
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
}

const WORKSPACE = path.join(FE, 'src/components/task/TaskWorkspace.tsx')
const DRAWER = path.join(FE, 'src/components/task/PendingApprovalsDrawer.tsx')
const AGENTS_API = path.join(FE, 'src/api/agents.ts')
const CAP_TAB = path.join(FE, 'src/components/agent/HermesCapabilityTab.tsx')
const AGENT_GO = path.join(BE, 'internal/handler/agent.go')

/** 截取 Go 的 `func (h *AgentHandler) Name(...) {...}` 源码块。 */
function goFnBlock(source: string, name: string): string {
  const m = source.match(new RegExp(`func \\(h \\*AgentHandler\\) ${name}\\([\\s\\S]*?\\n\\}`))
  if (!m) throw new Error(`go func ${name} not found`)
  return m[0]
}

describe('任务详情 · 描述与执行过程等宽', () => {
  const src = read(WORKSPACE)

  it('🔴 header 必须是 flex-wrap 容器，否则描述块无法换行独占整行', () => {
    expect(src).toMatch(/padding: '10px 16px'[\s\S]{0,160}flexWrap: 'wrap'/)
  })

  it('🔴 标题列必须是 flex:1 + minWidth:0（只写 minWidth:0 会被右侧按钮组挤到很窄）', () => {
    expect(src).toMatch(/\{\{ minWidth: 0, flex: 1 \}\}/)
  })

  it('🔴 描述与附件必须落在 width:100% 的独立块里，且该块不在标题列内', () => {
    const marker = "style={{ width: '100%', minWidth: 0 }}"
    const idx = src.indexOf(marker)
    expect(idx).toBeGreaterThan(-1)
    const block = src.slice(idx, idx + 420)
    expect(block).toMatch(/TaskDescription/)
    expect(block).toMatch(/AttachedFilesSection/)

    // 防回归：这两者不得再出现在 `minWidth: 0, flex: 1` 的标题列内部
    const colIdx = src.indexOf('{{ minWidth: 0, flex: 1 }}')
    const colEnd = src.indexOf('</div>', src.indexOf('<Space size={6}', colIdx))
    expect(src.slice(colIdx, colEnd)).not.toMatch(/TaskDescription/)
  })

  it('执行过程区仍是整宽容器（对照组：描述块要与它对齐）', () => {
    expect(src).toMatch(/ref=\{feedRef\} style=\{\{ flex: 1, overflowY: 'auto', padding: 16/)
  })
})

describe('待确认抽屉 · 全部确认完自动关闭', () => {
  const src = stripComments(read(DRAWER))

  it('🔴 必须有「从有到无」的自动关闭，且用 had > 0 守卫', () => {
    expect(src).toMatch(/hadVisibleRef/)
    expect(src).toMatch(/visible\.length === 0 && had > 0/)
    expect(src).toMatch(/onClose\(\)/)
  })

  it('🔴 守卫写在「只在从有到无那一刻」上：多项时确认一项不关、主动打开空抽屉不闪关', () => {
    const blk = src.match(/useEffect\(\(\) => \{[\s\S]*?\}, \[open, visible\.length, onClose\]\)/)
    expect(blk).not.toBeNull()
    // open=false 时计数归零 ⇒ 下次打开重新计数
    expect(blk![0]).toMatch(/if \(!open\) \{[\s\S]*?hadVisibleRef\.current = 0/)
    // 关闭判定必须在「记录本次项数」之后，且依赖上一次的 had
    const recIdx = blk![0].indexOf('hadVisibleRef.current = visible.length')
    const closeIdx = blk![0].indexOf('onClose()')
    expect(recIdx).toBeGreaterThan(-1)
    expect(closeIdx).toBeGreaterThan(recIdx)
  })

  it('自动关闭不得依赖 dismissed（「暂时忽略」也要能收敛到关闭）', () => {
    const blk = src.match(/useEffect\(\(\) => \{[\s\S]*?\}, \[open, visible\.length, onClose\]\)/)![0]
    expect(blk).not.toMatch(/dismissed/)
  })
})

describe('节点能力写回 · 超时预算必须配对（超时 ≠ 失败）', () => {
  it('🔴 前端该端点必须显式放宽超时（ky 默认 10s 会必然先于后端超时）', () => {
    const src = stripComments(read(AGENTS_API))
    expect(src).toMatch(/CAPABILITY_WRITE_TIMEOUT_MS = 70_000/)
    expect(src).toMatch(/timeout: CAPABILITY_WRITE_TIMEOUT_MS/)
  })

  it('🔴 前端超时必须严格大于后端 ctx，否则前端仍先放弃、用户还是看到超时', () => {
    const feSrc = stripComments(read(AGENTS_API))
    const goSrc = stripComments(read(AGENT_GO))
    const feMs = Number(feSrc.match(/CAPABILITY_WRITE_TIMEOUT_MS = ([\d_]+)/)![1].replace(/_/g, ''))
    const goBlk = goFnBlock(goSrc, 'SetCapabilities')
    const goSec = Number(goBlk.match(/context\.WithTimeout\([^,]+,\s*(\d+)\*time\.Second\)/)![1])
    expect(goSec).toBeGreaterThan(0)
    expect(feMs).toBeGreaterThan(goSec * 1000)
  })

  it('🔴 后端写回 ctx 不得停在 10s（生产实测单次 9.97s，紧贴边界随时提前放弃）', () => {
    const goBlk = goFnBlock(stripComments(read(AGENT_GO)), 'SetCapabilities')
    expect(goBlk).toMatch(/60\*time\.Second/)
    expect(goBlk).not.toMatch(/10\*time\.Second/)
  })

  it('🔴 后端在读路径上仍保持自己的短超时（放宽只针对写回，别一刀切）', () => {
    const goSrc = stripComments(read(AGENT_GO))
    expect(goFnBlock(goSrc, 'GetCapabilities')).toMatch(/10\*time\.Second/)
  })

  it('🔴 超时必须走中性提示而非报错（节点侧很可能已生效，报错会诱导重复提交）', () => {
    const src = stripComments(read(CAP_TAB))
    expect(src).toMatch(/isTimeoutError/)
    const blk = src.match(/function notifyWritebackError\([\s\S]*?\n\}/)![0]
    expect(blk).toMatch(/isTimeoutError\(err\)/)
    expect(blk).toMatch(/message\.warning\(/)
  })

  it('🔴 三个写回入口（新增/切换/删除模型 + 部署技能）都要走同一个超时提示', () => {
    const src = stripComments(read(CAP_TAB))
    for (const label of ['添加模型失败', '切换默认模型失败', '删除模型失败', '部署技能失败']) {
      expect(src).toMatch(new RegExp(`notifyWritebackError\\(err, message, '${label}'\\)`))
    }
  })

  it('🔴 写回失败后要延迟重拉能力（超时那刻节点还在写回，立刻拉只会拿到旧值）', () => {
    const src = stripComments(read(path.join(FE, 'src/hooks/useAgents.ts')))
    const blk = src.match(/export function useSetAgentCapabilities\([\s\S]*?\n\}/)![0]
    expect(blk).toMatch(/onError/)
    expect(blk).toMatch(/setTimeout/)
    expect(blk).toMatch(/invalidateQueries\(\{ queryKey: \['agents', id, 'capabilities'\] \}\)/)
  })
})
