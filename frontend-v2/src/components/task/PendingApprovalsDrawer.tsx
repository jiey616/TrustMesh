import { useState } from 'react'
import { App, Button, Drawer, Input, Tag, Typography } from 'antd'
import { CheckCircleOutlined, EditOutlined, SendOutlined } from '@ant-design/icons'
import {
  useAnswerTodo,
  useAppendTaskMessage,
  useApprovePlan,
  useRejectPlan,
  useReviewTodo,
} from '@/hooks/useTasks'
import { usePendingStore } from '@/stores/pendingStore'
import { UIResponsePanel } from '@/components/task/UIResponsePanel'
import {
  pendingKindLabel,
  type PendingItem,
  type PlanClarifyPending,
  type PlanReviewPending,
  type TodoAskPending,
  type TodoReviewPending,
} from '@/lib/pendingItems'

const { Text } = Typography

const ACCENT: Record<PendingItem['kind'], string> = {
  plan_clarify: '#6d5ff5',
  plan_review: '#f59e0b',
  todo_ask: '#a5b4fc',
  todo_review: '#f59e0b',
}

const inputStyle = {
  background: 'rgba(0,0,0,0.25)',
  borderColor: 'rgba(255,255,255,0.12)',
  color: '#fff',
  resize: 'none' as const,
  fontSize: 14,
}

function CardShell({ label, accent, children }: { label: string; accent: string; children: React.ReactNode }) {
  return (
    <div
      style={{
        borderRadius: 14,
        border: `1px solid ${accent}59`,
        background: `${accent}0f`,
        padding: 14,
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
      }}
    >
      <div style={{ fontSize: 12, fontWeight: 600, color: accent, letterSpacing: 0.5 }}>{label}</div>
      {children}
    </div>
  )
}

/* ------------------------------------------------------------------
 *  1. 规划澄清：PM 抛出的 ui_blocks 逐步问答
 * ------------------------------------------------------------------ */

function PlanClarifyCard({ item }: { item: PlanClarifyPending }) {
  const { message } = App.useApp()
  const appendMessage = useAppendTaskMessage()
  const draft = usePendingStore((s) => s.drafts[item.id])
  const setDraft = usePendingStore((s) => s.setDraft)
  const clearDraft = usePendingStore((s) => s.clearDraft)

  return (
    <CardShell label={pendingKindLabel.plan_clarify} accent={ACCENT.plan_clarify}>
      <UIResponsePanel
        key={item.id}
        blocks={item.blocks}
        disabled={appendMessage.isPending}
        draft={{ step: draft?.step ?? 0, responses: draft?.responses ?? {} }}
        onDraftChange={(d) => setDraft(item.id, d)}
        onSubmit={async (content, uiResponse) => {
          try {
            await appendMessage.mutateAsync({ taskId: item.taskId, content, uiResponse })
            clearDraft(item.id)
            message.success('已提交，PM 继续规划中')
          } catch (err) {
            message.error(err instanceof Error ? err.message : '提交失败，请稍后重试')
          }
        }}
      />
    </CardShell>
  )
}

/* ------------------------------------------------------------------
 *  2. 方案确认：PM 规划完毕，等人点头或打回
 * ------------------------------------------------------------------ */

