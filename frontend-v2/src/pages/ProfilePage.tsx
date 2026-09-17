import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import {
  App,
  Avatar,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  Modal,
  Skeleton,
  Table,
  Tag,
  Typography,
} from 'antd'
import {
  AppstoreOutlined,
  EditOutlined,
  LockOutlined,
  LogoutOutlined,
  UserOutlined,
} from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { ServerConfigCard } from '@/components/settings/ServerConfigCard'
import { ORG_DOMAIN_PERMS } from '@/lib/perms'
import { useAuthStore } from '@/stores/authStore'
import { usePermStore } from '@/stores/permStore'
import { isElectronRuntime } from '@/stores/serverConfigStore'
import { useOrganizations, orgKeys } from '@/hooks/useOrgs'
import { updateProfile, changePassword } from '@/api/user'
import { ApiRequestError } from '@/types'
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

function errMessage(e: unknown) {
  return e instanceof ApiRequestError ? e.message : '操作失败'
}

export function ProfilePage() {
  const navigate = useNavigate()
  const { message, modal } = App.useApp()
  const user = useAuthStore((s) => s.user)
  const activeOrgId = useAuthStore((s) => s.activeOrgId)
  const logout = useAuthStore((s) => s.logout)
  const setUser = useAuthStore((s) => s.setUser)
  const queryClient = useQueryClient()
  const { data: orgs, isLoading } = useOrganizations()

  const [editOpen, setEditOpen] = useState(false)
  const [pwdOpen, setPwdOpen] = useState(false)

  const activeOrg = (orgs ?? []).find((o) => o.id === activeOrgId)
  const enterpriseOrgs = (orgs ?? []).filter((o) => o.kind === 'enterprise')

  // 「组织管理」「外部应用」两个入口原本在左侧主菜单，现统一收在本页。
  // 可见性规则与各自路由的门禁对齐：平台管理员碰不到业务页（业务命名空间整体 403），
  // 故一律不展示；组织管理再按组织域任一权限点判定。权限视图未就绪时 fail-open，
  // 由路由 / 后端鉴权兜底（与 MainLayout 侧边栏同一规则）。
  const permReady = usePermStore((s) => s.ready)
  const permissions = usePermStore((s) => s.permissions)
  const isPlatformAdmin = usePermStore((s) => s.isPlatformAdmin)
  const canManageOrg =
    !isPlatformAdmin && (!permReady || ORG_DOMAIN_PERMS.some((p) => permissions.includes(p)))
  const showExternalApps = !isPlatformAdmin

  const handleLogout = () => {
    modal.confirm({
      title: '退出登录',
      content: '退出后需要重新登录才能继续操作。',
      okText: '退出登录',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => {
        logout()
        // 清掉跨账号存活的 react-query 缓存（同 MainLayout.handleLogout）
        queryClient.clear()
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

      <Card
        title="账号资料"
        style={{ marginBottom: 16 }}
        extra={
          <Button type="link" icon={<EditOutlined />} onClick={() => setEditOpen(true)}>
            编辑资料
          </Button>
        }
      >
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

      {/* 服务器地址配置仅桌面端提供；Web 端固定同源 /api/v1/ */}
      {isElectronRuntime() && (
        <div id="server">
          <ServerConfigCard />
        </div>
      )}

      <Card
        title="安全"
        style={{ marginBottom: 16 }}
        extra={
          <Button type="link" icon={<LockOutlined />} onClick={() => setPwdOpen(true)}>
            修改密码
          </Button>
        }
      >
        <Text type="secondary" style={{ fontSize: 13 }}>
          修改密码需要验证当前密码。密码长度至少 8 位。
        </Text>
      </Card>

      <Card
        title="工作区"
        style={{ marginBottom: 16 }}
        extra={
          canManageOrg ? (
            <Button
              type="link"
              onClick={() => navigate('/organizations')}
              style={{ paddingInline: 4 }}
            >
              组织管理
            </Button>
          ) : null
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

            <div style={{ marginBottom: 8, color: 'var(--text-secondary)', fontSize: 13 }}>我所属的组织</div>
            {enterpriseOrgs.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="尚未加入任何组织"
                style={{ margin: '12px 0' }}
              />
            ) : (
              <Table<OrgView>
                dataSource={enterpriseOrgs}
                rowKey="id"
                size="small"
                pagination={false}
                columns={[
                  { title: '组织名称', dataIndex: 'name', key: 'name' },
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

      {showExternalApps && (
        <Card
          title="外部应用"
          style={{ marginBottom: 16 }}
          extra={
            <Button
              type="link"
              icon={<AppstoreOutlined />}
              onClick={() => navigate('/external-apps')}
              style={{ paddingInline: 4 }}
            >
              管理外部应用
            </Button>
          }
        >
          <Text type="secondary" style={{ fontSize: 13 }}>
            查看并管理外部应用（全局 / 组织 / 个人三级）：SSO 连接状态、打开方式（内嵌 / 新窗口）与挂载位置。
          </Text>
        </Card>
      )}

      <EditProfileModal
        open={editOpen}
        currentName={user.name}
        onClose={() => setEditOpen(false)}
        onSuccess={(updated) => {
          setUser(updated)
          // 个人租户名称随用户名同步，刷新工作区列表
          queryClient.invalidateQueries({ queryKey: orgKeys.all })
          message.success('资料已更新')
          setEditOpen(false)
        }}
      />

      <ChangePasswordModal
        open={pwdOpen}
        onClose={() => setPwdOpen(false)}
        onSuccess={() => {
          message.success('密码已修改，下次登录请使用新密码')
          setPwdOpen(false)
        }}
      />
    </div>
  )
}

function EditProfileModal({
  open,
  currentName,
  onClose,
  onSuccess,
}: {
  open: boolean
  currentName: string
  onClose: () => void
  onSuccess: (user: import('@/types').User) => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [submitting, setSubmitting] = useState(false)

  const handleOk = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      const res = await updateProfile({ name: values.name.trim() })
      onSuccess(res.data)
      form.resetFields()
    } catch (e) {
      message.error(errMessage(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="编辑资料"
      open={open}
      onCancel={() => {
        form.resetFields()
        onClose()
      }}
      onOk={handleOk}
      confirmLoading={submitting}
      okText="保存"
      cancelText="取消"
      destroyOnHidden
    >
      <Form form={form} layout="vertical" initialValues={{ name: currentName }} style={{ marginTop: 16 }}>
        <Form.Item
          name="name"
          label="名称"
          rules={[
            { required: true, message: '请输入名称' },
            { max: 64, message: '名称最多 64 个字符' },
          ]}
        >
          <Input placeholder="你的显示名称" maxLength={64} />
        </Form.Item>
        <Text type="secondary" style={{ fontSize: 12 }}>
          邮箱暂不支持修改。改名会同步更新你的「个人空间」工作区名称。
        </Text>
      </Form>
    </Modal>
  )
}

function ChangePasswordModal({
  open,
  onClose,
  onSuccess,
}: {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [submitting, setSubmitting] = useState(false)

  const handleOk = async () => {
    const values = await form.validateFields()
    setSubmitting(true)
    try {
      await changePassword({
        old_password: values.oldPassword,
        new_password: values.newPassword,
      })
      onSuccess()
      form.resetFields()
    } catch (e) {
      message.error(errMessage(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="修改密码"
      open={open}
      onCancel={() => {
        form.resetFields()
        onClose()
      }}
      onOk={handleOk}
      confirmLoading={submitting}
      okText="确认修改"
      cancelText="取消"
      destroyOnHidden
    >
      <Form form={form} layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item
          name="oldPassword"
          label="当前密码"
          rules={[{ required: true, message: '请输入当前密码' }]}
        >
          <Input.Password placeholder="当前使用的密码" autoComplete="current-password" />
        </Form.Item>
        <Form.Item
          name="newPassword"
          label="新密码"
          rules={[
            { required: true, message: '请输入新密码' },
            { min: 8, message: '新密码至少 8 位' },
          ]}
        >
          <Input.Password placeholder="至少 8 位" autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          name="confirmPassword"
          label="确认新密码"
          dependencies={['newPassword']}
          rules={[
            { required: true, message: '请再次输入新密码' },
            ({ getFieldValue }) => ({
              validator(_, value) {
                if (!value || getFieldValue('newPassword') === value) {
                  return Promise.resolve()
                }
                return Promise.reject(new Error('两次输入的新密码不一致'))
              },
            }),
          ]}
        >
          <Input.Password placeholder="再次输入新密码" autoComplete="new-password" />
        </Form.Item>
      </Form>
    </Modal>
  )
}
