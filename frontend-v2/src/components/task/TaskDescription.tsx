import { useEffect, useRef, useState } from 'react'
import { DownOutlined } from '@ant-design/icons'
import { Markdown } from './Markdown'

const COLLAPSED_HEIGHT_PX = 48

interface TaskDescriptionProps {
  description: string
}

/** 任务描述 markdown 渲染，超长可展开/收起 */
export function TaskDescription({ description }: TaskDescriptionProps) {
  const [expanded, setExpanded] = useState(false)
  const [overflows, setOverflows] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = contentRef.current
    if (!el) return
    setOverflows(el.scrollHeight > COLLAPSED_HEIGHT_PX + 4)
  }, [description])

  return (
    <div style={{ marginTop: 6 }}>
      <div
        ref={contentRef}
        style={{
          fontSize: 13,
          color: 'var(--text-secondary)',
          overflow: 'hidden',
          transition: 'max-height 0.2s',
          maxHeight: !expanded && overflows ? COLLAPSED_HEIGHT_PX : undefined,
        }}
      >
        <Markdown content={description} size="small" />
      </div>
      {overflows && (
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 4,
            fontSize: 13,
            color: 'var(--text-tertiary)',
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            marginTop: 4,
            padding: 0,
            fontFamily: 'inherit',
          }}
        >
          {expanded ? '收起' : '展开'}
          <DownOutlined style={{ fontSize: 12, transform: expanded ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s' }} />
        </button>
      )}
    </div>
  )
}
