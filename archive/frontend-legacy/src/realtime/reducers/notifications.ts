import type { QueryClient } from '@tanstack/react-query'
import type { Notification } from '@/types'

function sortNotifications(items: Notification[]) {
  return items.slice().sort((left, right) => right.created_at.localeCompare(left.created_at))
}

function visitNotificationLists(
  queryClient: QueryClient,
  updater: (
    items: Notification[] | undefined,
    filter: string,
    limit: number
  ) => Notification[] | undefined
) {
  const queries = queryClient.getQueriesData<Notification[]>({ queryKey: ['notifications'] })
  for (const [queryKey, items] of queries) {
    if (!Array.isArray(queryKey) || queryKey.length !== 3) {
      continue
    }
    if (queryKey[0] !== 'notifications' || typeof queryKey[1] !== 'string' || typeof queryKey[2] !== 'number') {
      continue
    }
    queryClient.setQueryData<Notification[] | undefined>(queryKey, updater(items, queryKey[1], queryKey[2]))
  }
}

export function applyNotificationCreated(
  queryClient: QueryClient,
  payload: { notification: Notification; unread_count: number }
) {
  visitNotificationLists(queryClient, (items, filter, limit) => {
    if (!items) {
      return items
    }
    if (items.some((item) => item.id === payload.notification.id)) {
      return items
    }
    if (filter === 'unread' && payload.notification.is_read) {
      return items
    }
    return sortNotifications([payload.notification, ...items]).slice(0, limit)
  })

  queryClient.setQueryData(['notifications', 'unread-count'], payload.unread_count)
}

export function applyNotificationRead(
  queryClient: QueryClient,
  payload: { notification_id: string; read_at: string; unread_count: number }
) {
  visitNotificationLists(queryClient, (items, filter) => {
    if (!items) {
      return items
    }
    if (filter === 'unread') {
      return items.filter((item) => item.id !== payload.notification_id)
    }
    return items.map((item) =>
      item.id === payload.notification_id ? { ...item, is_read: true, read_at: payload.read_at } : item
    )
  })

  queryClient.setQueryData(['notifications', 'unread-count'], payload.unread_count)
}

export function applyNotificationsAllRead(
  queryClient: QueryClient,
  payload: { notification_ids: string[]; read_at: string; unread_count: number }
) {
  const readIDs = new Set(payload.notification_ids)

  visitNotificationLists(queryClient, (items, filter) => {
    if (!items) {
      return items
    }
    if (filter === 'unread') {
      return []
    }
    return items.map((item) =>
      readIDs.has(item.id) ? { ...item, is_read: true, read_at: payload.read_at } : item
    )
  })

  queryClient.setQueryData(['notifications', 'unread-count'], payload.unread_count)
}
