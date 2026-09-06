import { useCallback, useRef } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { chatAssistant } from '@/api/assistant'
import { useAssistantStore } from '@/stores/assistantStore'

const TOOL_LABEL_MAP: Record<string, string> = {
  search_knowledge: '搜索知识库',
  search_tasks: '搜索任务',
  get_task_detail: '获取任务详情',
  get_dashboard_stats: '获取统计数据',
  list_projects: '获取项目列表',
  navigate: '导航',
}

export function useAssistant() {
  const location = useLocation()
  const navigate = useNavigate()
  const abortRef = useRef<AbortController | null>(null)

  const {
    messages,
    isOpen,
    isProcessing,
    toggle,
    open,
    close,
    addUserMessage,
    addAssistantMessage,
    appendDelta,
    addToolCall,
    markToolCallDone,
    addToolResult,
    setNavigateAction,
    finishMessage,
    setProcessing,
    clearMessages,
  } = useAssistantStore()

  const sendMessage = useCallback(
    (content: string) => {
      if (isProcessing) return

      abortRef.current?.abort()

      addUserMessage(content)
      const assistantMsgId = addAssistantMessage()
      setProcessing(true)

      // 历史上下文：排除当前这轮（正在流式的消息）
      const history = messages
        .filter((m) => !m.isStreaming)
        .map((m) => ({ role: m.role, content: m.content }))

      // 项目页上下文带上 project_id
      const projectMatch = location.pathname.match(/\/projects\/([^/]+)/)
      const projectId = projectMatch?.[1]

      abortRef.current = chatAssistant(
        {
          message: content,
          context: { current_page: location.pathname, project_id: projectId },
          history,
        },
        {
          onDelta: (delta) => appendDelta(assistantMsgId, delta),
          onToolCall: (tool, args) => addToolCall(assistantMsgId, tool, args),
          onToolResult: (tool, result) => {
            markToolCallDone(assistantMsgId, tool)
            const parsed = parseToolResult(tool, result)
            if (parsed) addToolResult(assistantMsgId, parsed)
          },
          onNavigate: (path, label) => {
            markToolCallDone(assistantMsgId, 'navigate')
            setNavigateAction(assistantMsgId, path, label)
            navigate(path)
          },
          onDone: () => {
            finishMessage(assistantMsgId)
            setProcessing(false)
          },
          onError: (err) => {
            appendDelta(assistantMsgId, `\n\n发生错误: ${err.message}`)
            finishMessage(assistantMsgId)
            setProcessing(false)
          },
        }
      )
    },
    [
      isProcessing, messages, location.pathname, navigate,
      addUserMessage, addAssistantMessage, appendDelta,
      addToolCall, markToolCallDone, addToolResult,
      setNavigateAction, finishMessage, setProcessing,
    ]
  )

  const cancel = useCallback(() => {
    abortRef.current?.abort()
    setProcessing(false)
  }, [setProcessing])

  return {
    messages,
    isOpen,
    isProcessing,
    toggle,
    open,
    close,
    sendMessage,
    cancel,
    clearMessages,
    getToolLabel: (tool: string) => TOOL_LABEL_MAP[tool] ?? tool,
  }
}

function parseToolResult(tool: string, result: unknown): {
  type: 'knowledge' | 'tasks' | 'task_detail' | 'stats'
  items?: unknown[]
  task?: unknown
  stats?: unknown
} | null {
  if (!result || typeof result !== 'object') return null
  const r = result as Record<string, unknown>

  switch (tool) {
    case 'search_knowledge': {
      if (Array.isArray(r.results)) return { type: 'knowledge', items: r.results }
      return null
    }
    case 'search_tasks': {
      if (Array.isArray(r.tasks)) return { type: 'tasks', items: r.tasks }
      return null
    }
    case 'get_task_detail':
      return { type: 'task_detail', task: r }
    case 'get_dashboard_stats':
      return { type: 'stats', stats: r }
    default:
      return null
  }
}
