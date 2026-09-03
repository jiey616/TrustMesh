import type { UIBlock, UIBlockResponse } from '@/types'
import { Markdown } from './Markdown'

interface UIBlockRendererProps {
  blocks: UIBlock[]
  /** 只读模式：已回复的历史消息 */
  responses?: Record<string, UIBlockResponse>
}

/** 只读渲染 PM 消息中的 ui_blocks（在消息气泡内使用）。 */
export function UIBlockRenderer({ blocks, responses }: UIBlockRendererProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginTop: 10, paddingTop: 10, borderTop: '1px solid var(--line)' }}>
      {blocks.map((block) => {
        const response = responses?.[block.id]
        switch (block.type) {
          case 'single_select':
            return <SelectBlockReadonly key={block.id} block={block} response={response} />
          case 'text_input':
            return <TextInputBlockReadonly key={block.id} block={block} response={response} />
          case 'confirm':
            return <ConfirmBlockReadonly key={block.id} block={block} response={response} />
          case 'info':
            return <InfoBlockReadonly key={block.id} block={block} />
          default:
            return null
        }
      })}
    </div>
  )
}

function BlockLabel({ label }: { label: string }) {
  return <div style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-tertiary)', marginBottom: 6 }}>{label}</div>
}

function SelectBlockReadonly({ block, response }: { block: UIBlock; response?: UIBlockResponse }) {
  const selected = response?.selected ?? []
  return (
    <div>
      <BlockLabel label={block.label} />
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {block.options?.map((opt) => {
          const isSelected = selected.includes(opt.value)
          return (
            <span
              key={opt.value}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 4,
                borderRadius: 'var(--radius-pill)',
                padding: '2px 10px',
                fontSize: 12,
                background: isSelected ? 'rgba(109,95,245,0.18)' : 'rgba(255,255,255,0.06)',
                color: isSelected ? '#8b7ff8' : 'rgba(255,255,255,0.55)',
                border: `1px solid ${isSelected ? 'rgba(109,95,245,0.5)' : 'rgba(255,255,255,0.08)'}`,
                fontWeight: isSelected ? 500 : 400,
              }}
            >
              {isSelected && '✓'}
              {opt.label}
            </span>
          )
        })}
      </div>
    </div>
  )
}

function TextInputBlockReadonly({ block, response }: { block: UIBlock; response?: UIBlockResponse }) {
  const text = response?.text
  return (
    <div>
      <BlockLabel label={block.label} />
      {text ? (
        <div style={{ fontSize: 12, background: 'var(--surface-raised)', borderRadius: 'var(--radius-control)', padding: '6px 10px', color: 'var(--text-primary)' }}>
          {text}
        </div>
      ) : (
        <div style={{ fontSize: 12, fontStyle: 'italic', color: 'var(--text-quaternary)' }}>{block.placeholder ?? '未填写'}</div>
      )}
    </div>
  )
}

function ConfirmBlockReadonly({ block, response }: { block: UIBlock; response?: UIBlockResponse }) {
  const confirmed = response?.confirmed
  return (
    <div>
      <BlockLabel label={block.label} />
      {confirmed != null ? (
        <span
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            borderRadius: 'var(--radius-pill)',
            padding: '2px 10px',
            fontSize: 12,
            fontWeight: 500,
            background: confirmed ? 'rgba(16,185,129,0.15)' : 'rgba(245,158,11,0.15)',
            color: confirmed ? '#34d399' : '#fbbf24',
            border: `1px solid ${confirmed ? 'rgba(16,185,129,0.4)' : 'rgba(245,158,11,0.4)'}`,
          }}
        >
          {confirmed ? (block.confirm_label ?? '已确认') : (block.cancel_label ?? '已取消')}
        </span>
      ) : (
        <div style={{ display: 'flex', gap: 6 }}>
          <span style={{ borderRadius: 'var(--radius-pill)', padding: '2px 10px', fontSize: 12, background: 'var(--surface-raised)', color: 'var(--text-tertiary)' }}>
            {block.confirm_label ?? '确认'}
          </span>
          <span style={{ borderRadius: 'var(--radius-pill)', padding: '2px 10px', fontSize: 12, background: 'var(--surface-raised)', color: 'var(--text-tertiary)' }}>
            {block.cancel_label ?? '取消'}
          </span>
        </div>
      )}
    </div>
  )
}

function InfoBlockReadonly({ block }: { block: UIBlock }) {
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
        <span style={{ color: 'var(--info)', fontSize: 12 }}>ℹ</span>
        <span style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-tertiary)' }}>{block.label}</span>
      </div>
      {block.content && (
        <div style={{ fontSize: 12, background: 'rgba(59,130,246,0.06)', borderRadius: 'var(--radius-control)', padding: '6px 10px' }}>
          <Markdown content={block.content} size="small" />
        </div>
      )}
    </div>
  )
}
