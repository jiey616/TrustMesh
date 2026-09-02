import { Row, Col, Card, Tag, Typography, Space, Input, Select, Empty, Button } from 'antd'
import { SearchOutlined, RobotOutlined, EnvironmentOutlined, ClockCircleOutlined, UserAddOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { PageHeader } from '@/components/shared/PageHeader'
import type { ApiListResponse, Agent, AgentStatus, AgentRole } from '@/types'
import dayjs from 'dayjs'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'

const { Text, Paragraph } = Typography

const roleMap: Record<AgentRole, { color: string; label: string }> = {
  pm: { color: 'purple', label: 'PM' },
  developer: { color: 'blue', label: '开发' },
  reviewer: { color: 'green', label: '审核' },
  custom: { color: 'default', label: '自定义' },
}

const statusMap: Record<AgentStatus, { color: string; label: string }> = {
  online: { color: 'success', label: '在线' },
  offline: { color: 'default', label: '离线' },
  busy: { color: 'warning', label: '忙碌' },
}

export function AgentListPage() {
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState<string>('all')
  const navigate = useNavigate()

  const { data: agents, isLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: async () => {
      const res = await apiClient.get('/api/v1/agents').json<ApiListResponse<Agent>>()
      return res.data.items
    },
  })

  const filtered = (agents || []).filter((a) => {
    if (search && !a.name.includes(search) && !a.description.includes(search)) return false
    if (roleFilter !== 'all' && a.role !== roleFilter) return false
    return true
  })

  return (
    <>
      <PageHeader
        title="数字员工列表"
        icon={<RobotOutlined />}
        actions={
          <Button type="primary" icon={<UserAddOutlined />} onClick={() => navigate('/agent-invite')}>
            招聘数字员工
          </Button>
        }
      />
      <Space style={{ marginBottom: 16 }}>
        <Input
          prefix={<SearchOutlined />}
          placeholder="搜索数字员工..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: 240 }}
        />
        <Select
          value={roleFilter}
          onChange={setRoleFilter}
          style={{ width: 120 }}
          options={[
            { label: '全部角色', value: 'all' },
            { label: 'PM', value: 'pm' },
            { label: '开发', value: 'developer' },
            { label: '审核', value: 'reviewer' },
          ]}
        />
      </Space>

      {filtered.length === 0 && !isLoading ? (
        <Empty description="没有找到数字员工" />
      ) : (
        <Row gutter={[16, 16]}>
          {filtered.map((agent) => (
            <Col xs={24} sm={12} lg={8} xl={6} key={agent.id} style={{ display: 'flex' }}>
              <Card
                hoverable
                loading={isLoading}
                onClick={() => navigate(`/agents/${agent.id}`)}
                style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column' }}
                styles={{ body: { flex: 1, display: 'flex', flexDirection: 'column' } }}
                actions={[
                  <Tag key="role" color={roleMap[agent.role]?.color}>{roleMap[agent.role]?.label}</Tag>,
                  <Tag key="status" color={statusMap[agent.status]?.color}>{statusMap[agent.status]?.label}</Tag>,
                ]}
              >
                <Card.Meta
                  avatar={
                    <AgentAvatar
                      name={agent.name}
                      role={agent.role}
                      seed={agent.id}
                      size={48}
                      status={agent.status}
                    />
                  }
                  title={agent.name}
                  description={
                    <Space direction="vertical" size={4} style={{ width: '100%', flex: 1, display: 'flex', flexDirection: 'column' }}>
                      <Paragraph type="secondary" ellipsis={{ rows: 2 }} style={{ marginBottom: 0, flex: 1 }}>{agent.description || '暂无描述'}</Paragraph>
                      <Text type="secondary" style={{ fontSize: 12, fontFamily: "'JetBrains Mono', monospace" }}>
                        <EnvironmentOutlined /> {agent.node_id?.slice(0, 12)}...
                      </Text>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        <ClockCircleOutlined /> {agent.last_seen_at ? dayjs(agent.last_seen_at).format('MM-DD HH:mm') : '从未在线'}
                      </Text>
                    </Space>
                  }
                />
              </Card>
            </Col>
          ))}
        </Row>
      )}
    </>
  )
}