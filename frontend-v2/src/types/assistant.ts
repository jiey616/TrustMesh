// ========== AI 助手（移植自旧前端，SSE 流式 + 工具调用） ==========

export interface AssistantToolCall {
  tool: string
  args: Record<string, unknown>
  status: 'running' | 'done'
}

export interface AssistantResult {
  type: 'knowledge' | 'tasks' | 'task_detail' | 'stats'
  items?: unknown[]
  task?: unknown
  stats?: unknown
}

export interface AssistantMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  toolCalls?: AssistantToolCall[]
  results?: AssistantResult[]
  navigateAction?: { path: string; label: string }
  timestamp: number
  isStreaming?: boolean
}
