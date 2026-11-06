import type { NotificationPage } from '@/types/api'
import { api } from './client'

export const notificationApi = {
  list(cursor: string | null, limit = 20) {
    return api.get<NotificationPage>('/notifications', { cursor: cursor ?? '', limit })
  },
  unreadCount() {
    return api.get<{ unread_count: number }>('/notifications/unread-count')
  },
  markRead(id: number) {
    return api.patch<void>(`/notifications/${id}/read`)
  },
  markAllRead() {
    return api.post<Record<string, never>>('/notifications/read-all')
  },
}
