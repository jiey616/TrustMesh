import type {
  Agent,
  AgentChatDetail,
  Comment,
  Event,
  JoinRequest,
  Notification,
  TaskDetail,
} from '@/types'

// ─── 实时事件（SSE）───
// 后端 SSE 统一以 `snapshot` 事件名推送，业务类型在 data.type 字段。
// 这里描述的是 data 的结构，供 realtimeBus 广播与消费者（办公室等）使用。
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

// ─── 办公室可视化 ───
// 这些状态是从真实任务数据派生的视图状态，刻意不进 TanStack Query 缓存：
//   种子 = agents 列表 + 活跃任务详情快照
//   更新 = realtimeBus 转发的 SSE 事件（真实驱动，不表演）

/** 派生状态：meeting(会议中) > asking(有未答问题) > working(有 in_progress todo) > thinking(PM 规划中) > idle */
export type AgentVisualState =
  | 'idle'
  | 'working'
  | 'asking'
  | 'thinking'
  | 'meeting'

export type AgentSticker = 'celebrate' | 'failed'

export interface AgentVisual {
  id: string
  name: string
  role: string
  presence: Agent['status']
  state: AgentVisualState
  sticker: AgentSticker | null
  stickerUntil: number
  bubble: string | null
  /** epoch ms；0 表示常驻（等待用户回答），到点自动消失 */
  bubbleUntil: number
  bubbleKind: 'normal' | 'question'
  /** 最近关联的任务，供点击跳转 */
  taskId: string | null
}
