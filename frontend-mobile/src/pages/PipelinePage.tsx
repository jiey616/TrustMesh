import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { PageScaffold } from '@/components/PageScaffold'
import { StatusTag } from '@/components/StatusTag'
import { EmptyState } from '@/components/EmptyState'
import { getWorkflowProgress } from '@/api/projects'
import { useProjects } from '@/hooks/useTasks'

/** 单步产出最多直显条数，超出折叠 */
const OUTPUT_PREVIEW = 3

const STEP_DOT: Record<string, string> = {
  done: 'var(--tm-ok)',
  in_progress: '#185fa5',
  awaiting_review: '#ba7517',
  failed: 'var(--tm-danger)',
  pending: '#d3d1c7',
  unassigned: '#d3d1c7',
  canceled: '#d3d1c7',
}

/** 项目总流程流水线总览：每步状态 + 关联任务 + 步骤产出，点任务跳详情。 */
export function PipelinePage() {
  const { projectId = '' } = useParams()
  const navigate = useNavigate()
  const { data: projects = [] } = useProjects()
  const { data: progress, isLoading } = useQuery({
    queryKey: ['workflowProgress', projectId],
    queryFn: () => getWorkflowProgress(projectId),
    enabled: Boolean(projectId),
    // SSE 失效 ['workflowProgress'] 前缀；30s 轮询兜底
    refetchInterval: 30_000,
  })

  const project = projects.find((p) => p.id === projectId)
  const steps = progress?.steps ?? []
  const doneCount = steps.filter((s) => s.status === 'done').length
  const [expanded, setExpanded] = useState<Set<number>>(new Set())

  function toggleOutputs(index: number) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(index)) next.delete(index)
      else next.add(index)
      return next
    })
  }

  return (
    <PageScaffold title={project?.name ?? '项目流程'}>
      {isLoading ? (
        <div className="h-[160px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
      ) : !progress || steps.length === 0 ? (
        <EmptyState
          title="该项目暂未配置总流程"
          description="在桌面端为项目配置工作流后，这里会显示流水线进度。"
        />
      ) : (
        <>
          <section className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-4">
            <div className="flex items-center justify-between gap-2">
              <h2 className="min-w-0 flex-1 truncate text-[15px] font-medium">
                {progress.workflow_name}
              </h2>
              <span className="shrink-0 text-[12px] text-[var(--tm-text-3)]">
                {doneCount}/{steps.length} 步完成
              </span>
            </div>
            <div className="mt-3 h-[6px] w-full overflow-hidden rounded-[3px] bg-[var(--tm-line)]">
              <div
                className="h-full rounded-[3px] bg-[var(--tm-brand)]"
                style={{
                  width: `${steps.length ? Math.round((doneCount / steps.length) * 100) : 0}%`,
                }}
              />
            </div>
          </section>

          <div className="mt-4 flex flex-col gap-2">
            {steps.map((s) => (
              <div
                key={s.index}
                className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-3"
              >
                <div className="flex items-start gap-2">
                  <span
                    className="mt-[6px] h-[8px] w-[8px] shrink-0 rounded-full"
                    style={{ background: STEP_DOT[s.status] ?? '#d3d1c7' }}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-2">
                      <span className="min-w-0 flex-1 text-[14px]">
                        {s.index + 1}. {s.name}
                      </span>
                      <StatusTag status={s.status} />
                    </div>
                    {s.role ? (
                      <p className="mt-[2px] text-[12px] text-[var(--tm-text-3)]">{s.role}</p>
                    ) : null}
                    {s.task_id ? (
                      <button
                        type="button"
                        onClick={() => navigate(`/tasks/detail/${s.task_id}`)}
                        className="mt-2 flex w-full items-center justify-between rounded-[8px] bg-[var(--tm-surface)] px-3 py-2 text-left"
                      >
                        <span className="min-w-0 flex-1 truncate text-[13px] text-[var(--tm-text-2)]">
                          {s.task_title ?? '查看任务'}
                        </span>
                        <span className="ml-2 shrink-0 text-[13px] text-[var(--tm-text-3)]">›</span>
                      </button>
                    ) : null}
                    {s.outputs && s.outputs.length > 0 ? (
                      <div className="mt-2 flex flex-col gap-1">
                        {(expanded.has(s.index) ? s.outputs : s.outputs.slice(0, OUTPUT_PREVIEW)).map(
                          (o, i) => (
                            <p key={i} className="truncate text-[12px] text-[var(--tm-text-3)]">
                              📎 {o.output_name}
                              {o.file_name ? `：${o.file_name}` : ''}
                            </p>
                          ),
                        )}
                        {s.outputs.length > OUTPUT_PREVIEW ? (
                          <button
                            type="button"
                            onClick={() => toggleOutputs(s.index)}
                            className="self-start text-[12px] text-[var(--tm-brand)]"
                          >
                            {expanded.has(s.index)
                              ? '收起产出'
                              : `展开全部 ${s.outputs.length} 条产出`}
                          </button>
                        ) : null}
                      </div>
                    ) : null}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </PageScaffold>
  )
}
