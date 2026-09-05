import { useNavigate } from 'react-router-dom'
import { App, Avatar, Button, Card, Descriptions, Empty, Skeleton, Table, Tag, Typography } from 'antd'
import { LogoutOutlined, UserOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { useAuthStore } from '@/stores/authStore'
import { useOrganizations } from '@/hooks/useOrgs'
import type { OrgView } from '@/types'

const { Text } = Typography

const roleTagColor: Record<string, string> = {
  owner: 'gold',
  admin: 'purple',
  member: 'default',
}
const roleLabels: Record<string, string> = {
  owner: 'Owner',
  admin: 'Admin',
  member: '成员',
}

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleString() : '—')

export function ProfilePage() {
  const navigate = useNavigate()
  const { message, modal } = App.useApp()
  const user = useAuthStore((s) => s.user)
  const activeOrgId = useAuthStore((s) => s.activeOrgId)
  const logout = useAuthStore((s) => s.logout)
  const { data: orgs, isLoading } = useOrganizations()

  const activeOrg = (orgs ?? []).find((o) => o.id === activeOrgId)
  const enterpriseOrgs = (orgs ?? []).filter((o) => o.kind === 'enterprise')

  const handleLogout = () => {
    modal.confirm({
      title: '退出登录',
      content: '退出后需要重新登录才能继续操作。',
      okText: '退出登录',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => {
        logout()
        message.success('已退出登录')
        navigate('/login')
      },
    })
  }

  if (!user) {
    return (
      <div style={{ padding: 24 }}>
        <Empty description="未获取到登录信息，请重新登录" />
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="个人信息"
        icon={<UserOutlined />}
        subtitle="账号资料与所属工作区"
        actions={
          <Button danger icon={<LogoutOutlined />} onClick={handleLogout}>
            退出登录
          </Button>
        }
      />

      <Card title="账号资料" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 20 }}>
          <Avatar
            size={56}
            icon={<UserOutlined />}
            style={{ background: 'linear-gradient(135deg, var(--signal), var(--signal))', flexShrink: 0 }}
          />
          <div style={{ minWidth: 0 }}>
            <div style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-primary)' }}>{user.name}</div>
            <Text type="secondary" style={{ fontSize: 13 }}>
              {user.email}
            </Text>
          </div>
        </div>
        <Descriptions column={1} size="small" bordered>
          <Descriptions.Item label="用户 ID">
            <Text copyable style={{ fontSize: 12 }}>
              {user.id}
            </Text>
          </Descriptions.Item>
          <Descriptions.Item label="邮箱">{user.email}</Descriptions.Item>
          <Descriptions.Item label="名称">{user.name}</Descriptions.Item>
          <Descriptions.Item label="注册时间">{fmtDate(user.created_at)}</Descriptions.Item>
          <Descriptions.Item label="更新时间">{fmtDate(user.updated_at)}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title="工作区"
        extra={
          <Button type="link" onClick={() => navigate('/organizations')} style={{ paddingInline: 4 }}>
            企业管理
          </Button>
        }
      >
        {isLoading ? (
          <Skeleton active paragraph={{ rows: 3 }} />
        ) : (
          <>
            <Descriptions column={1} size="small" bordered style={{ marginBottom: 20 }}>
              <Descriptions.Item label="当前工作区">
                {activeOrg ? (
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                    {activeOrg.name}
                    <Tag color={roleTagColor[activeOrg.my_role] ?? 'default'}>
                      {roleLabels[activeOrg.my_role] ?? activeOrg.my_role}
                    </Tag>
                  </span>
                ) : (
                  '个人空间'
                )}
              </Descriptions.Item>
            </Descriptions>

            <div style={{ marginBottom: 8, color: 'var(--text-secondary)', fontSize: 13 }}>我所属的企业</div>
            {enterpriseOrgs.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="尚未加入任何企业"
                style={{ margin: '12px 0' }}
              />
            ) : (
              <Table<OrgView>
                dataSource={enterpriseOrgs}
                rowKey="id"
                size="small"
                pagination={false}
                columns={[
                  { title: '企业名称', dataIndex: 'name', key: 'name' },
                  {
                    title: '标识',
                    dataIndex: 'slug',
                    key: 'slug',
                    render: (v: string) => <Text type="secondary">{v}</Text>,
                  },
                  {
                    title: '我的角色',
                    dataIndex: 'my_role',
                    key: 'my_role',
                    width: 120,
                    render: (r: string) => (
                      <Tag color={roleTagColor[r] ?? 'default'}>{roleLabels[r] ?? r}</Tag>
                    ),
                  },
                  {
                    title: '',
                    key: 'current',
                    width: 100,
                    render: (_, row) =>
                      row.id === activeOrgId ? <Tag color="blue">当前</Tag> : null,
                  },
                ]}
              />
            )}
          </>
        )}
      </Card>
    </div>
  )
}
