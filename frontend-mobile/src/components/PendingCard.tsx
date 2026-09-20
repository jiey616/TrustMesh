import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, TextArea, Toast } from 'antd-mobile'
import { UiBlocksForm } from './UiBlocksForm'
import {
  useAnswerTodo,
  useAppendMessage,
  useApprovePlan,
  useRejectPlan,
  useReviewTodo,
} from '@/hooks/useTasks'
import { pendingKindLabel, type PendingItem } from '@/lib/pendingItems'

const ACCENT: Record<PendingItem['kind'], string> = {
  plan_clarify: '#6d5ff5',
  plan_review: '#ba7517',
  todo_ask: '#6d5ff5',
  todo_review: '#ba7517',
}

export function PendingCard({ item }: { item: PendingItem }) {
  const navigate = useNavigate()
  const accent = ACCENT[item.kind]
  const [rejecting, setRejecting] = useState(false)
  const [text, setText] = useState('')

  const approvePlan = useApprovePlan()
  const rejectPlan = useRejectPlan()
  const reviewTodo = useReviewTodo()
  const answerTodo = useAnswerTodo()
  const appendMessage = useAppendMessage()

  const busy =
    approvePlan.isPending ||
    rejectPlan.isPending ||
    reviewTodo.isPending ||
    answerTodo.isPending ||
    appendMessage.isPending

  function fail(err: unknown) {
    Toast.show({ content: err instanceof Error ? err.message : '操作失败，请稍后重试' })
  }

  function ok(msg: string) {
    Toast.show({ content: msg })
    setText('')
    setRejecting(false)
  }

  async function act(fn: () => Promise<unknown>, msg: string) {
    try {
      await fn()
      ok(msg)
    } catch (err) {
      fail(err)
    }
  }

  return (
    <div
      className="rounded-[var(--tm-radius-card)] border bg-white p-3"
      style={{ borderColor: `${accent}59` }}
    >
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12px] font-medium" style={{ color: accent }}>
          {pendingKindLabel[item.kind]}
        </span>
        <button
          type="button"
          className="text-[12px] text-[var(--tm-text-3)]"
          onClick={() => navigate(`/tasks/detail/${item.taskId}`)}
        >
          打开任务
        </button>
      </div>
      <p className="mb-2 line-clamp-1 text-[13px] text-[var(--tm-text-2)]">{item.taskTitle}</p>

      {item.kind === 'plan_review' ? (
        <>
          <p className="text-[15px] font-medium">PM 已完成规划，请确认后开始执行</p>
          <p className="mt-1 text-[13px] text-[var(--tm-text-2)]">
            共 {item.todos.length} 个子任务
          </p>
          {rejecting ? (
            <div className="mt-2 flex flex-col gap-2">
              <TextArea
                placeholder="说明需要调整的地方…"
                value={text}
                onChange={setText}
                rows={3}
                autoFocus
              />
              <div className="flex gap-2">
                <Button
                  size="small"
                  color="danger"
                  loading={busy}
                  disabled={!text.trim()}
                  onClick={() =>
                    act(() => rejectPlan.mutateAsync({ taskId: item.taskId, feedback: text.trim() }), '已提交修改意见')
                  }
                >
                  提交修改意见
                </Button>
                <Button size="small" fill="none" onClick={() => setRejecting(false)}>
                  取消
                </Button>
              </div>
            </div>
          ) : (
            <div className="mt-3 flex gap-2">
              <Button
                size="small"
                color="primary"
                loading={busy}
                onClick={() => act(() => approvePlan.mutateAsync({ taskId: item.taskId }), '已确认，任务开始执行')}
              >
                确认执行
              </Button>
              <Button size="small" fill="none" onClick={() => setRejecting(true)}>
                修改规划
              </Button>
            </div>
          )}
        </>
      ) : null}

      {item.kind === 'plan_clarify' ? (
        <UiBlocksForm
          blocks={item.blocks}
          submitting={busy}
          onSubmit={(content, uiResponse) =>
            act(
              () => appendMessage.mutateAsync({ taskId: item.taskId, content, uiResponse }),
              '已提交，PM 继续规划中',
            )
          }
        />
      ) : null}

      {item.kind === 'todo_ask' ? (
        <>
          <p className="text-[15px] leading-relaxed">{item.question}</p>
          {item.options.length > 0 ? (
            <div className="mt-2 flex flex-wrap gap-2">
              {item.options.map((opt) => (
                <button
                  key={opt}
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    act(
                      () =>
                        answerTodo.mutateAsync({
                          taskId: item.taskId,
                          todoId: item.todoId,
                          questionId: item.questionId,
                          answer: opt,
                        }),
                      '已提交，数字员工继续执行中',
                    )
                  }
                  className="rounded-[8px] border px-3 py-[6px] text-left text-[13px]"
                  style={{ borderColor: 'var(--tm-brand)', color: 'var(--tm-brand)', background: 'var(--tm-brand-soft)' }}
                >
                  {opt}
                </button>
              ))}
            </div>
          ) : null}
          <div className="mt-3 flex gap-2">
            <TextArea
              placeholder={item.options.length > 0 ? '或输入自定义回答…' : '输入你的回答…'}
              value={text}
              onChange={setText}
              rows={2}
            />
          </div>
          <Button
            block
            size="small"
            color="primary"
            loading={busy}
            disabled={!text.trim()}
            style={{ marginTop: 8 }}
            onClick={() =>
              act(
                () =>
                  answerTodo.mutateAsync({
                    taskId: item.taskId,
                    todoId: item.todoId,
                    questionId: item.questionId,
                    answer: text.trim(),
                  }),
                '已提交，数字员工继续执行中',
              )
            }
          >
            提交
          </Button>
        </>
      ) : null}

      {item.kind === 'todo_review' ? (
        <>
          <p className="text-[15px] font-medium">{item.todoTitle}</p>
          <p className="mt-1 text-[12px] text-[var(--tm-text-3)]">
            {item.todo.assignee?.name ?? '未指派'} · 待人工确认
          </p>
          {typeof item.todo.result === 'string' && item.todo.result ? (
            <p className="mt-2 max-h-[160px] overflow-y-auto rounded-[8px] bg-[var(--tm-surface)] p-2 text-[13px] leading-relaxed whitespace-pre-wrap">
              {item.todo.result}
            </p>
          ) : null}
          {rejecting ? (
            <div className="mt-2 flex flex-col gap-2">
              <TextArea
                placeholder="请填写退回原因（必填）"
                value={text}
                onChange={setText}
                rows={3}
                autoFocus
              />
              <div className="flex gap-2">
                <Button
                  size="small"
                  color="danger"
                  loading={busy}
                  disabled={!text.trim()}
                  onClick={() =>
                    act(
                      () =>
                        reviewTodo.mutateAsync({
                          taskId: item.taskId,
                          todoId: item.todoId,
                          action: 'reject',
                          reason: text.trim(),
                        }),
                      '已退回上一个 Todo 重做',
                    )
                  }
                >
                  确认退回
                </Button>
                <Button size="small" fill="none" onClick={() => setRejecting(false)}>
                  取消
                </Button>
              </div>
            </div>
          ) : (
            <div className="mt-3 flex gap-2">
              <Button
                size="small"
                color="primary"
                loading={busy}
                onClick={() =>
                  act(
                    () =>
                      reviewTodo.mutateAsync({
                        taskId: item.taskId,
                        todoId: item.todoId,
                        action: 'approve',
                      }),
                    '已确认通过',
                  )
                }
              >
                通过
              </Button>
              <Button size="small" fill="none" onClick={() => setRejecting(true)}>
                退回重做
              </Button>
            </div>
          )}
        </>
      ) : null}
    </div>
  )
}
