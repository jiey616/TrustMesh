import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button, Empty, Input, Skeleton, Spin, Typography } from 'antd'
import { EditOutlined, SearchOutlined } from '@ant-design/icons'
import { usePermStore } from '@/stores/permStore'
import { getManual, getPlatformManual } from '@/api/platformManual'
import type { ManualSection, PlatformManual } from '@/api/platformManual'
import { ManualBlockView } from './ManualBlocks'
import { ManualEditor } from './ManualEditor'

const { Title, Text, Paragraph } = Typography

/** 章节滚动定位：顶栏 64px + 呼吸留白。 */
const SCROLL_OFFSET = 84

/**
 * 左侧目录 + 右侧正文的阅读视图。
 *
 * 滚动定位用**容器内 scrollTo**而非锚点跳转：#/manual 在 Electron 下是 HashRouter，
 * 改 location.hash 会破坏路由；且正文容器自身滚动，非 window 滚动。
 * 因此用「元素 offsetTop 相对容器」计算，避免依赖 scrollIntoView 的滚动祖先推断。
 */
function ManualView({ manual }: { manual: PlatformManual }) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({})
  const [activeId, setActiveId] = useState<string>('')
  const [keyword, setKeyword] = useState('')

  // sections 每次渲染都新建数组会让下面 useMemo/useEffect 的依赖恒变 → 自身先 memo 住。
  const sections = useMemo(() => manual.sections ?? [], [manual.sections])

  // 关键词过滤：命中标题/导语/任意块文本即保留该章。空关键词 = 全量。
  const filtered = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    if (!kw) return sections
    const hit = (s: ManualSection) => {
      const hay = [
        s.title,
        s.lead ?? '',
        ...(s.blocks ?? []).flatMap((b) => {
          const d = b.data ?? {}
          return [
            d.text ?? '',
            ...(d.items ?? []),
            d.caption ?? '',
            d.alt ?? '',
            ...(d.head ?? []),
            ...((d.rows ?? []).flat() as string[]),
            ...((d.qa_pairs ?? []).flatMap((p) => [p.q, p.a])),
          ]
        }),
      ]
        .join('\n')
        .toLowerCase()
      return hay.includes(kw)
    }
    return sections.filter(hit)
  }, [sections, keyword])

  // 滚动高亮：取「最后一个顶部已越过基准线」的章节。
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const onScroll = () => {
      let cur = ''
      for (const s of sections) {
        const node = sectionRefs.current[s.id]
        if (!node) continue
        if (node.offsetTop - el.scrollTop <= SCROLL_OFFSET) cur = s.id
        else break
      }
      setActiveId(cur)
    }
    onScroll()
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [sections, filtered])

  const jumpTo = useCallback((id: string) => {
    const el = scrollRef.current
    const node = sectionRefs.current[id]
    if (!el || !node) return
    el.scrollTo({ top: Math.max(0, node.offsetTop - SCROLL_OFFSET + 16), behavior: 'smooth' })
  }, [])

  return (
    <div style={{ display: 'flex', gap: 24, height: '100%', minHeight: 0 }}>
      {/* ── 左：目录 ── */}
      <aside
        style={{
          flex: '0 0 216px',
          display: 'flex',
          flexDirection: 'column',
          gap: 10,
          minHeight: 0,
        }}
      >
        <Input
          allowClear
          size="small"
          prefix={<SearchOutlined style={{ color: 'var(--text-quaternary)' }} />}
          placeholder="搜索章节内容"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <nav style={{ flex: 1, minHeight: 0, overflowY: 'auto', paddingRight: 4 }}>
          {filtered.map((s, i) => {
            const on = activeId === s.id
            return (
              <button
                key={s.id}
                type="button"
                onClick={() => jumpTo(s.id)}
                style={{
                  display: 'block',
                  width: '100%',
                  textAlign: 'left',
                  padding: '7px 10px',
                  marginBottom: 2,
                  border: 'none',
                  borderLeft: `2px solid ${on ? 'var(--signal)' : 'transparent'}`,
                  borderRadius: 'var(--radius-control)',
                  background: on ? 'var(--signal-soft)' : 'transparent',
                  color: on ? 'var(--signal)' : 'var(--text-tertiary)',
                  fontSize: 13,
                  lineHeight: 1.5,
                  cursor: 'pointer',
                  transition: 'background .15s, color .15s',
                }}
              >
                <span style={{ marginRight: 6, fontVariantNumeric: 'tabular-nums', opacity: 0.6 }}>
                  {i + 1}
                </span>
                {s.title}
              </button>
            )
          })}
          {!filtered.length && (
            <Text type="secondary" style={{ fontSize: 12 }}>
              无匹配章节
            </Text>
          )}
        </nav>
      </aside>

      {/* ── 右：正文 ── */}
      <div
        ref={scrollRef}
        style={{
          flex: 1,
          minWidth: 0,
          minHeight: 0,
          overflowY: 'auto',
          paddingRight: 8,
        }}
      >
        <header style={{ marginBottom: 28 }}>
          <Title level={3} style={{ margin: 0 }}>
            {manual.title}
          </Title>
          {manual.subtitle && (
            <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
              {manual.subtitle}
            </Paragraph>
          )}
        </header>

        {filtered.map((s, si) => (
          <section
            key={s.id}
            id={`manual-sec-${s.id}`}
            ref={(n) => {
              sectionRefs.current[s.id] = n
            }}
            style={{
              paddingTop: si === 0 ? 0 : 26,
              marginTop: si === 0 ? 0 : 26,
              borderTop: si === 0 ? 'none' : '1px solid var(--line)',
            }}
          >
            <Title level={4} style={{ margin: 0 }}>
              <span
                style={{
                  display: 'inline-block',
                  marginRight: 10,
                  color: 'var(--signal)',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {String(si + 1).padStart(2, '0')}
              </span>
              {s.title}
            </Title>
            {s.lead && (
              <Paragraph style={{ marginTop: 8, marginBottom: 0, color: 'var(--text-tertiary)' }}>
                {s.lead}
              </Paragraph>
            )}
            <div>
              {(s.blocks ?? []).map((b, bi) => (
                <ManualBlockView key={`${s.id}-${bi}`} block={b} prefix={`${s.id}-${bi}`} />
              ))}
            </div>
          </section>
        ))}

        {manual.footer && (
          <footer
            style={{
              marginTop: 40,
              paddingTop: 16,
              borderTop: '1px solid var(--line)',
              color: 'var(--text-quaternary)',
              fontSize: 12,
            }}
          >
            {manual.footer}
          </footer>
        )}
      </div>
    </div>
  )
}

export function ManualPage() {
  const isPlatformAdmin = usePermStore((s) => s.isPlatformAdmin)
  const permReady = usePermStore((s) => s.ready)
  const [editing, setEditing] = useState(false)

  const { data, isLoading, error } = useQuery({
    // 🔴 读接口必须按身份分流（与写路径 savePlatformManual 保持一致）：
    //   - 普通用户 → GET /manual          （登录基础权限）
    //   - 平台管理员 → GET /platform/manual（platform.guide.mgr）
    // 不能对所有人一律用 /manual：本部署设了 PLATFORM_ADMIN_EMAILS（种子模式），
    // 后端会**刻意反向拒绝**平台管理员访问业务 API，返回
    //   403 "platform admin account cannot access business APIs; use /api/v1/platform/*"
    // ⇒ 管理员一进页面就 403 → 误显示「手册加载失败」（实为权限模型的预期行为）。
    queryKey: ['manual', isPlatformAdmin ? 'platform' : 'user'],
    queryFn: isPlatformAdmin ? getPlatformManual : getManual,
    enabled: permReady,
  })

  if (!permReady) return <Spin />

  const manual = data?.data.manual ?? null

  if (editing) {
    return <ManualEditor initial={manual} onClose={() => setEditing(false)} />
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div
        style={{
          flexShrink: 0,
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: 16,
        }}
      >
        <div>
          <Title level={4} style={{ margin: 0 }}>
            平台操作手册
          </Title>
          <Text type="secondary">
            平台功能与操作说明
            {manual?.updated_at ? ` · 更新于 ${manual.updated_at.slice(0, 10)}` : ''}
            {isPlatformAdmin ? ' · 内容由平台管理员维护' : ''}
          </Text>
        </div>
        <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
          {isPlatformAdmin && (
            <Button type="primary" icon={<EditOutlined />} onClick={() => setEditing(true)} disabled={isLoading}>
              编辑手册
            </Button>
          )}
        </div>
      </div>

      <div style={{ flex: 1, minHeight: 0 }}>
        {isLoading ? (
          <Skeleton active paragraph={{ rows: 8 }} />
        ) : error ? (
          <Empty description="手册加载失败" />
        ) : !manual || !(manual.sections ?? []).length ? (
          <Empty
            description={
              isPlatformAdmin ? '手册尚未创建，点右上角「编辑手册」开始撰写' : '手册尚未创建，请联系平台管理员'
            }
          />
        ) : (
          <ManualView manual={manual} />
        )}
      </div>
    </div>
  )
}
