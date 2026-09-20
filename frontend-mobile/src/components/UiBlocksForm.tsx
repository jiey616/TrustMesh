import { useState } from 'react'
import { Button, TextArea } from 'antd-mobile'
import type { UIBlock, UIBlockResponse } from '@/types'

/**
 * PM 规划澄清的动态表单（移动端精简版）。
 *
 * 与桌面端 `UIResponsePanel` 同口径：答案按 block.id 汇总成
 * `{ blocks: { [id]: { selected | text | confirmed } } }`，连同正文一起回给 PM。
 * 移动端只支持 single_select / text_input / confirm / info 四类，够覆盖实际使用。
 */
export function UiBlocksForm({
  blocks,
  submitting,
  onSubmit,
}: {
  blocks: UIBlock[]
  submitting: boolean
  onSubmit: (content: string, uiResponse: { blocks: Record<string, UIBlockResponse> }) => void
}) {
  const [selected, setSelected] = useState<Record<string, string[]>>({})
  const [texts, setTexts] = useState<Record<string, string>>({})
  const [confirmed, setConfirmed] = useState<Record<string, boolean>>({})

  function buildResponse() {
    const out: Record<string, UIBlockResponse> = {}
    for (const b of blocks) {
      if (b.type === 'single_select') out[b.id] = { selected: selected[b.id] ?? [] }
      else if (b.type === 'text_input') out[b.id] = { text: texts[b.id] ?? '' }
      else if (b.type === 'confirm') out[b.id] = { confirmed: confirmed[b.id] ?? false }
    }
    return out
  }

  function buildContent() {
    return blocks
      .map((b) => {
        if (b.type === 'single_select') return `${b.label}：${(selected[b.id] ?? []).join('、')}`
        if (b.type === 'text_input') return `${b.label}：${texts[b.id] ?? ''}`
        if (b.type === 'confirm') return `${b.label}：${confirmed[b.id] ? '是' : '否'}`
        return ''
      })
      .filter(Boolean)
      .join('\n')
  }

  return (
    <div className="flex flex-col gap-3">
      {blocks.map((b) => (
        <div key={b.id}>
          {b.type === 'info' ? (
            <p className="text-[13px] leading-relaxed text-[var(--tm-text-2)]">{b.content ?? b.label}</p>
          ) : (
            <>
              <p className="mb-[6px] text-[14px] font-medium">{b.label}</p>

              {b.type === 'single_select' ? (
                <div className="flex flex-wrap gap-2">
                  {(b.options ?? []).map((opt) => {
                    const on = (selected[b.id] ?? [])[0] === opt.value
                    return (
                      <button
                        key={opt.value}
                        type="button"
                        onClick={() => setSelected((s) => ({ ...s, [b.id]: [opt.value] }))}
                        className="rounded-[8px] border px-3 py-[6px] text-[13px]"
                        style={{
                          color: on ? 'var(--tm-brand)' : 'var(--tm-text-2)',
                          borderColor: on ? 'var(--tm-brand)' : 'var(--tm-line)',
                          background: on ? 'var(--tm-brand-soft)' : '#fff',
                        }}
                      >
                        {opt.label}
                      </button>
                    )
                  })}
                </div>
              ) : null}

              {b.type === 'text_input' ? (
                <TextArea
                  placeholder={b.placeholder ?? '请输入'}
                  value={texts[b.id] ?? ''}
                  onChange={(v) => setTexts((s) => ({ ...s, [b.id]: v }))}
                  rows={2}
                />
              ) : null}

              {b.type === 'confirm' ? (
                <div className="flex gap-2">
                  {['是', '否'].map((label, idx) => {
                    const on = (confirmed[b.id] ?? false) === (idx === 0)
                    return (
                      <button
                        key={label}
                        type="button"
                        onClick={() => setConfirmed((s) => ({ ...s, [b.id]: idx === 0 }))}
                        className="rounded-[8px] border px-4 py-[6px] text-[13px]"
                        style={{
                          color: on ? 'var(--tm-brand)' : 'var(--tm-text-2)',
                          borderColor: on ? 'var(--tm-brand)' : 'var(--tm-line)',
                          background: on ? 'var(--tm-brand-soft)' : '#fff',
                        }}
                      >
                        {label}
                      </button>
                    )
                  })}
                </div>
              ) : null}
            </>
          )}
        </div>
      ))}

      <Button
        block
        size="small"
        color="primary"
        loading={submitting}
        onClick={() => onSubmit(buildContent(), { blocks: buildResponse() })}
      >
        提交
      </Button>
    </div>
  )
}
