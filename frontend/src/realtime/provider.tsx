import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { subscribeSSE } from '@/lib/sse'
import { useAuthStore } from '@/stores/authStore'
import type { RealtimeEvent } from './types'
import { RealtimeStatusContext } from './context'
import type { RealtimeStatus } from './context'
import {
  applyNotificationCreated,
  applyNotificationRead,
  applyNotificationsAllRead,
} from './reducers/notifications'
import { applyTaskCommentCreated, applyTaskEventCreated, applyTaskUpdated } from './reducers/tasks'
import { applyAgentStatusChanged } from './reducers/agents'
import { applyAgentChatUpdated } from './reducers/agent-chat'

export function RealtimeProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const isAuthenticated = useAuthStore((state) => !!state.refreshToken)
  const [status, setStatus] = useState<RealtimeStatus>('idle')

  useEffect(() => {
    if (!isAuthenticated) {
      return undefined
    }

    const connectingHandle = window.setTimeout(() => {
      setStatus('connecting')
    }, 0)

    const stop = subscribeSSE<RealtimeEvent>({
      path: 'events/stream',
      onOpen: () => {
        setStatus('connected')
        void queryClient.invalidateQueries({ queryKey: ['notifications'] })
        void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
        void queryClient.invalidateQueries({ queryKey: ['projects'] })
        void queryClient.invalidateQueries({ queryKey: ['agents'] })
        void queryClient.invalidateQueries({ queryKey: ['tasks'] })
      },
      onError: () => {
        setStatus((current) => (current === 'connected' ? 'reconnecting' : 'disconnected'))
      },
      onMessage: (event) => {
        switch (event.type) {
          case 'notification.created':
            applyNotificationCreated(queryClient, event.payload)
            break
          case 'notification.read':
            applyNotificationRead(queryClient, event.payload)
            break
          case 'notifications.all_read':
            applyNotificationsAllRead(queryClient, event.payload)
            break
          case 'task.updated':
            applyTaskUpdated(queryClient, event.payload)
            break
          case 'task.event.created':
            applyTaskEventCreated(queryClient, event.payload)
            break
          case 'task.comment.created':
            applyTaskCommentCreated(queryClient, event.payload)
            break
          case 'agent.status.changed':
            applyAgentStatusChanged(queryClient, event.payload)
            break
          case 'agent_chat.updated':
            applyAgentChatUpdated(queryClient, event.payload)
            break
          case 'join_request.created':
            void queryClient.invalidateQueries({ queryKey: ['join-requests'] })
            break
        }
      },
    })
    return () => {
      window.clearTimeout(connectingHandle)
      stop()
    }
  }, [isAuthenticated, queryClient])

  return (
    <RealtimeStatusContext.Provider value={isAuthenticated ? status : 'idle'}>
      {isAuthenticated && (status === 'reconnecting' || status === 'disconnected') && (
        <div className="fixed top-0 left-0 right-0 z-[9999] flex items-center justify-center gap-2 bg-amber-500 px-4 py-1.5 text-sm font-medium text-white shadow-md dark:bg-amber-600">
          <span className="inline-block size-2 animate-pulse rounded-full bg-white" />
          {status === 'reconnecting' ? '连接断开，正在重连…' : '无法连接服务器，请检查网络'}
        </div>
      )}
      {children}
    </RealtimeStatusContext.Provider>
  )
}
