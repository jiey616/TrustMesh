import { Collapse, Table, Tag, Typography } from 'antd'
import { RightOutlined } from '@ant-design/icons'
import type { ManualBlock, ManualBlockData } from '@/api/platformManual'
import { resolveAssetUrl } from '@/lib/assetUrl'

const { Paragraph, Title, Text } = Typography

/**
 * 内联标记渲染：只支持 **加粗**。
 *
 * 🔴 刻意用「拆分 + React 节点」而非 dangerouslySetInnerHTML：
 * 手册内容由管理员自由编辑，绝不能把用户输入当 HTML 注入 DOM。
 * 支持能力刻意收窄到加粗一种，收益是注入面为零。
 */
function renderInline(text: string, keyPrefix: string) {
  const parts = text.split(/(\*\*[^*]+\*\*)/g)
  return parts.map((p, i) => {
    if (p.startsWith('**') && p.endsWith('**') && p.length > 4) {
      return (
        <Text strong key={`${keyPrefix}-${i}`}>
          {p.slice(2, -2)}
        </Text>
      )
    }
    return <span key={`${keyPrefix}-${i}`}>{p}</span>
  })
}

/** 带序号的步骤列表：平台风格（方形序号 + 细线连接），不用 antd Steps（其默认样式过于「向导化」）。 */
function Steps({ items }: { items: string[] }) {
  return (
    <ol style={{ listStyle: 'none', margin: '12px 0', padding: 0 }}>
      {items.map((it, i) => (
        <li
          key={i}
          style={{ display: 'flex', gap: 12, alignItems: 'flex-start', paddingBottom: i === items.length - 1 ? 0 : 12 }}
        >
          <span
            style={{
              flex: '0 0 auto',
              width: 22,
              height: 22,
              marginTop: 1,
              borderRadius: 'var(--radius-control)',
              background: 'var(--signal-soft)',
              border: '1px solid var(--signal-border)',
              color: 'var(--signal)',
              fontSize: 12,
              fontWeight: 600,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            {i + 1}
          </span>
          <span style={{ color: 'var(--text-secondary)', lineHeight: 1.75, flex: 1 }}>
            {renderInline(it, `st-${i}`)}
          </span>
        </li>
      ))}
    </ol>
  )
}

/** 流程链：横向箭头连接，窄屏自动换行。 */
function Flow({ items }: { items: string[] }) {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, margin: '12px 0' }}>
      {items.map((it, i) => (
        <span key={i} style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <span
            style={{
              padding: '5px 12px',
              borderRadius: 'var(--radius-control)',
              background: 'var(--surface-inset)',
              border: '1px solid var(--line)',
              color: 'var(--text-secondary)',
              fontSize: 13,
            }}
          >
            {it}
          </span>
          {i < items.length - 1 && <RightOutlined style={{ fontSize: 10, color: 'var(--text-quaternary)' }} />}
        </span>
      ))}
    </div>
  )
}

/** 提示 / 警示条：左侧 2px 信号线 + 淡底，不使用 antd Alert（其图标与留白偏重）。 */
function Callout({ kind, text }: { kind: 'tip' | 'warn'; text: string }) {
  const accent = kind === 'warn' ? 'var(--warning)' : 'var(--info)'
  const label = kind === 'warn' ? '注意' : '提示'
  return (
    <div
      style={{
        display: 'flex',
        gap: 10,
        margin: '12px 0',
        padding: '10px 14px',
        borderLeft: `2px solid ${accent}`,
        borderRadius: 'var(--radius-control)',
        background: 'var(--surface-inset)',
      }}
    >
      <Tag
        style={{
          marginInlineEnd: 0,
          flex: '0 0 auto',
          border: 'none',
          background: 'transparent',
          color: accent,
          paddingInline: 0,
          fontSize: 12,
          fontWeight: 600,
        }}
      >
        {label}
      </Tag>
      <span style={{ color: 'var(--text-secondary)', lineHeight: 1.75, flex: 1 }}>
        {renderInline(text, 'co')}
      </span>
    </div>
  )
}

function QaList({ pairs }: { pairs: ManualBlockData['qa_pairs'] }) {
  const items = (pairs ?? [])
    .filter((p) => p && (p.q || p.a))
    .map((p, i) => ({
      key: String(i),
      label: <span style={{ fontWeight: 500 }}>{p.q}</span>,
      children: <span style={{ color: 'var(--text-secondary)', lineHeight: 1.75 }}>{renderInline(p.a, `qa-${i}`)}</span>,
    }))
  if (!items.length) return null
  return <Collapse ghost items={items} style={{ margin: '8px 0' }} />
}

function ManualTable({ head, rows }: { head?: string[]; rows?: string[][] }) {
  const cols = head ?? []
  if (!cols.length) return null
  return (
    <Table
      size="small"
      bordered={false}
      pagination={false}
      style={{ margin: '12px 0' }}
      columns={cols.map((h, i) => ({
        title: h,
        dataIndex: String(i),
        key: String(i),
        render: (v: string) => (v ? renderInline(v, `td-${i}`) : <Text type="secondary">—</Text>),
      }))}
      dataSource={(rows ?? []).map((r, ri) => {
        const rec: Record<string, string> = { key: String(ri) }
        cols.forEach((_, ci) => {
          rec[String(ci)] = r[ci] ?? ''
        })
        return rec
      })}
      scroll={{ x: 'max-content' }}
    />
  )
}

function ImageBlock({ d }: { d: ManualBlockData }) {
  if (!d.url) return null
  return (
    <figure style={{ margin: '16px 0' }}>
      <img
        // 🔴 必须过 resolveAssetUrl：库里的 url 是根相对路径 `/api/v1/manual/images/{id}`，
        // 桌面端页面 origin 是 file:// ⇒ 不解析就成 file:///api/v1/... 必裂。
        src={resolveAssetUrl(d.url)}
        alt={d.alt || d.caption || ''}
        loading="lazy"
        style={{
          display: 'block',
          maxWidth: '100%',
          borderRadius: 'var(--radius-control)',
          border: '1px solid var(--line)',
        }}
      />
      {d.caption && (
        <figcaption
          style={{ marginTop: 8, fontSize: 12, color: 'var(--text-tertiary)', textAlign: 'center' }}
        >
          {d.caption}
        </figcaption>
      )}
    </figure>
  )
}

/** 单个内容块渲染。未知 type 返回 null（向前兼容，绝不抛错）。 */
export function ManualBlockView({ block, prefix }: { block: ManualBlock; prefix: string }) {
  const d = block.data ?? {}
  switch (block.type) {
    case 'paragraph':
      return (
        <Paragraph style={{ color: 'var(--text-secondary)', lineHeight: 1.85, margin: '10px 0' }}>
          {renderInline(d.text ?? '', prefix)}
        </Paragraph>
      )
    case 'heading':
      return (
        <Title level={5} style={{ margin: '22px 0 8px', fontWeight: 600 }}>
          {d.text}
        </Title>
      )
    case 'steps':
      return <Steps items={(d.items ?? []).filter(Boolean)} />
    case 'flow':
      return <Flow items={(d.items ?? []).filter(Boolean)} />
    case 'tip':
      return <Callout kind="tip" text={d.text ?? ''} />
    case 'warn':
      return <Callout kind="warn" text={d.text ?? ''} />
    case 'qa':
      return <QaList pairs={d.qa_pairs} />
    case 'table':
      return <ManualTable head={d.head} rows={d.rows} />
    case 'image':
      return <ImageBlock d={d} />
    default:
      return null
  }
}
