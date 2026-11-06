import type { Friendship, RelationPage } from '@/types/api'
import { api } from './client'

export const relationApi = {
  follow(userId: number) {
    return api.post<Record<string, never>>(`/users/${userId}/follow`)
  },
  unfollow(userId: number) {
    return api.del<Record<string, never>>(`/users/${userId}/follow`)
  },
  /** Hard-auth: for a guest the page cannot know the state, so it must not guess. */
  relation(userId: number) {
    return api.get<Friendship>(`/users/${userId}/relation`)
  },
  followings(userId: number, cursor: number | null, limit = 20) {
    return api.get<RelationPage>(`/users/${userId}/followings`, { cursor: cursor ?? 0, limit })
  },
  followers(userId: number, cursor: number | null, limit = 20) {
    return api.get<RelationPage>(`/users/${userId}/followers`, { cursor: cursor ?? 0, limit })
  },
}
