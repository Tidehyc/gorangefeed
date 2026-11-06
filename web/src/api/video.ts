import type { AuthorVideosPage, CommentPage, FeedPage, VideoDetail } from '@/types/api'
import { api } from './client'

export interface PublishPayload {
  title: string
  description: string
  play_url: string
  cover_url: string
  duration_ms: number
  tags: string[]
}

export const videoApi = {
  detail(videoId: number) {
    return api.get<VideoDetail>(`/videos/${videoId}`)
  },

  authorVideos(authorId: number, cursor: number | null, limit = 12) {
    return api.get<AuthorVideosPage>(`/users/${authorId}/videos`, {
      cursor: cursor ?? 0,
      limit,
    })
  },

  myLikes(cursor: number | null, limit = 12) {
    return api.get<FeedPage>('/user/me/likes', { cursor: cursor ?? 0, limit })
  },

  publish(payload: PublishPayload, idempotencyKey: string) {
    return api.post<VideoDetail>('/videos', payload, { 'Idempotency-Key': idempotencyKey })
  },

  remove(videoId: number) {
    return api.del<Record<string, never>>(`/videos/${videoId}`)
  },

  comments(videoId: number, cursor: string | null, sort: 'latest' | 'hot' = 'latest', limit = 12) {
    return api.get<CommentPage>(`/videos/${videoId}/comments`, {
      cursor: cursor ?? '',
      sort,
      limit,
    })
  },

  createComment(videoId: number, content: string, idempotencyKey: string) {
    // The backend rejects an empty key with 400, so the caller must always mint one per attempt.
    return api.post<{
      id: number
      video_id: number
      user_id: number
      parent_id: number
      content: string
      like_count: number
      created_at: string
    }>(`/videos/${videoId}/comments`, { content, idempotency_key: idempotencyKey }, {
      'Idempotency-Key': idempotencyKey,
    })
  },

  deleteComment(commentId: number) {
    return api.del<Record<string, never>>(`/comments/${commentId}`)
  },

  likeComment(commentId: number) {
    return api.post<Record<string, never>>(`/comments/${commentId}/like`)
  },

  unlikeComment(commentId: number) {
    return api.del<Record<string, never>>(`/comments/${commentId}/like`)
  },
}
