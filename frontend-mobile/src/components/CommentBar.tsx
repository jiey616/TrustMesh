import { useState } from 'react'
import { Button, TextArea, Toast } from 'antd-mobile'
import { useAddComment } from '@/hooks/useTasks'
import { useAuthStore } from '@/stores/authStore'

/** 底部固定评论条：手机上一律单手可达，不做 hover / 浮动工具栏。 */
export function CommentBar({ taskId }: { taskId: string }) {
  const addComment = useAddComment()
  const [text, setText] = useState('')

  async function send() {
    const content = text.trim()
    if (!content) return
    try {
      await addComment.mutateAsync({ taskId, content })
      setText('')
    } catch (err) {
      Toast.show({ content: err instanceof Error ? err.message : '发送失败' })
    }
  }

  return (
    <div className="tm-safe-bottom shrink-0 border-t border-[var(--tm-line)] bg-white px-3 py-2">
      <div className="flex items-end gap-2">
        <div className="flex-1 rounded-[10px] bg-[var(--tm-surface)] px-2 py-1">
          <TextArea
            placeholder="说点什么…"
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
