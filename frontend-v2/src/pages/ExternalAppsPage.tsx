import { useState } from 'react'
import {
  Button,
  Card,
  Checkbox,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Skeleton,
  Table,
  Tag,
  App,
  Alert,
  Typography,
} from 'antd'
import {
  AppstoreOutlined,
  PlusOutlined,
  ExportOutlined,
  EditOutlined,
  DeleteOutlined,
  ApiOutlined,
} from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { useCreateExternalApp, useDeleteExternalApp, useExternalApps, useLaunchExternalApp, useUpdateExternalApp } from '@/hooks/useExternalApps'
import { ApiRequestError } from '@/types'
import type { CreateExternalAppRequest, ExternalAppStatus, ExternalAppView } from '@/types'

const { Text, Paragraph } = Typography

const SSO_TYPE_OPTIONS = [{ value: 'trustmesh_jwt', label: 'TrustMesh 签发 JWT（推荐）' }]
const FRAME_MODE_OPTIONS = [
  { value: 'newtab', label: '新标签页打开' },
  { value: 'iframe', label: '内嵌 iframe（需外部平台支持）' },
]
/** 挂载点：决定这个平台出现在侧边栏还是项目详情页的 tab */
const PLACEMENT_OPTIONS = [
  { value: 'sidebar', label: '侧边栏菜单' },
  { value: 'project_tab', label: '项目详情 tab' },
]
const VISIBILITY_OPTIONS = [
  { value: 'private', label: '私有（仅我可见可用）' },
  { value: 'public', label: '公共（全员可见可用）' },
]

/** 表单里的 placement 是多选数组，提交前压成后端要求的逗号分隔串 */
function normalizeFormValues(values: Record<string, unknown>): CreateExternalAppRequest {
  const { placement, ...rest } = values as Record<string, unknown> & { placement?: unknown }
  return {
    ...(rest as unknown as CreateExternalAppRequest),
    placement: Array.isArray(placement) ? (placement as string[]).join(',') : ((placement as string) ?? ''),
  }
}

