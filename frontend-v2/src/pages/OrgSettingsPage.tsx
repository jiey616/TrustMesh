import { useState } from 'react'
import {
  App,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Skeleton,
  Table,
  Tag,
  Typography,
} from 'antd'
import { PlusOutlined, TeamOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { useAuthStore } from '@/stores/authStore'
import { LLMConfigCard } from '@/components/settings/LLMConfigCard'
import {
  useAddOrgMember,
  useCreateOrg,
  useOrganizations,
  useOrgMembers,
  useRemoveOrgMember,
  useUpdateOrgMemberRole,
} from '@/hooks/useOrgs'
import { ApiRequestError } from '@/types'
import type { OrgMemberView, OrgView } from '@/types'

const { Text, Paragraph } = Typography

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

function formatBytes(n: number) {
  if (n < 0) return '不限'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleDateString() : '—')

function errMessage(e: unknown) {
  return e instanceof ApiRequestError ? e.message : '操作失败'
}

function CreateOrgModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const createOrg = useCreateOrg()
  const [form] = Form.useForm()

  const handleOk = async () => {
    const values = await form.validateFields()
    try {
      await createOrg.mutateAsync({ name: values.name.trim(), slug: values.slug?.trim() || undefined })
      message.success('企业已创建，已切换到新企业')
      onClose()
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  return (
    <Modal
      title="创建企业"
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      okText="创建"
      confirmLoading={createOrg.isPending}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="name"
          label="企业名称"
          rules={[{ required: true, message: '请输入企业名称' }]}
        >
          <Input placeholder="例如：山雨影业" maxLength={64} />
        </Form.Item>
        <Form.Item
          name="slug"
          label="标识（可选）"
          extra="小写字母/数字/连字符，3-64 位；留空自动从名称生成"
        >
          <Input placeholder="例如：shanyu-studio" maxLength={64} />
        </Form.Item>
        <Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
          创建后你将成为该企业的 Owner，可在成员管理中邀请已有账号加入。
        </Paragraph>
      </Form>
    </Modal>
  )
}

function MembersCard({ org }: { org: OrgView }) {
  const { message } = App.useApp()
  const { data: members, isLoading } = useOrgMembers(org.id)
  const addMember = useAddOrgMember(org.id)
  const updateRole = useUpdateOrgMemberRole(org.id)
  const removeMember = useRemoveOrgMember(org.id)

  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'admin' | 'member'>('member')

  const canManage = org.my_role === 'owner' || org.my_role === 'admin'

  const handleAdd = async () => {
    const addr = email.trim()
    if (!addr) return
    try {
      await addMember.mutateAsync({ email: addr, role })
      message.success(`已添加 ${addr}`)
      setEmail('')
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  const columns = [
    {
      title: '成员',
      key: 'member',
      render: (_: unknown, m: OrgMemberView) => (
        <span>
          <Text strong style={{ marginRight: 8 }}>{m.name || m.email || m.user_id}</Text>
          {m.email && m.name && <Text type="secondary" style={{ fontSize: 12 }}>{m.email}</Text>}
        </span>
      ),
    },
    {
      title: '角色',
      key: 'role',
      width: 130,
      render: (_: unknown, m: OrgMemberView) => (
        <Tag color={roleTagColor[m.role]} style={{ borderRadius: 'var(--radius-control)' }}>
          {roleLabels[m.role] ?? m.role}
        </Tag>
      ),
    },
    {
      title: '加入时间',
      key: 'joined',
      width: 110,
      render: (_: unknown, m: OrgMemberView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>{fmtDate(m.joined_at)}</Text>
      ),
    },
    ...(canManage
      ? [
          {
            title: '操作',
            key: 'actions',
            width: 150,
            render: (_: unknown, m: OrgMemberView) => {
              const isSelf = false // owner 行在下方被禁用操作
              const isOwner = m.role === 'owner'
              const canTouch =
                !isOwner && (org.my_role === 'owner' || m.role === 'member') && !isSelf
              return (
                <span style={{ display: 'inline-flex', gap: 8 }}>
                  <Select
                    size="small"
                    value={m.role === 'owner' ? 'owner' : m.role}
                    disabled={isOwner || !canTouch || updateRole.isPending}
                    style={{ width: 96 }}
                    onChange={async (r) => {
                      try {
                        await updateRole.mutateAsync({ userId: m.user_id, role: r as 'admin' | 'member' })
                        message.success('角色已更新')
                      } catch (e) {
                        message.error(errMessage(e))
                      }
                    }}
                    options={[
                      { value: 'admin', label: 'Admin' },
                      { value: 'member', label: '成员' },
                    ]}
                  />
                  <Popconfirm
                    title="确定移除该成员？"
                    onConfirm={async () => {
                      try {
                        await removeMember.mutateAsync(m.user_id)
                        message.success('已移除')
                      } catch (e) {
                        message.error(errMessage(e))
                      }
                    }}
                    disabled={!canTouch}
                  >
                    <Button size="small" danger disabled={!canTouch}>
                      移除
                    </Button>
                  </Popconfirm>
                </span>
              )
            },
          },
        ]
      : []),
  ]

  return (
    <Card
      title={
        <span>
          <TeamOutlined style={{ marginRight: 8 }} />
          成员管理
        </span>
      }
      style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
    >
      {canManage && (
        <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
          <Input
            placeholder="对方注册邮箱"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            onPressEnter={handleAdd}
            style={{ maxWidth: 280 }}
            disabled={addMember.isPending}
          />
          <Select
            value={role}
            onChange={setRole}
            style={{ width: 110 }}
            options={[
              { value: 'member', label: '成员' },
              { value: 'admin', label: 'Admin' },
            ]}
          />
          <Button type="primary" loading={addMember.isPending} onClick={handleAdd}>
            添加
          </Button>
        </div>
      )}
      <Table<OrgMemberView>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={members ?? []}
        columns={columns as never}
        pagination={false}
        locale={{ emptyText: <Empty description="暂无成员" /> }}
      />
      {canManage && (
        <Paragraph type="secondary" style={{ fontSize: 12, marginTop: 12, marginBottom: 0 }}>
          仅能邀请已在平台注册的账号；Owner 不可移除或降级（转让功能二期提供）。
        </Paragraph>
      )}
    </Card>
  )
}

export function OrgSettingsPage() {
  const { activeOrgId } = useAuthStore()
  const { user } = useAuthStore()
  const { data: orgs, isLoading } = useOrganizations()
  const [createOpen, setCreateOpen] = useState(false)

  const orgsList = orgs ?? []
  const current: OrgView | undefined =
    orgsList.find((o) => o.id === activeOrgId) ?? orgsList.find((o) => o.kind === 'personal')

  const isPersonal = !current || current.kind === 'personal'

  return (
    <div style={{ maxWidth: 880, margin: '0 auto', paddingBottom: 40 }}>
      <PageHeader
        title="企业管理"
        subtitle="租户信息、成员与权限（配额为预留字段，当前不做限额执行）"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            创建企业
          </Button>
        }
      />

      {isLoading ? (
        <Skeleton active paragraph={{ rows: 6 }} />
      ) : !current ? (
        <Empty description="没有可用租户" />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <Card
            title={
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                {current.name}
                <Tag color={isPersonal ? 'default' : 'blue'} style={{ borderRadius: 'var(--radius-control)' }}>
                  {isPersonal ? '个人空间' : '企业'}
                </Tag>
                <Tag color={roleTagColor[current.my_role]} style={{ borderRadius: 'var(--radius-control)' }}>
                  我的角色：{roleLabels[current.my_role]}
                </Tag>
              </span>
            }
            style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
          >
            <Descriptions size="small" column={2}>
              <Descriptions.Item label="标识">{current.slug}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{fmtDate(current.created_at)}</Descriptions.Item>
              <Descriptions.Item label="成员上限">{current.quota.max_members < 0 ? '不限' : current.quota.max_members}</Descriptions.Item>
              <Descriptions.Item label="节点上限">{current.quota.max_nodes < 0 ? '不限' : current.quota.max_nodes}</Descriptions.Item>
              <Descriptions.Item label="项目上限">{current.quota.max_projects < 0 ? '不限' : current.quota.max_projects}</Descriptions.Item>
              <Descriptions.Item label="存储上限">{formatBytes(current.quota.max_storage_bytes)}</Descriptions.Item>
            </Descriptions>
          </Card>

          {isPersonal ? (
            <Card style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}>
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="个人空间不支持成员管理。创建企业后即可邀请其他账号协同。"
              />
            </Card>
          ) : (
            <MembersCard org={current} />
          )}

          {/* LLM 配置：平台默认层（仅平台管理员）+ 本租户覆盖层（org owner/admin) */}
          {user?.is_admin && <LLMConfigCard />}
          {!isPersonal && (current.my_role === 'owner' || current.my_role === 'admin') && (
            <LLMConfigCard orgId={current.id} />
          )}
        </div>
      )}

      <CreateOrgModal open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  )
}
