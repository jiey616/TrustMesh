import { useState } from 'react'
import { BookOpen, BrainCircuit, CalendarClock, WifiOff, CircleSlash2, Plus, Upload, Play, Pause, RotateCcw, Trash2, Check, History } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/shared/EmptyState'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useAgentCapabilities, useCronExecutions, useSetAgentCapabilities, useUploadSkillFile, type WritebackStatus } from '@/hooks/useAgents'
import { toast } from 'sonner'
import { formatRelativeTime } from '@/lib/utils'
import type { CapabilityExecution, CapabilityJob, CapabilitySkill, SetCapabilityResult } from '@/types'

interface HermesCapabilityTabProps {
  agentId: string
}

// 统一的写回结果提示：处理 restartStatus / error
// 返回刷新状态回调：写入成功后进入「正在刷新」，刷新完成/失败时提示
function handleWritebackResult(result: SetCapabilityResult, okLabel: string, onRefresh?: (state: WritebackStatus) => void) {
  if (!result.ok) {
    toast.error(result.error || '写回失败')
    return
  }
  if (result.restartStatus === 'restart_failed') {
    toast.warning('写回已生效，但 gateway 重启失败，请检查节点状态')
    return
  }
  if (onRefresh) {
    // 写回成功：提示已生效，进入后台刷新
    toast.success(`${okLabel}，正在刷新…`)
    onRefresh('start')
  } else {
    toast.success(okLabel)
  }
}

// 刷新状态跟随提示：start → 静默（已提示），done/failed → 补充提示
function applyRefreshFeedback(state: WritebackStatus) {
  if (state === 'done') {
    toast.success('已同步最新状态')
  } else if (state === 'failed') {
    toast.warning('写入已生效，数据同步中，请稍后手动刷新查看最新状态')
  }
}

// --- 通用展示元素 ---

function SectionCard({
  icon: Icon,
  title,
  count,
  action,
  children,
}: {
  icon: typeof BookOpen
  title: string
  count: number
  action?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="rounded-xl border bg-card">
      <header className="flex items-center gap-2 border-b px-4 py-3">
        <Icon className="size-4 text-muted-foreground" />
        <h3 className="text-sm font-semibold">{title}</h3>
        <Badge variant="secondary" className="ml-auto text-xs">{count}</Badge>
        {action}
      </header>
      <div className="divide-y">{children}</div>
    </section>
  )
}

function EmptyList({ label }: { label: string }) {
  return (
    <p className="px-4 py-4 text-sm text-muted-foreground">{label}</p>
  )
}

function CapabilitySkeleton() {
  return (
    <div className="space-y-2 p-4">
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} className="h-8 w-full" />
      ))}
    </div>
  )
}

function CapabilityUnavailable({ reason }: { reason?: string }) {
  return (
    <EmptyState
      icon={WifiOff}
      title="能力信息暂不可用"
      description={reason || '节点未响应能力查询，请稍后重试或检查节点连接状态。'}
    />
  )
}

// --- 技能 Tab ---

function SkillRow({ skill }: { skill: CapabilitySkill }) {
  return (
    <div className="flex items-start gap-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">{skill.name}</span>
          {skill.category && <Badge variant="outline" className="shrink-0 text-[10px]">{skill.category}</Badge>}
        </div>
        {skill.description && (
          <p className="mt-0.5 text-xs text-muted-foreground line-clamp-2">{skill.description}</p>
        )}
      </div>
    </div>
  )
}

