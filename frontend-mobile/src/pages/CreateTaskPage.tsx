import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Button, Switch, Toast } from 'antd-mobile'
import { PageScaffold } from '@/components/PageScaffold'
import { EmptyState } from '@/components/EmptyState'
import { createTaskFromText } from '@/api/tasks'
import { getWorkflowProgress } from '@/api/projects'
import { useProjects } from '@/hooks/useTasks'

/** 一段话建任务：选项目 + 一段自然语言描述；可选按项目总流程的步骤区间建任务。 */
export function CreateTaskPage() {
  const navigate = useNavigate()
  const { data: projects = [], isLoading } = useProjects()
  const [projectId, setProjectId] = useState('')
  const [content, setContent] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // 切片建任务：开关 + 起止步骤（点击第一步设起点，再点后续步骤设终点，闭区间）
  const [useSlice, setUseSlice] = useState(false)
  const [range, setRange] = useState<{ from: number | null; to: number | null }>({
    from: null,
    to: null,
  })

  // 项目列表异步到达前默认选第一个
  const selected = projectId || projects[0]?.id || ''
  const selectedProject = projects.find((p) => p.id === selected)

  const { data: progress } = useQuery({
    queryKey: ['workflowProgress', selected],
    queryFn: () => getWorkflowProgress(selected),
    enabled: useSlice && Boolean(selected),
  })
  const steps = progress?.steps ?? []

  function pickStep(index: number) {
    setRange(({ from, to }) => {
      if (from === null || to !== null) return { from: index, to: null }
      if (index >= from) return { from, to: index }
      return { from: index, to: null }
    })
  }

  function rangeLabel() {
    if (range.from === null) return '未选择'
    if (range.to === null) return `第 ${range.from + 1} 步起（再点一步作为结束）`
    return `第 ${range.from + 1} – ${range.to + 1} 步（共 ${range.to - range.from + 1} 步）`
  }

  async function submit() {
    if (!selected) {
      Toast.show({ content: '请选择项目' })
      return
    }
    if (!content.trim()) {
      Toast.show({ content: '请输入任务描述' })
      return
    }
    if (useSlice && (range.from === null || range.to === null)) {
      Toast.show({ content: '请选择流程步骤范围' })
      return
    }
    setSubmitting(true)
    try {
      const slice =
        useSlice && range.from !== null && range.to !== null
          ? {
              workflowIndex: selectedProject?.primary_workflow_index ?? 0,
              stepFrom: range.from,
              stepTo: range.to,
            }
          : undefined
      const task = await createTaskFromText(selected, content.trim(), slice)
      Toast.show({ content: '已创建，PM 正在规划' })
      navigate(`/tasks/detail/${task.id}`, { replace: true })
    } catch (err) {
      Toast.show({ content: err instanceof Error ? err.message : '创建失败' })
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <PageScaffold title="新建任务">
      {isLoading ? (
        <div className="h-[160px] animate-pulse rounded-[var(--tm-radius-card)] bg-white" />
      ) : projects.length === 0 ? (
        <EmptyState title="暂无项目" description="先在桌面端创建项目，才能在这里发起任务。" />
      ) : (
        <>
          <h3 className="mb-2 text-[13px] text-[var(--tm-text-2)]">选择项目</h3>
          <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
            <div className="max-h-[220px] overflow-y-auto">
              {projects.map((p) => {
                const active = p.id === selected
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => {
                      setProjectId(p.id)
                      setRange({ from: null, to: null })
                    }}
                    className="flex w-full items-center justify-between border-b border-[var(--tm-line)] px-4 py-[10px] text-left last:border-b-0"
                  >
                    <span className="min-w-0 flex-1 truncate text-[14px]">{p.name}</span>
                    {active ? (
                      <span className="ml-2 shrink-0 text-[13px]" style={{ color: 'var(--tm-brand)' }}>
                        ✓
                      </span>
                    ) : null}
                  </button>
                )
              })}
            </div>
          </div>

          <div className="mt-4 flex items-center justify-between rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white px-4 py-3">
            <span className="text-[14px]">按总流程步骤建任务</span>
            <Switch
              checked={useSlice}
              onChange={(v) => {
                setUseSlice(v)
                setRange({ from: null, to: null })
              }}
            />
          </div>

          {useSlice ? (
            <div className="mt-2 rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-3">
              {!progress ? (
                <p className="py-2 text-center text-[13px] text-[var(--tm-text-3)]">加载流程中…</p>
              ) : steps.length === 0 ? (
                <p className="py-2 text-center text-[13px] text-[var(--tm-text-3)]">
                  该项目未配置总流程，请关闭开关直接创建。
                </p>
              ) : (
                <>
                  <p className="text-[12px] text-[var(--tm-text-3)]">
                    {progress.workflow_name} · {rangeLabel()}
                  </p>
                  <div className="mt-2 flex max-h-[240px] flex-col gap-1 overflow-y-auto">
                    {steps.map((s) => {
                      const inRange =
                        range.from !== null &&
                        range.to !== null &&
                        s.index >= range.from &&
                        s.index <= range.to
                      const isEdge = s.index === range.from || s.index === range.to
                      return (
                        <button
                          key={s.index}
                          type="button"
                          onClick={() => pickStep(s.index)}
                          className="flex items-center justify-between rounded-[8px] px-3 py-[8px] text-left"
                          style={{
                            background: inRange ? 'var(--tm-brand-soft)' : 'var(--tm-surface)',
                          }}
                        >
                          <span
                            className="min-w-0 flex-1 truncate text-[13px]"
                            style={{ color: isEdge ? 'var(--tm-brand)' : 'var(--tm-text-1)' }}
                          >
                            {s.index + 1}. {s.name}
                          </span>
                          {isEdge ? (
                            <span className="ml-2 shrink-0 text-[12px] text-[var(--tm-brand)]">
                              {s.index === range.from ? '起' : '终'}
                            </span>
                          ) : null}
                        </button>
                      )
                    })}
                  </div>
                </>
              )}
            </div>
          ) : null}

          <h3 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">任务描述</h3>
          <div className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white p-3">
            <textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={6}
              maxLength={4000}
              placeholder="用一段话描述你要做的事，例如：为《双羊尊》项目制作一支 2 分钟的文物归乡故事短视频，横屏 16:9，温情叙事风格。PM 会自动拆解为执行步骤。"
              className="w-full resize-none border-0 bg-transparent text-[14px] leading-relaxed outline-none placeholder:text-[var(--tm-text-3)]"
            />
          </div>
          <p className="mt-2 text-[12px] leading-relaxed text-[var(--tm-text-3)]">
            {useSlice
              ? '任务将只负责所选步骤区间，PM 会按流程步骤规划执行；创建即生效。'
              : '提交后 PM agent 会立即开始规划并执行（与桌面端一致，创建即生效）。'}
          </p>

          <Button
            block
            size="large"
            color="primary"
            loading={submitting}
            onClick={submit}
            style={{ marginTop: 20, borderRadius: 12 }}
          >
            创建任务
          </Button>
        </>
      )}
    </PageScaffold>
  )
}
