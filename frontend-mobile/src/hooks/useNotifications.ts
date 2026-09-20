import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getUnreadCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
} from '@/api/notifications'
import { useSettingsStore } from '@/stores/settingsStore'
import { useWorkspaceStore } from '@/stores/workspaceStore'

export function useNotifications(filter: 'recent' | 'unread' = 'recent') {
  const ready = useWorkspaceStore((s) => s.calibrated)
  return useQuery({
    queryKey: ['notifications', filter],
    queryFn: () => listNotifications(filter),
    enabled: ready,
  })
}

/** 未读数：轮询间隔来自本地设置（0 = 不轮询）。 */
export function useUnreadCount() {
  const ready = useWorkspaceStore((s) => s.calibrated)
  const interval = useSettingsStore((s) => s.unreadPollInterval)
  return useQuery({
    queryKey: ['unreadCount'],
    queryFn: getUnreadCount,
    enabled: ready,
    refetchInterval: interval > 0 ? interval : false,
    refetchIntervalInBackground: false,
  })
}

export function useMarkNotificationRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => markNotificationRead(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['notifications'] })
      void qc.invalidateQueries({ queryKey: ['unreadCount'] })
    },
  })
}

export function useMarkAllRead() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => markAllNotificationsRead(),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['notifications'] })
      void qc.invalidateQueries({ queryKey: ['unreadCount'] })
    },
  })
}
