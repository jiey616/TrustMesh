import { useState } from 'react'
import {
  App,
  Button,
  Card,
  Checkbox,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { EditOutlined, LockOutlined, PlusOutlined, SafetyOutlined } from '@ant-design/icons'
import {
  useCreateOrgRole,
  useDeleteOrgRole,
  useOrgRoles,
  useUpdateOrgRole,
} from '@/hooks/useOrgRoles'
import { PERM, PERM_GROUPS, PERM_LABELS } from '@/lib/perms'
import { ApiRequestError } from '@/types'
import type { OrgRoleView } from '@/types'

const { Text, Paragraph } = Typography

/** 自定义角色不可勾选的权限点：企业设置与角色管理是 owner 专属（设计文档 §5）。 */
const OWNER_ONLY_PERMS: readonly string[] = [PERM.ORG_SETTINGS, PERM.ORG_ROLE_MGR]

function errMessage(e: unknown) {
  if (!(e instanceof ApiRequestError)) return '操作失败'
  if (e.code === 'BUILTIN_ROLE_LOCKED') return '内置角色的权限集已锁定，如需差异化请新建自定义角色'
  if (e.code === 'ROLE_NAME_TAKEN') return '角色名称已被占用'
  if (e.code === 'ROLE_IN_USE') {
    const n = e.details?.member_count
    return `该角色仍有 ${typeof n === 'number' ? n : '若干'} 名成员，请先改派后再删除`
  }
  if (e.code === 'VALIDATION_ERROR' && e.details?.permission) {
    return `权限点不可授予：${String(e.details.permission)}`
  }
  return e.message
}

function RolePermTags({ role }: { role: OrgRoleView }) {
  return (
    <Space size={[4, 4]} wrap>
      {role.permissions.length === 0 ? (
        <Text type="secondary" style={{ fontSize: 12 }}>无管理权限（仅基础功能）</Text>
      ) : (
        role.permissions.map((p) => (
          <Tag key={p} style={{ marginInlineEnd: 0, fontSize: 11 }}>
            {PERM_LABELS[p] ?? p}
          </Tag>
        ))
      )}
    </Space>
  )
}

function RoleEditor({
  orgId,
  role,
  open,
  onClose,
}: {
  orgId: string
  /** 缺省 = 新建 */
  role?: OrgRoleView
  open: boolean
  onClose: () => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const createRole = useCreateOrgRole(orgId)
  const updateRole = useUpdateOrgRole(orgId)
  const isEdit = !!role
  const pending = createRole.isPending || updateRole.isPending

  const handleOk = async () => {
    const values = await form.validateFields()
    const payload = {
      name: values.name.trim(),
      permissions: (values.permissions ?? []) as string[],
    }
    try {
      if (isEdit) {
        await updateRole.mutateAsync({ roleId: role.id, input: payload })
        message.success('角色已更新')
      } else {
        await createRole.mutateAsync(payload)
        message.success('角色已创建')
      }
      onClose()
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  return (
    <Modal
      title={isEdit ? `编辑角色：${role?.name}` : '新建自定义角色'}
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      okText={isEdit ? '保存' : '创建'}
      confirmLoading={pending}
      width={640}
      destroyOnHidden
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          name: role?.name ?? '',
          permissions: role?.permissions ?? [PERM.AGENT_VIEW, PERM.MARKET_BROWSE],
        }}
      >
        <Form.Item
          name="name"
          label="角色名称"
          rules={[{ required: true, message: '请输入角色名称' }]}
        >
          <Input placeholder="例如：项目执行人" maxLength={32} />
        </Form.Item>
        <Form.Item
          name="permissions"
          label="权限点（只能勾选内置 Admin 全集的子集）"
          extra="组织设置与角色管理为 Owner 专属，不可授予自定义角色；菜单是否可见还会再受「组织级菜单可见性」缩小。"
        >
          <Checkbox.Group style={{ width: '100%' }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {PERM_GROUPS.map((g) => (
                <div key={g.group}>
                  <Text type="secondary" style={{ fontSize: 12 }}>{g.group}</Text>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 2, marginTop: 4 }}>
                    {g.items.map((i) => {
                      const locked = OWNER_ONLY_PERMS.includes(i.perm)
                      return (
                        <Checkbox key={i.perm} value={i.perm} disabled={locked}>
                          {i.label}
                          {locked && (
                            <Tooltip title="仅内置 Owner 角色拥有，不可授予自定义角色">
                              <LockOutlined style={{ marginLeft: 6, fontSize: 11, color: 'var(--text-quaternary)' }} />
                            </Tooltip>
                          )}
                        </Checkbox>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
          </Checkbox.Group>
        </Form.Item>
      </Form>
    </Modal>
  )
}

/**
 * 角色管理（设计文档 §5）：内置三角色只读锁定，自定义角色可增删改。
 *
 * 勾选上限 = Admin 全集，且不含 Owner 专属权限点；越界由后端二次拒绝
 * （422 VALIDATION_ERROR / 409 BUILTIN_ROLE_LOCKED），前端不做唯一仲裁。
 */
export function OrgRolesCard({ orgId }: { orgId: string }) {
  const { data: roles, isLoading } = useOrgRoles(orgId)
  const deleteRole = useDeleteOrgRole(orgId)
  const { message } = App.useApp()
  const [editing, setEditing] = useState<OrgRoleView | undefined>(undefined)
  const [editorOpen, setEditorOpen] = useState(false)

  const openEditor = (role?: OrgRoleView) => {
    setEditing(role)
    setEditorOpen(true)
  }

  const columns = [
    {
      title: '角色',
      key: 'name',
      width: 200,
      render: (_: unknown, r: OrgRoleView) => (
        <Space size={6}>
          <Text strong>{r.name}</Text>
          <Tag
            color={r.builtin ? 'default' : 'purple'}
            style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}
          >
            {r.builtin ? '内置' : '自定义'}
          </Tag>
        </Space>
      ),
    },
    {
      title: '权限点',
      key: 'permissions',
      render: (_: unknown, r: OrgRoleView) => <RolePermTags role={r} />,
    },
    {
      title: '成员数',
      key: 'members',
      width: 80,
      render: (_: unknown, r: OrgRoleView) => (
        <Text type="secondary">{r.member_count}</Text>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 140,
      render: (_: unknown, r: OrgRoleView) =>
        r.builtin ? (
          <Tooltip title="内置角色权限集锁定，不可修改或删除">
            <Text type="secondary" style={{ fontSize: 12 }}>
              <LockOutlined style={{ marginRight: 4 }} />
              已锁定
            </Text>
          </Tooltip>
        ) : (
          <Space size={4}>
            <Button size="small" icon={<EditOutlined />} onClick={() => openEditor(r)}>
              编辑
            </Button>
            <Popconfirm
              title={`删除角色「${r.name}」？`}
              description="仍有成员使用该角色时不可删除。"
              okText="删除"
              cancelText="取消"
              onConfirm={async () => {
                try {
                  await deleteRole.mutateAsync(r.id)
                  message.success('角色已删除')
                } catch (e) {
                  message.error(errMessage(e))
                }
              }}
            >
              <Button size="small" danger disabled={deleteRole.isPending}>
                删除
              </Button>
            </Popconfirm>
          </Space>
        ),
    },
  ]

  return (
    <Card
      title={
        <span>
          <SafetyOutlined style={{ marginRight: 8 }} />
          角色管理
        </span>
      }
      extra={
        <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => openEditor()}>
          新建角色
        </Button>
      }
      style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}
    >
      <Table<OrgRoleView>
        rowKey="id"
        size="small"
        loading={isLoading}
        dataSource={roles ?? []}
        columns={columns as never}
        pagination={false}
        locale={{ emptyText: <Empty description="暂无角色" /> }}
      />
      <Paragraph type="secondary" style={{ fontSize: 12, marginTop: 12, marginBottom: 0 }}>
        内置角色（Owner / Admin / 成员）权限集锁定不可改；需要差异化权限请新建自定义角色。
        自定义角色最多拥有内置 Admin 的全集，且不能授予「组织设置」「角色管理」。
      </Paragraph>

      {editorOpen && (
        <RoleEditor
          orgId={orgId}
          role={editing}
          open={editorOpen}
          onClose={() => setEditorOpen(false)}
        />
      )}
    </Card>
  )
}