import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import * as notificationsApi from '@/api/notifications'
import type { Notification } from '@/types'

const DEFAULT_LIMIT = 50

function notificationListKey(filter: string, limit: number) {
  return ['notifications', filter, limit] as const
}

function patchNotificationLists(
  qc: QueryClient,
  updater: (items: Notification[] | undefined, filter: string) => Notification[] | undefined
) {
  for (const filter of ['recent', 'unread', 'all'] as const) {
    qc.setQueryData<Notification[] | undefined>(notificationListKey(filter, DEFAULT_LIMIT), (items) =>
      updater(items, filter)
    )
  }
}

export function useNotifications(filter = 'recent', limit = 50) {
  return useQuery({
    queryKey: notificationListKey(filter, limit),
    queryFn: async () => {
      const res = await notificationsApi.listNotifications(filter, limit)
      return res.data.items
    },
    staleTime: 15_000,
  })
}

export function useUnreadCount() {
  return useQuery({
    queryKey: ['notifications', 'unread-count'],
    queryFn: async () => {
      const res = await notificationsApi.getUnreadCount()
      return res.data.count
    },
    staleTime: 15_000,
  })
}

export function useMarkNotificationRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => notificationsApi.markNotificationRead(id),
    onMutate: async (id) => {
      const previousCount = qc.getQueryData<number>(['notifications', 'unread-count'])
      const unreadItems = qc.getQueryData<Notification[]>(notificationListKey('unread', DEFAULT_LIMIT))
      const wasUnread = unreadItems?.some((item) => item.id === id) ?? false
      const readAt = new Date().toISOString()

      patchNotificationLists(qc, (items, filter) => {
        if (!items) {
          return items
        }
        if (filter === 'unread') {
          return items.filter((item) => item.id !== id)
        }
        return items.map((item) => (item.id === id ? { ...item, is_read: true, read_at: readAt } : item))
      })

      if (wasUnread && typeof previousCount === 'number') {
        qc.setQueryData(['notifications', 'unread-count'], Math.max(0, previousCount - 1))
      }

      return { previousCount, wasUnread }
    },
    onError: (_error, _id, context) => {
      if (typeof context?.previousCount === 'number') {
        qc.setQueryData(['notifications', 'unread-count'], context.previousCount)
      }
      qc.invalidateQueries({ queryKey: ['notifications'] })
      qc.invalidateQueries({ queryKey: ['notifications', 'unread-count'] })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['notifications', 'unread-count'] })
    },
  })
}

export function useMarkAllRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => notificationsApi.markAllNotificationsRead(),
    onMutate: async () => {
      const previousCount = qc.getQueryData<number>(['notifications', 'unread-count'])
      const readAt = new Date().toISOString()

      patchNotificationLists(qc, (items, filter) => {
        if (!items) {
          return items
        }
        if (filter === 'unread') {
          return []
        }
        return items.map((item) => ({ ...item, is_read: true, read_at: readAt }))
      })

      qc.setQueryData(['notifications', 'unread-count'], 0)
      return { previousCount }
    },
    onError: (_error, _vars, context) => {
      if (typeof context?.previousCount === 'number') {
        qc.setQueryData(['notifications', 'unread-count'], context.previousCount)
      }
      qc.invalidateQueries({ queryKey: ['notifications'] })
      qc.invalidateQueries({ queryKey: ['notifications', 'unread-count'] })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['notifications', 'unread-count'] })
    },
  })
}
