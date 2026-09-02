import type { AgentStatus, TaskPriority, TaskStatus } from '@/types'

export const TASK_STATUS: Record<TaskStatus, { color: string; label: string }> = {
  planning: { color: '#b795dd', label: '规划中' },
  review: { color: '#b795dd', label: '审核中' },
  pending: { color: '#8b8f9e', label: '待处理' },
  in_progress: { color: '#6f8ff0', label: '进行中' },
  awaiting_review: { color: '#e0a35e', label: '待审核' },
  waiting_user: { color: '#f59e0b', label: '等待用户' },
  done: { color: '#6dc67f', label: '已完成' },
  failed: { color: '#e25563', label: '失败' },
  canceled: { color: '#6b6f7d', label: '已取消' },
}

export const TASK_PRIORITY: Record<TaskPriority, { color: string; label: string }> = {
  low: { color: '#8b8f9e', label: '低' },
  medium: { color: '#60a5fa', label: '中' },
  high: { color: '#f59e0b', label: '高' },
  urgent: { color: '#ef4444', label: '紧急' },
}

export const AGENT_STATUS: Record<AgentStatus, { color: string; label: string; dot: string }> = {
  online: { color: '#27a644', label: '在线', dot: '#4ade80' },
  busy: { color: '#3b82f6', label: '忙碌', dot: '#60a5fa' },
  offline: { color: '#71717a', label: '离线', dot: '#71717a' },
}