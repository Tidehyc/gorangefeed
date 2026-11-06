import type { PublicUserProfile } from '@/types/api'
import { api } from './client'

export const userApi = {
  /** Public profile omits username and follow state; callers must not render those. */
  publicProfile(userId: number) {
    return api.get<PublicUserProfile>(`/users/${userId}`)
  },
}
