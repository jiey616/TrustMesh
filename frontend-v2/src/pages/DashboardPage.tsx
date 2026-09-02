import { Row, Col, Space, Typography } from 'antd'
import {
  RobotOutlined,
  CheckCircleOutlined,
  ThunderboltOutlined,
  ClockCircleOutlined,
  TrophyOutlined,
  DashboardOutlined,
  RightOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { motion } from 'framer-motion'
import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'
import { CountUp } from '@/components/CountUp'
import { GlowCard } from '@/components/GlowCard'
import { NeonBadge } from '@/components/NeonBadge'
import { PageHeader } from '@/components/shared/PageHeader'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { NodeStatusIndicator } from '@/components/dashboard/NodeStatusIndicator'
import { EventTimeline } from '@/components/dashboard/EventTimeline'
import { RecentTasksList } from '@/components/dashboard/RecentTasksList'
import { useDashboardStats, useDashboardEvents, useDashboardTasks } from '@/hooks/useDashboard'
import type { ApiListResponse, Agent } from '@/types'

const { Text } = Typography

interface StatCard {
  title: string
  value: number
  suffix?: string
  icon: React.ReactNode
  glowColor: string
  delay: number
}

export function DashboardPage() {
  const navigate = useNavigate()
  const { data: stats, isLoading: statsLoading } = useDashboardStats()
  const { data: events, isLoading: eventsLoading } = useDashboardEvents(10)
  const { data: recentTasks, isLoading: tasksLoading } = useDashboardTasks(10)

  const { data: agents } = useQuery({
    queryKey: ['agents'],
    queryFn: async () => {
      const res = await apiClient.get('/api/v1/agents').json<ApiListResponse<Agent>>()
      return res.data.items
    },
  })

  const agentList = agents ?? []
  const onlineCount = agentList.filter((a) => a.status === 'online').length

  const successRate = stats?.success_rate ?? 0

  const statCards: StatCard[] = [
    { title: '数字员工总数', value: stats?.agents_total || 0, icon: <RobotOutlined style={{ fontSize: 20 }} />, glowColor: '#6d5ff5', delay: 0 },
    { title: '在线数字员工', value: stats?.agents_online || 0, icon: <CheckCircleOutlined style={{ fontSize: 20 }} />, glowColor: '#22d3ee', delay: 0.05 },
    { title: '任务总数', value: stats?.tasks_total || 0, icon: <ThunderboltOutlined style={{ fontSize: 20 }} />, glowColor: '#f59e0b', delay: 0.1 },
    { title: '进行中', value: stats?.tasks_in_progress || 0, icon: <ClockCircleOutlined style={{ fontSize: 20 }} />, glowColor: '#3b82f6', delay: 0.15 },
    { title: '已完成', value: stats?.tasks_done_count || 0, icon: <CheckCircleOutlined style={{ fontSize: 20 }} />, glowColor: '#10b981', delay: 0.2 },
    { title: '成功率', value: successRate, suffix: successRate > 0 ? '%' : '', icon: <TrophyOutlined style={{ fontSize: 20 }} />, glowColor: '#f43f5e', delay: 0.25 },
  ]

  return (
    <div style={{ height: 'calc(100vh - 48px)', display: 'flex', flexDirection: 'column', minHeight: 0, overflow: 'hidden' }}>
      {/* ===== 统计卡（固定，不随滚动） ===== */}
      <div
        style={{
          flexShrink: 0,
          paddingTop: 8,
          paddingBottom: 4,
          background: 'rgba(8,8,18,0.98)',
          borderBottom: '1px solid rgba(255,255,255,0.06)',
        }}
      >
        <PageHeader
          title="仪表盘"
          icon={<DashboardOutlined />}
          subtitle="实时系统概览 · 多数字员工协作状态"
          actions={<NodeStatusIndicator />}
          marginBottom={10}
        />

        <Row gutter={[16, 16]}>
          {statCards.map((card) => (
            <Col xs={12} sm={8} lg={4} key={card.title}>
              <motion.div
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.4, delay: card.delay, ease: 'easeOut' }}
              >
                <GlowCard glowColor={card.glowColor} intensity="subtle">
                  <div style={{ padding: '8px 16px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                      <Text style={{ color: 'rgba(255,255,255,0.45)', fontSize: 12, fontWeight: 500 }}>{card.title}</Text>
                      <span style={{ color: card.glowColor, opacity: 0.8, fontSize: 16 }}>{card.icon}</span>
                    </div>
                    <div>
                      {statsLoading ? (
                        <div style={{ height: 26, width: '60%', borderRadius: 6, background: 'rgba(255,255,255,0.04)' }} />
                      ) : card.title === '成功率' && successRate <= 0 ? (
                        <span style={{ fontSize: 22, fontWeight: 700, color: '#f4f4f8', lineHeight: 1, letterSpacing: '-1px' }}>—</span>
                      ) : (
                        <span style={{ display: 'inline-flex', alignItems: 'baseline' }}>
                          <CountUp
                            value={card.value}
                            style={{ fontSize: 22, fontWeight: 700, color: '#f4f4f8', lineHeight: 1, letterSpacing: '-1px' }}
                          />
                          {card.suffix && (
                            <span style={{ fontSize: 14, fontWeight: 600, color: '#f4f4f8', marginLeft: 2 }}>{card.suffix}</span>
                          )}
                        </span>
                      )}
                    </div>
                  </div>
                </GlowCard>
              </motion.div>
            </Col>
          ))}
        </Row>
      </div>

      {/* ===== 数字员工概览条（入口化，不占大块空间） ===== */}
      <div
        onClick={() => navigate('/agents')}
        style={{
          flexShrink: 0,
          marginTop: 14,
          display: 'flex',
          alignItems: 'center',
          gap: 12,
          padding: '10px 16px',
          borderRadius: 12,
          background: 'rgba(255,255,255,0.03)',
          border: '1px solid rgba(255,255,255,0.07)',
          cursor: 'pointer',
          transition: 'border-color 0.15s',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.borderColor = 'rgba(158,76,255,0.4)')}
        onMouseLeave={(e) => (e.currentTarget.style.borderColor = 'rgba(255,255,255,0.07)')}
      >
        <RobotOutlined style={{ color: '#9e4cff', fontSize: 16 }} />
        <Text style={{ color: '#f4f4f8', fontWeight: 600, fontSize: 14 }}>数字员工</Text>
        <NeonBadge label={`${onlineCount}/${agentList.length} 在线`} variant="green" pulse size="sm" />
        <span style={{ flex: 1 }} />
        <Space size={-2}>
          {agentList.slice(0, 6).map((a) => (
            <AgentAvatar key={a.id} name={a.name} role={a.role} seed={a.node_id} size={26} />
          ))}
        </Space>
        <Text type="secondary" style={{ fontSize: 12, marginLeft: 8, whiteSpace: 'nowrap' }}>
          查看全部 <RightOutlined style={{ fontSize: 10 }} />
        </Text>
      </div>

      {/* ===== 最近活动 + 最近任务（占满剩余空间） ===== */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', marginTop: 14, paddingBottom: 12 }}>
        <Row gutter={[16, 16]}>
          <Col xs={24} lg={12}>
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.4, delay: 0.3, ease: 'easeOut' }}
            >
              <GlassPanel title="最近活动" icon={<ThunderboltOutlined style={{ color: '#6d5ff5', fontSize: 16 }} />}>
                <EventTimeline events={events ?? []} loading={eventsLoading} showActorName emptyText="暂无活动记录" />
              </GlassPanel>
            </motion.div>
          </Col>
          <Col xs={24} lg={12}>
            <motion.div
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.4, delay: 0.35, ease: 'easeOut' }}
            >
              <GlassPanel title="最近任务" icon={<CheckCircleOutlined style={{ color: '#22d3ee', fontSize: 16 }} />}>
                <RecentTasksList tasks={recentTasks ?? []} loading={tasksLoading} />
              </GlassPanel>
            </motion.div>
          </Col>
        </Row>
      </div>
    </div>
  )
}

function GlassPanel({ title, icon, children }: { title: string; icon?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div
      style={{
        background: 'rgba(255,255,255,0.03)',
        backdropFilter: 'blur(24px)',
        border: '1px solid rgba(255,255,255,0.07)',
        borderRadius: 16,
        overflow: 'hidden',
        height: '100%',
      }}
    >
      <div
        style={{
          padding: '12px 20px',
          borderBottom: '1px solid rgba(255,255,255,0.06)',
          display: 'flex',
          alignItems: 'center',
          flexShrink: 0,
        }}
      >
        <Space>
          {icon}
          <Text style={{ color: '#f4f4f8', fontWeight: 600, fontSize: 15 }}>{title}</Text>
        </Space>
      </div>
      <div style={{ padding: 16 }}>{children}</div>
    </div>
  )
}