function PlanReviewCard({ item }: { item: PlanReviewPending }) {
  const { message } = App.useApp()
  const approvePlan = useApprovePlan()
  const rejectPlan = useRejectPlan()
  const draft = usePendingStore((s) => s.drafts[item.id])
  const setDraft = usePendingStore((s) => s.setDraft)
  const clearDraft = usePendingStore((s) => s.clearDraft)

  const feedback = draft?.text ?? ''
  const rejecting = draft?.rejecting ?? false

  const handleApprove = async () => {
    try {
      await approvePlan.mutateAsync({ taskId: item.taskId })
      clearDraft(item.id)
      message.success('已确认，任务开始执行')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    }
  }

  const handleReject = async () => {
    if (!feedback.trim()) return
    try {
      await rejectPlan.mutateAsync({ taskId: item.taskId, feedback: feedback.trim() })
      clearDraft(item.id)
      message.success('已提交修改意见')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    }
  }

  return (
    <CardShell label={pendingKindLabel.plan_review} accent={ACCENT.plan_review}>
      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <span style={{ fontSize: 15, fontWeight: 600, color: '#f4f4f8' }}>PM 已完成规划，请确认后开始执行</span>
          {item.workflow && item.workflow.steps.length > 0 && (
            <Tag color="gold" style={{ margin: 0, fontSize: 12 }}>工作流：{item.workflow.name || '未命名'}</Tag>
          )}
        </div>
        <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.5)', marginTop: 4 }}>共 {item.todos.length} 个子任务</div>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {item.todos.map((todo, idx) => (
          <div key={todo.id} style={{ display: 'flex', alignItems: 'flex-start', gap: 8, borderRadius: 8, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)', padding: '8px 10px', fontSize: 14 }}>
            <span style={{ flexShrink: 0, fontSize: 13, color: 'rgba(255,255,255,0.45)', paddingTop: 1 }}>{idx + 1}.</span>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 500, color: '#f4f4f8' }}>{todo.title}</div>
              {todo.description && (
                <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.55)', marginTop: 2, whiteSpace: 'pre-wrap' }}>{todo.description}</div>
              )}
            </div>
            <span style={{ flexShrink: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', paddingTop: 1 }}>{todo.assignee?.name ?? ''}</span>
          </div>
        ))}
      </div>

      {rejecting ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Input.TextArea
            value={feedback}
            onChange={(e) => setDraft(item.id, { text: e.target.value })}
            placeholder="说明需要调整的地方..."
            rows={3}
            style={inputStyle}
            autoFocus
          />
          <div style={{ display: 'flex', gap: 8 }}>
            <Button size="small" danger type="primary" disabled={!feedback.trim() || rejectPlan.isPending} loading={rejectPlan.isPending} onClick={() => void handleReject()}>
              提交修改意见
            </Button>
            <Button size="small" onClick={() => setDraft(item.id, { rejecting: false, text: '' })}>取消</Button>
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', gap: 8 }}>
          <Button size="small" type="primary" loading={approvePlan.isPending} onClick={() => void handleApprove()}>确认执行</Button>
          <Button size="small" onClick={() => setDraft(item.id, { rejecting: true })}>修改规划</Button>
        </div>
      )}
    </CardShell>
  )
}

/* ------------------------------------------------------------------
 *  3. 执行提问：agent 中途 ask，等人回答
 * ------------------------------------------------------------------ */

