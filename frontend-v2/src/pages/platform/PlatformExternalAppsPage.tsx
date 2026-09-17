import { useState } from 'react'
import { App, Button, Card, Empty, Popconfirm, Table, Tag, Typography } from 'antd'
import {
  ApiOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
} from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { ExternalAppFormModal } from '@/components/externalApps/ExternalAppFormModal'
import { ClientSecretModal } from '@/components/externalApps/ClientSecretModal'
import {
  useCreateGlobalExternalApp,
  useDeleteGlobalExternalApp,
  useGlobalExternalApps,
  useUpdateGlobalExternalApp,
} from '@/hooks/useExternalApps'
import { ApiRequestError } from '@/types'
import type { CreateExternalAppRequest, ExternalAppStatus, ExternalAppView } from '@/types'

const { Text } = Typography

const PLACEMENT_LABELS: Record<string, string> = { sidebar: '侧边栏', project_tab: '项目 tab' }

/**
 * 平台管理 · 全局级外部应用（三级作用域 2026-09-17）。
 *
 * 全局级应用不属于任何租户、全员可见可打开，因此只允许平台管理员创建与管理
 * （权限点 platform.extapp.mgr）；企业角色调本命名空间一律 403。
 */
export function PlatformExternalAppsPage() {
  const { message } = App.useApp()
  const { data: apps, isLoading } = useGlobalExternalApps()
  const createApp = useCreateGlobalExternalApp()
  const updateApp = useUpdateGlobalExternalApp()
  const deleteApp = useDeleteGlobalExternalApp()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ExternalAppView | null>(null)
  const [secretOpen, setSecretOpen] = useState(false)
  const [secretValue, setSecretValue] = useState('')
  const [secretAppName, setSecretAppName] = useState('')

  const handleDelete = async (id: string) => {
    try {
      await deleteApp.mutateAsync(id)
      message.success('已删除全局外部应用')
    } catch (err) {
      message.error(err instanceof ApiRequestError ? err.message : '删除失败')
    }
  }

  const handleSubmit = async (values: CreateExternalAppRequest) => {
    try {
      if (editing) {
        await updateApp.mutateAsync({ id: editing.id, input: values })
        message.success('全局外部应用已更新')
        setFormOpen(false)
      } else {
        const res = await createApp.mutateAsync(values)
        setSecretValue(res.data.client_secret)
        setSecretAppName(values.name)
        setSecretOpen(true)
        setFormOpen(false)
        message.success('全局外部应用已创建')
      }
    } catch (err) {
      if (err instanceof ApiRequestError) message.error(err.message)
    }
  }

  const columns = [
    {
      title: '平台名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <ApiOutlined style={{ color: 'var(--signal)' }} />
          <b>{name}</b>
          <Tag color="blue">全局</Tag>
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
      title: '挂载位置',
      dataIndex: 'placement',
      key: 'placement',
      render: (placement: string) => {
        if (!placement) return <Text type="secondary" style={{ fontSize: 12 }}>未挂载</Text>
        return (
          <span style={{ display: 'inline-flex', gap: 4, flexWrap: 'wrap' }}>
            {placement
              .split(',')
              .map((p) => p.trim())
              .filter(Boolean)
              .map((p) => (
                <Tag key={p} color="purple">
                  {PLACEMENT_LABELS[p] ?? p}
                </Tag>
              ))}
          </span>
        )
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (v: ExternalAppStatus) =>
        v === 'enabled' ? <Tag color="success">已启用</Tag> : <Tag color="default">已停用</Tag>,
    },
    {
      title: '操作',
      key: 'actions',
      render: (_: unknown, app: ExternalAppView) => (
        <span style={{ display: 'inline-flex', gap: 4 }}>
          <Button
            type="link"
            size="small"
            icon={<EditOutlined />}
            onClick={() => {
              setEditing(app)
              setFormOpen(true)
            }}
          >
            编辑
          </Button>
          <Popconfirm
            title={`确定删除「${app.name}」？`}
            description="删除后所有账号都将无法再免密打开该平台。"
            onConfirm={() => void handleDelete(app.id)}
            okText="删除"
            cancelText="取消"
          >
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </span>
      ),
    },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 外部应用"
        icon={<ApiOutlined />}
        subtitle="全局级外部平台：所有账号可见可打开，仅平台管理员可维护"
        actions={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            新增全局应用
          </Button>
        }
      />

      <Card bordered={false} style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}>
        <Table
          rowKey="id"
          columns={columns}
          dataSource={apps ?? []}
          loading={isLoading}
          pagination={false}
          locale={{ emptyText: <Empty description="还没有全局外部应用" /> }}
        />
      </Card>

      <ExternalAppFormModal
        open={formOpen}
        editing={editing}
        scope="global"
        confirmLoading={createApp.isPending || updateApp.isPending}
        onCancel={() => setFormOpen(false)}
        onSubmit={(values) => void handleSubmit(values)}
      />

      <ClientSecretModal
        open={secretOpen}
        appName={secretAppName}
        secret={secretValue}
        onClose={() => setSecretOpen(false)}
      />
    </div>
  )
}