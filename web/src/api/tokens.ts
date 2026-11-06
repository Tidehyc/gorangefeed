// Token persistence. sessionStorage (not localStorage) per NFR-FESEC-02: the access token must
// not survive the tab, and must never be written to the URL or the console.
import type { TokenPair } from '@/types/api'

const KEY = 'grf.session'

interface Persisted {
  accessToken: string
  refreshToken: string
}

let accessToken: string | null = null
let refreshToken: string | null = null
let expireHandler: (() => void) | null = null

function readStore(): Persisted | null {
  try {
    const raw = window.sessionStorage.getItem(KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Persisted
    if (!parsed.accessToken || !parsed.refreshToken) return null
    return parsed
  } catch {
    return null
  }
}

function hydrate() {
  const stored = readStore()
  accessToken = stored?.accessToken ?? null
  refreshToken = stored?.refreshToken ?? null
}

hydrate()

export const session = {
  getAccessToken(): string | null {
    return accessToken
  },
  getRefreshToken(): string | null {
    return refreshToken
  },
  isAuthenticated(): boolean {
    return Boolean(accessToken)
  },
  set(pair: TokenPair) {
    accessToken = pair.access_token
    refreshToken = pair.refresh_token
    try {
      window.sessionStorage.setItem(
        KEY,
        JSON.stringify({ accessToken, refreshToken } satisfies Persisted),
      )
    } catch {
      // Private mode with storage disabled: the session lives in memory only.
    }
  },
  clear() {
    accessToken = null
    refreshToken = null
    try {
      window.sessionStorage.removeItem(KEY)
    } catch {
      // ignore
    }
  },
  onExpire(handler: (() => void) | null) {
    expireHandler = handler
  },
  notifyExpired() {
    expireHandler?.()
  },
}
