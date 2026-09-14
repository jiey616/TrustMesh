import { useEffect, useState } from 'react'
import {
  Alert,
  App,
  AutoComplete,
  Button,
  Card,
  Form,
  Input,
  Popconfirm,
  Space,
  Tag,
  Typography,
} from 'antd'
import {
  ApiOutlined,
  CheckCircleOutlined,
  ExperimentOutlined,
  SaveOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons'
import {
  deleteOrgLLMConfig,
  deletePersonalLLMConfig,
  deletePlatformLLMConfig,
  fetchLLMModels,
  getOrgLLMConfig,
  getPersonalLLMConfig,
  getPlatformLLMConfig,
  testLLMConfig,
  updateOrgLLMConfig,
  updatePersonalLLMConfig,
  updatePlatformLLMConfig,
  type LLMConfigTestResult,
  type LLMConfigView,
} from '@/api/llmConfig'
import { ApiRequestError } from '@/types'

const { Text } = Typography

const SOURCE_LABEL: Record<string, { text: string; color: string }> = {
  org: { text: '生效来源：本租户配置', color: 'var(--signal)' },
  platform: { text: '生效来源：平台默认', color: 'var(--info)' },
  personal: { text: '生效来源：个人空间配置', color: 'var(--signal)' },
  env: { text: '生效来源：环境变量兜底', color: 'var(--text-tertiary)' },
}

type LLMScope = 'platform' | 'org' | 'personal'

function errText(err: unknown): string {
  return err instanceof ApiRequestError ? err.message : '请求失败'
}

/**
 * LLM 配置卡片（scope=platform 时仅平台管理员可见可改；personal 为个人空间层）。
 * D1：key write-only，只回掩码；留空 = 保持不变。C1：保存即生效。
 * 模型字段支持从 provider 拉取可选列表（OpenAI 兼容 /models）。
 */
export function LLMConfigCard({ scope = 'platform', orgId }: { scope?: LLMScope; orgId?: string }) {
  const { message } = App.useApp()
  const isPlatform = scope === 'platform'
  const isPersonal = scope === 'personal'
  const [form] = Form.useForm()
  const [config, setConfig] = useState<LLMConfigView | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<LLMConfigTestResult | null>(null)
  const [modelOptions, setModelOptions] = useState<{ value: string }[]>([])
  const [fetchingModels, setFetchingModels] = useState(false)

  const title =
    isPlatform ? '平台默认 LLM 配置' : isPersonal ? '个人空间 LLM 配置' : '本租户 LLM 配置（覆盖平台默认）'

  const load = async () => {
    setLoading(true)
    try {
      const res =
        isPlatform
          ? await getPlatformLLMConfig()
          : isPersonal
            ? await getPersonalLLMConfig()
            : await getOrgLLMConfig(orgId!)
      setConfig(res.data.config)
      form.setFieldsValue({
        api_url: res.data.config.api_url,
        model: res.data.config.model,
        ops_model: res.data.config.ops_model,
        api_key: '',
      })
    } catch (err) {
      message.error(errText(err))
    } finally {
      setLoading(false)
    }
  }

  // 配置随 scope / orgId 变化重新加载：用 setTimeout 把 setState 移出 effect 的
  // 同步执行体，以满足 react-hooks/set-state-in-effect（加载延后一个 tick，行为不变）。
  useEffect(() => {
    const t = setTimeout(() => {
      void load()
    }, 0)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scope, orgId])

  const scopeInput = () => {
    if (isPlatform) return {}
    if (isPersonal) return { scope: 'personal' }
    return { scope: 'org', org_id: orgId }
  }

  const handleSave = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      const input = {
        api_url: values.api_url,
        model: values.model,
        ops_model: values.ops_model || '',
        api_key: values.api_key || '',
      }
      const res =
        isPlatform
          ? await updatePlatformLLMConfig(input)
          : isPersonal
            ? await updatePersonalLLMConfig(input)
            : await updateOrgLLMConfig(orgId!, input)
      setConfig(res.data.config)
      form.setFieldValue('api_key', '')
      message.success(isPersonal ? '个人空间配置已保存并生效' : '配置已保存并生效')
    } catch (err) {
      message.error(errText(err))
    } finally {
      setSaving(false)
    }
  }

  const handleReset = async () => {
    try {
      if (isPlatform) {
        await deletePlatformLLMConfig()
      } else if (isPersonal) {
        await deletePersonalLLMConfig()
      } else {
        await deleteOrgLLMConfig(orgId!)
      }
      message.success('已清除该层配置，回退到下一级')
      await load()
    } catch (err) {
      message.error(errText(err))
    }
  }

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const res = await testLLMConfig(scopeInput())
      setTestResult(res.data.test)
    } catch (err) {
      setTestResult({ ok: false, error: errText(err) })
    } finally {
      setTesting(false)
    }
  }

  const handleFetchModels = async () => {
    setFetchingModels(true)
    try {
      // 提交表单当前 url/key（未保存也能拉），缺省字段由后端回落生效配置。
      const values = form.getFieldsValue()
      const res = await fetchLLMModels({
        ...scopeInput(),
        api_url: values.api_url || undefined,
        api_key: values.api_key || undefined,
      })
      const payload = res.data.models
      if (!payload.ok) {
        message.error(payload.error || '获取模型列表失败')
        return
      }
      setModelOptions((payload.items ?? []).map((m) => ({ value: m })))
      message.success(`获取到 ${payload.items?.length ?? 0} 个模型`)
    } catch (err) {
      message.error(errText(err))
    } finally {
      setFetchingModels(false)
    }
  }

  const sourceMeta = config ? SOURCE_LABEL[config.source] : null

  return (
    <Card
      loading={loading}
      title={
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontSize: 14 }}>
          <ApiOutlined style={{ color: 'var(--signal)' }} />
          {title}
          {config && (
            <Tag
              style={{
                marginInlineEnd: 0,
                fontSize: 10,
                background: 'transparent',
                borderColor: sourceMeta?.color,
                color: sourceMeta?.color,
              }}
            >
              {sourceMeta?.text}
            </Tag>
          )}
        </span>
      }
      style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
    >
      {config && !config.has_override && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          message={
            isPlatform
              ? '尚未保存平台配置，当前使用 .env 兜底值（下方表单已带出）。保存后即热生效，无需重启。'
              : isPersonal
                ? '个人空间尚未配置，当前回落到平台默认 / env。填写并保存后个人空间独立生效。'
                : '本租户尚未覆盖，当前回落到上一级配置。填写并保存后本租户独立生效。'
          }
        />
      )}

      <Form form={form} layout="vertical" style={{ maxWidth: 560 }} requiredMark={false}>
        <Form.Item
          label="API 地址（OpenAI 兼容，含 /v1）"
          name="api_url"
          rules={[{ required: true, message: '必填' }]}
          style={{ marginBottom: 12 }}
        >
          <Input placeholder="https://api.deepseek.com/v1" style={{ fontSize: 13 }} />
        </Form.Item>
        <Form.Item
          label={config?.api_key_masked ? `API Key（当前 ${config.api_key_masked}，留空保持不变）` : 'API Key'}
          name="api_key"
          rules={
            config?.api_key_masked
              ? []
              : [{ required: true, message: '该层尚无 key，必填' }]
          }
          style={{ marginBottom: 12 }}
        >
          <Input.Password
            placeholder={config?.api_key_masked ? '留空 = 保持已有 key' : 'sk-...'}
            autoComplete="new-password"
            style={{ fontSize: 13 }}
          />
        </Form.Item>
        <Form.Item
          label="模型（助手对话 + 工单归因默认）"
          required
          style={{ marginBottom: 12 }}
        >
          <Space.Compact style={{ width: '100%' }}>
            <Form.Item
              name="model"
              noStyle
              rules={[{ required: true, message: '必填' }]}
            >
              <AutoComplete
                options={modelOptions}
                placeholder="deepseek-v4-flash / gpt-4o-mini / ..."
                filterOption={(input, option) =>
                  (option?.value ?? '').toLowerCase().includes(input.toLowerCase())
                }
                style={{ fontSize: 13 }}
              />
            </Form.Item>
            <Button
              icon={<UnorderedListOutlined />}
              loading={fetchingModels}
              onClick={() => void handleFetchModels()}
              style={{ fontSize: 12 }}
            >
              获取模型列表
            </Button>
          </Space.Compact>
        </Form.Item>
        <Form.Item
          label="运维归因模型（可选，留空用上面的模型）"
          name="ops_model"
          style={{ marginBottom: 12 }}
        >
          <AutoComplete
            options={modelOptions}
            placeholder="留空 = 与对话模型一致"
            filterOption={(input, option) =>
              (option?.value ?? '').toLowerCase().includes(input.toLowerCase())
            }
            style={{ fontSize: 13 }}
          />
        </Form.Item>
        <Space wrap>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={saving}
            onClick={() => void handleSave()}
            style={{ fontSize: 12 }}
          >
            保存并生效
          </Button>
          <Button
            icon={<ExperimentOutlined />}
            loading={testing}
            onClick={() => void handleTest()}
            style={{ fontSize: 12 }}
          >
            测试连接
          </Button>
          {config?.has_override && (
            <Popconfirm
              title={isPlatform ? '删除平台配置？' : isPersonal ? '清除个人空间配置？' : '清除本租户覆盖？'}
              description={
                isPlatform
                  ? '将回退到 .env 兜底配置。'
                  : isPersonal
                    ? '将回退到平台默认 / env 配置。'
                    : '将回退到平台默认 / env 配置。'
              }
              okText="确认"
              cancelText="取消"
              onConfirm={() => void handleReset()}
            >
              <Button danger style={{ fontSize: 12 }}>
                清除该层配置
              </Button>
            </Popconfirm>
          )}
        </Space>
      </Form>

      {testResult && (
        <Alert
          style={{ marginTop: 12, maxWidth: 560 }}
          type={testResult.ok ? 'success' : 'error'}
          showIcon
          icon={testResult.ok ? <CheckCircleOutlined /> : undefined}
          message={testResult.ok ? `连接成功（${testResult.model}，${testResult.latency_ms}ms）` : '连接失败'}
          description={testResult.ok ? undefined : testResult.error}
        />
      )}

      <div style={{ marginTop: 12 }}>
        <Text style={{ fontSize: 11, color: 'var(--text-quaternary)' }}>
          {isPersonal
            ? '解析优先级：个人空间配置 → 平台默认 → .env 兜底；保存后立即生效，无需重启服务。'
            : '解析优先级：本租户配置 → 平台默认 → .env 兜底；保存后立即生效，无需重启服务。'}
        </Text>
      </div>
    </Card>
  )
}
