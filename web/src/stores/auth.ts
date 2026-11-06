import { create } from 'zustand'
import { authApi } from '@/api/auth'
import { ApiError } from '@/api/client'
import { session } from '@/api/tokens'
import type { UserProfile } from '@/types/api'

interface AuthState {
  user: UserProfile | null
  /** 'unknown' until bootstrap() resolves, so guarded pages do not flash the login screen. */
  status: 'unknown' | 'guest' | 'authed'
  pending: boolean
  bootstrap: () => Promise<void>
  login: (username: string, password: string) => Promise<void>
  register: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  reload: () => Promise<void>
  patchUser: (patch: Partial<UserProfile>) => void
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  status: 'unknown',
  pending: false,

  async bootstrap() {
    if (!session.isAuthenticated()) {
      set({ status: 'guest', user: null })
      return
    }
    try {
      const user = await authApi.me()
      set({ user, status: 'authed' })
    } catch {
      // A dead token must not leave the UI in a half-authenticated state.
      session.clear()
      set({ user: null, status: 'guest' })
    }
  },

  async login(username, password) {
    set({ pending: true })
    try {
      const res = await authApi.login(username, password)
      session.set(res.tokens)
      set({ user: res.user, status: 'authed' })
    } finally {
      set({ pending: false })
    }
  },

  async register(username, password) {
    set({ pending: true })
    try {
      await authApi.register(username, password)
      const res = await authApi.login(username, password)
      session.set(res.tokens)
      set({ user: res.user, status: 'authed' })
    } finally {
      set({ pending: false })
    }
  },

  async logout() {
    const refreshToken = session.getRefreshToken()
    try {
      await authApi.logout(refreshToken)
    } catch (err) {
      // Logging out locally must succeed even when the network does not.
      if (err instanceof ApiError) console.error('[auth] logout failed', err.code)
    } finally {
      session.clear()
      set({ user: null, status: 'guest' })
    }
  },

  async reload() {
    if (!session.isAuthenticated()) return
    try {
      const user = await authApi.me()
      set({ user, status: 'authed' })
    } catch {
      if (get().status === 'authed') set({ status: 'guest', user: null })
    }
  },

  patchUser(patch) {
    const current = get().user
    if (!current) return
    set({ user: { ...current, ...patch } })
  },
}))
