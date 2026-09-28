import { useEffect, useState } from 'react'
import { Modal, Button, Card, Tag, Input, AutoComplete, Space, App, Empty, Spin, Upload } from 'antd'
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
  UnorderedListOutlined,
  WifiOutlined,
  SyncOutlined,
  LoadingOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'
import { isTimeoutError } from 'ky'
import {
  useAgentCapabilities,
  useSetAgentCapabilities,
  useUploadSkillFile,
  useCronExecutions,
} from '@/hooks/useAgents'
import { fetchLLMModels, testLLMConfig } from '@/api/llmConfig'
import { usePermStore } from '@/stores/permStore'
import { PERM } from '@/lib/perms'
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

type AppMessage = ReturnType<typeof App.useApp>['message']

/**
 * 写回类操作的统一失败提示。
 *
 * 🔴 **超时 ≠ 失败**：写回要经 daemon → NATS 穿透到远端节点、同步等回执，并重启该节点
 * gateway，生产实测单次 9.97s（贴近旧的 10s 上限，现已放宽到 70s）。真的超时发生时，
 * 节点侧很可能**已经写成功**；此时若照常弹「失败」，用户会去点第二次、第三次。
 * 因此超时单独走中性提示；真正的失败由 `applyWritebackResult`（`ok=false`）或这里报错。
 */
function notifyWritebackError(err: unknown, message: AppMessage, fallback: string) {
  if (isTimeoutError(err)) {
    message.warning('写回已下发，节点正在重启 gateway（实测约 25 秒）；结果稍后自动刷新')
    return
  }
  message.error(err instanceof Error ? err.message : fallback)
}

/**
 * 原始错误串的折叠展示：默认收起，排查时再展开。
 *
 * 后端把 Go 的原始错误直接塞进了 reason（handler/agent.go 的 GetCapabilities），
 * 形如 `Get "http://clawsynapse:18080/v1/peers/n1-.../capabilities": ... Client.Timeout`
 * —— 含内部地址与实现细节，摊给用户既看不懂又像故障，但排查时又确实需要它。
 */
function CapabilityErrorDetail({ reason }: { reason: string }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="max-w-[520px]">
      <button
        type="button"
        className="cursor-pointer text-xs text-[color:var(--text-quaternary)] hover:underline underline-offset-2"
        onClick={() => setOpen((v) => !v)}
      >
        {open ? '收起技术详情' : '技术详情'}
      </button>
      {open && (
        <pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded border border-[color:var(--line)] bg-[color:var(--surface-inset)] p-2 text-left text-[11px] text-[color:var(--text-quaternary)]">
          {reason}
        </pre>
      )}
    </div>
  )
}

/**
 * 「连旧数据都没有」时的兜底页：首次加载就失败，无可降级内容。
 *
 * 🔴 正文只说人话 —— 用户关心的是「等一会儿就好」，不是 clawsynapse 的地址。
 * 真正需要原文的场景（排查）走折叠区。
 */
function CapabilityUnavailable({ reason, onRetry, retrying }: { reason?: string; onRetry: () => void; retrying: boolean }) {
  return (
    <Empty
      image={<WifiOutlined style={{ fontSize: 32, color: 'var(--text-quaternary)' }} />}
      description={
        <div className="flex flex-col items-center gap-2">
          <span className="text-sm text-[color:var(--text-secondary)]">
            节点正在重启 gateway，能力信息暂时读不到，稍后会自动重试
          </span>
          <Button size="small" icon={<RedoOutlined />} loading={retrying} onClick={onRetry}>
            立即重试
          </Button>
          {reason && <CapabilityErrorDetail reason={reason} />}
        </div>
      }
    />
  )
}

/**
 * 「有旧数据、但当前读失败」时的顶部细提示。
 *
 * 与 CapabilityUnavailable 的分工：那个用于手里**什么都没有**的场景；
 * 这个用于「上一次的数据还在，只是这一两秒同步不上」—— 此时**保留列表**、
 * 只在顶部挂一条提示。一次 3 秒的网络抖动不该以「整页消失」为代价。
 */
