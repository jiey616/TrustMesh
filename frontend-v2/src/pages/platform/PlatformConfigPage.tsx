import { useEffect } from 'react'
import {
  App,
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  InputNumber,
  Space,
  Typography,
} from 'antd'
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { LLMConfigCard } from '@/components/settings/LLMConfigCard'
import { HIDEABLE_MENUS } from '@/lib/menuDefs'
import { usePlatformConfig, useUpdatePlatformConfig } from '@/hooks/usePlatformAdmin'
import { ApiRequestError } from '@/types'
import type { PlatformGlobalConfig } from '@/types'

const { Text, Paragraph } = Typography

const UNLIMITED = -1

interface NodeParamRow {
  key?: string
  value?: string
}

/**
 * 平台管理 · 全局配置（设计文档 §6.4）：默认模型 + 新建企业默认配额 + 节点参数。
 *
 * 配额只影响**新建**企业租户（个人空间恒不限），不改动存量企业；
 * 平台默认 LLM 配置（密钥）在下方独立卡片维护。
 */
export function PlatformConfigPage() {
  const { message } = App.useApp()
  const { data: config, isLoading } = usePlatformConfig()
  const save = useUpdatePlatformConfig()
  const [form] = Form.useForm()

  useEffect(() => {
    if (!config) return
    form.setFieldsValue({
      default_model: config.default_model,
      max_members: config.quota?.max_members ?? UNLIMITED,
      max_nodes: config.quota?.max_nodes ?? UNLIMITED,
      max_projects: config.quota?.max_projects ?? UNLIMITED,
      max_storage_bytes: config.quota?.max_storage_bytes ?? UNLIMITED,
      node_parameters: Object.entries(config.node_parameters ?? {}).map(([key, value]) => ({
        key,
        value,
      })),
      hidden_menus: config.hidden_menus ?? [],
    })
  }, [config, form])

  const handleSave = async () => {
    const values = await form.validateFields()
    const rows: NodeParamRow[] = values.node_parameters ?? []
    const nodeParameters: Record<string, string> = {}
    for (const row of rows) {
      const k = (row.key ?? '').trim()
      if (!k) continue
      nodeParameters[k] = (row.value ?? '').trim()
    }
    const payload: PlatformGlobalConfig = {
      default_model: (values.default_model ?? '').trim(),
      quota: {
        max_members: values.max_members ?? UNLIMITED,
        max_nodes: values.max_nodes ?? UNLIMITED,
        max_projects: values.max_projects ?? UNLIMITED,
        max_storage_bytes: values.max_storage_bytes ?? UNLIMITED,
      },
      node_parameters: nodeParameters,
      // 显式带上：payload 是逐字段构造的，漏了它「保存配额」会顺手清空全局隐藏项。
      hidden_menus: values.hidden_menus ?? [],
    }
    try {
      await save.mutateAsync(payload)
      message.success('全局配置已保存（对新建企业生效）')
    } catch (e) {
      message.error(e instanceof ApiRequestError ? e.message : '保存失败')
    }
  }

  return (
    <div style={{ maxWidth: 880, paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 全局配置"
        subtitle="默认模型、新建企业默认配额与节点参数；-1 表示不限"
      />

      <Card
        loading={isLoading}
        title="全局配置"
        style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
        extra={
          <Button
            type="primary"
            size="small"
            icon={<SaveOutlined />}
            loading={save.isPending}
            onClick={() => void handleSave()}
          >
            保存
          </Button>
        }
      >
        <Form form={form} layout="vertical" style={{ maxWidth: 560 }} requiredMark={false}>
          <Form.Item
            name="default_model"
            label="默认模型"
            extra="未覆盖租户的推荐模型；留空 = 跟随平台 LLM 配置"
          >
            <Input placeholder="deepseek-v4-flash / gpt-4o-mini / ..." style={{ fontSize: 13 }} />
          </Form.Item>

          <Space size={12} wrap>
            <Form.Item name="max_members" label="成员上限">
              <InputNumber min={UNLIMITED} style={{ width: 140 }} />
            </Form.Item>
            <Form.Item name="max_nodes" label="节点上限">
              <InputNumber min={UNLIMITED} style={{ width: 140 }} />
            </Form.Item>
            <Form.Item name="max_projects" label="项目上限">
              <InputNumber min={UNLIMITED} style={{ width: 140 }} />
            </Form.Item>
            <Form.Item name="max_storage_bytes" label="存储上限（字节）">
              <InputNumber min={UNLIMITED} style={{ width: 180 }} />
            </Form.Item>
          </Space>

          <Form.List name="node_parameters">
            {(fields, { add, remove }) => (
              <div>
                <Text type="secondary" style={{ fontSize: 12 }}>节点参数（预留，最多 20 项）</Text>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 8 }}>
                  {fields.map((field) => (
                    <Space key={field.key} align="baseline">
                      <Form.Item name={[field.name, 'key']} noStyle>
                        <Input placeholder="键" style={{ width: 200, fontSize: 13 }} maxLength={64} />
                      </Form.Item>
                      <Form.Item name={[field.name, 'value']} noStyle>
                        <Input placeholder="值" style={{ width: 240, fontSize: 13 }} maxLength={64} />
                      </Form.Item>
                      <Button
                        type="text"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() => remove(field.name)}
                      />
                    </Space>
                  ))}
                </div>
                {fields.length < 20 && (
                  <Button
                    type="dashed"
                    icon={<PlusOutlined />}
                    style={{ marginTop: 8 }}
                    onClick={() => add({ key: '', value: '' })}
                  >
                    添加参数
                  </Button>
                )}
              </div>
            )}
          </Form.List>

          <Card
            size="small"
            title="菜单可见性（全局基线）"
            style={{ marginTop: 16, background: 'var(--surface)', border: '1px solid var(--line)' }}
          >
            <Paragraph type="secondary" style={{ fontSize: 12 }}>
              勾选的菜单对全平台所有租户（企业空间与个人空间）全员隐藏，平台管理菜单不受影响。
              这里只能缩小可见范围，不会放大任何角色的权限；企业 owner 只能在此基础上继续缩小，
              不能抵消；且只影响菜单显示，不影响接口鉴权。
            </Paragraph>
            <Form.Item name="hidden_menus" noStyle>
              <Checkbox.Group>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  {HIDEABLE_MENUS.map((m) => (
                    <Checkbox key={m.key} value={m.key}>
                      {m.label}
                    </Checkbox>
                  ))}
                </div>
              </Checkbox.Group>
            </Form.Item>
          </Card>

          <Paragraph type="secondary" style={{ fontSize: 12, marginTop: 16, marginBottom: 0 }}>
            配额变更只影响此后新建的企业租户，不会改动存量企业已有配额；菜单可见性变更即时生效。
          </Paragraph>
        </Form>
      </Card>

      <div style={{ marginTop: 16 }}>
        <LLMConfigCard scope="platform" />
      </div>
    </div>
  )
}