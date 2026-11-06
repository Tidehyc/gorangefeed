import type { Envelope, TokenPair } from '@/types/api'
import { session } from './tokens'

export const API_BASE = '/api/v1'

export class ApiError extends Error {
  readonly code: string
  readonly status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
  }

  /** Messages are user-facing Chinese; the switch keeps the mapping in one place (FR-UX-03). */
  get friendlyMessage(): string {
    switch (this.code) {
      case 'unauthorized':
        return '登录状态已失效，请重新登录'
      case 'forbidden':
        return '没有权限执行该操作'
      case 'not_found':
        return '内容不存在或已被删除'
      case 'rate_limited':
        return '操作太频繁了，稍后再试'
      case 'conflict':
        return '操作冲突，请刷新后重试'
      case 'invalid_param':
        return this.message || '请求参数有误'
      default:
        return this.message || '服务异常，请稍后重试'
    }
  }
}

interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
  /** Multipart uploads pass FormData and must not have a JSON content type. */
  form?: FormData
  /** Skip the Authorization header even when a token exists (login/register). */
  anonymous?: boolean
  /** Skip the 401-refresh dance (refresh itself, logout). */
  noRetry?: boolean
}

// Single-flight refresh: a burst of 401s must produce one refresh call, not one per request
// (NFR-FEREL-02).
let refreshInFlight: Promise<boolean> | null = null

async function refreshTokens(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight

  refreshInFlight = (async () => {
    const token = session.getRefreshToken()
    if (!token) return false
    try {
      const res = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: token }),
      })
      const payload = (await res.json()) as Envelope<TokenPair>
      if (!res.ok || !payload.data) return false
      session.set(payload.data)
      return true
    } catch {
      return false
    } finally {
      // Cleared in a microtask so concurrent callers still observe the same promise result.
      setTimeout(() => {
        refreshInFlight = null
      }, 0)
    }
  })()

  return refreshInFlight
}

function buildHeaders(options: RequestOptions): Headers {
  const headers = new Headers(options.headers)
  if (!options.form && options.body !== undefined) {
    headers.set('Content-Type', 'application/json')
  }
  if (!options.anonymous) {
    const token = session.getAccessToken()
    if (token) headers.set('Authorization', `Bearer ${token}`)
  }
  return headers
}

async function send(path: string, options: RequestOptions, retried: boolean): Promise<Response> {
  const headers = buildHeaders(options)
  const init: RequestInit = {
    method: options.method ?? (options.body !== undefined || options.form ? 'POST' : 'GET'),
    headers,
    body: options.form ?? (options.body !== undefined ? JSON.stringify(options.body) : undefined),
    signal: options.signal,
  }

  let res: Response
  try {
    res = await fetch(`${API_BASE}${path}`, init)
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError('network', '网络连接失败，请检查网络后重试', 0)
  }

  if (res.status === 401 && !retried && !options.noRetry) {
    const refreshed = await refreshTokens()
    if (refreshed) return send(path, options, true)
    session.clear()
    session.notifyExpired()
  }
  return res
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const res = await send(path, options, false)

  if (res.status === 204) return undefined as T

  let payload: Envelope<T> | null = null
  try {
    payload = (await res.json()) as Envelope<T>
  } catch {
    payload = null
  }

  if (!res.ok || !payload || payload.error) {
    const code = payload?.error?.code ?? httpFallbackCode(res.status)
    const message = payload?.error?.message ?? res.statusText ?? ''
    if (code === 'unauthorized') {
      session.clear()
      session.notifyExpired()
    }
    // NFR-FEOBS-02: log with the request id so a frontend report can be traced in backend logs.
    const requestId = res.headers.get('x-request-id')
    console.error(`[api] ${options.method ?? 'GET'} ${path} failed`, code, requestId ?? '')
    throw new ApiError(code, message, res.status)
  }

  return payload.data as T
}

function httpFallbackCode(status: number): string {
  if (status === 401) return 'unauthorized'
  if (status === 403) return 'forbidden'
  if (status === 404) return 'not_found'
  if (status === 409) return 'conflict'
  if (status === 429) return 'rate_limited'
  return 'internal'
}

export const api = {
  get<T>(path: string, params?: Record<string, string | number | undefined | null>) {
    const query = new URLSearchParams()
    if (params) {
      for (const [key, value] of Object.entries(params)) {
        if (value === undefined || value === null || value === '') continue
        query.set(key, String(value))
      }
    }
    const suffix = query.toString() ? `?${query.toString()}` : ''
    return request<T>(`${path}${suffix}`)
  },
  post<T>(path: string, body?: unknown, headers?: Record<string, string>) {
    return request<T>(path, { method: 'POST', body: body ?? {}, headers })
  },
  put<T>(path: string, body?: unknown) {
    return request<T>(path, { method: 'PUT', body: body ?? {} })
  },
  del<T>(path: string, body?: unknown) {
    return request<T>(path, { method: 'DELETE', body })
  },
  patch<T>(path: string, body?: unknown) {
    return request<T>(path, { method: 'PATCH', body: body ?? {} })
  },
  form<T>(path: string, form: FormData) {
    return request<T>(path, { method: 'POST', form })
  },
}
