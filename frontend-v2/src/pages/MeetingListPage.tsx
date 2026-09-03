import { Typography, Tag, Button, Card, Select, Empty, Space, Spin, Avatar } from 'antd'
import { PlusOutlined, TeamOutlined, ClockCircleOutlined, MessageOutlined, CheckCircleOutlined, PaperClipOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import type { ApiListResponse, Meeting, MeetingStatus, Project } from '@/types'
import dayjs from 'dayjs'
import { useNavigate } from 'react-router-dom'
import { CreateMeetingModal } from '@/components/meeting/CreateMeetingModal'
import { PageHeader } from '@/components/shared/PageHeader'

const { Text } = Typography

const statusConfig: Record<MeetingStatus, { label: string; color: string; borderColor: string; icon: React.ReactNode }> = {
  waiting: { label: '待开始', color: 'var(--warning)', borderColor: 'var(--warning)', icon: <ClockCircleOutlined /> },
  in_progress: { label: '进行中', color: 'var(--success)', borderColor: 'var(--success)', icon: <MessageOutlined /> },
  completed: { label: '已结束', color: 'var(--text-quaternary)', borderColor: 'var(--line-strong)', icon: <CheckCircleOutlined /> },
}

function formatRelativeTime(dateStr: string): string {
  const now = Date.now()
  const date = new Date(dateStr).getTime()
  const diff = now - date
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return '刚刚'
  if (mins < 60) return `${mins}分钟前`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}天前`
  return dayjs(dateStr).format('YYYY-MM-DD')
}

interface Props {
  projectId?: string
}

export function MeetingListPage({ projectId }: Props) {
  const navigate = useNavigate()
  const [selectedProject, setSelectedProject] = useState<string | undefined>(projectId)
  const [createOpen, setCreateOpen] = useState(false)
  const activeProjectId = projectId ?? selectedProject

  const { data: projects } = useQuery({
    queryKey: ['projects'],
    queryFn: async () => {
      const res = await apiClient.get('/api/v1/projects').json<ApiListResponse<Project>>()
      return res.data.items
    },
    enabled: !projectId,
  })

  const { data: meetings, isLoading } = useQuery({
    queryKey: ['meetings', activeProjectId],
    queryFn: async () => {
      const res = await apiClient.get(`/api/v1/projects/${activeProjectId}/meetings`).json<ApiListResponse<Meeting>>()
      return res.data.items
    },
    enabled: !!activeProjectId,
  })

  const groups: Record<MeetingStatus, Meeting[]> = { waiting: [], in_progress: [], completed: [] }
  ;(meetings ?? []).forEach((m) => {
    if (groups[m.status]) groups[m.status].push(m)
  })
  ;(['in_progress', 'waiting', 'completed'] as const).forEach((s) => {
    groups[s].sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  })

  const totalCount = meetings?.length ?? 0

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <PageHeader
        title="会议室"
        icon={<TeamOutlined />}
        subtitle={totalCount > 0 ? `共 ${totalCount} 场会议` : undefined}
        actions={
          <Button type="primary" icon={<PlusOutlined />} disabled={!activeProjectId} onClick={() => setCreateOpen(true)}>
            新建会议
          </Button>
        }
      />

      {!projectId && (
        <Space style={{ marginBottom: 16 }}>
          <span>选择项目：</span>
          <Select
            placeholder="请选择项目"
            style={{ width: 280 }}
            value={selectedProject}
            onChange={setSelectedProject}
            allowClear
            options={(projects || []).map((p) => ({ label: p.name, value: p.id }))}
          />
        </Space>
      )}

      <div style={{ flex: 1, overflow: 'auto' }}>
        {!activeProjectId ? (
          <Empty description={projects?.length ? '请选择项目查看会议' : '暂无项目，请先创建项目'} />
        ) : isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}><Spin /></div>
        ) : totalCount === 0 ? (
          <Empty description="暂无会议">
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>创建第一个会议</Button>
          </Empty>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
            {(['in_progress', 'waiting', 'completed'] as const).map((status) => {
              const list = groups[status]
              const cfg = statusConfig[status]
              if (list.length === 0) return null
              return (
                <div key={status}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                    <span style={{ color: cfg.color }}>{cfg.icon}</span>
                    <Text style={{ fontSize: 13, fontWeight: 500, color: 'var(--text-secondary)' }}>{cfg.label}</Text>
                    <Text type="secondary" style={{ fontSize: 12 }}>{list.length}</Text>
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    {list.map((m) => (
                      <Card
                        key={m.id}
                        size="small"
                        hoverable
                        bordered={false}
                        onClick={() => navigate(`/meetings/${m.id}`)}
                        style={{
                          background: 'var(--surface)',
                          borderLeft: `3px solid ${cfg.borderColor}`,
                          cursor: 'pointer',
                        }}
                        styles={{ body: { padding: '12px 16px' } }}
                      >
                        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
                            <Text strong style={{ color: 'var(--text-primary)', fontSize: 14 }} ellipsis>{m.title}</Text>
                            <Tag style={{ fontSize: 11, color: cfg.color, borderColor: cfg.color, background: 'transparent', margin: 0 }}>{cfg.label}</Tag>
                          </div>
                          <Text type="secondary" style={{ fontSize: 11, flexShrink: 0 }}>{formatRelativeTime(m.created_at)}</Text>
                        </div>
                        {m.agenda && (
                          <Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 4 }} ellipsis>{m.agenda}</Text>
                        )}
                        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginTop: 8, fontSize: 11, color: 'var(--text-tertiary)' }}>
                          <span><TeamOutlined style={{ fontSize: 12 }} /> {m.participants?.length ?? m.participant_count ?? 0} 位</span>
                          {m.attached_files && m.attached_files.length > 0 && (
                            <span><PaperClipOutlined style={{ fontSize: 12 }} /> {m.attached_files.length} 个文件</span>
                          )}
                          <span style={{ marginLeft: 'auto' }}>{m.todo_count ?? 0} 待办</span>
                        </div>
                        {(m.participants?.length ?? 0) > 0 && (
                          <div style={{ display: 'flex', alignItems: 'center', gap: -4, marginTop: 8 }}>
                            <Avatar.Group maxCount={5} size="small">
                              {m.participants.map((p) => (
                                <Avatar
                                  key={p.agent_id}
                                  size="small"
                                  style={{
                                    background: p.agent_id === m.host_agent_id
                                      ? 'linear-gradient(135deg, var(--warning), var(--error))'
                                      : 'linear-gradient(135deg, var(--info), var(--cyan))',
                                    color: 'var(--text-primary)',
                                    fontSize: 11,
                                  }}
                                >
                                  {p.agent_name?.slice(0, 1).toUpperCase() ?? '?'}
                                </Avatar>
                              ))}
                            </Avatar.Group>
                            {(m.participants?.length ?? 0) > 5 && (
                              <span style={{ fontSize: 10, color: 'var(--text-tertiary)', marginLeft: 6 }}>+{(m.participants?.length ?? 0) - 5}</span>
                            )}
                          </div>
                        )}
                      </Card>
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {activeProjectId && <CreateMeetingModal open={createOpen} onClose={() => setCreateOpen(false)} projectId={activeProjectId} />}
    </div>
  )
}