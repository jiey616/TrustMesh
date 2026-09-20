import { useMemo, useState } from 'react'
import type { TaskEvent } from '@/types'

// 对齐桌面端 TaskWorkspace 的执行过程事件口径（icon 换成移动端彩点形态）。
// 后端实际事件类型多于此处列举，未覆盖类型走 fallback（label 用原始 event_type）。
const EVENT_META: Record<string, { label: string; color: string }> = {
  task_created: { label: '任务创建', color: 'var(--tm-brand)' },
  task_plan_ready: { label: '规划完成', color: 'var(--tm-brand)' },
  task_status_changed: { label: '状态变更', color: 'var(--tm-warn)' },
  todo_assigned: { label: '分配步骤', color: 'var(--tm-brand)' },
  todo_started: { label: '开始执行', color: 'var(--tm-brand)' },
  todo_progress: { label: '执行中', color: 'var(--tm-brand)' },
  todo_completed: { label: '步骤完成', color: 'var(--tm-ok)' },
  todo_failed: { label: '步骤失败', color: 'var(--tm-danger)' },
  task_comment: { label: '评论', color: 'var(--tm-text-3)' },
  planning_reply: { label: 'PM 规划回复', color: 'var(--tm-brand)' },
  agent_status_changed: { label: '数字员工状态', color: 'var(--tm-warn)' },
  artifact_received: { label: '交付文件', color: 'var(--tm-ok)' },
  todo_ask_received: { label: '请求确认', color: 'var(--tm-warn)' },
  todo_output_bound_manually: { label: '交付物绑定', color: 'var(--tm-ok)' },
  todo_review_requested: { label: '提交审核', color: 'var(--tm-warn)' },
  todo_review_approved: { label: '审核通过', color: 'var(--tm-ok)' },
  todo_review_rejected: { label: '审核驳回', color: 'var(--tm-danger)' },
}

const STATUS_LABEL: Record<string, string> = {
  planning: '规划中', review: '待确认', pending: '待开始', in_progress: '进行中',
  awaiting_review: '待审核', waiting_user: '等你处理', done: '已完成',
  failed: '失败', canceled: '已取消',
}

function relTime(iso?: string): string {
  if (!iso) return ''
  const diff = Date.now() - new Date(iso).getTime()
  if (Number.isNaN(diff)) return ''
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}小时前`
  return `${Math.floor(diff / 86_400_000)}天前`
}

export function EventFeed({ events, defaultCount = 15 }: { events: TaskEvent[]; defaultCount?: number }) {
  const [expanded, setExpanded] = useState(false)

  // 后端按创建时间倒序返回；直接取最新在前。折叠时只显示最近 defaultCount 条。
  const shown = useMemo(
    () => (expanded ? events.slice(0, 100) : events.slice(0, defaultCount)),
    [events, expanded, defaultCount],
  )

  if (events.length === 0) return null

  return (
    <div className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-3">
      <ol className="flex flex-col gap-3">
        {shown.map((ev) => {
          const meta = EVENT_META[ev.event_type] ?? { label: ev.event_type, color: 'var(--tm-text-3)' }
          const m = ev.metadata ?? {}
          const from = typeof m.from === 'string' ? m.from : undefined
          const to = typeof m.to === 'string' ? m.to : undefined
          const todoTitle = typeof m.todo_title === 'string' ? m.todo_title : undefined
          const error = typeof m.error === 'string' ? m.error : undefined
          return (
            <li key={ev.id} className="flex items-start gap-2">
              <span
                className="mt-[6px] h-[8px] w-[8px] shrink-0 rounded-full"
                style={{ background: meta.color }}
              />
              <span className="min-w-0 flex-1">
                <span className="flex items-baseline gap-1.5">
                  <span className="text-[13px] font-medium" style={{ color: meta.color }}>
                    {meta.label}
                  </span>
                  {ev.actor_name ? (
                    <span className="min-w-0 truncate text-[12px] text-[var(--tm-text-3)]">{ev.actor_name}</span>
                  ) : null}
                  <span className="ml-auto shrink-0 text-[11px] text-[var(--tm-text-3)]">{relTime(ev.created_at)}</span>
                </span>
                {ev.content ? (
                  <span className="mt-[2px] block text-[13px] leading-relaxed break-words whitespace-pre-wrap text-[var(--tm-text-2)]">
                    {ev.content}
                  </span>
                ) : null}
                {todoTitle ? (
                  <span className="mt-[2px] block text-[13px] text-[var(--tm-text-2)]">
                    {ev.event_type === 'todo_completed' ? <s>{todoTitle}</s> : todoTitle}
                  </span>
                ) : null}
                {from && to ? (
                  <span className="mt-[2px] block text-[12px] text-[var(--tm-text-3)]">
                    {STATUS_LABEL[from] ?? from} → {STATUS_LABEL[to] ?? to}
                  </span>
                ) : null}
                {error ? (
                  <span className="mt-[2px] block text-[13px] break-words whitespace-pre-wrap text-[var(--tm-danger)]">
                    {error}
                  </span>
                ) : null}
              </span>
            </li>
          )
        })}
      </ol>
      {events.length > defaultCount ? (
        <button
          type="button"
          className="mt-3 w-full text-center text-[13px] text-[var(--tm-text-2)]"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? '收起' : `展开更早的执行过程（共 ${events.length} 条）`}
        </button>
      ) : null}
    </div>
  )
}
