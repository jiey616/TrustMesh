import { Timeline, Typography, Empty, Skeleton, Tag, Space } from 'antd'
import {
  PlusCircleOutlined,
  BulbOutlined,
  SwapOutlined,
  UserAddOutlined,
  PlayCircleOutlined,
  LoadingOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  MessageOutlined,
  ApiOutlined,
  PaperClipOutlined,
  ClockCircleOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons'
import { Link } from 'react-router-dom'
import dayjs from 'dayjs'
import { stripReplyPrefix } from '@/lib/text'
import type { Event, EventType } from '@/types'

const { Text } = Typography

// Partial：后端实际事件类型多于此处列举（如 todo_review_* / todo_timeout_* 等），
// 未覆盖的类型走消费处的 fallback，不强制穷尽。
const eventConfig: Partial<Record<EventType, { icon: React.ReactNode; color: string; label: string }>> = {
  task_created: { icon: <PlusCircleOutlined />, color: '#3b82f6', label: '任务创建' },
  task_plan_ready: { icon: <BulbOutlined />, color: '#3b82f6', label: '规划完成' },
  task_status_changed: { icon: <SwapOutlined />, color: '#f59e0b', label: '状态变更' },
  todo_assigned: { icon: <UserAddOutlined />, color: '#3b82f6', label: '分配 Todo' },
  todo_started: { icon: <PlayCircleOutlined />, color: '#3b82f6', label: '开始执行' },
  todo_progress: { icon: <LoadingOutlined />, color: 'rgba(255,255,255,0.5)', label: '执行中' },
  todo_completed: { icon: <CheckCircleOutlined />, color: '#10b981', label: 'Todo 完成' },
  todo_failed: { icon: <CloseCircleOutlined />, color: '#ef4444', label: 'Todo 失败' },
  todo_ask_received: { icon: <QuestionCircleOutlined />, color: '#8b5cf6', label: '提问' },
  task_comment: { icon: <MessageOutlined />, color: 'rgba(255,255,255,0.5)', label: '评论' },
  planning_reply: { icon: <MessageOutlined />, color: '#3b82f6', label: 'PM 规划回复' },
  agent_status_changed: { icon: <ApiOutlined />, color: '#f59e0b', label: '数字员工状态' },
  artifact_received: { icon: <PaperClipOutlined />, color: '#3b82f6', label: '上传了文件' },
}

const statusLabel: Record<string, string> = {
  planning: '规划中',
  pending: '待处理',
  in_progress: '进行中',
  awaiting_review: '待人工确认',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
}

interface Props {
  events: Event[]
  loading?: boolean
  showActorName?: boolean
  emptyText?: string
}

export function EventTimeline({ events, loading, showActorName = true, emptyText = '暂无活动记录' }: Props) {
  if (loading) {
    return <Skeleton active paragraph={{ rows: 4 }} />
  }
  if (!events || events.length === 0) {
    return <Empty description={emptyText} />
  }

  return (
    <Timeline
      items={events.map((ev) => {
        const cfg = eventConfig[ev.event_type] ?? { icon: <ClockCircleOutlined />, color: '#888', label: ev.event_type }
        const taskTitle = ev.metadata?.task_title as string | undefined
        const todoTitle = ev.metadata?.todo_title as string | undefined
        const fromStatus = ev.metadata?.from_status as string | undefined
        const toStatus = ev.metadata?.to_status as string | undefined

        return {
          dot: <span style={{ color: cfg.color, fontSize: 16 }}>{cfg.icon}</span>,
          color: cfg.color,
          children: (
            <div>
              <Space size={6} wrap>
                <Text style={{ color: '#f4f4f8', fontSize: 13 }}>{cfg.label}</Text>
                {showActorName && ev.actor_name && (
                  <Text type="secondary" style={{ fontSize: 12 }}>· {ev.actor_name}</Text>
                )}
                <Text type="secondary" style={{ fontSize: 11 }} title={dayjs(ev.created_at).format('YYYY-MM-DD HH:mm:ss')}>
                  · {dayjs(ev.created_at).fromNow()}
                </Text>
              </Space>
              {ev.content && (
                <div style={{ color: 'rgba(255,255,255,0.7)', fontSize: 12, marginTop: 4, lineHeight: 1.5 }}>
                  {stripReplyPrefix(ev.content)}
                </div>
              )}
              {fromStatus && toStatus && statusLabel[fromStatus] && statusLabel[toStatus] && (
                <div style={{ marginTop: 4 }}>
                  <Tag style={{ marginInlineEnd: 4 }}>{statusLabel[fromStatus]}</Tag>
                  <SwapOutlined style={{ color: 'rgba(255,255,255,0.4)', fontSize: 10 }} />
                  <Tag color="cyan" style={{ marginInlineStart: 4 }}>{statusLabel[toStatus]}</Tag>
                </div>
              )}
              {taskTitle && (
                <div style={{ marginTop: 4, fontSize: 12 }}>
                  <PaperClipOutlined style={{ color: 'rgba(255,255,255,0.4)', marginRight: 4 }} />
                  {ev.project_id ? (
                    <Link to={`/projects/${ev.project_id}`} style={{ color: '#22d3ee' }}>
                      {taskTitle}
                    </Link>
                  ) : (
                    <span style={{ color: 'rgba(255,255,255,0.7)' }}>{taskTitle}</span>
                  )}
                  {todoTitle && (
                    <>
                      <span style={{ color: 'rgba(255,255,255,0.3)', margin: '0 4px' }}>›</span>
                      <span style={{ color: 'rgba(255,255,255,0.7)' }}>{todoTitle}</span>
                    </>
                  )}
                </div>
              )}
            </div>
          ),
        }
      })}
    />
  )
}