export function ExternalAppsPage() {
  const { message } = App.useApp()
  const { data: apps, isLoading } = useExternalApps()
  const createApp = useCreateExternalApp()
  const updateApp = useUpdateExternalApp()
  const deleteApp = useDeleteExternalApp()
  const launchApp = useLaunchExternalApp()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ExternalAppView | null>(null)
  const [secretOpen, setSecretOpen] = useState(false)
  const [secretValue, setSecretValue] = useState('')
  const [secretAppName, setSecretAppName] = useState('')
  const [form] = Form.useForm()

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({
      sso_type: 'trustmesh_jwt',
      frame_mode: 'newtab',
      placement: [],
      visibility: 'private',
      sort_order: 0,
    })
    setFormOpen(true)
  }

  const openEdit = (app: ExternalAppView) => {
    setEditing(app)
    form.setFieldsValue({
      name: app.name,
      base_url: app.base_url,
      client_id: app.client_id,
      sso_type: app.sso_type,
      frame_mode: app.frame_mode,
      scopes: app.scopes,
      placement: app.placement ? app.placement.split(',').map((p) => p.trim()).filter(Boolean) : [],
      icon_url: app.icon_url,
      sort_order: app.sort_order,
      visibility: app.visibility || 'private',
    })
    setFormOpen(true)
  }

  const handleLaunch = async (app: ExternalAppView) => {
    try {
      const res = await launchApp.mutateAsync({ id: app.id })
      window.open(res.launch_url, '_blank', 'noopener,noreferrer')
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '启动失败')
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteApp.mutateAsync(id)
      message.success('已断开外部平台')
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '删除失败')
    }
  }

  const handleSubmit = async () => {
    const raw = await form.validateFields()
    const values = normalizeFormValues(raw)
    try {
      if (editing) {
        await updateApp.mutateAsync({ id: editing.id, input: values })
        message.success('外部平台已更新')
        setFormOpen(false)
      } else {
        const res = await createApp.mutateAsync(values)
        // client_secret 仅返回一次，立即展示并要求用户保存
        setSecretValue(res.data.client_secret)
        setSecretAppName(values.name as string)
        setSecretOpen(true)
        setFormOpen(false)
        message.success('外部平台已创建')
      }
    } catch (err) {
      if (err instanceof ApiRequestError) {
        message.error(err.message)
      }
      // 表单校验错误由 antd 自行展示，不重复提示
    }
  }

  const copySecret = async () => {
    try {
      await navigator.clipboard.writeText(secretValue)
      message.success('client_secret 已复制')
    } catch {
      message.error('复制失败，请手动选择复制')
    }
  }

  const columns = [
    {
      title: '平台名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <AppstoreOutlined style={{ color: '#6d5ff5' }} />
          <b>{name}</b>
        </span>
      ),
    },
    {
      title: '基础 URL',
      dataIndex: 'base_url',
      key: 'base_url',
      render: (url: string) => (
        <Text type="secondary" style={{ fontSize: 12 }} ellipsis>
          {url}
        </Text>
      ),
    },
    {
      title: 'Client ID',
      dataIndex: 'client_id',
      key: 'client_id',
      render: (v: string) => <Tag>{v}</Tag>,
    },
    {
      title: '打开方式',
      dataIndex: 'frame_mode',
      key: 'frame_mode',
      render: (v: string) => <Tag color="default">{v === 'iframe' ? 'iframe' : '新标签页'}</Tag>,
    },
    {
      title: '挂载位置',
      dataIndex: 'placement',
      key: 'placement',
      render: (placement: string, app: ExternalAppView) => {
        if (!placement) return <Text type="secondary" style={{ fontSize: 12 }}>未挂载</Text>
        const labels: Record<string, string> = { sidebar: '侧边栏', project_tab: '项目 tab' }
        return (
          <span style={{ display: 'inline-flex', gap: 4, flexWrap: 'wrap' }}>
            {placement.split(',').map((p) => p.trim()).filter(Boolean).map((p) => (
              <Tag key={p} color="purple">{labels[p] ?? p}</Tag>
            ))}
            {app.visibility === 'public' && <Tag color="blue">公共</Tag>}
          </span>
        )
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (v: ExternalAppStatus) =>
        v === 'enabled' ? (
          <Tag color="success">已启用</Tag>
        ) : (
          <Tag color="default">已停用</Tag>
        ),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_: unknown, app: ExternalAppView) => (
        <span style={{ display: 'inline-flex', gap: 4 }}>
          <Button
            type="link"
            size="small"
            icon={<ExportOutlined />}
            disabled={app.status !== 'enabled'}
            onClick={() => handleLaunch(app)}
          >
            打开
          </Button>
          <Button type="link" size="small" icon={<EditOutlined />} onClick={() => openEdit(app)}>
            编辑
          </Button>
          <Popconfirm title={`确定断开「${app.name}」？`} description="断开后将无法再免密打开该平台。" onConfirm={() => handleDelete(app.id)} okText="断开" cancelText="取消">
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </span>
      ),
    },
  ]

  return (
    <div>
      <PageHeader
        title="外部应用"
        icon={<ApiOutlined />}
        subtitle="通过 SSO 连接外部平台，在 TrustMesh 内免密打开。统一在此登记与管理。"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新增外部平台
          </Button>
        }
      />

      {isLoading ? (
        <Card bordered={false} style={{ background: 'rgba(255,255,255,0.03)' }}>
          <Skeleton active paragraph={{ rows: 4 }} />
        </Card>
      ) : !apps || apps.length === 0 ? (
        <Empty description="还没有连接外部平台">
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新增外部平台
          </Button>
        </Empty>
      ) : (
        <Card bordered={false} style={{ background: 'rgba(255,255,255,0.03)' }}>
          <Table
            rowKey="id"
            columns={columns}
            dataSource={apps}
            pagination={false}
          />
        </Card>
      )}

      <Modal
        title={editing ? '编辑外部平台' : '新增外部平台'}
        open={formOpen}
        onCancel={() => setFormOpen(false)}
        onOk={handleSubmit}
        confirmLoading={createApp.isPending || updateApp.isPending}
        okText={editing ? '保存' : '创建'}
        cancelText="取消"
        destroyOnClose
      >
        <Paragraph type="secondary" style={{ fontSize: 12 }}>
          {editing ? '修改该平台的连接配置。' : '登记一个可通过 SSO 免密打开的外部平台。创建后会生成 client_secret，请妥善保存。'}
        </Paragraph>
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item name="name" label="平台名称" rules={[{ required: true, message: '请输入平台名称' }]}>
            <Input placeholder="例如：画宗 AIGC 工厂" />
          </Form.Item>
          <Form.Item name="base_url" label="基础 URL" rules={[{ required: true, message: '请输入基础 URL' }]}>
            <Input placeholder="https://external.example.com/sso/entry" />
          </Form.Item>
          <Form.Item name="client_id" label="Client ID" rules={[{ required: true, message: '请输入 Client ID' }]}>
            <Input placeholder="外部平台分配给 TrustMesh 的标识" />
          </Form.Item>
          <Form.Item name="sso_type" label="SSO 类型">
            <Select options={SSO_TYPE_OPTIONS} />
          </Form.Item>
          <Form.Item name="frame_mode" label="打开方式">
            <Select options={FRAME_MODE_OPTIONS} />
          </Form.Item>
          <Form.Item name="scopes" label="Scopes（可选）">
            <Input.TextArea rows={2} placeholder="逗号分隔的权限范围，如：read,write" />
          </Form.Item>
          <Form.Item
            name="placement"
            label="挂载位置"
            extra="勾选后该平台会出现在对应入口；侧边栏不依赖 iframe，项目 tab 建议同时开启内嵌模式"
          >
            <Checkbox.Group options={PLACEMENT_OPTIONS} />
          </Form.Item>
          <Form.Item name="visibility" label="可见性">
            <Select options={VISIBILITY_OPTIONS} />
          </Form.Item>
          <Form.Item name="icon_url" label="图标 URL（可选）">
            <Input placeholder="https://cdn.example.com/icon.png" />
          </Form.Item>
          <Form.Item name="sort_order" label="排序权重（小的在前）">
            <InputNumber style={{ width: 140 }} min={0} max={9999} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="保存 client_secret" open={secretOpen} onCancel={() => setSecretOpen(false)} footer={[
        <Button key="copy" type="primary" onClick={copySecret}>
          复制
        </Button>,
        <Button key="done" onClick={() => setSecretOpen(false)}>
          我已保存，完成
        </Button>,
      ]}>
        <Alert
          type="warning"
          showIcon
          message="请立即复制并妥善保存 client_secret"
          description="该密钥仅显示这一次，关闭后将无法再次查看，需重新创建平台。"
          style={{ marginBottom: 12 }}
        />
        <Input readOnly value={secretValue} className="font-mono" style={{ fontSize: 12 }} />
        <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
          {secretAppName}
        </Text>
      </Modal>
    </div>
  )
}