function SkillAddDialog({ agentId, open, onOpenChange }: { agentId: string; open: boolean; onOpenChange: (v: boolean) => void }) {
  const [name, setName] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const uploadSkill = useUploadSkillFile(agentId)
  const setCapabilities = useSetAgentCapabilities(agentId, {
    onWriteback: applyRefreshFeedback,
  })
  const busy = uploadSkill.isPending || setCapabilities.isPending

  const submit = async () => {
    if (!name.trim() || !file) {
      toast.error('请填写技能名并选择技能包文件')
      return
    }
    try {
      const upRes = await uploadSkill.mutateAsync(file)
      const fileId = upRes.data.fileId
      const res = await setCapabilities.mutateAsync({
        target: 'skill',
        action: 'add',
        skill: name.trim(),
        fileIds: [fileId],
      })
      handleWritebackResult(res.data, `技能 "${name.trim()}" 已部署`, applyRefreshFeedback)
      setName('')
      setFile(null)
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '部署技能失败')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>部署新技能</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="skill-name">技能名称</Label>
            <Input id="skill-name" placeholder="如 my-custom-skill" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="skill-file">技能包文件（zip / SKILL.md + 脚本）</Label>
            <Input
              id="skill-file"
              type="file"
              accept=".zip,.md,.txt,.yaml,.yml,.json,.js,.py,.sh"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>取消</Button>
          <Button onClick={submit} disabled={busy}>
            <Upload className="size-3.5 mr-1.5" />
            {busy ? '部署中...' : '部署'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function HermesSkillsTab({ agentId }: HermesCapabilityTabProps) {
  const { data, isLoading } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <div className="rounded-xl border bg-card"><CapabilitySkeleton /></div>
      </div>
    )
  }
  if (!data?.available) {
    return <CapabilityUnavailable reason={data?.reason} />
  }

  const skills = data.skills ?? []

  return (
    <div className="flex flex-col gap-4">
      <SectionCard
        icon={BookOpen}
        title="技能"
        count={skills.length}
        action={
          <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
            <Plus className="size-4 mr-1.5" />
            部署技能
          </Button>
        }
      >
        {skills.length === 0 ? (
          <EmptyList label="该节点暂无可展示的技能，点击右上角部署新技能" />
        ) : (
          skills.map((s) => <SkillRow key={s.name} skill={s} />)
        )}
      </SectionCard>
      <SkillAddDialog agentId={agentId} open={addOpen} onOpenChange={setAddOpen} />
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <CircleSlash2 className="size-3.5" />
        技能写回会重启节点 gateway，可能有数秒不可用窗口。
      </p>
    </div>
  )
}

// --- 模型 Tab ---

function ModelAddDialog({ agentId, open, onOpenChange }: { agentId: string; open: boolean; onOpenChange: (v: boolean) => void }) {
  const [provider, setProvider] = useState('openai')
  const [model, setModel] = useState('')
  const [apiKey, setApiKey] = useState('')
  const setCapabilities = useSetAgentCapabilities(agentId, {
    onWriteback: applyRefreshFeedback,
  })
  const busy = setCapabilities.isPending

  const submit = async () => {
    if (!provider.trim() || !model.trim()) {
      toast.error('请填写 Provider 和模型名')
      return
    }
    try {
      const res = await setCapabilities.mutateAsync({
        target: 'model',
        action: 'add',
        model: provider.trim(), // 契约：model 字段为 provider 标识
        provider: {
          name: provider.trim(), // hermes 适配器校验：model add 必须带 provider.name
          model: model.trim(),
          api_key: apiKey.trim() || undefined,
        },
      })
      handleWritebackResult(res.data, '模型 provider 已添加', applyRefreshFeedback)
      setModel('')
      setApiKey('')
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '添加模型失败')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新增模型 provider</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="model-provider">Provider</Label>
            <Input id="model-provider" placeholder="如 openai" value={provider} onChange={(e) => setProvider(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="model-name">模型名</Label>
            <Input id="model-name" placeholder="如 gpt-4o" value={model} onChange={(e) => setModel(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="model-key">API Key（选填）</Label>
            <Input id="model-key" type="password" placeholder="••••••••" value={apiKey} onChange={(e) => setApiKey(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>取消</Button>
          <Button onClick={submit} disabled={busy}>
            <Check className="size-3.5 mr-1.5" />
            {busy ? '提交中...' : '添加'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function HermesModelsTab({ agentId }: HermesCapabilityTabProps) {
  const { data, isLoading } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)
  const setCapabilities = useSetAgentCapabilities(agentId, {
    onWriteback: applyRefreshFeedback,
    // model switch 校验：读回的默认模型必须是写回目标（避免 gateway 重启窗口
    // 读到旧 config 却误判"已同步最新状态"）
    verify: (input, info) => {
      if (input.target !== 'model' || input.action !== 'switch' || !input.model) return true
      const target = input.model
      return (info.models ?? []).some(
        (m) => m.isDefault && (m.provider === target || m.provider === target),
      )
    },
  })

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <div className="rounded-xl border bg-card"><CapabilitySkeleton /></div>
      </div>
    )
  }
  if (!data?.available) {
    return <CapabilityUnavailable reason={data?.reason} />
  }

  const models = data.models ?? []

  const switchDefault = async (modelId: string) => {
    try {
      const res = await setCapabilities.mutateAsync({ target: 'model', action: 'switch', model: modelId })
      handleWritebackResult(res.data, '默认模型已切换', applyRefreshFeedback)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '切换默认模型失败')
    }
  }

  const removeModel = async (modelId: string) => {
    if (!confirm('确定删除该模型 provider 吗？')) return
    try {
      const res = await setCapabilities.mutateAsync({ target: 'model', action: 'delete', model: modelId })
      handleWritebackResult(res.data, '模型 provider 已删除', applyRefreshFeedback)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '删除模型失败')
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <SectionCard
        icon={BrainCircuit}
        title="推理模型"
        count={models.length}
        action={
          <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
            <Plus className="size-4 mr-1.5" />
            添加 provider
          </Button>
        }
      >
        {models.length === 0 ? (
          <EmptyList label="该节点暂无可展示的模型，点击右上角添加 provider" />
        ) : (
          models.map((m) => (
            <div key={m.id ?? m.model} className="flex items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium truncate">{m.model}</span>
                  {m.isDefault && <Badge variant="info" className="shrink-0 text-[10px]">默认</Badge>}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground truncate font-mono">{m.provider}</p>
              </div>
              {!m.isDefault && (
                <div className="flex items-center gap-1 shrink-0">
                  <Button size="sm" variant="ghost" className="h-6 text-xs" onClick={() => switchDefault(m.id ?? m.model)} disabled={setCapabilities.isPending}>
                    设为默认
                  </Button>
                  <Button size="sm" variant="ghost" className="h-6 w-6 p-0 text-destructive" onClick={() => removeModel(m.id ?? m.model)} disabled={setCapabilities.isPending} title="删除">
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              )}
            </div>
          ))
        )}
      </SectionCard>
      <ModelAddDialog agentId={agentId} open={addOpen} onOpenChange={setAddOpen} />
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <CircleSlash2 className="size-3.5" />
        模型写回会重启节点 gateway，API Key 仅写回时传输、不回显。
      </p>
    </div>
  )
}

// --- 定时任务 Tab ---

const executionStatusStyle: Record<CapabilityExecution['status'], { label: string; className: string }> = {
  completed: { label: '成功', className: 'text-success' },
  failed: { label: '失败', className: 'text-destructive' },
  running: { label: '执行中', className: 'text-info' },
  unknown: { label: '未知', className: 'text-muted-foreground' },
}

function formatMsTime(ms: number): string {
  if (!ms) return ''
  return new Date(ms).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })
}

function formatDuration(ms: number): string {
  if (ms <= 0) return ''
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

// 结果预览弹窗：渲染 markdown（优先完整 output，缺省用 preview），[SILENT] 提示"本次无新内容"但不隐藏原文
function ExecutionResultDialog({ execution, open, onOpenChange }: {
  execution: CapabilityExecution | null
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  if (!execution) return null
  const body = (execution.output || execution.outputPreview || '').trim()
  const isSilent = body.startsWith('[SILENT]')
  const renderBody = isSilent ? body.slice('[SILENT]'.length).trim() : body

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>执行结果 — {formatMsTime(execution.startedAtMs)}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex items-center gap-2 text-xs text-muted-foreground flex-wrap">
            <Badge variant="outline" className={executionStatusStyle[execution.status]?.className}>
              {executionStatusStyle[execution.status]?.label ?? execution.status}
            </Badge>
            {formatDuration(execution.durationMs) && <span>耗时 {formatDuration(execution.durationMs)}</span>}
            {execution.error && <span className="text-destructive">错误: {execution.error}</span>}
          </div>
          {isSilent && (
            <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              本次执行无新内容（[SILENT]）
            </div>
          )}
          {renderBody ? (
            <div className="prose prose-sm max-w-none dark:prose-invert">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{renderBody}</ReactMarkdown>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">该执行无结果输出</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// 执行历史 Dialog：打开时拉取完整历史（第 2 层详情接口）
function JobExecutionsDialog({ agentId, job, open, onOpenChange }: {
  agentId: string
  job: CapabilityJob
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const { data, isLoading, refetch } = useCronExecutions(agentId, job.id, open)
  const [viewing, setViewing] = useState<CapabilityExecution | null>(null)
  const executions = data?.executions ?? []
  const error = data?.error

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-2xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>执行历史 — {job.name}</DialogTitle>
          </DialogHeader>
          <div className="flex items-center justify-between">
            <p className="text-xs text-muted-foreground">cron: {job.schedule} {job.enabled ? '' : '（已暂停）'}</p>
            <Button size="sm" variant="ghost" className="h-6 text-xs" onClick={() => refetch()} disabled={isLoading}>
              <RotateCcw className="size-3 mr-1" />
              刷新
            </Button>
          </div>

          {isLoading ? (
            <div className="py-8 text-center text-sm text-muted-foreground">加载中...</div>
          ) : executions.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              {error ? `暂无执行记录（${error}）` : '暂无执行记录'}
            </div>
          ) : (
            <div className="flex flex-col divide-y rounded-lg border">
              {executions.map((ex) => (
                <button
                  key={ex.executionId}
                  className="flex items-center gap-3 px-3 py-2.5 text-left hover:bg-muted/50 cursor-pointer"
                  onClick={() => setViewing(ex)}
                >
                  <Badge variant="outline" className={`shrink-0 ${executionStatusStyle[ex.status]?.className ?? ''}`}>
                    {executionStatusStyle[ex.status]?.label ?? ex.status}
                  </Badge>
                  <div className="min-w-0 flex-1">
                    <div className="text-xs text-muted-foreground">
                      {formatMsTime(ex.startedAtMs)} · 耗时 {formatDuration(ex.durationMs)}
                    </div>
                    {ex.error && <div className="text-xs text-destructive truncate">{ex.error}</div>}
                  </div>
                  <span className="text-xs text-muted-foreground shrink-0">查看结果 ›</span>
                </button>
              ))}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ExecutionResultDialog execution={viewing} open={!!viewing} onOpenChange={(v) => !v && setViewing(null)} />
    </>
  )
}

function JobRow({ job, agentId }: { job: CapabilityJob; agentId: string }) {
  const setCapabilities = useSetAgentCapabilities(agentId, {
    onWriteback: applyRefreshFeedback,
  })
  const busy = setCapabilities.isPending
  const [historyOpen, setHistoryOpen] = useState(false)
  const latest = job.executions?.[0]
  const latestStatus = latest ? executionStatusStyle[latest.status] : null

  const runAction = async (action: string) => {
    try {
      const res = await setCapabilities.mutateAsync({ target: 'cron', action, jobId: job.id })
      handleWritebackResult(res.data, action === 'run' ? '任务已触发运行' : action === 'pause' ? '任务已暂停' : '任务已恢复')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    }
  }

  return (
    <div className="flex items-start gap-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">{job.name}</span>
          {!job.enabled && <Badge variant="secondary" className="shrink-0 text-[10px]">已暂停</Badge>}
          {latestStatus && <Badge variant="outline" className={`shrink-0 text-[10px] ${latestStatus.className}`}>{latestStatus.label}</Badge>}
        </div>
        <p className="mt-0.5 text-xs text-muted-foreground truncate font-mono">{job.schedule}</p>
        {latest && (
          <p className="mt-0.5 text-xs text-muted-foreground truncate">
            最近执行: {formatRelativeTime(new Date(latest.startedAtMs).toISOString())}
            {formatDuration(latest.durationMs) ? ` · 耗时 ${formatDuration(latest.durationMs)}` : ''}
          </p>
        )}
        {job.nextRun && <p className="mt-0.5 text-xs text-muted-foreground">下次运行: {job.nextRun}</p>}
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => setHistoryOpen(true)} title="执行历史">
          <History className="size-3.5" />
        </Button>
        <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => runAction('run')} disabled={busy} title="立即运行">
          <Play className="size-3.5" />
        </Button>
        {job.enabled ? (
          <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => runAction('pause')} disabled={busy} title="暂停">
            <Pause className="size-3.5" />
          </Button>
        ) : (
          <Button size="sm" variant="ghost" className="h-6 w-6 p-0" onClick={() => runAction('resume')} disabled={busy} title="恢复">
            <RotateCcw className="size-3.5" />
          </Button>
        )}
        <Button size="sm" variant="ghost" className="h-6 w-6 p-0 text-destructive" onClick={() => runAction('delete')} disabled={busy} title="删除">
          <Trash2 className="size-3.5" />
        </Button>
      </div>
      <JobExecutionsDialog agentId={agentId} job={job} open={historyOpen} onOpenChange={setHistoryOpen} />
    </div>
  )
}

function JobAddDialog({ agentId, open, onOpenChange }: { agentId: string; open: boolean; onOpenChange: (v: boolean) => void }) {
  const [name, setName] = useState('')
  const [schedule, setSchedule] = useState('0 9 * * *')
  const [prompt, setPrompt] = useState('')
  const setCapabilities = useSetAgentCapabilities(agentId, {
    onWriteback: applyRefreshFeedback,
  })
  const busy = setCapabilities.isPending

  const submit = async () => {
    if (!name.trim() || !prompt.trim()) {
      toast.error('请填写任务名称和任务提示词')
      return
    }
    try {
      const res = await setCapabilities.mutateAsync({
        target: 'cron',
        action: 'create',
        job: {
          name: name.trim(),
          schedule: schedule.trim(),
          prompt: prompt.trim(),
        },
      })
      handleWritebackResult(res.data, '定时任务已创建')
      setName('')
      setSchedule('0 9 * * *')
      setPrompt('')
      onOpenChange(false)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建定时任务失败')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建定时任务</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="job-name">任务名称</Label>
            <Input id="job-name" placeholder="如 daily-report" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="job-schedule">Cron 表达式</Label>
            <Input id="job-schedule" placeholder="0 9 * * *" value={schedule} onChange={(e) => setSchedule(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="job-prompt">任务提示词</Label>
            <Input id="job-prompt" placeholder="每天生成日报并发送" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>取消</Button>
          <Button onClick={submit} disabled={busy}>
            <Check className="size-3.5 mr-1.5" />
            {busy ? '创建中...' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function HermesJobsTab({ agentId }: HermesCapabilityTabProps) {
  const { data, isLoading } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)

  if (isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <div className="rounded-xl border bg-card"><CapabilitySkeleton /></div>
      </div>
    )
  }
  if (!data?.available) {
    return <CapabilityUnavailable reason={data?.reason} />
  }

  const jobs = data.jobs ?? []

  return (
    <div className="flex flex-col gap-4">
      <SectionCard
        icon={CalendarClock}
        title="定时任务"
        count={jobs.length}
        action={
          <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
            <Plus className="size-4 mr-1.5" />
            新建任务
          </Button>
        }
      >
        {jobs.length === 0 ? (
          <EmptyList label="该节点暂无定时任务，点击右上角新建" />
        ) : (
          jobs.map((j) => <JobRow key={j.id} job={j} agentId={agentId} />)
        )}
      </SectionCard>
      <JobAddDialog agentId={agentId} open={addOpen} onOpenChange={setAddOpen} />
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <CircleSlash2 className="size-3.5" />
        定时任务由节点 gateway 原生调度，写回无需重启。
      </p>
    </div>
  )
}
