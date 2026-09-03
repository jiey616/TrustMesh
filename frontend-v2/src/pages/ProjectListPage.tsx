import { useNavigate } from 'react-router-dom'
import { Card, Table, Button, Tag, Typography, Space, Modal, Form, Input, Select, Empty, App, Drawer } from 'antd'
import { PlusOutlined, ProjectOutlined, ExportOutlined, InboxOutlined, SearchOutlined } from '@ant-design/icons'
import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { useAgents } from '@/hooks/useAgents'
import { useWorkflowTemplates } from '@/hooks/useWorkflows'
import { PageHeader } from '@/components/shared/PageHeader'
import type { ApiListResponse, Project, ProjectWorkStatus, AgentStatus } from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography

const workStatusMap: Record<ProjectWorkStatus, { color: string; label: string }> = {
  empty: { color: 'default', label: '空' },
  idle: { color: 'default', label: '空闲' },
  queued: { color: 'processing', label: '排队中' },
  running: { color: 'success', label: '运行中' },
  attention: { color: 'warning', label: '需关注' },
  archived: { color: 'default', label: '归档' },
}

const pmStatusColor: Record<AgentStatus, string> = {
  online: 'var(--success)',
  busy: 'var(--warning)',
  offline: 'var(--text-quaternary)',
}

