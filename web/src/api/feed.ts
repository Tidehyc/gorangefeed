import type { FeedPage, VideoDetail } from '@/types/api'
import { api } from './client'

export type FeedScene = 'recommend' | 'following'

export const feedApi = {
  /** 推荐流: personal when a token is present, rule-based recall for guests. */
  recommend(cursor: string | number | null, limit = 6) {
    return api.get<FeedPage>('/feed/recommend', {
      cursor: cursor ?? '',
      limit,
    })
  },

  /** 关注流: hard-auth endpoint, a guest gets 401 and must be routed to /login. */
  following(cursor: string | number | null, limit = 6) {
    return api.get<FeedPage>('/feed/following', {
      cursor: cursor ?? 0,
      limit,
    })
  },

  global(cursor: string | number | null, limit = 6) {
    return api.get<FeedPage>('/feed/global', { cursor: cursor ?? 0, limit })
  },

  byTag(tag: string, cursor: string | number | null, limit = 6) {
    return api.get<FeedPage>(`/tags/${encodeURIComponent(tag)}/videos`, {
      cursor: cursor ?? 0,
      limit,
    })
  },

  /** Playback telemetry. Silent: a failure here must never surface in the UI. */
  reportView(videoId: number, eventType: 'play' | 'complete', watchDurationMs: number) {
    return api
      .post<Record<string, never>>(`/videos/${videoId}/view`, {
        event_type: eventType,
        watch_duration_ms: watchDurationMs,
      })
      .catch(() => undefined)
  },
}

export type { VideoDetail }
