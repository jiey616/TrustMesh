import { Typography, List, Tag, Button, Segmented, Empty } from 'antd'
import { InboxOutlined, CheckOutlined, CheckCircleOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useNotifications, useMarkAllRead, useMarkNotificationRead } from '@/hooks/useNotifications'
import { PageHeader } from '@/components/shared/PageHeader'
import { groupNotificationsByDate } from '@/lib/notifications'
import type { Notification } from '@/types'
import dayjs from 'dayjs'

const { Text } = Typography

const typeColor: Record<string, string> = {
  task: 'blue',
  todo: 'cyan',
  agent: 'purple',
  system: 'default',
  meeting: 'gold',
}

const typeLabel: Record<string, string> = {
  task: '任务',
  todo: 'Todo',
  agent: '数字员工',
  system: '系统',
  meeting: '会议',
}

/** 根据通知类型跳转到对应页面 */
function useNotificationNavigate() {
  const navigate = useNavigate()
  return (item: Notification) => {
    const isJoinRequest = item.category === 'agent' && (item.title.includes('入职') || item.body.includes('申请加入'))
    if (isJoinRequest) {
      navigate('/agent-invite')
      return
    }
    if (item.category === 'agent' && item.actor_id) {
      navigate(`/agents/${item.actor_id}`)
      return
    }
    if (item.project_id) {
      navigate(`/projects/${item.project_id}`)
      return
    }
    if (item.task_id) {
      navigate(`/projects/${item.project_id ?? ''}`)
    }
  }
}

export function InboxPage() {
  const [filter, setFilter] = useState<string>('recent')
  const { data: notifications, isLoading } = useNotifications(filter)
  const markAllRead = useMarkAllRead()
  const markRead = useMarkNotificationRead()
  const navigateTo = useNotificationNavigate()
  const hasUnread = notifications?.some((n) => !n.is_read) ?? false

  const groups = notifications ? groupNotificationsByDate(notifications) : []

  return (
    <>
      <PageHeader
        title="收件箱"
        icon={<InboxOutlined />}
        actions={
          hasUnread ? (
            <Button icon={<CheckOutlined />} onClick={() => markAllRead.mutate()} loading={markAllRead.isPending}>
              全部已读
            </Button>
          ) : undefined
        }
      />
      <Segmented
        options={[
          { label: '最近', value: 'recent' },
          { label: '未读', value: 'unread' },
          { label: '全部', value: 'all' },
        ]}
        value={filter}
        onChange={(v) => setFilter(v as string)}
        style={{ marginBottom: 16 }}
      />

      {isLoading ? (
        <div style={{ padding: '40px 0', textAlign: 'center', color: 'var(--text-tertiary)' }}>加载中...</div>
      ) : groups.length === 0 ? (
        <Empty
          description={filter === 'unread' ? '没有未读通知' : '暂无通知'}
          style={{ padding: '60px 0' }}
        />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {groups.map((group) => (
            <div
              key={group.label}
              style={{
                background: 'var(--surface)',
                border: '1px solid var(--line)',
                borderRadius: 'var(--radius-control)',
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  padding: '8px 16px',
                  fontSize: 12,
                  fontWeight: 600,
                  color: 'var(--text-tertiary)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.5px',
                  background: 'var(--surface-sunken)',
                  borderBottom: '1px solid var(--line)',
                }}
              >
                {group.label} <Text type="secondary" style={{ fontSize: 11, marginLeft: 6 }}>{group.items.length}</Text>
              </div>
              <List
                dataSource={group.items}
                renderItem={(item) => (
                  <List.Item
                    style={{
                      padding: '12px 16px',
                      borderBottom: '1px solid var(--line)',
                      opacity: item.is_read ? 0.6 : 1,
                      background: item.is_read ? 'transparent' : 'rgba(109,95,245,0.04)',
                      cursor: 'pointer',
                    }}
                    onClick={() => {
                      if (!item.is_read) markRead.mutate(item.id)
                      navigateTo(item)
                    }}
                    actions={
                      item.is_read
                        ? [
                            <span key="read" style={{ color: 'var(--text-quaternary)', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                              <CheckCircleOutlined /> 已读
                            </span>,
                          ]
                        : [
                            <Button
                              key="mark"
                              type="link"
                              size="small"
                              icon={<CheckOutlined />}
                              loading={markRead.isPending && markRead.variables === item.id}
                              onClick={(e) => {
                                e.stopPropagation()
                                markRead.mutate(item.id)
                              }}
                            >
                              标为已读
                            </Button>,
                          ]
                    }
                  >
                    <List.Item.Meta
                      title={
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                          {!item.is_read && <span style={{ width: 6, height: 6, borderRadius: 'var(--radius-avatar)', background: 'var(--signal)', display: 'inline-block' }} />}
                          <span style={{ color: 'var(--text-primary)', fontWeight: item.is_read ? 400 : 600 }}>{item.title.replace(/智能体/g, '数字员工')}</span>
                          {item.category && (
                            <Tag color={typeColor[item.category] || 'default'} style={{ marginInlineStart: 0 }}>
                              {typeLabel[item.category] ?? item.category}
                            </Tag>
                          )}
                          {item.category === 'agent' && item.title.includes('入职') && (
                            <Tag color="gold" style={{ marginInlineStart: 0 }}>待审批</Tag>
                          )}
                        </span>
                      }
                      description={
                        <span style={{ display: 'inline-flex', gap: 12, flexWrap: 'wrap' }}>
                          <span style={{ color: 'var(--text-secondary)' }}>{item.body.replace(/\bAgent\b/g, '数字员工')}</span>
                          {item.actor_name && item.category === 'agent' && (
                            <span style={{ color: 'var(--text-tertiary)' }}>来自 {item.actor_name}</span>
                          )}
                          <span style={{ color: 'var(--text-quaternary)' }}>{dayjs(item.created_at).fromNow()}</span>
                        </span>
                      }
                    />
                  </List.Item>
                )}
              />
            </div>
          ))}
        </div>
      )}
    </>
  )
}
