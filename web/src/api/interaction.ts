import type { FeedPage } from '@/types/api'
import { api } from './client'

export const interactionApi = {
  like(videoId: number) {
    return api.post<Record<string, never>>(`/videos/${videoId}/like`)
  },
  unlike(videoId: number) {
    return api.del<Record<string, never>>(`/videos/${videoId}/like`)
  },
  favorite(videoId: number) {
    // No folder_id means "the default folder"; the backend creates it on demand.
    return api.post<{ id: number; favorite_id: number; video_id: number; user_id: number; is_new: boolean }>(
      `/videos/${videoId}/favorite`,
      {},
    )
  },
  unfavorite(videoId: number) {
    return api.del<Record<string, never>>(`/videos/${videoId}/favorite`)
  },
  listFavorites() {
    return api.get<{ folders: { id: number; name: string; item_count: number; is_default: boolean }[] }>(
      '/user/favorites',
    )
  },
  favoriteItems(folderId: number, cursor: number | null, limit = 20) {
    return api.get<{ items: { id: number; video_id: number; created_at: string }[]; next_cursor: number; has_more: boolean }>(
      `/user/favorites/${folderId}/items`,
      { cursor: cursor ?? 0, limit },
    )
  },
}

export type { FeedPage }
