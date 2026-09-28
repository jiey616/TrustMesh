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

  it('🔴 六个写回入口（增/切/删模型、部署技能、cron 操作/新建）都要走同一个超时提示', () => {
    const src = stripComments(read(CAP_TAB))
    for (const label of [
      '添加模型失败',
      '切换默认模型失败',
      '删除模型失败',
      '部署技能失败',
      // 这两个此前漏了，用的是裸 message.error ⇒ 超时会被误报成失败
      '定时任务操作失败',
      '创建定时任务失败',
    ]) {
      expect(src).toMatch(new RegExp(`notifyWritebackError\\(err, message, '${label}'\\)`))
    }
  })

  it('🔴 写回结束后必须重拉能力，且用 onSettled 而非 onSuccess', () => {
    const src = stripComments(read(path.join(FE, 'src/hooks/useAgents.ts')))
    const blk = src.match(/export function useSetAgentCapabilities\([\s\S]*?\n\}/)![0]
    expect(blk).toMatch(/onSettled/)
    expect(blk).toMatch(/invalidateQueries\(\{ queryKey: \['agents', id, 'capabilities'\] \}\)/)
    // 契约里写回**失败也返 HTTP 200**（handler/agent.go 的 SetCapabilities），
    // 所以 onSuccess/onError 的划分本身就是错的判据 —— 上一版正是因此，
    // 那个「延迟 5s 重拉」的 onError 从未执行过一次。
    expect(blk).not.toMatch(/onSuccess/)
    expect(blk).not.toMatch(/setTimeout/)
  })
})