function CapabilityStaleBanner({ onRetry, retrying }: { onRetry: () => void; retrying: boolean }) {
  return (
    <div className="flex items-center gap-2 rounded-md border border-[color:var(--line)] bg-[color:var(--surface)] px-3 py-2 text-xs text-[color:var(--text-tertiary)]">
      <SyncOutlined spin={retrying} className="text-[color:var(--info)]" />
      <span>节点正在同步，下方为最近一次成功读取的数据</span>
      <Button size="small" type="link" loading={retrying} onClick={onRetry}>
        重试
      </Button>
    </div>
  )
}

/**
 * 写回进行中的横幅。
 *
 * 🔴 写回是「同步穿透到远端节点 + 重启该节点 gateway」的重操作，**生产实测 25.5s**
 * （clawsynapse 的 restartGateway：等端口释放 15s + 等 health 30s 两段预算）。
 * 上一版只把按钮置灰，25.5 秒里界面毫无变化 —— 用户正是因此以为「点了没反应」
 * 而反复点击（生产日志里同一操作 3 分钟内出现 3 次）。这条横幅就是为了让
 * 「已被接受、正在处理」变得可见。
 */
function WritebackProgressBanner({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 rounded-md border border-[color:var(--info)] bg-[color:var(--surface)] px-3 py-2 text-xs text-[color:var(--text-secondary)]">
      <LoadingOutlined />
      <span>{label}：正在写回节点并重启 gateway，实测约 25 秒，请勿重复操作</span>
    </div>
  )
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
      notifyWritebackError(err, message, '部署技能失败')
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
          <span className="text-sm text-[color:var(--text-secondary)]">技能名称</span>
          <Input placeholder="如 my-custom-skill" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">技能包文件（zip / SKILL.md + 脚本）</span>
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
  const { data, isLoading, isError, error, refetch, isFetching } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)
  // 技能部署 = POST /agents/:id/capabilities + 技能上传（agent.manage），无权限隐藏入口。
  const canManageAgent = usePermStore((s) => s.hasPerm(PERM.AGENT_MANAGE))

  if (isLoading && !data) return <Spin />
  // 只有「连旧数据都没有」才整页兜底；有旧数据的降级见下方横幅分支。
  if (!data) return <CapabilityUnavailable reason={error?.message} onRetry={() => refetch()} retrying={isFetching} />

  const skills = data.skills ?? []

  return (
    <div className="flex flex-col gap-4">
      {isError && <CapabilityStaleBanner onRetry={() => refetch()} retrying={isFetching} />}
      <Card bordered={false} className="!bg-[color:var(--surface)]">
        <div className="flex items-center gap-2 border-b border-[color:var(--line)] px-4 py-3 -mx-4 -mt-4 mb-2">
          <BookOutlined className="text-[color:var(--text-tertiary)]" />
          <span className="text-sm font-semibold text-[color:var(--text-primary)]">技能</span>
          <Tag className="!text-xs" color="default">{skills.length}</Tag>
          {canManageAgent && (
            <Button size="small" className="ml-auto" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
              部署技能
            </Button>
          )}
        </div>
        {skills.length === 0 ? (
          <p className="px-1 py-3 text-sm text-[color:var(--text-quaternary)]">该节点暂无可展示的技能，点击右上角部署新技能</p>
        ) : (
          <div className="divide-y divide-[color:var(--line)]">
            {skills.map((s) => (
              <div key={s.name} className="flex items-start gap-3 px-1 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-[color:var(--text-primary)] truncate">{s.name}</span>
                    {s.category && <Tag className="!text-[10px]" color="default">{s.category}</Tag>}
                  </div>
                  {s.description && <p className="mt-0.5 text-xs text-[color:var(--text-tertiary)] line-clamp-2">{s.description}</p>}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
      <SkillAddModal agentId={agentId} open={addOpen} onClose={() => setAddOpen(false)} />
      <p className="flex items-center gap-1.5 text-xs text-[color:var(--text-quaternary)]">
        <WarningOutlined /> 技能写回会重启节点 gateway，可能有数秒不可用窗口。
      </p>
    </div>
  )
}

/* ---------------- 模型 Tab ---------------- */

function guessProviderName(url: string): string {
  try {
    const host = new URL(url).hostname.replace(/^api\./, '')
    const base = host.split('.')[0]
    return base || ''
  } catch {
    return ''
  }
}

function ModelAddModal({ agentId, open, onClose }: { agentId: string; open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const [provider, setProvider] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [model, setModel] = useState('')
  const [modelOptions, setModelOptions] = useState<{ value: string }[]>([])
  const [fetchingModels, setFetchingModels] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ ok: boolean; text: string } | null>(null)
  const [providerTouched, setProviderTouched] = useState(false)
  const setCapabilities = useSetAgentCapabilities(agentId)
  const busy = setCapabilities.isPending

  const handleBaseUrlChange = (v: string) => {
    setBaseUrl(v)
    setTestResult(null)
    if (!providerTouched) {
      const guessed = guessProviderName(v.trim())
      if (guessed) setProvider(guessed)
    }
  }

  const fetchModels = async () => {
    setFetchingModels(true)
    try {
      const res = await fetchLLMModels({
        scope: 'personal',
        api_url: baseUrl.trim() || undefined,
        api_key: apiKey.trim() || undefined,
      })
      const r = res.data?.models
      if (r?.ok && r.items?.length) {
        setModelOptions(r.items.map((m) => ({ value: m })))
        message.success(`拉取到 ${r.items.length} 个模型`)
      } else {
        message.error(r?.error || '未获取到模型列表，请检查 API 地址是否含 /v1')
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : '拉取模型列表失败')
    } finally {
      setFetchingModels(false)
    }
  }

  const runTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const res = await testLLMConfig({
        scope: 'personal',
        api_url: baseUrl.trim() || undefined,
        api_key: apiKey.trim() || undefined,
        model: model.trim() || undefined,
      })
      const t = res.data?.test
      if (t?.ok) {
        setTestResult({ ok: true, text: `连接成功（${t.model ?? model}，${t.latency_ms ?? '?'}ms）` })
      } else {
        setTestResult({ ok: false, text: t?.error || '连接失败' })
      }
    } catch (err) {
      setTestResult({ ok: false, text: err instanceof Error ? err.message : '测试请求失败' })
    } finally {
      setTesting(false)
    }
  }

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
          base_url: baseUrl.trim() || undefined,
          model: model.trim(),
          default_model: model.trim(),
          api_key: apiKey.trim() || undefined,
        },
      })
      const r = applyWritebackResult(res.data, '模型 provider 已添加')
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
      setModel('')
      setApiKey('')
      setTestResult(null)
      onClose()
    } catch (err) {
      notifyWritebackError(err, message, '添加模型失败')
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
      width={520}
    >
      <div className="flex flex-col gap-3 mt-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">API 地址（OpenAI 兼容，含 /v1）</span>
          <Input
            placeholder="https://api.deepseek.com/v1"
            value={baseUrl}
            onChange={(e) => handleBaseUrlChange(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">API Key</span>
          <Input.Password
            placeholder="••••••••（留空则用你在个人/平台 LLM 配置里的 key）"
            value={apiKey}
            autoComplete="new-password"
            onChange={(e) => {
              setApiKey(e.target.value)
              setTestResult(null)
            }}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">Provider 名称（按 API 地址自动推断，可改）</span>
          <Input
            placeholder="如 deepseek / openai"
            value={provider}
            onChange={(e) => {
              setProvider(e.target.value)
              setProviderTouched(true)
            }}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">模型名</span>
          <Space.Compact style={{ width: '100%' }}>
            <AutoComplete
              options={modelOptions}
              value={model}
              onChange={(v) => {
                setModel(v)
                setTestResult(null)
              }}
              placeholder="手输或点右侧拉取列表选择"
              filterOption={(input, option) =>
                (option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
              style={{ width: '100%' }}
            />
            <Button icon={<UnorderedListOutlined />} loading={fetchingModels} onClick={() => void fetchModels()}>
              获取模型列表
            </Button>
          </Space.Compact>
        </div>
        <div>
          <Button size="small" icon={<ExperimentOutlined />} loading={testing} onClick={() => void runTest()}>
            测试连接
          </Button>
          {testResult && (
            <span
              className="ml-2 text-xs"
              style={{ color: testResult.ok ? 'var(--success)' : 'var(--error)' }}
            >
              {testResult.ok ? '✓ ' : '✗ '}
              {testResult.text}
            </span>
          )}
        </div>
        <p className="text-xs text-[color:var(--text-quaternary)]">
          提示：API 地址/Key 留空时，「拉取列表/测试」会用你在 个人空间/平台 LLM 配置 里保存的生效配置。
        </p>
      </div>
    </Modal>
  )
}

function HermesModelsTab({ agentId }: Props) {
  const { data, isLoading, isError, error, refetch, isFetching } = useAgentCapabilities(agentId)
  const { message } = App.useApp()
  const [addOpen, setAddOpen] = useState(false)
  /**
   * 用户点过的目标模型。注意它只是「意图」，**不是**「是否还在同步中」——
   * 后者由服务端返回的数据**推导**出来（见下方 pendingModelId）。
   *
   * 🔴 为什么乐观标记不能做成一份 state 再「事后清理」：
   * 生产实证（同一天 9 次切换）里，写回 25.5s 返回后 0.07s 触发的那次重读
   * **必被后端 3s 客户端超时打掉**（239B 失败体），第二读要 +4.1s 才发出、
   * +4.4~6.5s 才拿到数据（6 次是 0.31~0.61s 的节点缓存命中，3 次是 2.1~2.5s 的真重算）。
   * 若在 POST 返回那一刻清标记，界面会先把「默认」弹回旧模型、几秒后才跳到新模型；
   * 一旦那几次重试全部失败，它会**永久**停在旧值 —— 用户看到的正是
   * 「切换完成之后页面上默认模型显示的还是老的，只有刷新页面才更新为新的」。
   */
  const [requestedModelId, setRequestedModelId] = useState<string | null>(null)
  const setCapabilities = useSetAgentCapabilities(agentId)
  // 模型新增/设为默认/删除 = POST /agents/:id/capabilities（agent.manage），无权限隐藏入口。
  const canManageAgent = usePermStore((s) => s.hasPerm(PERM.AGENT_MANAGE))

  const models = data?.models ?? []
  /**
   * 「还在同步中」= 用户点过某个模型，但服务端还没把它回报成默认。
   * 纯推导 ⇒ 数据一确认就自动变 null，不需要任何 effect 去清理；反过来，读失败时
   * 它会如实停在「同步中」+ 错误横幅，而不是谎报旧模型是默认。
   */
  const pendingModelId =
    requestedModelId && !models.some((m) => (m.id ?? m.model) === requestedModelId && m.isDefault)
      ? requestedModelId
      : null

  // 兜底：既读不到确认、也不报错（理论上不该发生）⇒ 60s 后放弃标记并再拉一次，
  // 避免界面永久卡在「同步中」（那比误报旧值更难排查）。
  useEffect(() => {
    if (!pendingModelId) return
    const timer = window.setTimeout(() => {
      setRequestedModelId(null)
      void refetch()
    }, 60_000)
    return () => window.clearTimeout(timer)
  }, [pendingModelId, refetch])

  if (isLoading && !data) return <Spin />
  // 只有「连旧数据都没有」才整页兜底；有旧数据的降级见下方横幅分支。
  if (!data) return <CapabilityUnavailable reason={error?.message} onRetry={() => refetch()} retrying={isFetching} />

  const switchDefault = async (modelId: string) => {
    // 🔴 乐观反馈：写回要 25 秒、期间界面本无任何变化，用户会以为没点上而反复点击
    //（生产日志里同一操作 3 分钟内出现 3 次，就是这么来的）。
    // 点完立刻标记为「默认（同步中）」，撤销条件见上方 pendingModelId 的推导。
    setRequestedModelId(modelId)
    try {
      const res = await setCapabilities.mutateAsync({ target: 'model', action: 'switch', model: modelId })
      const r = applyWritebackResult(res.data, '默认模型已切换')
      if (r.type === 'success') message.success(r.text)
      else if (r.type === 'warning') message.warning(r.text)
      else message.error(r.text)
    } catch (err) {
      // 请求本身失败（网络 / 超时）：立刻回滚，别让界面停在假的「同步中」。
      setRequestedModelId(null)
      notifyWritebackError(err, message, '切换默认模型失败')
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
          notifyWritebackError(err, message, '删除模型失败')
        }
      },
    })
  }

  return (
    <div className="flex flex-col gap-4">
      {/* 写回进行中优先显示进度横幅；否则在「读失败」或「乐观标记还没被服务端确认」
          时挂同步中提示。两者互斥：写回期间用户最需要知道的是「正在跑、别重复点」。 */}
      {setCapabilities.isPending ? (
        <WritebackProgressBanner label={pendingModelId ? '切换默认模型' : '写回模型配置'} />
      ) : (
        (isError || !!pendingModelId) && <CapabilityStaleBanner onRetry={() => refetch()} retrying={isFetching} />
      )}
      <Card bordered={false} className="!bg-[color:var(--surface)]">
        <div className="flex items-center gap-2 border-b border-[color:var(--line)] px-4 py-3 -mx-4 -mt-4 mb-2">
          <ExperimentOutlined className="text-[color:var(--text-tertiary)]" />
          <span className="text-sm font-semibold text-[color:var(--text-primary)]">推理模型</span>
          <Tag className="!text-xs" color="default">{models.length}</Tag>
          {canManageAgent && (
            <Button size="small" className="ml-auto" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
              添加 provider
            </Button>
          )}
        </div>
        {models.length === 0 ? (
          <p className="px-1 py-3 text-sm text-[color:var(--text-quaternary)]">该节点暂无可展示的模型，点击右上角添加 provider</p>
        ) : (
          <div className="divide-y divide-[color:var(--line)]">
            {models.map((m) => (
              <div key={m.id ?? m.model} className="flex items-center gap-3 px-1 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-[color:var(--text-primary)] truncate">{m.model}</span>
                    {/* 写回期间把「默认」从旧模型上摘下来、只在目标项上显示「同步中」，
                        避免同时出现两个「默认」标签让用户更困惑。 */}
                    {m.isDefault && !pendingModelId && <Tag className="!text-[10px]" color="green">默认</Tag>}
                    {(m.id ?? m.model) === pendingModelId && (
                      <Tag className="!text-[10px]" color="processing">默认（同步中）</Tag>
                    )}
                  </div>
                  <p className="mt-0.5 text-xs text-[color:var(--text-tertiary)] truncate font-mono">{m.provider}</p>
                </div>
                {canManageAgent && !m.isDefault && (
                  <div className="flex items-center gap-1 shrink-0">
                    <Button size="small" type="link" onClick={() => switchDefault(m.id ?? m.model)} disabled={setCapabilities.isPending || !!pendingModelId}>
                      设为默认
                    </Button>
                    <Button size="small" type="link" danger onClick={() => removeModel(m.id ?? m.model)} disabled={setCapabilities.isPending || !!pendingModelId}>
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
      <p className="flex items-center gap-1.5 text-xs text-[color:var(--text-quaternary)]">
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
        <div className="flex items-center gap-2 text-xs text-[color:var(--text-tertiary)] flex-wrap">
          <Tag style={{ color: status.color, borderColor: status.color }}>{status.label}</Tag>
          {formatDuration(execution.durationMs) && <span>耗时 {formatDuration(execution.durationMs)}</span>}
          {execution.error && <span className="text-[color:var(--error)]">错误: {execution.error}</span>}
        </div>
        {isSilent && (
          <div className="rounded-md bg-[color:var(--surface)] px-3 py-2 text-xs text-[color:var(--text-tertiary)]">本次执行无新内容（[SILENT]）</div>
        )}
        {renderBody ? (
          <pre className="max-h-[50vh] overflow-auto whitespace-pre-wrap break-words rounded-lg border border-[color:var(--line)] bg-[color:var(--surface-inset)] p-3 text-xs text-[color:var(--text-primary)]">{renderBody}</pre>
        ) : (
          <p className="text-sm text-[color:var(--text-quaternary)]">该执行无结果输出</p>
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
          <p className="text-xs text-[color:var(--text-tertiary)]">cron: {job.schedule} {job.enabled ? '' : '（已暂停）'}</p>
          <Button size="small" icon={<RedoOutlined />} onClick={() => refetch()} loading={isLoading}>刷新</Button>
        </div>
        {isLoading ? (
          <div className="py-8 text-center text-sm text-[color:var(--text-quaternary)]">加载中...</div>
        ) : executions.length === 0 ? (
          <div className="py-8 text-center text-sm text-[color:var(--text-quaternary)]">{error ? `暂无执行记录（${error}）` : '暂无执行记录'}</div>
        ) : (
          <div className="flex flex-col divide-y divide-[color:var(--line)] rounded-lg border border-[color:var(--line)]">
            {executions.map((ex) => {
              const st = executionStatusStyle[ex.status]
              return (
                <button
                  key={ex.executionId}
                  type="button"
                  className="flex items-center gap-3 px-3 py-2.5 text-left hover:bg-[color:var(--surface-sunken)] cursor-pointer"
                  onClick={() => setViewing(ex)}
                >
                  <Tag style={{ color: st.color, borderColor: st.color }} className="shrink-0">{st.label}</Tag>
                  <div className="min-w-0 flex-1">
                    <div className="text-xs text-[color:var(--text-tertiary)]">
                      {formatMsTime(ex.startedAtMs)} · 耗时 {formatDuration(ex.durationMs)}
                    </div>
                    {ex.error && <div className="text-xs text-[color:var(--error)] truncate">{ex.error}</div>}
                  </div>
                  <span className="text-xs text-[color:var(--text-quaternary)] shrink-0">查看结果 ›</span>
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
  // 立即运行/暂停/恢复/删除定时任务 = POST /agents/:id/capabilities（agent.manage），无权限隐藏按钮组。
  const canManageAgent = usePermStore((s) => s.hasPerm(PERM.AGENT_MANAGE))
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
      // 与另外三个写回入口统一：定时任务同样会经 daemon 穿透到节点，
      // 超时不代表失败（节点侧可能已完成），不能直接报错。
      notifyWritebackError(err, message, '定时任务操作失败')
    }
  }

  return (
    <div className="flex items-start gap-3 px-1 py-3 first:pt-0 last:pb-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-[color:var(--text-primary)] truncate">{job.name}</span>
          {!job.enabled && <Tag className="!text-[10px]" color="default">已暂停</Tag>}
          {latestStatus && (
            <Tag className="!text-[10px]" style={{ color: latestStatus.color, borderColor: latestStatus.color }}>
              {latestStatus.label}
            </Tag>
          )}
        </div>
        <p className="mt-0.5 text-xs text-[color:var(--text-tertiary)] truncate font-mono">{job.schedule}</p>
        {latest && (
          <p className="mt-0.5 text-xs text-[color:var(--text-tertiary)] truncate">
            最近执行: {dayjs(latest.startedAtMs).fromNow()}
            {formatDuration(latest.durationMs) ? ` · 耗时 ${formatDuration(latest.durationMs)}` : ''}
          </p>
        )}
        {job.nextRun && <p className="mt-0.5 text-xs text-[color:var(--text-tertiary)]">下次运行: {job.nextRun}</p>}
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <Button size="small" type="text" icon={<HistoryOutlined />} onClick={() => setHistoryOpen(true)} title="执行历史" />
        {canManageAgent && (
          <>
            <Button size="small" type="text" icon={<PlayCircleOutlined />} onClick={() => runAction('run')} loading={setCapabilities.isPending} title="立即运行" />
            {job.enabled ? (
              <Button size="small" type="text" icon={<PauseCircleOutlined />} onClick={() => runAction('pause')} loading={setCapabilities.isPending} title="暂停" />
            ) : (
              <Button size="small" type="text" icon={<RedoOutlined />} onClick={() => runAction('resume')} loading={setCapabilities.isPending} title="恢复" />
            )}
            <Button size="small" type="text" danger icon={<DeleteOutlined />} onClick={() => runAction('delete')} loading={setCapabilities.isPending} title="删除" />
          </>
        )}
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
      notifyWritebackError(err, message, '创建定时任务失败')
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
          <span className="text-sm text-[color:var(--text-secondary)]">任务名称</span>
          <Input placeholder="如 daily-report" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">Cron 表达式</span>
          <Input placeholder="0 9 * * *" value={schedule} onChange={(e) => setSchedule(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-[color:var(--text-secondary)]">任务提示词</span>
          <Input.TextArea rows={3} placeholder="每天生成日报并发送" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
        </div>
      </div>
    </Modal>
  )
}

function HermesJobsTab({ agentId }: Props) {
  const { data, isLoading, isError, error, refetch, isFetching } = useAgentCapabilities(agentId)
  const [addOpen, setAddOpen] = useState(false)
  // 新建定时任务 = POST /agents/:id/capabilities（agent.manage），无权限隐藏入口。
  const canManageAgent = usePermStore((s) => s.hasPerm(PERM.AGENT_MANAGE))

  if (isLoading && !data) return <Spin />
  // 只有「连旧数据都没有」才整页兜底；有旧数据的降级见下方横幅分支。
  if (!data) return <CapabilityUnavailable reason={error?.message} onRetry={() => refetch()} retrying={isFetching} />

  const jobs = data.jobs ?? []

  return (
    <div className="flex flex-col gap-4">
      {isError && <CapabilityStaleBanner onRetry={() => refetch()} retrying={isFetching} />}
      <Card bordered={false} className="!bg-[color:var(--surface)]">
        <div className="flex items-center gap-2 border-b border-[color:var(--line)] px-4 py-3 -mx-4 -mt-4 mb-2">
          <ScheduleOutlined className="text-[color:var(--text-tertiary)]" />
          <span className="text-sm font-semibold text-[color:var(--text-primary)]">定时任务</span>
          <Tag className="!text-xs" color="default">{jobs.length}</Tag>
          {canManageAgent && (
            <Button size="small" className="ml-auto" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
              新建任务
            </Button>
          )}
        </div>
        {jobs.length === 0 ? (
          <p className="px-1 py-3 text-sm text-[color:var(--text-quaternary)]">该节点暂无定时任务，点击右上角新建</p>
        ) : (
          <div className="divide-y divide-[color:var(--line)]">
            {jobs.map((j) => <JobRow key={j.id} job={j} agentId={agentId} />)}
          </div>
        )}
      </Card>
      <JobAddModal agentId={agentId} open={addOpen} onClose={() => setAddOpen(false)} />
      <p className="flex items-center gap-1.5 text-xs text-[color:var(--text-quaternary)]">
        <WarningOutlined /> 定时任务由节点 gateway 原生调度，写回无需重启。
      </p>
    </div>
  )
}

export { HermesSkillsTab, HermesModelsTab, HermesJobsTab }