export function ProjectListPage() {
  const [open, setOpen] = useState(false)
  const [archivedOpen, setArchivedOpen] = useState(false)
  const [archivedKeyword, setArchivedKeyword] = useState('')
  const [form] = Form.useForm()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { message } = App.useApp()

  // 进入项目：从抽屉里点进去时顺手关掉抽屉，避免返回后还挂着一层。
  const goProject = (id: string) => {
    setArchivedOpen(false)
    navigate(`/projects/${id}`)
  }

  const { data: projects, isLoading } = useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const res = await apiClient.get('/api/v1/projects').json<ApiListResponse<Project>>()
      return res.data.items
    },
  })

  const { data: agents } = useAgents()
  const pmAgents = agents?.filter((a) => a.role === 'pm') || []
  const { data: templates } = useWorkflowTemplates()

  const createMutation = useMutation({
    mutationFn: (data: { name: string; description: string; pm_agent_id: string; template_id?: string }) =>
      apiClient.post('/api/v1/projects', { json: data }).json<{ data: Project }>(),
    onSuccess: () => {
      message.success('项目创建成功')
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      setOpen(false)
      form.resetFields()
    },
    onError: () => message.error('创建失败'),
  })

  const columns = [
    {
      title: '项目名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: Project) => (
        <a onClick={() => goProject(record.id)} style={{ fontWeight: 500 }}>{name}</a>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (desc: string) => (
        <Text type="secondary" ellipsis style={{ maxWidth: 240 }} title={desc}>
          {desc || '—'}
        </Text>
      ),
    },
    {
      title: 'PM 数字员工',
      key: 'pm_agent',
      render: (_: unknown, record: Project) => (
        <Space size={6}>
          <span
            style={{ width: 8, height: 8, borderRadius: 'var(--radius-avatar)', background: pmStatusColor[record.pm_agent.status] || '#6b7280', display: 'inline-block' }}
            title={record.pm_agent.status}
          />
          <span>{record.pm_agent.name}</span>
        </Space>
      ),
    },
    {
      title: '任务',
      key: 'tasks',
      render: (_: unknown, record: Project) => {
        const ts = record.task_summary
        const total = ts.task_total
        const inProgress = ts.in_progress_count
        const pending = ts.pending_count
        const failed = ts.failed_count
        const canceled = ts.canceled_count
        return (
          <Space size={4} wrap>
            <span style={{ color: 'var(--text-secondary)' }}>{total} 个</span>
            {inProgress > 0 && <Tag color="cyan">{inProgress} 执行中</Tag>}
            {pending > 0 && <Tag color="blue">{pending} 待处理</Tag>}
            {failed > 0 && <Tag color="red">{failed} 失败</Tag>}
            {canceled > 0 && <Tag color="default">{canceled} 已取消</Tag>}
          </Space>
        )
      },
    },
    {
      title: '状态',
      key: 'work_status',
      render: (_: unknown, record: Project) => {
        const status = record.status === 'archived' ? 'archived' : record.task_summary.work_status
        return <Tag color={workStatusMap[status]?.color}>{workStatusMap[status]?.label}</Tag>
      },
    },
    {
      title: '更新于',
      dataIndex: 'updated_at',
      key: 'updated_at',
      render: (val: string) => (
        <Text type="secondary" style={{ fontSize: 12 }} title={dayjs(val).format('YYYY-MM-DD HH:mm:ss')}>
          {dayjs(val).fromNow()}
        </Text>
      ),
    },
    {
      title: '最近任务',
      key: 'latest_task_at',
      render: (_: unknown, record: Project) => {
        const latest = record.task_summary.latest_task_at
        if (!latest) return <Text type="secondary" style={{ fontSize: 12 }}>—</Text>
        return (
          <Text type="secondary" style={{ fontSize: 12 }} title={dayjs(latest).format('YYYY-MM-DD HH:mm:ss')}>
            {dayjs(latest).fromNow()}
          </Text>
        )
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_: unknown, record: Project) => (
        <Button type="link" size="small" icon={<ExportOutlined />} onClick={() => goProject(record.id)}>
          进入
        </Button>
      ),
    },
  ]

  const { activeProjects, archivedProjects } = useMemo(() => {
    const all = projects || []
    return {
      activeProjects: all.filter((p) => p.status !== 'archived'),
      archivedProjects: all.filter((p) => p.status === 'archived'),
    }
  }, [projects])

  // 抽屉内的关键字过滤（名称 / 描述，忽略大小写）
  const filteredArchived = useMemo(() => {
    const kw = archivedKeyword.trim().toLowerCase()
    if (!kw) return archivedProjects
    return archivedProjects.filter(
      (p) =>
        (p.name ?? '').toLowerCase().includes(kw) || (p.description ?? '').toLowerCase().includes(kw),
    )
  }, [archivedProjects, archivedKeyword])

  return (
    <>
      <PageHeader
        title="项目列表"
        icon={<ProjectOutlined />}
        actions={
          <>
            <Button icon={<InboxOutlined />} onClick={() => setArchivedOpen(true)}>
              已归档项目{archivedProjects.length > 0 ? ` (${archivedProjects.length})` : ''}
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
              新建项目
            </Button>
          </>
        }
      />

      {!isLoading && (projects || []).length === 0 ? (
        <Card>
          <Empty description="还没有项目，点击右上角新建第一个项目" />
        </Card>
      ) : (
        <Card>
          <Table
            columns={columns}
            dataSource={activeProjects}
            rowKey="id"
            loading={isLoading}
            pagination={false}
            locale={{
              emptyText: (
                <Empty
                  description={
                    archivedProjects.length > 0
                      ? '当前没有活跃项目，已归档项目请从右上角「已归档项目」查看'
                      : '暂无活跃项目'
                  }
                />
              ),
            }}
          />
        </Card>
      )}

      <Drawer
        title="已归档项目"
        open={archivedOpen}
        onClose={() => {
          setArchivedOpen(false)
          setArchivedKeyword('')
        }}
        width={860}
        styles={{ body: { paddingTop: 12 } }}
        extra={
          <Text type="secondary" style={{ fontSize: 12 }}>
            共 {archivedProjects.length} 个
          </Text>
        }
      >
        {archivedProjects.length === 0 ? (
          <div style={{ padding: '64px 0' }}>
            <Empty description="还没有已归档的项目" />
          </div>
        ) : (
          <>
            <Input
              allowClear
              prefix={<SearchOutlined style={{ color: 'var(--text-quaternary)' }} />}
              placeholder="搜索项目名称或描述"
              value={archivedKeyword}
              onChange={(e) => setArchivedKeyword(e.target.value)}
              style={{ marginBottom: 12, maxWidth: 320 }}
            />
            <Table
              columns={columns}
              dataSource={filteredArchived}
              rowKey="id"
              size="small"
              loading={isLoading}
              pagination={
                filteredArchived.length > 10
                  ? { pageSize: 10, showTotal: (total) => `共 ${total} 个` }
                  : false
              }
              locale={{ emptyText: <Empty description="没有匹配的归档项目" /> }}
            />
          </>
        )}
      </Drawer>

      <Modal title="新建项目" open={open} onOk={form.submit} onCancel={() => setOpen(false)} confirmLoading={createMutation.isPending}>
        <Form form={form} layout="vertical" onFinish={(values) => createMutation.mutate(values)}>
          <Form.Item name="name" label="项目名称" rules={[{ required: true }]}>
            <Input placeholder="输入项目名称" />
          </Form.Item>
          <Form.Item name="description" label="项目描述">
            <Input.TextArea rows={3} placeholder="输入项目描述" />
          </Form.Item>
          <Form.Item name="pm_agent_id" label="PM 数字员工" rules={[{ required: true, message: '请选择 PM 数字员工' }]}>
            <Select placeholder="选择 PM 数字员工" options={pmAgents.map((a) => ({ label: a.name, value: a.id }))} />
          </Form.Item>
          <Form.Item name="template_id" label="继承全局工作流模板（可选）" extra="创建后可在项目内二次修改，后续可手动同步模板更新">
            <Select
              allowClear
              placeholder="不继承"
              options={(templates ?? []).map((t) => ({
                label: `${t.name}（v${t.version} · ${t.steps.length} 步）`,
                value: t.id,
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
