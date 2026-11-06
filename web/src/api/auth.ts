import type { LoginResponse, UserProfile } from '@/types/api'
import { api, request } from './client'

export interface RegisterResult {
  id: number
  username: string
  nickname: string
}

export const authApi = {
  login(username: string, password: string) {
    return request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: { username, password },
      anonymous: true,
    })
  },

  register(username: string, password: string) {
    // Register does not return tokens; the caller signs in right after.
    return request<RegisterResult>('/auth/register', {
      method: 'POST',
      body: { username, password },
      anonymous: true,
    })
  },

  me() {
    return api.get<UserProfile>('/users/me')
  },

  updateProfile(nickname: string, bio: string) {
    return api.put<UserProfile>('/users/me', { nickname, bio })
  },

  changePassword(oldPassword: string, newPassword: string) {
    return api.put<Record<string, never>>('/users/me/password', {
      old_password: oldPassword,
      new_password: newPassword,
    })
  },

  deleteAccount(password: string) {
    return api.del<Record<string, never>>('/users/me', { password })
  },

  uploadAvatar(file: File) {
    const form = new FormData()
    form.append('file', file)
    return api.form<{ avatar_url: string }>('/users/avatar', form)
  },

  logout(refreshToken: string | null) {
    return request<{ message: string }>('/auth/logout', {
      method: 'POST',
      body: { refresh_token: refreshToken ?? '' },
      noRetry: true,
    })
  },
}
