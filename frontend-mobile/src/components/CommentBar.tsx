import { useMemo, useRef, useState } from 'react'
import { Button, TextArea, Toast } from 'antd-mobile'
import { useAddComment } from '@/hooks/useTasks'
import { useAuthStore } from '@/stores/authStore'
import type { MentionCandidate } from '@/types'

/**
 * 底部固定评论条：手机上一律单手可达，不做 hover / 浮动工具栏。
 *
 * @ 提及（对齐桌面端）：输入尾部出现 `@`（或 `@关键字`）⇒ 上方弹出任务参与数字员工
 * 候选（PM + 各步骤执行员工，去重）；点选插入 `@名字 ` 并记录 agent_id；发送时随评论
 * 带 mentions。投递失败（部分员工未收到）时 Toast 警告（桌面端同款文案）。
 */
export function CommentBar({
  taskId,
  candidates,
}: {
  taskId: string
  candidates: MentionCandidate[]
}) {
  const addComment = useAddComment()
  const [text, setText] = useState('')
  const mentionedRef = useRef<Map<string, string>>(new Map())

  // 尾部 @ 触发：匹配最后一个未闭合的 @关键字（@ 后无空格/换行即视为正在输入）。
  // 🔴 不要求 @ 前有空格——中文输入前面常是标点（如「收到，@」）。
  const mentionSearch = useMemo(() => {
    const m = /@([^\s@]*)$/.exec(text)
    if (!m) return null
    const query = m[1].trim().toLowerCase()
    const filtered = query
      ? candidates.filter((c) => c.name.toLowerCase().includes(query))
      : candidates
    return filtered.length > 0 ? filtered : null
  }, [text, candidates])

  function pickCandidate(c: MentionCandidate) {
    // 替换尾部未闭合的 @关键字 为 @名字␣（mentionSearch 已保证尾部有 @…）
    setText((prev) => prev.replace(/@[^\s@]*$/, `@${c.name} `))
    mentionedRef.current.set(c.name, c.id)
  }

  async function send() {
    const content = text.trim()
    if (!content) return
    const mentions = [...new Set(mentionedRef.current.values())].map((agent_id) => ({ agent_id }))
    try {
      const data = await addComment.mutateAsync({ taskId, content, mentions: mentions.length > 0 ? mentions : undefined })
      setText('')
      mentionedRef.current.clear()
      const failed = data?.mention_deliveries?.filter((d) => d.status !== 'sent') ?? []
      if (failed.length > 0) {
        Toast.show({ content: `评论已发布，但有 ${failed.length} 个数字员工未收到 mention`, duration: 2500 })
      }
    } catch (err) {
      Toast.show({ content: err instanceof Error ? err.message : '发送失败' })
    }
  }

  return (
    <div className="tm-safe-bottom relative shrink-0 border-t border-[var(--tm-line)] bg-white px-3 py-2">
      {mentionSearch ? (
        <div className="absolute bottom-full left-3 right-3 z-20 mb-1 overflow-hidden rounded-[10px] border border-[var(--tm-line)] bg-white shadow-lg">
          {mentionSearch.map((c) => (
            <button
              key={c.id}
              type="button"
              onClick={() => pickCandidate(c)}
              className="flex w-full items-center justify-between px-3 py-[9px] text-left active:bg-[var(--tm-brand-soft)]"
            >
              <span className="text-[14px]">{c.name}</span>
              <span className="text-[11px] text-[var(--tm-text-3)]">{c.roleLabel}</span>
            </button>
          ))}
        </div>
      ) : null}
      <div className="flex items-end gap-2">
        <div className="flex-1 rounded-[10px] bg-[var(--tm-surface)] px-2 py-1">
          <TextArea
            placeholder={candidates.length > 0 ? '说点什么… 输入 @ 提醒数字员工' : '说点什么…'}
            value={text}
            onChange={setText}
            rows={1}
            autoSize={{ minRows: 1, maxRows: 4 }}
          />
        </div>
        <Button
          size="small"
          color="primary"
          loading={addComment.isPending}
          disabled={!text.trim()}
          onClick={() => void send()}
        >
          发送
        </Button>
      </div>
    </div>
  )
}

export function CommentList({
  comments,
}: {
  comments: Array<{ id: string; actor_name: string; actor_type: string; content: string; created_at: string }>
}) {
  const me = useAuthStore((s) => s.user)
  if (comments.length === 0) {
    return <p className="px-1 py-2 text-[13px] text-[var(--tm-text-3)]">还没有评论</p>
  }
  return (
    <div className="flex flex-col gap-3">
      {comments.map((c) => {
        const mine = c.actor_type === 'user' && c.actor_name === me?.name
        return (
          <div key={c.id} className={mine ? 'flex justify-end' : 'flex justify-start'}>
            <div
              className="max-w-[80%] rounded-[12px] px-3 py-2"
              style={{
                background: mine ? 'var(--tm-brand-soft)' : '#fff',
                border: mine ? 'none' : '1px solid var(--tm-line)',
              }}
            >
              <p className="text-[11px] text-[var(--tm-text-3)]">{c.actor_name}</p>
              <p className="mt-[2px] text-[14px] leading-relaxed whitespace-pre-wrap">{c.content}</p>
            </div>
          </div>
        )
      })}
    </div>
  )
}