import { useNavigate, useParams } from 'react-router-dom'
import {
  App,
  Button,
  Card,
  Descriptions,
  Popconfirm,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { ArrowLeftOutlined, ReloadOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import {
  usePlatformOrg,
  usePlatformOrgMembers,
  useResetPlatformUserPassword,
  useSetPlatformUserDisabled,
} from '@/hooks/usePlatformAdmin'
import { ApiRequestError } from '@/types'
import type { PlatformOrgMemberView } from '@/types'

const { Text, Paragraph } = Typography

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleString() : '—')
const fmtQuota = (v: number) => (v < 0 ? '不限' : String(v))

function errMessage(e: unknown) {
  return e instanceof ApiRequestError ? e.message : '操作失败'
}

/**
 * 平台管理 · 企业详情：企业元数据 + 成员列表（含账号禁用态）。
 *
 * 读不到企业业务内容 —— 成员列表也只有账号元数据与角色，不含业务数据。
 */
export function PlatformOrgDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { message, modal } = App.useApp()

  const { data: org, isLoading } = usePlatformOrg(id)
  const {
    data: members,
    isLoading: membersLoading,
    isFetching: membersFetching,
    refetch: refetchMembers,
  } = usePlatformOrgMembers(id)
  const setDisabled = useSetPlatformUserDisabled()
  const resetPassword = useResetPlatformUserPassword()

  const handleReset = async (m: PlatformOrgMemberView) => {
    try {
      const res = await resetPassword.mutateAsync(m.user_id)
      const temp = res.data.temp_password
      modal.success({
        title: `已重置 ${res.data.email} 的密码`,
        width: 460,
        content: (
          <Space direction="vertical" size={6} style={{ marginTop: 8 }}>
            <Paragraph copyable={{ text: temp }} style={{ marginBottom: 0, fontSize: 16 }}>
              <Text code>{temp}</Text>
            </Paragraph>
            <Text type="secondary" style={{ fontSize: 12 }}>
              此密码只显示一次，请立即转交该用户；用户登录后请自行修改密码。
            </Text>
          </Space>
        ),
      })
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  const handleToggle = async (m: PlatformOrgMemberView) => {
    try {
      await setDisabled.mutateAsync({ id: m.user_id, disabled: !m.disabled })
      message.success(m.disabled ? '账号已启用' : '账号已禁用（登录与刷新令牌一律被拒）')
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  const memberColumns = [
    {
      title: '成员',
      key: 'member',
      render: (_: unknown, m: PlatformOrgMemberView) => (
        <Space direction="vertical" size={0}>
          <Text strong>{m.name || m.email || m.user_id}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>{m.email || m.user_id}</Text>
        </Space>
      ),
    },
    {
      title: '角色',
      key: 'role',
      width: 140,
      render: (_: unknown, m: PlatformOrgMemberView) => (
        <Text style={{ fontSize: 12 }}>{m.role_name || m.role}</Text>
      ),
    },
    {
      title: '状态',
      key: 'status',
      width: 110,
      render: (_: unknown, m: PlatformOrgMemberView) => (
        <Tag
          color={m.disabled ? 'red' : 'green'}
          style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}
        >
          {m.disabled ? '已禁用' : '正常'}
        </Tag>
      ),
    },
    {
      title: '加入时间',
      key: 'joined',
      width: 170,
      render: (_: unknown, m: PlatformOrgMemberView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>{fmtDate(m.joined_at)}</Text>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_: unknown, m: PlatformOrgMemberView) => (
        <Space size={8}>
          <Popconfirm
            title="重置该成员密码？"
            description="系统会生成一个随机临时密码，旧密码立即失效。临时密码只显示一次。"
            okText="确认"
            cancelText="取消"
            onConfirm={() => void handleReset(m)}
          >
            <Button size="small" loading={resetPassword.isPending}>
              重置密码
            </Button>
          </Popconfirm>
          <Popconfirm
            title={m.disabled ? '启用该账号？' : '禁用该账号？'}
            description={
              m.disabled
                ? '启用后可立即正常登录。'
                : '禁用后登录与刷新令牌一律被拒；已签发的访问令牌会在 15 分钟内自然过期。'
            }
            okText="确认"
            cancelText="取消"
            onConfirm={() => void handleToggle(m)}
          >
            <Button size="small" danger={!m.disabled} loading={setDisabled.isPending}>
              {m.disabled ? '启用' : '禁用'}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title={org ? `平台管理 · ${org.name}` : '平台管理 · 企业详情'}
        subtitle="企业元数据与成员构成（成员只有账号信息与角色，不含业务数据）"
        actions={
          <Space>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/platform/orgs')}>
              返回列表
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => void refetchMembers()}
              loading={membersFetching}
            >
              刷新成员
            </Button>
          </Space>
        }
      />

      <Card
        loading={isLoading}
        style={{ background: 'var(--surface)', border: '1px solid var(--line)', marginBottom: 16 }}
      >
        <Descriptions size="small" column={3}>
          <Descriptions.Item label="企业名称">{org?.name ?? '—'}</Descriptions.Item>
          <Descriptions.Item label="标识">{org?.slug ?? '—'}</Descriptions.Item>
          <Descriptions.Item label="状态">
            {org ? (
              <Tag
                color={org.status === 'disabled' ? 'red' : 'green'}
                style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}
              >
                {org.status === 'disabled' ? '已禁用' : '正常'}
              </Tag>
            ) : (
              '—'
            )}
          </Descriptions.Item>
          <Descriptions.Item label="Owner">
            {org ? `${org.owner_name || '—'}（${org.owner_email || org.owner_id}）` : '—'}
          </Descriptions.Item>
          <Descriptions.Item label="创建时间">{fmtDate(org?.created_at)}</Descriptions.Item>
          <Descriptions.Item label="最后更新">{fmtDate(org?.updated_at)}</Descriptions.Item>
          <Descriptions.Item label="用量（成员/项目/任务）" span={2}>
            {org ? `${org.member_count} / ${org.project_count} / ${org.task_count}` : '—'}
          </Descriptions.Item>
          <Descriptions.Item label="配额（成员/节点/项目/存储）">
            {org
              ? `${fmtQuota(org.quota.max_members)} / ${fmtQuota(org.quota.max_nodes)} / ${fmtQuota(
                  org.quota.max_projects,
                )} / ${fmtQuota(org.quota.max_storage_bytes)}`
              : '—'}
            <Tooltip title="企业 owner 可在企业设置里继续限制成员可见的菜单">
              <Text type="secondary" style={{ fontSize: 12, marginLeft: 8 }}>
                菜单隐藏项：{org?.menu_overrides?.length ?? 0}
              </Text>
            </Tooltip>
          </Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title={`成员（${members?.length ?? 0}）`}
        style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
      >
        <Table<PlatformOrgMemberView>
          rowKey="user_id"
          size="small"
          loading={membersLoading}
          dataSource={members ?? []}
          columns={memberColumns as never}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
        />
      </Card>
    </div>
  )
}