// ─────────────────────────────────────────────────────────────────────
// 第四轮（2026-09-28 下午）：写回成功了，界面却渲染成错误页。
//
// 生产实证链路：POST 写回 25.5s → 返回 200 + ok=true + restartStatus=restarted
//（绿色「默认模型已切换，已同步最新状态」）→ 前端 onSuccess **立即**重读能力
// → 撞上 gateway 刚重启完的恢复窗口 → 读 3s 超时（CLAWSYNAPSE_TIMEOUT 默认值）
// → 后端按契约返 200 + available:false → 旧代码直接 `if (!data?.available) return <Empty>`
// ⇒ **整页模型列表被清空成一段 Go 原始报错**。而且 retry:false，不会自愈。
//
// 所以这一轮钉的不是「某个数字」，而是三个判据：
//   ① 业务失败必须翻译成查询失败（否则 retry / 保留旧数据 / isError 全都拿不到）；
//   ② 有旧数据时只挂横幅、绝不清空；
//   ③ 25.5s 的写回期间界面必须有可见进度，且切换动作要有乐观反馈。
// ─────────────────────────────────────────────────────────────────────
describe('节点能力读失败 · 降级而非清空（写回会重启 gateway）', () => {
  const hook = stripComments(read(path.join(FE, 'src/hooks/useAgents.ts')))
  const tab = stripComments(read(CAP_TAB))

  it('🔴 必须把「HTTP 200 + available:false」翻译成查询失败，否则 retry 永远不触发', () => {
    expect(hook).toMatch(/if \(!res\.data\?\.available\)/)
    expect(hook).toMatch(/throw new Error\(res\.data\?\.reason/)
  })

  it('🔴 读失败要有指数退避重试，覆盖 gateway 重启后的恢复窗口', () => {
    expect(hook).toMatch(/retry: 5/)
    expect(hook).toMatch(/retryDelay: \(attempt\) => Math\.min\(1000 \* 2 \*\* attempt, 8_000\)/)
  })

  it('🔴 三个 Tab 都不得再「读失败即整页清空」，只能在没有旧数据时兜底', () => {
    const fallbacks = tab.match(/if \(!data\) return <CapabilityUnavailable/g) ?? []
    expect(fallbacks.length).toBe(3)
    // 反向断言：旧的「清空成错误页」写法必须彻底消失
    expect(tab).not.toMatch(/data\?\.available/)
    expect(tab).not.toMatch(/能力信息暂不可用/)
  })

  it('🔴 有旧数据时只能挂顶部横幅（一次 3 秒抖动不该以整页消失为代价）', () => {
    // 注意：ModelsTab 里横幅落在三元表达式的 else 分支，前缀是 `(` 而不是 `{`，
    // 所以正则不能带花括号 —— 带了只会数到 2 处，把正确的代码判成漏改。
    expect(tab).toMatch(/isError && <CapabilityStaleBanner/)
    // ModelsTab 的条件额外带上「乐观标记未撤」（见第五轮 describe），Skills / Jobs 保持原样。
    expect(tab).toMatch(/\(isError \|\| !!pendingModelId\) && <CapabilityStaleBanner/)
    const banners = tab.match(/<CapabilityStaleBanner/g) ?? []
    expect(banners.length).toBe(3)
    const blk = tab.match(/function CapabilityStaleBanner\([\s\S]*?\n\}/)![0]
    expect(blk).toMatch(/最近一次成功读取的数据/)
  })

  it('🔴 原始错误串必须收进折叠区，不得直接铺给用户', () => {
    const detail = tab.match(/function CapabilityErrorDetail\([\s\S]*?\n\}/)![0]
    expect(detail).toMatch(/技术详情/)
    expect(detail).toMatch(/useState\(false\)/) // 默认收起
    const unavail = tab.match(/function CapabilityUnavailable\([\s\S]*?\n\}/)![0]
    expect(unavail).toMatch(/节点正在重启 gateway/) // 正文是人话
    expect(unavail).toMatch(/<CapabilityErrorDetail reason=\{reason\} \/>/) // 原文只经折叠区
  })

  it('🔴 写回期间必须有可见进度（25.5s 里界面不能毫无变化）', () => {
    expect(tab).toMatch(/function WritebackProgressBanner/)
    expect(tab).toMatch(/实测约 25 秒/)
    // 「切换默认模型」是内联操作、没有 Modal 的 confirmLoading，必须挂横幅
    expect(tab).toMatch(
      /<WritebackProgressBanner label=\{pendingModelId \? '切换默认模型' : '写回模型配置'\} \/>/,
    )
  })

  it('🔴 切换默认模型要给乐观反馈，且写回期间旧的「默认」标签要被摘掉', () => {
    expect(tab).toMatch(/const \[requestedModelId, setRequestedModelId\] = useState<string \| null>\(null\)/)
    expect(tab).toMatch(/setRequestedModelId\(modelId\)/)
    expect(tab).toMatch(/m\.isDefault && !pendingModelId/)
    expect(tab).toMatch(/\(m\.id \?\? m\.model\) === pendingModelId/)
  })
})

// ─────────────────────────────────────────────────────────────────────
// 第五轮（2026-09-28 傍晚）：切换成功了，界面却先把「默认」弹回旧模型。
//
// 生产实证（最近 70 分钟内同一 agent 的 9 次切换，逐条对齐日志）：
//   POST 25.5s 返回 → 0.07s 后触发 GET#1，**必被后端 3s 客户端超时打掉**（239B 失败体）
//   → +4.1s 后 GET#2 才发出、+4.4~6.5s 才拿到数据
//   （其中 6 次是 0.31~0.61s 的节点缓存命中、3 次是 2.1~2.5s 的真重算）。
// 旧代码却在 `finally` 里清 pendingModelId —— 那正是 **POST 返回的瞬间**，
// 此时 RQ 缓存里仍是**写回前**的那份 payload ⇒「默认」标签啪地弹回旧模型，
// 4~7 秒后才自己跳到新模型；一旦那几次重试全部失败，它会**永久**停在旧值
// ⇒ 用户看到的「切换完成之后页面上默认模型显示的还是老的，只有刷新页面才更新为新的」。
//
// 所以这一轮钉的不是某个数字，而是**乐观标记的生命周期**：
//   ① 不能「先置一份 state、再事后清理」（`finally` 或 effect 都一样 —— 前者太早、
//      后者会触犯 react-hooks/set-state-in-effect），必须由服务端数据**纯推导**；
//   ② 必须有兜底，否则既读不到确认也不报错时会永久卡在「同步中」；
//   ③ 同步窗口内要禁止重复提交。
// ─────────────────────────────────────────────────────────────────────
describe('切换默认模型 · 乐观标记必须活到服务端确认', () => {
  const tab = stripComments(read(CAP_TAB))
  const modelsTab = tab.match(/function HermesModelsTab\([\s\S]*?\n\}\n/)![0]

  it('🔴 不得在 finally 里撤标记（那等于在 POST 返回那一刻撤回，比新数据早 4~7 秒）', () => {
    expect(modelsTab).not.toMatch(/finally/)
  })

  it('🔴 标记必须是「服务端是否已确认」的纯推导，而不是一份事后清理的 state', () => {
    // 推导表达式：点过的模型在服务端数据里还不是默认 ⇒ 仍在同步中
    expect(modelsTab).toMatch(
      /requestedModelId && !models\.some\(\(m\) => \(m\.id \?\? m\.model\) === requestedModelId && m\.isDefault\)/,
    )
    // 反向断言：旧的「一份 state + 事后清理」写法必须彻底消失
    expect(modelsTab).toMatch(/const \[requestedModelId, setRequestedModelId\] = useState<string \| null>\(null\)/)
    expect(modelsTab).not.toMatch(/setPendingModelId/)
    expect(modelsTab).not.toMatch(/pendingConfirmed/)
  })

  it('🔴 必须有兜底，避免既读不到新状态也不报错时永久卡在「同步中」', () => {
    expect(modelsTab).toMatch(/window\.setTimeout/)
    expect(modelsTab).toMatch(/setRequestedModelId\(null\)/)
    expect(modelsTab).toMatch(/void refetch\(\)/)
    expect(modelsTab).toMatch(/window\.clearTimeout/)
  })

  it('🔴 同步窗口内必须禁用其它行的「设为默认 / 删除」，否则可以重复提交', () => {
    const dis = modelsTab.match(/disabled=\{setCapabilities\.isPending \|\| !!pendingModelId\}/g) ?? []
    expect(dis.length).toBe(2)
  })

  it('🔴 同步窗口内要挂同步中横幅（POST 已返回但服务端还没确认）', () => {
    expect(modelsTab).toMatch(/\(isError \|\| !!pendingModelId\) && <CapabilityStaleBanner/)
  })
})
