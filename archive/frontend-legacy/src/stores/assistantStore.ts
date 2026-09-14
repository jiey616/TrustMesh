import { create } from 'zustand'
import type { AssistantMessage, AssistantResult, AssistantToolCall } from '@/types'

interface AssistantState {
  isOpen: boolean
  messages: AssistantMessage[]
  isProcessing: boolean
  fabVisibility: 'visible' | 'hidden'

  toggle: () => void
  open: () => void
  close: () => void
  setFabVisibility: (visibility: 'visible' | 'hidden') => void

  addUserMessage: (content: string) => string
  addAssistantMessage: () => string
  appendDelta: (messageId: string, delta: string) => void
  addToolCall: (messageId: string, tool: string, args: Record<string, unknown>) => void
  addToolResult: (messageId: string, result: AssistantResult) => void
  markToolCallDone: (messageId: string, tool: string) => void
  setNavigateAction: (messageId: string, path: string, label: string) => void
  finishMessage: (messageId: string) => void
  setProcessing: (v: boolean) => void
  clearMessages: () => void
}

let msgCounter = 0
function nextId() {
  return `msg_${Date.now()}_${++msgCounter}`
}

export const useAssistantStore = create<AssistantState>()((set) => ({
  isOpen: false,
  messages: [],
  isProcessing: false,
  fabVisibility: 'visible',

  toggle: () => set((s) => ({ isOpen: !s.isOpen })),
  open: () => set({ isOpen: true }),
  close: () => set({ isOpen: false }),
  setFabVisibility: (visibility) => set({ fabVisibility: visibility }),

  addUserMessage: (content: string) => {
    const id = nextId()
    set((s) => ({
      messages: [...s.messages, {
        id,
        role: 'user',
        content,
        timestamp: Date.now(),
      }],
    }))
    return id
  },

  addAssistantMessage: () => {
    const id = nextId()
    set((s) => ({
      messages: [...s.messages, {
        id,
        role: 'assistant',
        content: '',
        toolCalls: [],
        results: [],
        timestamp: Date.now(),
        isStreaming: true,
      }],
    }))
    return id
  },

  appendDelta: (messageId, delta) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId ? { ...m, content: m.content + delta } : m
      ),
    })),

  addToolCall: (messageId, tool, args) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId
          ? {
              ...m,
              toolCalls: [...(m.toolCalls ?? []), { tool, args, status: 'running' as const }],
            }
          : m
      ),
    })),

  markToolCallDone: (messageId, tool) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId
          ? {
              ...m,
              toolCalls: m.toolCalls?.map((tc: AssistantToolCall) =>
                tc.tool === tool && tc.status === 'running'
                  ? { ...tc, status: 'done' as const }
                  : tc
              ),
            }
          : m
      ),
    })),

  addToolResult: (messageId, result) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId
          ? { ...m, results: [...(m.results ?? []), result] }
          : m
      ),
    })),

  setNavigateAction: (messageId, path, label) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId ? { ...m, navigateAction: { path, label } } : m
      ),
    })),

  finishMessage: (messageId) =>
    set((s) => ({
      messages: s.messages.map((m) =>
        m.id === messageId ? { ...m, isStreaming: false } : m
      ),
    })),

  setProcessing: (v) => set({ isProcessing: v }),

  clearMessages: () => set({ messages: [] }),
}))
