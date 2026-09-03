import { useState } from 'react'
import { Modal, Button, Card, Tag, Input, App, Empty, Spin, Upload } from 'antd'
import type { UploadFile } from 'antd'
import {
  PlusOutlined,
  UploadOutlined,
  BookOutlined,
  ExperimentOutlined,
  ScheduleOutlined,
  HistoryOutlined,
  PlayCircleOutlined,
  PauseCircleOutlined,
  RedoOutlined,
  DeleteOutlined,
  WarningOutlined,
  WifiOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import {
  useAgentCapabilities,
  useSetAgentCapabilities,
  useUploadSkillFile,
  useCronExecutions,
} from '@/hooks/useAgents'
import type { CapabilityExecution, CapabilityJob, SetCapabilityResult } from '@/types'

interface Props {
  agentId: string
}

function applyWritebackResult(result: SetCapabilityResult, okLabel: string) {
  if (!result.ok) return { type: 'error' as const, text: result.error || '写回失败' }
  if (result.restartStatus === 'restart_failed')
    return { type: 'warning' as const, text: '写回已生效，但 gateway 重启失败，请检查节点状态' }
  return { type: 'success' as const, text: `${okLabel}，已同步最新状态` }
}

const executionStatusStyle: Record<CapabilityExecution['status'], { label: string; color: string }> = {
  completed: { label: '成功', color: 'var(--success)' },
  failed: { label: '失败', color: 'var(--error)' },
  running: { label: '执行中', color: 'var(--info)' },
  unknown: { label: '未知', color: 'var(--text-quaternary)' },
}

function formatMsTime(ms: number): string {
  if (!ms) return '-'
  return dayjs(ms).format('YYYY-MM-DD HH:mm:ss')
}

function formatDuration(ms: number): string {
  if (ms <= 0) return ''
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

/* ---------------- 技能 Tab ---------------- */

function SkillAddModal({ agentId, open, onClose }: { agentId: string; open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const [name, setName] = useState('')
  const [fileList, setFileList] = useState<UploadFile[]>([])
  const [file, setFile] = useState<File | null>(null)
  const uploadSkill = useUploadSkillFile(agentId)
  const setCapabilities = useSetAgentCapabilities(agentId)
  const busy = uploadSkill.isPending || setCapabilities.isPending

  const reset = () => {
    setName('')
    setFileList([])
    setFile(null)
  }

  const submit = async () => {
    if (!name.trim() || !file) {
      message.error('请填写技能名并选择技能包文件')
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
      const r = applyWritebackResult(res.data, `技能 "${name.trim()}" 已部署`)
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
      reset()
      onClose()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '部署技能失败')
    }
  }

  return (
    <Modal
      open={open}
      title="部署新技能"
      onCancel={() => {
        reset()
        onClose()
      }}
      onOk={submit}
      confirmLoading={busy}
      okText="部署"
      cancelText="取消"
      destroyOnClose
    >
      <div className="flex flex-col gap-4 mt-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">技能名称</span>
          <Input placeholder="如 my-custom-skill" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">技能包文件（zip / SKILL.md + 脚本）</span>
          <Upload
            beforeUpload={(f) => {
              setFile(f)
              setFileList([f as unknown as UploadFile])
              return false
            }}
            onRemove={() => {
              setFile(null)
              setFileList([])
            }}
            fileList={fileList}
            maxCount={1}
            accept=".zip,.md,.txt,.yaml,.yml,.json,.js,.py,.sh"
          >
            <Button icon={<UploadOutlined />}>选择文件</Button>
          </Upload>
        </div>
      </div>
    </Modal>
  )
}

function HermesSkillsTab({ agentId }: Props) {
  const { data, isLoading } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)

  if (isLoading) return <Spin />
  if (!data?.available)
    return (
      <Empty
        image={<WifiOutlined style={{ fontSize: 32, color: 'var(--text-quaternary)' }} />}
        description={`能力信息暂不可用${data?.reason ? `：${data.reason}` : '，节点未响应能力查询'}`}
      />
    )

  const skills = data.skills ?? []

  return (
    <div className="flex flex-col gap-4">
      <Card bordered={false} className="!bg-white/[0.03]">
        <div className="flex items-center gap-2 border-b border-white/[0.08] px-4 py-3 -mx-4 -mt-4 mb-2">
          <BookOutlined className="text-white/50" />
          <span className="text-sm font-semibold text-white/90">技能</span>
          <Tag className="!text-xs" color="default">{skills.length}</Tag>
          <Button size="small" className="ml-auto" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
            部署技能
          </Button>
        </div>
        {skills.length === 0 ? (
          <p className="px-1 py-3 text-sm text-white/40">该节点暂无可展示的技能，点击右上角部署新技能</p>
        ) : (
          <div className="divide-y divide-white/[0.05]">
            {skills.map((s) => (
              <div key={s.name} className="flex items-start gap-3 px-1 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-white/90 truncate">{s.name}</span>
                    {s.category && <Tag className="!text-[10px]" color="default">{s.category}</Tag>}
                  </div>
                  {s.description && <p className="mt-0.5 text-xs text-white/50 line-clamp-2">{s.description}</p>}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
      <SkillAddModal agentId={agentId} open={addOpen} onClose={() => setAddOpen(false)} />
      <p className="flex items-center gap-1.5 text-xs text-white/40">
        <WarningOutlined /> 技能写回会重启节点 gateway，可能有数秒不可用窗口。
      </p>
    </div>
  )
}

/* ---------------- 模型 Tab ---------------- */

function ModelAddModal({ agentId, open, onClose }: { agentId: string; open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const [provider, setProvider] = useState('openai')
  const [model, setModel] = useState('')
  const [apiKey, setApiKey] = useState('')
  const setCapabilities = useSetAgentCapabilities(agentId)
  const busy = setCapabilities.isPending

  const submit = async () => {
    if (!provider.trim() || !model.trim()) {
      message.error('请填写 Provider 和模型名')
      return
    }
    try {
      const res = await setCapabilities.mutateAsync({
        target: 'model',
        action: 'add',
        model: provider.trim(),
        provider: {
          name: provider.trim(),
          model: model.trim(),
          api_key: apiKey.trim() || undefined,
        },
      })
      const r = applyWritebackResult(res.data, '模型 provider 已添加')
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
      setModel('')
      setApiKey('')
      onClose()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '添加模型失败')
    }
  }

  return (
    <Modal
      open={open}
      title="新增模型 provider"
      onCancel={onClose}
      onOk={submit}
      confirmLoading={busy}
      okText="添加"
      cancelText="取消"
      destroyOnClose
    >
      <div className="flex flex-col gap-4 mt-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">Provider</span>
          <Input placeholder="如 openai" value={provider} onChange={(e) => setProvider(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">模型名</span>
          <Input placeholder="如 gpt-4o" value={model} onChange={(e) => setModel(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">API Key（选填）</span>
          <Input.Password placeholder="••••••••" value={apiKey} onChange={(e) => setApiKey(e.target.value)} />
        </div>
      </div>
    </Modal>
  )
}

function HermesModelsTab({ agentId }: Props) {
  const { data, isLoading } = useAgentCapabilities(agentId)
  const { message } = App.useApp()
  const [addOpen, setAddOpen] = useState(false)
  const setCapabilities = useSetAgentCapabilities(agentId)

  if (isLoading) return <Spin />
  if (!data?.available)
    return (
      <Empty
        image={<WifiOutlined style={{ fontSize: 32, color: 'var(--text-quaternary)' }} />}
        description={`能力信息暂不可用${data?.reason ? `：${data.reason}` : '，节点未响应能力查询'}`}
      />
    )

  const models = data.models ?? []

  const switchDefault = async (modelId: string) => {
    try {
      const res = await setCapabilities.mutateAsync({ target: 'model', action: 'switch', model: modelId })
      const r = applyWritebackResult(res.data, '默认模型已切换')
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '切换默认模型失败')
    }
  }

  const removeModel = async (modelId: string) => {
    Modal.confirm({
      title: '确定删除该模型 provider 吗？',
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        try {
          const res = await setCapabilities.mutateAsync({ target: 'model', action: 'delete', model: modelId })
          const r = applyWritebackResult(res.data, '模型 provider 已删除')
          if (r.type === 'success') message.success(r.text)
          else if (r.type === 'warning') message.warning(r.text)
          else message.error(r.text)
        } catch (err) {
          message.error(err instanceof Error ? err.message : '删除模型失败')
        }
      },
    })
  }

  return (
    <div className="flex flex-col gap-4">
      <Card bordered={false} className="!bg-white/[0.03]">
        <div className="flex items-center gap-2 border-b border-white/[0.08] px-4 py-3 -mx-4 -mt-4 mb-2">
          <ExperimentOutlined className="text-white/50" />
          <span className="text-sm font-semibold text-white/90">推理模型</span>
          <Tag className="!text-xs" color="default">{models.length}</Tag>
          <Button size="small" className="ml-auto" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
            添加 provider
          </Button>
        </div>
        {models.length === 0 ? (
          <p className="px-1 py-3 text-sm text-white/40">该节点暂无可展示的模型，点击右上角添加 provider</p>
        ) : (
          <div className="divide-y divide-white/[0.05]">
            {models.map((m) => (
              <div key={m.id ?? m.model} className="flex items-center gap-3 px-1 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-white/90 truncate">{m.model}</span>
                    {m.isDefault && <Tag className="!text-[10px]" color="green">默认</Tag>}
                  </div>
                  <p className="mt-0.5 text-xs text-white/50 truncate font-mono">{m.provider}</p>
                </div>
                {!m.isDefault && (
                  <div className="flex items-center gap-1 shrink-0">
                    <Button size="small" type="link" onClick={() => switchDefault(m.id ?? m.model)} disabled={setCapabilities.isPending}>
                      设为默认
                    </Button>
                    <Button size="small" type="link" danger onClick={() => removeModel(m.id ?? m.model)} disabled={setCapabilities.isPending}>
                      删除
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>
      <ModelAddModal agentId={agentId} open={addOpen} onClose={() => setAddOpen(false)} />
      <p className="flex items-center gap-1.5 text-xs text-white/40">
        <WarningOutlined /> 模型写回会重启节点 gateway，API Key 仅写回时传输、不回显。
      </p>
    </div>
  )
}

/* ---------------- 定时任务 Tab ---------------- */

function ExecutionResultModal({ execution, open, onClose }: { execution: CapabilityExecution | null; open: boolean; onClose: () => void }) {
  if (!execution) return null
  const body = (execution.output || execution.outputPreview || '').trim()
  const isSilent = body.startsWith('[SILENT]')
  const renderBody = isSilent ? body.slice('[SILENT]'.length).trim() : body
  const status = executionStatusStyle[execution.status]

  return (
    <Modal open={open} title={`执行结果 — ${formatMsTime(execution.startedAtMs)}`} onCancel={onClose} footer={<Button onClick={onClose}>关闭</Button>} width={680}>
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-2 text-xs text-white/50 flex-wrap">
          <Tag style={{ color: status.color, borderColor: status.color }}>{status.label}</Tag>
          {formatDuration(execution.durationMs) && <span>耗时 {formatDuration(execution.durationMs)}</span>}
          {execution.error && <span className="text-[color:var(--error)]">错误: {execution.error}</span>}
        </div>
        {isSilent && (
          <div className="rounded-md bg-white/[0.04] px-3 py-2 text-xs text-white/50">本次执行无新内容（[SILENT]）</div>
        )}
        {renderBody ? (
          <pre className="max-h-[50vh] overflow-auto whitespace-pre-wrap break-words rounded-lg border border-white/[0.08] bg-black/30 p-3 text-xs text-white/80">{renderBody}</pre>
        ) : (
          <p className="text-sm text-white/40">该执行无结果输出</p>
        )}
      </div>
    </Modal>
  )
}

function JobExecutionsModal({ agentId, job, open, onClose }: { agentId: string; job: CapabilityJob; open: boolean; onClose: () => void }) {
  const { data, isLoading, refetch } = useCronExecutions(agentId, job.id, open)
  const [viewing, setViewing] = useState<CapabilityExecution | null>(null)
  const executions = data?.executions ?? []
  const error = data?.error

  return (
    <>
      <Modal
        open={open}
        title={`执行历史 — ${job.name}`}
        onCancel={onClose}
        footer={<Button onClick={onClose}>关闭</Button>}
        width={680}
      >
        <div className="flex items-center justify-between mb-3">
          <p className="text-xs text-white/50">cron: {job.schedule} {job.enabled ? '' : '（已暂停）'}</p>
          <Button size="small" icon={<RedoOutlined />} onClick={() => refetch()} loading={isLoading}>刷新</Button>
        </div>
        {isLoading ? (
          <div className="py-8 text-center text-sm text-white/40">加载中...</div>
        ) : executions.length === 0 ? (
          <div className="py-8 text-center text-sm text-white/40">{error ? `暂无执行记录（${error}）` : '暂无执行记录'}</div>
        ) : (
          <div className="flex flex-col divide-y divide-white/10 rounded-lg border border-white/[0.08]">
            {executions.map((ex) => {
              const st = executionStatusStyle[ex.status]
              return (
                <button
                  key={ex.executionId}
                  type="button"
                  className="flex items-center gap-3 px-3 py-2.5 text-left hover:bg-white/5 cursor-pointer"
                  onClick={() => setViewing(ex)}
                >
                  <Tag style={{ color: st.color, borderColor: st.color }} className="shrink-0">{st.label}</Tag>
                  <div className="min-w-0 flex-1">
                    <div className="text-xs text-white/50">
                      {formatMsTime(ex.startedAtMs)} · 耗时 {formatDuration(ex.durationMs)}
                    </div>
                    {ex.error && <div className="text-xs text-[color:var(--error)] truncate">{ex.error}</div>}
                  </div>
                  <span className="text-xs text-white/40 shrink-0">查看结果 ›</span>
                </button>
              )
            })}
          </div>
        )}
      </Modal>
      <ExecutionResultModal execution={viewing} open={!!viewing} onClose={() => setViewing(null)} />
    </>
  )
}

function JobRow({ job, agentId }: { job: CapabilityJob; agentId: string }) {
  const { message } = App.useApp()
  const setCapabilities = useSetAgentCapabilities(agentId)
  const [historyOpen, setHistoryOpen] = useState(false)
  const latest = job.executions?.[0]
  const latestStatus = latest ? executionStatusStyle[latest.status] : null

  const runAction = async (action: string) => {
    try {
      const res = await setCapabilities.mutateAsync({ target: 'cron', action, jobId: job.id })
      const r = applyWritebackResult(
        res.data,
        action === 'run' ? '任务已触发运行' : action === 'pause' ? '任务已暂停' : action === 'resume' ? '任务已恢复' : '任务已删除',
      )
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
    } catch (err) {
      message.error(err instanceof Error ? err.message : '操作失败')
    }
  }

  return (
    <div className="flex items-start gap-3 px-1 py-3 first:pt-0 last:pb-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-white/90 truncate">{job.name}</span>
          {!job.enabled && <Tag className="!text-[10px]" color="default">已暂停</Tag>}
          {latestStatus && (
            <Tag className="!text-[10px]" style={{ color: latestStatus.color, borderColor: latestStatus.color }}>
              {latestStatus.label}
            </Tag>
          )}
        </div>
        <p className="mt-0.5 text-xs text-white/50 truncate font-mono">{job.schedule}</p>
        {latest && (
          <p className="mt-0.5 text-xs text-white/50 truncate">
            最近执行: {dayjs(latest.startedAtMs).fromNow()}
            {formatDuration(latest.durationMs) ? ` · 耗时 ${formatDuration(latest.durationMs)}` : ''}
          </p>
        )}
        {job.nextRun && <p className="mt-0.5 text-xs text-white/50">下次运行: {job.nextRun}</p>}
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <Button size="small" type="text" icon={<HistoryOutlined />} onClick={() => setHistoryOpen(true)} title="执行历史" />
        <Button size="small" type="text" icon={<PlayCircleOutlined />} onClick={() => runAction('run')} loading={setCapabilities.isPending} title="立即运行" />
        {job.enabled ? (
          <Button size="small" type="text" icon={<PauseCircleOutlined />} onClick={() => runAction('pause')} loading={setCapabilities.isPending} title="暂停" />
        ) : (
          <Button size="small" type="text" icon={<RedoOutlined />} onClick={() => runAction('resume')} loading={setCapabilities.isPending} title="恢复" />
        )}
        <Button size="small" type="text" danger icon={<DeleteOutlined />} onClick={() => runAction('delete')} loading={setCapabilities.isPending} title="删除" />
      </div>
      <JobExecutionsModal agentId={agentId} job={job} open={historyOpen} onClose={() => setHistoryOpen(false)} />
    </div>
  )
}

function JobAddModal({ agentId, open, onClose }: { agentId: string; open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const [name, setName] = useState('')
  const [schedule, setSchedule] = useState('0 9 * * *')
  const [prompt, setPrompt] = useState('')
  const setCapabilities = useSetAgentCapabilities(agentId)
  const busy = setCapabilities.isPending

  const submit = async () => {
    if (!name.trim() || !prompt.trim()) {
      message.error('请填写任务名称和任务提示词')
      return
    }
    try {
      const res = await setCapabilities.mutateAsync({
        target: 'cron',
        action: 'create',
        job: { name: name.trim(), schedule: schedule.trim(), prompt: prompt.trim() },
      })
      const r = applyWritebackResult(res.data, '定时任务已创建')
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
      setName('')
      setSchedule('0 9 * * *')
      setPrompt('')
      onClose()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建定时任务失败')
    }
  }

  return (
    <Modal
      open={open}
      title="新建定时任务"
      onCancel={onClose}
      onOk={submit}
      confirmLoading={busy}
      okText="创建"
      cancelText="取消"
      destroyOnClose
    >
      <div className="flex flex-col gap-4 mt-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">任务名称</span>
          <Input placeholder="如 daily-report" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">Cron 表达式</span>
          <Input placeholder="0 9 * * *" value={schedule} onChange={(e) => setSchedule(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-white/70">任务提示词</span>
          <Input.TextArea rows={3} placeholder="每天生成日报并发送" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
        </div>
      </div>
    </Modal>
  )
}

function HermesJobsTab({ agentId }: Props) {
  const { data, isLoading } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)

  if (isLoading) return <Spin />
  if (!data?.available)
    return (
      <Empty
        image={<WifiOutlined style={{ fontSize: 32, color: 'var(--text-quaternary)' }} />}
        description={`能力信息暂不可用${data?.reason ? `：${data.reason}` : '，节点未响应能力查询'}`}
      />
    )

  const jobs = data.jobs ?? []

  return (
    <div className="flex flex-col gap-4">
      <Card bordered={false} className="!bg-white/[0.03]">
        <div className="flex items-center gap-2 border-b border-white/[0.08] px-4 py-3 -mx-4 -mt-4 mb-2">
          <ScheduleOutlined className="text-white/50" />
          <span className="text-sm font-semibold text-white/90">定时任务</span>
          <Tag className="!text-xs" color="default">{jobs.length}</Tag>
          <Button size="small" className="ml-auto" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
            新建任务
          </Button>
        </div>
        {jobs.length === 0 ? (
          <p className="px-1 py-3 text-sm text-white/40">该节点暂无定时任务，点击右上角新建</p>
        ) : (
          <div className="divide-y divide-white/[0.05]">
            {jobs.map((j) => <JobRow key={j.id} job={j} agentId={agentId} />)}
          </div>
        )}
      </Card>
      <JobAddModal agentId={agentId} open={addOpen} onClose={() => setAddOpen(false)} />
      <p className="flex items-center gap-1.5 text-xs text-white/40">
        <WarningOutlined /> 定时任务由节点 gateway 原生调度，写回无需重启。
      </p>
    </div>
  )
}

export { HermesSkillsTab, HermesModelsTab, HermesJobsTab }
