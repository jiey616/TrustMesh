import { useMemo, useState } from 'react'
import {
  Alert,
  App,
  Button,
  Card,
  Empty,
  Popconfirm,
  Table,
  Tabs,
  Tag,
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
import { ExternalAppFormModal } from '@/components/externalApps/ExternalAppFormModal'
import { ClientSecretModal } from '@/components/externalApps/ClientSecretModal'
import {
  useCreateExternalApp,
  useDeleteExternalApp,
  useExternalApps,
  useLaunchExternalApp,
  useUpdateExternalApp,
} from '@/hooks/useExternalApps'
import { useOrganizations } from '@/hooks/useOrgs'
import { useAuthStore } from '@/stores/authStore'
import { usePermStore } from '@/stores/permStore'
import { PERM, hasPerm } from '@/lib/perms'
import { ApiRequestError } from '@/types'
import type { CreateExternalAppRequest, ExternalAppScope, ExternalAppStatus, ExternalAppView } from '@/types'

const { Text } = Typography

/** 挂载位置标签 */
const PLACEMENT_LABELS: Record<string, string> = { sidebar: '侧边栏', project_tab: '项目 tab' }
const SCOPE_META: Record<ExternalAppScope, { label: string; color: string }> = {
  global: { label: '全局', color: 'blue' },
  org: { label: '组织', color: 'purple' },
  personal: { label: '个人', color: 'default' },
}

type TabKey = ExternalAppScope

/**
 * 外部应用（三级作用域，2026-09-17）：
 *   全局 = 平台管理员在 /platform/external-apps 配置，全员可见可打开（本页只读）；
 *   组织 = 本组织成员可见，仅持 org.app.mgr 者（owner/admin）可增删改；
 *   个人 = 仅创建者本人可见可管。
 *
 * 列表由后端按三级可见性过滤后一次返回，这里只按 scope 分组投影。
 */
export function ExternalAppsPage() {
  const { message } = App.useApp()
  const { data: apps, isLoading } = useExternalApps()
  const createApp = useCreateExternalApp()
  const updateApp = useUpdateExternalApp()
  const deleteApp = useDeleteExternalApp()
  const launchApp = useLaunchExternalApp()

  const { activeOrgId } = useAuthStore()
  const { data: orgs } = useOrganizations()
  const { permissions, ready: permReady } = usePermStore()

  const activeOrg = useMemo(
    () => (orgs ?? []).find((o) => o.id === activeOrgId),
    [orgs, activeOrgId],
  )
  /** 组织级应用必须落在企业租户：个人空间下不允许新建 */
  const inEnterprise = activeOrg?.kind === 'enterprise'
  const canOrgManage = permReady && hasPerm(permissions, PERM.ORG_APP_MGR)

  const [tab, setTab] = useState<TabKey | null>(null)
  const activeTab: TabKey = tab ?? (inEnterprise ? 'org' : 'personal')

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<ExternalAppView | null>(null)
  const [secretOpen, setSecretOpen] = useState(false)
  const [secretValue, setSecretValue] = useState('')
  const [secretAppName, setSecretAppName] = useState('')

  const grouped = useMemo(() => {
    const out: Record<ExternalAppScope, ExternalAppView[]> = { global: [], org: [], personal: [] }
    for (const a of apps ?? []) out[a.scope]?.push(a)
    return out
  }, [apps])

  /** 能否管理该行：全局级本页只读，组织级看 org.app.mgr，个人级（列表里只有本人的）恒可管 */
  const canManageRow = (app: ExternalAppView) =>
    app.scope === 'org' ? canOrgManage : app.scope === 'personal'

  const openCreate = () => {
    setEditing(null)
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

  const handleSubmit = async (values: CreateExternalAppRequest) => {
    try {
      if (editing) {
        await updateApp.mutateAsync({ id: editing.id, input: values })
        message.success('外部平台已更新')
        setFormOpen(false)
      } else {
        const res = await createApp.mutateAsync(values)
        // client_secret 仅返回一次，立即展示并要求用户保存
        setSecretValue(res.data.client_secret)
        setSecretAppName(values.name)
        setSecretOpen(true)
        setFormOpen(false)
        message.success('外部平台已创建')
      }
    } catch (err) {
      if (err instanceof ApiRequestError) message.error(err.message)
      // 表单校验错误由 antd 自行展示，不重复提示
    }
  }

  const columns = [
    {
      title: '平台名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, app: ExternalAppView) => (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <AppstoreOutlined style={{ color: 'var(--signal)' }} />
          <b>{name}</b>
          <Tag color={SCOPE_META[app.scope].color}>{SCOPE_META[app.scope].label}</Tag>
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
            icon={<ExportOutlined />}
            disabled={app.status !== 'enabled'}
            onClick={() => void handleLaunch(app)}
          >
            打开
          </Button>
          {canManageRow(app) && (
            <>
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
                title={`确定断开「${app.name}」？`}
                description="断开后将无法再免密打开该平台。"
                onConfirm={() => void handleDelete(app.id)}
                okText="断开"
                cancelText="取消"
              >
                <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
            </>
          )}
        </span>
      ),
    },
  ]

  const newLabel = activeTab === 'org' ? '新增组织级应用' : '新增个人应用'
  const showCreate =
    activeTab === 'personal' || (activeTab === 'org' && canOrgManage && inEnterprise)

  const tabHint = () => {
    if (activeTab === 'global') {
      return (
        <Text type="secondary" style={{ fontSize: 12 }}>
          全局级应用由平台管理员统一配置，所有账号均可打开，但不提供编辑入口。
        </Text>
      )
    }
    if (activeTab === 'org') {
      if (!inEnterprise) {
        return (
          <Alert
            type="info"
            showIcon
            message="当前是个人空间"
            description="组织级应用只能在企业工作区中查看与创建，请先切换到企业空间。"
          />
        )
      }
      if (!canOrgManage) {
        return (
          <Alert
            type="info"
            showIcon
            message="你没有组织级外部应用的管理权限"
            description="组织级应用的创建/编辑/删除仅组织 owner/admin（org.app.mgr）可用；如需要，请联系组织管理员。"
          />
        )
      }
      return (
        <Text type="secondary" style={{ fontSize: 12 }}>
          组织级应用对本组织全体成员可见可打开，仅 owner/admin 可增删改。
        </Text>
      )
    }
    return (
      <Text type="secondary" style={{ fontSize: 12 }}>
        个人级应用仅你自己可见可用，在企业与个人工作区都能管理。
      </Text>
    )
  }

  return (
    <div>
      <PageHeader
        title="外部应用"
        icon={<ApiOutlined />}
        subtitle="通过 SSO 连接外部平台，在 TrustMesh 内免密打开。按全局 / 组织 / 个人三级管理。"
        actions={
          showCreate ? (
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              {newLabel}
            </Button>
          ) : undefined
        }
      />

      <Card bordered={false} style={{ background: 'var(--surface)' }}>
        <Tabs
          activeKey={activeTab}
          onChange={(k) => setTab(k as TabKey)}
          items={[
            {
              key: 'global',
              label: `全局（${grouped.global.length}）`,
              children: (
                <Table
                  rowKey="id"
                  columns={columns}
                  dataSource={grouped.global}
                  loading={isLoading}
                  pagination={false}
                  locale={{ emptyText: <Empty description="平台管理员还没有配置全局外部应用" /> }}
                />
              ),
            },
            {
              key: 'org',
              label: `组织（${grouped.org.length}）`,
              children: (
                <Table
                  rowKey="id"
                  columns={columns}
                  dataSource={grouped.org}
                  loading={isLoading}
                  pagination={false}
                  locale={{ emptyText: <Empty description="本组织还没有组织级外部应用" /> }}
                />
              ),
            },
            {
              key: 'personal',
              label: `个人（${grouped.personal.length}）`,
              children: (
                <Table
                  rowKey="id"
                  columns={columns}
                  dataSource={grouped.personal}
                  loading={isLoading}
                  pagination={false}
                  locale={{ emptyText: <Empty description="你还没有个人级外部应用" /> }}
                />
              ),
            },
          ]}
        />
        <div style={{ marginTop: 12 }}>{tabHint()}</div>
      </Card>

      <ExternalAppFormModal
        open={formOpen}
        editing={editing}
        scope={activeTab}
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