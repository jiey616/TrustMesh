import type { Agent, AgentChatDetail, Comment, Event, JoinRequest, Notification, TaskDetail } from '@/types'

export type RealtimeEvent =
  | {
      id: string
      type: 'notification.created'
      occurred_at: string
      payload: {
        notification: Notification
        unread_count: number
      }
    }
  | {
      id: string
      type: 'notification.read'
      occurred_at: string
      payload: {
        notification_id: string
        read_at: string
        unread_count: number
      }
    }
  | {
      id: string
      type: 'notifications.all_read'
      occurred_at: string
      payload: {
        notification_ids: string[]
        read_at: string
        unread_count: number
      }
    }
  | {
      id: string
      type: 'task.updated'
      occurred_at: string
      payload: {
        task: TaskDetail
      }
    }
  | {
      id: string
      type: 'task.event.created'
      occurred_at: string
      payload: {
        task_id: string
        project_id: string
        event: Event
      }
    }
  | {
      id: string
      type: 'task.comment.created'
      occurred_at: string
      payload: {
        task_id: string
        project_id: string
        comment: Comment
      }
    }
  | {
      id: string
      type: 'agent.status.changed'
      occurred_at: string
      payload: {
        agent: Agent
        event: Event
      }
    }
  | {
      id: string
      type: 'agent_chat.updated'
      occurred_at: string
      payload: {
        chat: AgentChatDetail
      }
    }
  | {
      id: string
      type: 'join_request.created'
      occurred_at: string
      payload: {
        join_request: JoinRequest
      }
    }
