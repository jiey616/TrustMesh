import { useMemo, useState } from 'react'
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
  Tabs,
  Tag,
  Typography,
} from 'antd'
import type { TabsProps } from 'antd'
import { PlusOutlined, TeamOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import { useAuthStore } from '@/stores/authStore'
import { usePermStore } from '@/stores/permStore'
import { PERM } from '@/lib/perms'
import { LLMConfigCard } from '@/components/settings/LLMConfigCard'
import { OrgRolesCard } from '@/components/settings/OrgRolesCard'
import { MenuVisibilityCard } from '@/components/settings/MenuVisibilityCard'
import {
  useAddOrgMember,
  useCreateOrg,
  useOrganizations,
  useOrgMembers,
  useRemoveOrgMember,
  useUpdateOrgMemberRole,
} from '@/hooks/useOrgs'
import { useOrgRoles } from '@/hooks/useOrgRoles'
import { ApiRequestError } from '@/types'
import type { OrgMemberView, OrgRoleView, OrgView } from '@/types'

const { Text, Paragraph } = Typography

/** 内置角色 ID 是确定性的（后端 model.BuiltinOrgRoleID：role_<orgId>_<key>）。 */
function builtinRoleId(orgId: string, key: 'owner' | 'admin' | 'member') {
  return `role_${orgId}_${key}`
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
  if (!(e instanceof ApiRequestError)) return '操作失败'
  if (e.code === 'PERSONAL_ORG') return '个人空间不支持成员管理'
  if (e.code === 'VALIDATION_ERROR') return '角色不可指派（Owner 角色只能通过转让流程变更）'
  return e.message
}

function CreateOrgModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const createOrg = useCreateOrg()
  const [form] = Form.useForm()

  const handleOk = async () => {
    const values = await form.validateFields()
    try {
      await createOrg.mutateAsync({ name: values.name.trim(), slug: values.slug?.trim() || undefined })
      message.success('组织已创建，已切换到新组织')
      onClose()
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  return (
    <Modal
      title="创建组织"
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
          label="组织名称"
          rules={[{ required: true, message: '请输入组织名称' }]}
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
          创建后你将成为该组织的 Owner，可在成员管理中邀请已有账号加入。
        </Paragraph>
      </Form>
    </Modal>
  )
}

function MembersCard({ org }: { org: OrgView }) {
  const { message } = App.useApp()
  const { data: members, isLoading } = useOrgMembers(org.id)
  const { data: roles } = useOrgRoles(org.id)
  const addMember = useAddOrgMember(org.id)
  const updateRole = useUpdateOrgMemberRole(org.id)
  const removeMember = useRemoveOrgMember(org.id)

  const [email, setEmail] = useState('')
  const [roleId, setRoleId] = useState<string>(builtinRoleId(org.id, 'member'))

  // 可指派角色：Owner 角色不可通过成员管理授予（只能走转让流程），故从选项中剔除。
  const assignableRoles = useMemo(
    () => (roles ?? []).filter((r) => r.id !== builtinRoleId(org.id, 'owner')),
    [roles, org.id],
  )
  const roleOptions = assignableRoles.map((r: OrgRoleView) => ({
    value: r.id,
    label: r.builtin ? r.name : `${r.name}（自定义）`,
  }))

  const canManage = org.my_role === 'owner' || org.my_role === 'admin'

  const handleAdd = async () => {
    const addr = email.trim()
    if (!addr) return
    try {
      await addMember.mutateAsync({ email: addr, role_id: roleId })
      message.success(`已添加 ${addr}`)
      setEmail('')
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  /** 成员当前的 role_id：新数据直接取；兼容期未回填时按内置语义键推导。 */
  const roleIdOf = (m: OrgMemberView) =>
    m.role_id ||
    (m.role === 'owner' || m.role === 'admin' || m.role === 'member'
      ? builtinRoleId(org.id, m.role)
      : m.role)

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
      width: 200,
      render: (_: unknown, m: OrgMemberView) => {
        const isOwner = roleIdOf(m) === builtinRoleId(org.id, 'owner')
        const label =
          m.role_name ?? (roles ?? []).find((r) => r.id === roleIdOf(m))?.name ?? m.role
        const custom = !isOwner && !/^(owner|admin|member)$/.test(m.role)
        return (
          <Tag
            color={isOwner ? 'gold' : custom ? 'purple' : 'default'}
            style={{ borderRadius: 'var(--radius-control)' }}
          >
            {label}
          </Tag>
        )
      },
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
            width: 260,
            render: (_: unknown, m: OrgMemberView) => {
              const isOwner = roleIdOf(m) === builtinRoleId(org.id, 'owner')
              // 内置 admin 只能由 owner 调整；owner 不可动
              const isAdmin = m.role === 'admin'
              const canTouch = !isOwner && (org.my_role === 'owner' || !isAdmin)
              return (
                <span style={{ display: 'inline-flex', gap: 8 }}>
                  <Select
                    size="small"
                    value={roleIdOf(m)}
                    disabled={isOwner || !canTouch || updateRole.isPending}
                    style={{ width: 130 }}
                    options={roleOptions}
                    onChange={async (next: string) => {
                      try {
                        await updateRole.mutateAsync({ userId: m.user_id, ref: { role_id: next } })
                        message.success('角色已更新')
                      } catch (e) {
                        message.error(errMessage(e))
                      }
                    }}
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
            value={roleId}
            onChange={setRoleId}
            style={{ width: 150 }}
            options={roleOptions}
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
          角色下拉包含本组织的自定义角色，其权限集在「角色管理」中维护。
        </Paragraph>
      )}
    </Card>
  )
}

export function OrgSettingsPage() {
  const { activeOrgId } = useAuthStore()
  const { data: orgs, isLoading } = useOrganizations()
  const permissions = usePermStore((s) => s.permissions)
  const permReady = usePermStore((s) => s.ready)
  const [createOpen, setCreateOpen] = useState(false)

  // 权限视图未就绪时 fail-open（菜单/标签先按可见渲染，后端 API 兜底鉴权）。
  const can = (perm: string) => !permReady || permissions.includes(perm)

  const orgsList = orgs ?? []
  const current: OrgView | undefined =
    orgsList.find((o) => o.id === activeOrgId) ?? orgsList.find((o) => o.kind === 'personal')

  const isPersonal = !current || current.kind === 'personal'
  const isOrgAdmin = !!current && (current.my_role === 'owner' || current.my_role === 'admin')

  const tabs: TabsProps['items'] = current
    ? [
        {
          key: 'overview',
          label: '概况',
          children: (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <Card
                title={
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                    {current.name}
                    <Tag
                      color={isPersonal ? 'default' : 'blue'}
                      style={{ borderRadius: 'var(--radius-control)' }}
                    >
                      {isPersonal ? '个人空间' : '组织'}
                    </Tag>
                    <Tag
                      color={current.my_role === 'owner' ? 'gold' : 'default'}
                      style={{ borderRadius: 'var(--radius-control)' }}
                    >
                      我的角色：{current.my_role === 'owner' ? 'Owner' : current.my_role === 'admin' ? 'Admin' : '成员'}
                    </Tag>
                  </span>
                }
                style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
              >
                <Descriptions size="small" column={2}>
                  <Descriptions.Item label="标识">{current.slug}</Descriptions.Item>
                  <Descriptions.Item label="创建时间">{fmtDate(current.created_at)}</Descriptions.Item>
                  <Descriptions.Item label="成员上限">
                    {current.quota.max_members < 0 ? '不限' : current.quota.max_members}
                  </Descriptions.Item>
                  <Descriptions.Item label="节点上限">
                    {current.quota.max_nodes < 0 ? '不限' : current.quota.max_nodes}
                  </Descriptions.Item>
                  <Descriptions.Item label="项目上限">
                    {current.quota.max_projects < 0 ? '不限' : current.quota.max_projects}
                  </Descriptions.Item>
                  <Descriptions.Item label="存储上限">
                    {formatBytes(current.quota.max_storage_bytes)}
                  </Descriptions.Item>
                </Descriptions>
              </Card>
              {isPersonal && <LLMConfigCard scope="personal" />}
            </div>
          ),
        },
        // 「企业设置」标签按 org.settings 单独隐藏（admin 有成员管理但没有企业设置）
        ...(!isPersonal && can(PERM.ORG_SETTINGS)
          ? [
              {
                key: 'settings',
                label: '组织设置',
                children: <MenuVisibilityCard org={current} />,
              },
            ]
          : []),
        ...(!isPersonal && can(PERM.ORG_MEMBER_MGR)
          ? [
              {
                key: 'members',
                label: '成员管理',
                children: <MembersCard org={current} />,
              },
            ]
          : []),
        ...(!isPersonal && can(PERM.ORG_ROLE_MGR)
          ? [
              {
                key: 'roles',
                label: '角色管理',
                children: <OrgRolesCard orgId={current.id} />,
              },
            ]
          : []),
        ...(!isPersonal && isOrgAdmin
          ? [
              {
                key: 'llm',
                label: 'LLM 配置',
                children: <LLMConfigCard scope="org" orgId={current.id} />,
              },
            ]
          : []),
      ]
    : []

  return (
    <div style={{ maxWidth: 880, margin: '0 auto', paddingBottom: 40 }}>
      <PageHeader
        title="组织管理"
        subtitle="租户信息、成员、角色与菜单可见性（配额为预留字段，当前不做限额执行）"
        actions={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
            创建组织
          </Button>
        }
      />

      {isLoading ? (
        <Skeleton active paragraph={{ rows: 6 }} />
      ) : !current ? (
        <Empty description="没有可用租户" />
      ) : (
        <Tabs items={tabs} />
      )}

      <CreateOrgModal open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  )
}