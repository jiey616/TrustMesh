import { apiUrl } from '@/lib/apiBase'
import { useAuthStore } from '@/stores/authStore'

interface ChatParams {
  message: string
  context?: { current_page: string; project_id?: string }
  history?: { role: string; content: string }[]
}

interface ChatCallbacks {
  onDelta: (content: string) => void
  onToolCall: (tool: string, args: Record<string, unknown>) => void
  onToolResult: (tool: string, result: unknown) => void
  onNavigate: (path: string, label: string) => void
  onDone: () => void
  onError: (err: Error) => void
}

/**
 * Send a chat message to the assistant and process the SSE stream.
 * Returns an AbortController to cancel the request.
 */
export function chatAssistant(params: ChatParams, callbacks: ChatCallbacks): AbortController {
  const controller = new AbortController()

  const run = async () => {
    const token = useAuthStore.getState().accessToken
    const response = await fetch(apiUrl('assistant/chat'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(params),
      signal: controller.signal,
    })

    if (!response.ok) {
      throw new Error(`Assistant request failed (${response.status})`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('No response body')
    }

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (controller.signal.aborted) {
        await reader.cancel()
        return
      }

      buffer += decoder.decode(value, { stream: true })
      const frames = buffer.split(/\r?\n\r?\n/)
      buffer = frames.pop() ?? ''

      for (const frame of frames) {
        const parsed = parseSSEFrame(frame)
        if (!parsed) continue

        switch (parsed.event) {
          case 'delta': {
            const d = parsed.data as { content?: string }
            if (d.content) callbacks.onDelta(d.content)
            break
          }
          case 'tool_call': {
            const d = parsed.data as { tool: string; args: Record<string, unknown> }
            callbacks.onToolCall(d.tool, typeof d.args === 'string' ? JSON.parse(d.args as string) : d.args)
            break
          }
          case 'tool_result': {
            const d = parsed.data as { tool: string; result: unknown }
            callbacks.onToolResult(d.tool, d.result)
            break
          }
          case 'navigate': {
            const d = parsed.data as { path: string; label: string }
            callbacks.onNavigate(d.path, d.label)
            break
          }
          case 'done':
            callbacks.onDone()
            return
          case 'error': {
            const d = parsed.data as { message?: string }
            callbacks.onError(new Error(d.message ?? 'Unknown assistant error'))
            return
          }
          case 'ping':
            break
        }
      }
    }

    callbacks.onDone()
  }

  run().catch((err) => {
    if (!controller.signal.aborted) {
      callbacks.onError(err instanceof Error ? err : new Error(String(err)))
    }
  })

  return controller
}

function parseSSEFrame(frame: string): { event: string; data: Record<string, unknown> } | null {
  const lines = frame.split(/\r?\n/)
  let event = 'message'
  const dataLines: string[] = []

  for (const line of lines) {
    if (!line || line.startsWith(':')) continue
    if (line.startsWith('event:')) {
      event = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }

  if (dataLines.length === 0) return null

  try {
    const data = JSON.parse(dataLines.join('\n'))
    return { event, data }
  } catch {
    return null
  }
}
