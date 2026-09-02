import { Tag, Typography, Empty, Skeleton, Progress, Space } from 'antd'
import { AgentAvatar } from '@/components/shared/AgentAvatar'
import { Link } from 'react-router-dom'
import dayjs from 'dayjs'
import type { TaskListItem, TaskStatus } from '@/types'

const { Text } = Typography

const statusMap: Record<TaskStatus, { color: string; label: string }> = {
  planning: { color: '#8b5cf6', label: '规划中' },
  review: { color: '#f59e0b', label: '待确认' },
  pending: { color: '#3b82f6', label: '待处理' },
  in_progress: { color: '#22d3ee', label: '进行中' },
  awaiting_review: { color: '#f43f5e', label: '待人工确认' },
  waiting_user: { color: '#f59e0b', label: '等待用户' },
  done: { color: '#10b981', label: '已完成' },
  failed: { color: '#ef4444', label: '失败' },
  canceled: { color: '#6b7280', label: '已取消' },
}

interface Props {
  tasks: TaskListItem[]
  loading?: boolean
}

export function RecentTasksList({ tasks, loading }: Props) {
  if (loading) {
    return <Skeleton active paragraph={{ rows: 4 }} />
  }
  if (!tasks || tasks.length === 0) {
    return <Empty description="暂无任务" />
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      {tasks.map((task) => {
        const status = statusMap[task.status]
        return (
          <Link
            key={task.id}
            to={`/projects/${task.project_id}`}
            style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 10,
              padding: 10,
              borderRadius: 10,
              background: 'rgba(255,255,255,0.02)',
              border: '1px solid rgba(255,255,255,0.04)',
              textDecoration: 'none',
              transition: 'background 0.15s',
            }}
          >
            <Tag color={status?.color} style={{ marginTop: 2, flexShrink: 0 }}>
              {status?.label ?? task.status}
            </Tag>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Text
                style={{
                  color: '#f4f4f8',
                  fontSize: 13,
                  fontWeight: 500,
                  display: 'block',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
                title={task.title}
              >
                {task.title}
              </Text>
              <Space size={6} wrap style={{ marginTop: 4 }}>
                <AgentAvatar name={task.pm_agent?.name || '用'} role="pm" seed={task.pm_agent?.node_id} size={18} />
                <Text type="secondary" style={{ fontSize: 11 }}>{task.pm_agent?.name || '用户创建'}</Text>
                <Text type="secondary" style={{ fontSize: 11 }} title={dayjs(task.updated_at).format('YYYY-MM-DD HH:mm:ss')}>
                  · {dayjs(task.updated_at).fromNow()}
                </Text>
                {task.todo_count > 0 && (
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 11, color: 'rgba(255,255,255,0.5)' }}>
                    ·
                    <Progress
                      percent={Math.round((task.completed_todo_count / task.todo_count) * 100)}
                      size="small"
                      showInfo={false}
                      style={{ width: 36, margin: 0 }}
                      strokeColor="#22d3ee"
                    />
                    {task.completed_todo_count}/{task.todo_count}
                  </span>
                )}
              </Space>
            </div>
          </Link>
        )
      })}
    </div>
  )
}
