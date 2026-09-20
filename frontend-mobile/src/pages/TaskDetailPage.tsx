import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { PageScaffold } from '@/components/PageScaffold'
import { StatusTag } from '@/components/StatusTag'
import { ArtifactList } from '@/components/ArtifactList'
import { CommentBar, CommentList } from '@/components/CommentBar'
import { EventFeed } from '@/components/EventFeed'
import { PendingCard } from '@/components/PendingCard'
import { EmptyState } from '@/components/EmptyState'
import { useTask, useTaskComments, useTaskEvents } from '@/hooks/useTasks'
import { collectPendingItems } from '@/lib/pendingItems'
import { splitArtifacts } from '@/lib/artifacts'
import type { MentionCandidate, Todo } from '@/types'

const TODO_DOT: Record<string, string> = {
  done: 'var(--tm-ok)',
  completed: 'var(--tm-ok)',
  in_progress: 'var(--tm-brand)',
  failed: 'var(--tm-danger)',
  pending: '#d3d1c7',
}

/** @ 提及候选：PM + 各步骤执行员工，按 agent_id 去重（桌面端同口径） */
function buildMentionCandidates(task: NonNullable<ReturnType<typeof useTask>['data']>): MentionCandidate[] {
  const seen = new Set<string>()
  const out: MentionCandidate[] = []
  if (task.pm_agent?.id && !seen.has(task.pm_agent.id)) {
    out.push({ id: task.pm_agent.id, name: task.pm_agent.name, roleLabel: 'PM 数字员工' })
    seen.add(task.pm_agent.id)
  }
  for (const todo of task.todos) {
    const agentId = todo.assignee?.agent_id
    if (!agentId || seen.has(agentId)) continue
    out.push({ id: agentId, name: todo.assignee?.name || '未命名员工', roleLabel: '执行数字员工' })
    seen.add(agentId)
  }
  return out
}

export function TaskDetailPage() {
  const { taskId = '' } = useParams()
  const navigate = useNavigate()
  const { data: task, isLoading } = useTask(taskId)
  const { data: comments = [] } = useTaskComments(taskId)
  const { data: events = [] } = useTaskEvents(taskId)
  const [showProcess, setShowProcess] = useState(false)
  const [showTodos, setShowTodos] = useState(false)
  const [showDeliverables, setShowDeliverables] = useState(false)

  const pending = collectPendingItems(task, events)
  const { deliverables, processes } = splitArtifacts(task?.artifacts)
  const doneCount = (task?.todos ?? []).filter((t) => t.status === 'done' || t.status === 'completed').length

  if (isLoading) {
    return (
      <PageScaffold title="任务详情">
        <div className="h-[120px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
      </PageScaffold>
    )
  }

  if (!task) {
    return (
      <PageScaffold title="任务详情">
        <EmptyState title="任务不存在或无权访问" description="若任务在其它空间，请到「我的」切换工作区后重试。" />
      </PageScaffold>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <PageScaffold
          title="任务详情"
          extra={
            <button type="button" className="text-[13px] text-[var(--tm-text-2)]" onClick={() => navigate(-1)}>
              返回
            </button>
          }
        >
          <section className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-4">
            <div className="flex items-start justify-between gap-2">
              <h2 className="min-w-0 flex-1 text-[17px] font-medium">{task.title}</h2>
              <StatusTag status={task.status} />
            </div>
            {task.description ? (
              <p className="mt-2 text-[13px] leading-relaxed whitespace-pre-wrap text-[var(--tm-text-2)]">
                {task.description}
              </p>
            ) : null}
            <div className="mt-3 h-[6px] w-full overflow-hidden rounded-[3px] bg-[var(--tm-line)]">
              <div
                className="h-full rounded-[3px] bg-[var(--tm-brand)]"
                style={{
                  width: `${task.todos.length ? Math.round((doneCount / task.todos.length) * 100) : 0}%`,
                }}
              />
            </div>
            <p className="mt-2 text-[12px] text-[var(--tm-text-3)]">
              已完成 {doneCount}/{task.todos.length}
            </p>
          </section>

          {pending.length > 0 ? (
            <>
              <h3 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">待你处理</h3>
              <div className="flex flex-col gap-3">
                {pending.map((item) => (
                  <PendingCard key={item.id} item={item} />
                ))}
              </div>
            </>
          ) : null}

          {events.length > 0 ? (
            <>
              <h3 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">执行过程</h3>
              <EventFeed events={events} />
            </>
          ) : null}

          <button
            type="button"
            onClick={() => setShowTodos((v) => !v)}
            className="mb-2 mt-5 flex w-full items-center justify-between text-[13px] text-[var(--tm-text-2)]"
          >
            <span>
              步骤（{doneCount}/{task.todos.length}）
            </span>
            <span>{showTodos ? '收起 ▲' : '展开 ▼'}</span>
          </button>
          {showTodos ? (
            <div className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-3">
              {task.todos.length === 0 ? (
                <p className="text-[13px] text-[var(--tm-text-3)]">暂无步骤</p>
              ) : (
                <ol className="flex flex-col gap-3">
                  {task.todos.map((todo: Todo) => (
                    <li key={todo.id} className="flex items-start gap-2">
                      <span
                        className="mt-[6px] h-[8px] w-[8px] shrink-0 rounded-full"
                        style={{ background: TODO_DOT[todo.status] ?? '#d3d1c7' }}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block text-[14px]">{todo.title}</span>
                        <span className="block text-[12px] text-[var(--tm-text-3)]">
                          {todo.assignee?.name ?? '未指派'}
                          {todo.review_status === 'pending_approval' ? ' · 待人工确认' : ''}
                        </span>
                      </span>
                    </li>
                  ))}
                </ol>
              )}
            </div>
          ) : null}

          <button
            type="button"
            onClick={() => setShowDeliverables((v) => !v)}
            className="mb-2 mt-5 flex w-full items-center justify-between text-[13px] text-[var(--tm-text-2)]"
          >
            <span>交付物（{deliverables.length}）</span>
            <span>{showDeliverables ? '收起 ▲' : '展开 ▼'}</span>
          </button>
          {showDeliverables ? <ArtifactList taskId={task.id} items={deliverables} /> : null}

          {processes.length > 0 ? (
            <>
              <button
                type="button"
                onClick={() => setShowProcess((v) => !v)}
                className="mt-3 text-[13px] text-[var(--tm-text-2)]"
              >
                过程产物（{processes.length}）{showProcess ? '收起' : '展开'}
              </button>
              {showProcess ? (
                <div className="mt-2">
                  <ArtifactList taskId={task.id} items={processes} />
                </div>
              ) : null}
            </>
          ) : null}

          <h3 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">评论（{comments.length}）</h3>
          <CommentList comments={comments} />
          <div className="h-4" />
        </PageScaffold>
      </div>
      <CommentBar taskId={task.id} candidates={buildMentionCandidates(task)} />
    </div>
  )
}
