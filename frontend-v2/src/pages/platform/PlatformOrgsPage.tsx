import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  App,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { PageHeader } from '@/components/shared/PageHeader'
import {
  useCreatePlatformOrg,
  usePlatformOrgs,
  useSetPlatformOrgStatus,
} from '@/hooks/usePlatformAdmin'
import { ApiRequestError } from '@/types'
import type { PlatformOrgView } from '@/types'

const { Text } = Typography

const fmtDate = (s?: string) => (s ? new Date(s).toLocaleDateString() : '—')

function errMessage(e: unknown) {
  return e instanceof ApiRequestError ? e.message : '操作失败'
}

function CreateOrgModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { message } = App.useApp()
  const createOrg = useCreatePlatformOrg()
  const [form] = Form.useForm()

  const handleOk = async () => {
    const values = await form.validateFields()
    try {
      await createOrg.mutateAsync({
        name: values.name.trim(),
        slug: values.slug?.trim() || undefined,
        owner_email: values.owner_email.trim(),
      })
      message.success('组织已开通')
      form.resetFields()
      onClose()
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  return (
    <Modal
      title="代开组织"
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      okText="开通"
      confirmLoading={createOrg.isPending}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item name="name" label="组织名称" rules={[{ required: true, message: '请输入组织名称' }]}>
          <Input placeholder="例如：山雨影业" maxLength={64} />
        </Form.Item>
        <Form.Item name="slug" label="标识（可选）" extra="留空自动从名称生成">
          <Input placeholder="例如：shanyu-studio" maxLength={64} />
        </Form.Item>
        <Form.Item
          name="owner_email"
          label="Owner 账号邮箱"
          rules={[{ required: true, message: '请输入 Owner 邮箱' }]}
          extra="必须是平台上已注册的账号；组织租户恒有唯一 Owner"
        >
          <Input placeholder="owner@example.com" />
        </Form.Item>
      </Form>
    </Modal>
  )
}

/**
 * 平台管理 · 企业列表（设计文档 §3.2）：只看元数据与 count 级用量，读不到企业业务内容。
 *
 * 禁用 = 立即拦截：该企业成员带此租户头的请求返回 403 ORG_DISABLED；恢复即时生效。
 */
export function PlatformOrgsPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [keyword, setKeyword] = useState('')
  const [status, setStatus] = useState<string | undefined>(undefined)
  const [createOpen, setCreateOpen] = useState(false)

  const { data: orgs, isLoading, isFetching, refetch } = usePlatformOrgs({ keyword, status })
  const setStatusMutation = useSetPlatformOrgStatus()

  const handleToggle = async (org: PlatformOrgView) => {
    const disabled = org.status === 'disabled'
    try {
      await setStatusMutation.mutateAsync({ id: org.id, disabled: !disabled })
      message.success(disabled ? '组织已恢复' : '组织已禁用（该组织成员立即被拦截）')
    } catch (e) {
      message.error(errMessage(e))
    }
  }

  const columns = [
    {
      title: '组织',
      key: 'name',
      render: (_: unknown, o: PlatformOrgView) => (
        <Space direction="vertical" size={0}>
          <Space size={6}>
            <Text strong>{o.name}</Text>
            <Tag
              color={o.status === 'disabled' ? 'red' : 'green'}
              style={{ marginInlineEnd: 0, fontSize: 10, lineHeight: '16px' }}
            >
              {o.status === 'disabled' ? '已禁用' : '正常'}
            </Tag>
          </Space>
          <Text type="secondary" style={{ fontSize: 12 }}>{o.slug}</Text>
        </Space>
      ),
    },
    {
      title: 'Owner',
      key: 'owner',
      width: 200,
      render: (_: unknown, o: PlatformOrgView) => (
        <Space direction="vertical" size={0}>
          <Text style={{ fontSize: 13 }}>{o.owner_name || '—'}</Text>
          <Text type="secondary" style={{ fontSize: 12 }}>{o.owner_email || o.owner_id}</Text>
        </Space>
      ),
    },
    {
      title: '用量（成员/项目/任务）',
      key: 'usage',
      width: 170,
      render: (_: unknown, o: PlatformOrgView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>
          {o.member_count} / {o.project_count} / {o.task_count}
        </Text>
      ),
    },
    {
      title: '创建时间',
      key: 'created',
      width: 110,
      render: (_: unknown, o: PlatformOrgView) => (
        <Text type="secondary" style={{ fontSize: 12 }}>{fmtDate(o.created_at)}</Text>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      render: (_: unknown, o: PlatformOrgView) => (
        <Space size={8}>
          <Button size="small" onClick={() => navigate(`/platform/orgs/${o.id}`)}>
            详情
          </Button>
          <Popconfirm
            title={o.status === 'disabled' ? '恢复该组织？' : '禁用该组织？'}
            description={
              o.status === 'disabled'
                ? '恢复后其成员可立即正常访问。'
                : '禁用后该组织成员的请求一律 403，恢复前无法使用。'
            }
            okText="确认"
            cancelText="取消"
            onConfirm={() => void handleToggle(o)}
          >
            <Button size="small" danger={o.status !== 'disabled'} loading={setStatusMutation.isPending}>
              {o.status === 'disabled' ? '恢复' : '禁用'}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div style={{ paddingBottom: 40 }}>
      <PageHeader
        title="平台管理 · 组织"
        subtitle="组织生命周期（元数据 / 开通 / 禁用 / 恢复）。平台管理员不接入组织业务数据"
        actions={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => void refetch()} loading={isFetching}>
              刷新
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              代开组织
            </Button>
          </Space>
        }
      />

      <Card style={{ background: 'var(--surface)', border: '1px solid var(--line)' }}>
        <Space style={{ marginBottom: 12 }} wrap>
          <Input.Search
            placeholder="按名称/Slug/Owner 邮箱搜索"
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
        <Table<PlatformOrgView>
          rowKey="id"
          size="small"
          loading={isLoading}
          dataSource={orgs ?? []}
          columns={columns as never}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
        />
      </Card>

      <CreateOrgModal open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  )
}