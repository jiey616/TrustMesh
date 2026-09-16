import { useState } from 'react'
import {
  App,
  Button,
  Card,
  Input,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import {
  usePlatformUsers,
  useResetPlatformUserPassword,
  useSetPlatformUserDisabled,
} from '@/hooks/usePlatformAdmin'
import { ApiRequestError } from '@/types'
import type { PlatformUserView } from '@/types'

const { Text, Paragraph } = Typography

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleString() : '—')

function errMessage(e: unknown) {
  return e instanceof ApiRequestError ? e.message : '操作失败'
}

/** 所属企业：平台视角只关心企业归属（个人空间不展示，元数据已足够）。 */
function orgLabel(u: PlatformUserView) {
  const names = (u.orgs ?? [])
    .filter((o) => o.kind === 'enterprise')
    .map((o) => o.role_name || o.name)
  return names.length > 0 ? names.join('、') : '—'
}

/**
 * 平台管理 · 用户（账号运维）：
 * 列表/搜索 → 重置密码（临时密码只显示一次）→ 禁用/启用（登录与 refresh 一律拒绝）。
 *
 * 平台管理员账号（is_platform_admin）不可在此禁用：其账号集合以部署 env 为唯一权威。
 */
export function PlatformUsersPage() {
  const { message, modal } = App.useApp()
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState<string | undefined>(undefined)

  const { data: users, isLoading, isFetching, refetch } = usePlatformUsers({ keyword, status })
  const setDisabled = useSetPlatformUserDisabled()
  const resetPassword = useResetPlatformUserPassword()

  const handleReset = async (u: PlatformUserView) => {
    try {
      const res = await resetPassword.mutateAsync(u.id)
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

  const handleToggle = async (u: PlatformUserView) => {
    try {
      await setDisabled.mutateAsync({ id: u.id, disabled: !u.disabled })
      message.success(u.disabled ? '账号已启用' : '账号已禁用（登录与刷新令牌一律被拒）')
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  const columns = [
    {
      title: '账号',
      key: 'account',
      render: (_: unknown, u: PlatformUserView) => (
        <Space size={6}>
          <Space direction="vertical" size={0}>
            <Text strong>{u.email}</Text>
            <Text type="secondary" style={{ fontSize: 12 }}>{u.name}</Text>
          </Space>
          {u.is_platform_admin && (
            <Tooltip title="平台管理员账号：由部署环境变量 PLATFORM_ADMIN_EMAILS 决定，控制台不可禁用">
              <Tag color="gold" style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}>
                平台管理员
              </Tag>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: '状态',
      key: 'status',
      width: 110,
      render: (_: unknown, u: PlatformUserView) => (
        <Tag
          color={u.disabled ? 'red' : 'green'}
          style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}
        >
          {u.disabled ? '已禁用' : '正常'}
        </Tag>
      ),
    },
    {
      title: '所属组织',
      key: 'orgs',
      width: 180,
      render: (_: unknown, u: PlatformUserView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>{orgLabel(u)}</Text>
      ),
    },
    {
      title: '注册时间',
      key: 'created',
      width: 170,
      render: (_: unknown, u: PlatformUserView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {fmtDate(u.created_at)}
          {u.disabled_at && (
            <>
              <br />
              禁用：{fmtDate(u.disabled_at)}
            </>
          )}
        </Text>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 180,
      render: (_: unknown, u: PlatformUserView) => (
        <Space size={8}>
          <Popconfirm
            title="重置该账号密码？"
            description="系统会生成一个随机临时密码，旧密码立即失效。临时密码只显示一次。"
            okText="确认"
            cancelText="取消"
            onConfirm={() => void handleReset(u)}
          >
            <Button size="small" loading={resetPassword.isPending}>
              重置密码
            </Button>
          </Popconfirm>
          {u.is_platform_admin ? (
            <Tooltip title="平台管理员账号不可禁用">
              <Button size="small" disabled>
                禁用
              </Button>
            </Tooltip>
          ) : (
            <Popconfirm
              title={u.disabled ? '启用该账号？' : '禁用该账号？'}
              description={
                u.disabled
                  ? '启用后可立即正常登录。'
                  : '禁用后登录与刷新令牌一律被拒；已签发的访问令牌会在 15 分钟内自然过期。'
              }
              okText="确认"
              cancelText="取消"
              onConfirm={() => void handleToggle(u)}
            >
              <Button size="small" danger={!u.disabled} loading={setDisabled.isPending}>
                {u.disabled ? '启用' : '禁用'}
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 用户"
        subtitle="账号运维：重置密码、禁用 / 启用。平台管理员只碰账号元数据，不读组织业务内容"
        actions={
          <Button icon={<ReloadOutlined />} onClick={() => void refetch()} loading={isFetching}>
            刷新
          </Button>
        }
      />

      <Card style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}>
        <Space style={{ marginBottom: 12 }} wrap>
          <Input.Search
            placeholder="按邮箱 / 姓名搜索"
            allowClear
            style={{ width: 280 }}
            onSearch={(v) => setKeyword(v.trim())}
          />
          <Select
            placeholder="状态"
            allowClear
            style={{ width: 120 }}
            value={status}
            onChange={(v) => setStatus(v)}
            options={[
              { value: 'active', label: '正常' },
              { value: 'disabled', label: '已禁用' },
            ]}
          />
        </Space>
        <Table<PlatformUserView>
          rowKey="id"
          size="small"
          loading={isLoading}
          dataSource={users ?? []}
          columns={columns as never}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
        />
      </Card>
    </div>
  )
}