function TodoAskCard({ item }: { item: TodoAskPending }) {
  const { message } = App.useApp()
  const answerTodo = useAnswerTodo()
  const draft = usePendingStore((s) => s.drafts[item.id])
  const setDraft = usePendingStore((s) => s.setDraft)
  const clearDraft = usePendingStore((s) => s.clearDraft)
  const value = draft?.text ?? ''

  const submit = async (answer: string) => {
    if (!answer.trim() || answerTodo.isPending) return
    try {
      await answerTodo.mutateAsync({ taskId: item.taskId, todoId: item.todoId, questionId: item.questionId, answer: answer.trim() })
      clearDraft(item.id)
      message.success('已提交，数字员工继续执行中')
    } catch (err) {
      message.error(err instanceof Error ? err.message : '提交失败，请稍后重试')
    }
  }

  return (
    <CardShell label={pendingKindLabel.todo_ask} accent={ACCENT.todo_ask}>
      <div style={{ fontSize: 14, color: 'rgba(255,255,255,0.85)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
        {item.question}
      </div>

      {item.options.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {item.options.map((opt) => (
            <Button
              key={opt}
              size="small"
              disabled={answerTodo.isPending}
              onClick={() => void submit(opt)}
              style={{ borderColor: 'rgba(109,95,245,0.5)', background: 'rgba(109,95,245,0.1)', color: '#a5b4fc' }}
            >
              {opt}
            </Button>
          ))}
        </div>
      )}

      <div style={{ display: 'flex', gap: 8 }}>
        <Input
          size="small"
          value={value}
          onChange={(e) => setDraft(item.id, { text: e.target.value })}
          onPressEnter={() => void submit(value)}
          disabled={answerTodo.isPending}
          placeholder={item.options.length > 0 ? '或输入自定义回答…' : '输入你的回答…'}
          style={{ flex: 1 }}
        />
        <Button
          size="small"
          type="primary"
          icon={<SendOutlined />}
          disabled={!value.trim() || answerTodo.isPending}
          loading={answerTodo.isPending}
          onClick={() => void submit(value)}
        >
          提交
        </Button>
      </div>
    </CardShell>
  )
}

/* ------------------------------------------------------------------
 *  4. 成果审核：todo 做完，等人通过或退回重做
 * ------------------------------------------------------------------ */

function TodoReviewCard({ item }: { item: TodoReviewPending }) {
  const { message } = App.useApp()
  const reviewTodo = useReviewTodo()
  const draft = usePendingStore((s) => s.drafts[item.id])
  const setDraft = usePendingStore((s) => s.setDraft)
  const clearDraft = usePendingStore((s) => s.clearDraft)
  const reason = draft?.text ?? ''
  const rejecting = draft?.rejecting ?? false

  const handleApprove = async () => {
    try {
      await reviewTodo.mutateAsync({ taskId: item.taskId, todoId: item.todoId, action: 'approve' })
      clearDraft(item.id)
      message.success(`${item.todoTitle} 已确认通过`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    }
  }

  const handleReject = async () => {
    if (!reason.trim()) {
      message.error('请填写退回原因')
      return
    }
    try {
      await reviewTodo.mutateAsync({ taskId: item.taskId, todoId: item.todoId, action: 'reject', reason: reason.trim() })
      clearDraft(item.id)
      message.success(`${item.todoTitle} 已退回上一个 Todo 重做`)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败，请稍后重试')
    }
  }

  const summary = typeof item.todo.result === 'string'
    ? item.todo.result
    : (item.todo.result as { summary?: string } | null)?.summary

  return (
    <CardShell label={pendingKindLabel.todo_review} accent={ACCENT.todo_review}>
      <div>
        <div style={{ fontSize: 14, fontWeight: 500, color: '#f4f4f8' }}>{item.todoTitle}</div>
        <div style={{ fontSize: 12, color: 'rgba(255,255,255,0.45)', marginTop: 2 }}>
          {item.todo.assignee?.name ?? '未指派'} · 待人工确认
        </div>
      </div>

      {summary && (
        <div style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)', whiteSpace: 'pre-wrap', background: 'rgba(255,255,255,0.03)', borderRadius: 8, padding: '8px 10px', maxHeight: 160, overflowY: 'auto' }}>
          {summary}
        </div>
      )}

      {rejecting ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Text type="secondary" style={{ fontSize: 12 }}>
            退回后该 Todo 会级联重置后续 Todo，原因将作为数字员工的重做依据。
          </Text>
          <Input.TextArea
            value={reason}
            onChange={(e) => setDraft(item.id, { text: e.target.value })}
            placeholder="请填写退回原因（必填）"
            rows={3}
            style={inputStyle}
            autoFocus
          />
          <div style={{ display: 'flex', gap: 8 }}>
            <Button size="small" danger type="primary" loading={reviewTodo.isPending} onClick={() => void handleReject()}>确认退回</Button>
            <Button size="small" onClick={() => setDraft(item.id, { rejecting: false, text: '' })}>取消</Button>
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', gap: 8 }}>
          <Button size="small" type="primary" style={{ background: '#10b981', borderColor: '#10b981' }} loading={reviewTodo.isPending} onClick={() => void handleApprove()}>
            通过
          </Button>
          <Button size="small" style={{ borderColor: '#f43f5e', color: '#f43f5e' }} onClick={() => setDraft(item.id, { rejecting: true })}>
            退回重做
          </Button>
        </div>
      )}
    </CardShell>
  )
}

/* ------------------------------------------------------------------
 *  抽屉主体
 * ------------------------------------------------------------------ */

function PendingCard({ item }: { item: PendingItem }) {
  switch (item.kind) {
    case 'plan_clarify':
      return <PlanClarifyCard item={item} />
    case 'plan_review':
      return <PlanReviewCard item={item} />
    case 'todo_ask':
      return <TodoAskCard item={item} />
    case 'todo_review':
      return <TodoReviewCard item={item} />
  }
}

export function PendingApprovalsDrawer({
  open,
  onClose,
  items,
}: {
  open: boolean
  onClose: () => void
  items: PendingItem[]
}) {
  const [dismissed, setDismissed] = useState<string[]>([])

  const visible = items.filter((i) => !dismissed.includes(i.id))

  return (
    <Drawer
      title={`待确认 (${visible.length})`}
      open={open}
      onClose={onClose}
      width={560}
      styles={{ body: { background: '#0a0a14', paddingTop: 8 } }}
      style={{ background: '#0a0a14' }}
    >
      {visible.length === 0 ? (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8, paddingTop: 64 }}>
          <CheckCircleOutlined style={{ fontSize: 28, color: '#10b981' }} />
          <Text type="secondary">没有待确认事项</Text>
          {dismissed.length > 0 && (
            <Button
              size="small"
              type="link"
              onClick={() => setDismissed([])}
            >
              恢复已忽略的 {dismissed.length} 项
            </Button>
          )}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {visible.map((item) => (
            <div key={item.id}>
              <PendingCard item={item} />
              <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 4 }}>
                <Button
                  type="text"
                  size="small"
                  icon={<EditOutlined />}
                  style={{ fontSize: 12, color: 'rgba(255,255,255,0.35)' }}
                  onClick={() => setDismissed((prev) => [...prev, item.id])}
                >
                  暂时忽略
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </Drawer>
  )
}
