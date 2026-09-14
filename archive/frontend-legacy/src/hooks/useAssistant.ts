import { useRef, useCallback } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { chatAssistant } from '@/api/assistant'
import { useAssistantStore } from '@/stores/assistantStore'
import type { AssistantResult, TaskDetail, DashboardStats } from '@/types'

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

      // Cancel any ongoing request
      abortRef.current?.abort()

      addUserMessage(content)
      const assistantMsgId = addAssistantMessage()
      setProcessing(true)

      // Build history from previous messages (exclude the current exchange)
      const history = messages
        .filter((m) => !m.isStreaming)
        .map((m) => ({ role: m.role, content: m.content }))

      // Extract project ID from current route if on a project page
      const projectMatch = location.pathname.match(/\/projects\/([^/]+)/)
      const projectId = projectMatch?.[1]

      abortRef.current = chatAssistant(
        {
          message: content,
          context: {
            current_page: location.pathname,
            project_id: projectId,
          },
          history,
        },
        {
          onDelta: (delta) => {
            appendDelta(assistantMsgId, delta)
          },
          onToolCall: (tool, args) => {
            addToolCall(assistantMsgId, tool, args)
          },
          onToolResult: (tool, result) => {
            markToolCallDone(assistantMsgId, tool)
            const parsed = parseToolResult(tool, result)
            if (parsed) {
              addToolResult(assistantMsgId, parsed)
            }
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
            appendDelta(assistantMsgId, `\n\n_发生错误: ${err.message}_`)
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

function parseToolResult(tool: string, result: unknown): AssistantResult | null {
  if (!result || typeof result !== 'object') return null
  const r = result as Record<string, unknown>

  switch (tool) {
    case 'search_knowledge': {
      const items = r.results
      if (Array.isArray(items)) {
        return { type: 'knowledge', items }
      }
      return null
    }
    case 'search_tasks': {
      const items = r.tasks
      if (Array.isArray(items)) {
        return { type: 'tasks', items }
      }
      return null
    }
    case 'get_task_detail': {
      return { type: 'task_detail', task: r as unknown as TaskDetail }
    }
    case 'get_dashboard_stats': {
      return { type: 'stats', stats: r as unknown as DashboardStats }
    }
    default:
      return null
  }
}
