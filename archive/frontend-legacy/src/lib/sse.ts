import { ApiRequestError } from '@/api/client'
import type { ApiError } from '@/types'
import { apiUrl } from '@/lib/apiBase'
import { useAuthStore } from '@/stores/authStore'

type MessageHandler<T> = (message: T) => void

const DEFAULT_SSE_IDLE_TIMEOUT_MS = 60_000

interface SubscribeSSEOptions<T> {
  path: string
  onMessage: MessageHandler<T>
  onOpen?: () => void
  onError?: (error: unknown) => void
  signal?: AbortSignal
  idleTimeoutMs?: number
}

interface SSEFrame {
  event: string
  data: string
}

async function readChunkWithIdleTimeout(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  signal: AbortSignal | undefined,
  idleTimeoutMs: number
) {
  let timer = 0
  let abortListener: (() => void) | undefined

  try {
    return await Promise.race([
      reader.read(),
      new Promise<never>((_, reject) => {
        timer = window.setTimeout(() => {
          reject(new Error(`SSE idle timeout after ${idleTimeoutMs}ms`))
        }, idleTimeoutMs)

        abortListener = () => {
          window.clearTimeout(timer)
        }
        signal?.addEventListener('abort', abortListener, { once: true })
      }),
    ])
  } finally {
    window.clearTimeout(timer)
    if (abortListener) {
      signal?.removeEventListener('abort', abortListener)
    }
  }
}

function parseFrame(chunk: string): SSEFrame | null {
  const lines = chunk.split(/\r?\n/)
  let event = 'message'
  const data: string[] = []

  for (const line of lines) {
    if (!line || line.startsWith(':')) {
      continue
    }
    if (line.startsWith('event:')) {
      event = line.slice(6).trim()
      continue
    }
    if (line.startsWith('data:')) {
      data.push(line.slice(5).trimStart())
    }
  }

  if (data.length === 0) {
    return null
  }

  return {
    event,
    data: data.join('\n'),
  }
}

async function readStream<T>(
  response: Response,
  onMessage: MessageHandler<T>,
  signal?: AbortSignal,
  idleTimeoutMs = DEFAULT_SSE_IDLE_TIMEOUT_MS
) {
  const reader = response.body?.getReader()
  if (!reader) {
    return
  }

  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    let done: boolean
    let value: Uint8Array | undefined
    try {
      ;({ done, value } = await readChunkWithIdleTimeout(reader, signal, idleTimeoutMs))
    } catch (error) {
      if (!signal?.aborted) {
        await reader.cancel().catch(() => undefined)
      }
      throw error
    }
    if (done) {
      break
    }
    if (signal?.aborted) {
      await reader.cancel()
      return
    }

    const chunk = decoder.decode(value!, { stream: true })
    buffer += chunk
    const frames = buffer.split(/\r?\n\r?\n/)
    buffer = frames.pop() ?? ''

    for (const frame of frames) {
      const parsed = parseFrame(frame)
      if (!parsed || parsed.event === 'ping') {
        continue
      }
      try {
        const obj = JSON.parse(parsed.data) as T
        onMessage(obj)
      } catch {
        throw new Error('Invalid SSE payload')
      }
    }
  }
}

function handleUnauthorized() {
  useAuthStore.getState().logout()
  if (window.location.pathname !== '/login' && window.location.pathname !== '/register') {
    window.location.href = '/login'
  }
}

async function connectOnce<T>(
  options: SubscribeSSEOptions<T>,
  signal?: AbortSignal
) {
  const token = useAuthStore.getState().accessToken
  const response = await fetch(apiUrl(options.path), {
    headers: {
      Accept: 'text/event-stream',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal,
  })

  if (!response.ok) {
    if (response.status === 401) {
      handleUnauthorized()
    }

    const body = await response.json().catch(() => null) as { error?: ApiError } | null
    if (body?.error) {
      throw new ApiRequestError(body.error, response.status)
    }
    throw new Error(`SSE request failed with status ${response.status}`)
  }

  options.onOpen?.()
  await readStream(response, options.onMessage, signal, options.idleTimeoutMs)
}

export function subscribeSSE<T>(options: SubscribeSSEOptions<T>) {
  let stopped = false
  let currentAbort: AbortController | null = null

  const stop = () => {
    stopped = true
    currentAbort?.abort()
  }

  if (options.signal) {
    options.signal.addEventListener('abort', stop, { once: true })
  }

  const run = async () => {
    while (!stopped) {
      currentAbort = new AbortController()
      const abortListener = () => currentAbort?.abort()
      options.signal?.addEventListener('abort', abortListener, { once: true })

      try {
        await connectOnce(options, currentAbort.signal)
      } catch (error) {
        if (stopped || currentAbort.signal.aborted) {
          return
        }
        currentAbort.abort()
        options.onError?.(error)
        if (error instanceof ApiRequestError && error.status < 500 && error.status !== 429) {
          return
        }
      } finally {
        options.signal?.removeEventListener('abort', abortListener)
      }

      await new Promise((resolve) => window.setTimeout(resolve, 2000))
    }
  }

  void run()

  return stop
}
