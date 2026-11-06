import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ApiError } from '@/api/client'

export interface CursorPaged {
  // The backend mixes numeric IDs and opaque strings across scenes; never parsed here.
  next_cursor: number | string
  has_more: boolean
}

export interface CursorPage<T> extends CursorPaged {
  items: T[]
}

interface Options<T, P> {
  /** Map the raw page into the rows this list keeps. Defaults to page.items. */
  select?: (page: P) => T[]
  /** Skip the initial load until the caller is ready (e.g. waiting for auth bootstrap). */
  enabled?: boolean
  /** De-duplicate by id: recommend can repeat a video across recall sources. */
  dedupeKey?: (item: T) => string | number
}

export interface CursorList<T> {
  items: T[]
  loading: boolean
  loadingMore: boolean
  error: string | null
  hasMore: boolean
  loadMore: () => Promise<void>
  reload: () => Promise<void>
  patch: (predicate: (item: T) => boolean, updater: (item: T) => T) => void
  prepend: (item: T) => void
}

/**
 * Cursor pagination state machine shared by every list in the app.
 *
 * Two type parameters because the endpoints disagree on the envelope: /feed/* returns
 * { items }, /users/:id/videos returns { videos }. The cursor itself is always opaque.
 */
export function useCursorList<T, P extends CursorPaged = CursorPage<T>>(
  fetcher: (cursor: number | string | null) => Promise<P>,
  options: Options<T, P> = {},
): CursorList<T> {
  const { select, enabled = true, dedupeKey } = options

  const [items, setItems] = useState<T[]>([])
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(true)

  const cursorRef = useRef<number | string | null>(null)
  const seenRef = useRef<Set<string | number>>(new Set())
  const requestRef = useRef(0)
  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher
  const selectRef = useRef(select)
  selectRef.current = select
  const dedupeRef = useRef(dedupeKey)
  dedupeRef.current = dedupeKey

  const fetchPage = useCallback(async (reset: boolean) => {
    const requestId = ++requestRef.current
    if (reset) {
      setLoading(true)
      setError(null)
    } else {
      setLoadingMore(true)
    }

    try {
      const page = await fetcherRef.current(reset ? null : cursorRef.current)
      if (requestId !== requestRef.current) return

      const rows = selectRef.current
        ? selectRef.current(page)
        : ((page as unknown as CursorPage<T>).items ?? [])
      const key = dedupeRef.current
      const fresh = key
        ? rows.filter((item) => {
            const id = key(item)
            if (seenRef.current.has(id)) return false
            seenRef.current.add(id)
            return true
          })
        : rows

      setItems((prev) => (reset ? fresh : [...prev, ...fresh]))
      cursorRef.current = page.next_cursor
      setHasMore(Boolean(page.has_more))
    } catch (err) {
      if (requestId !== requestRef.current) return
      const message = err instanceof ApiError ? err.friendlyMessage : '加载失败，请点击重试'
      setError(message)
      // Stop the infinite-scroll trigger from hammering a failing endpoint.
      if (!reset) setHasMore(false)
    } finally {
      if (requestId === requestRef.current) {
        setLoading(false)
        setLoadingMore(false)
      }
    }
  }, [])

  const reload = useCallback(async () => {
    cursorRef.current = null
    seenRef.current = new Set()
    setHasMore(true)
    await fetchPage(true)
  }, [fetchPage])

  const loadMore = useMemo(
    () => async () => {
      if (loading || loadingMore || !hasMore) return
      await fetchPage(false)
    },
    [fetchPage, hasMore, loading, loadingMore],
  )

  useEffect(() => {
    if (!enabled) return
    void reload()
    return () => {
      // Invalidate in-flight responses from the previous key (tab switch).
      requestRef.current += 1
    }
  }, [enabled, reload])

  const patch = useCallback((predicate: (item: T) => boolean, updater: (item: T) => T) => {
    setItems((prev) => prev.map((item) => (predicate(item) ? updater(item) : item)))
  }, [])

  const prepend = useCallback((item: T) => {
    setItems((prev) => [item, ...prev])
  }, [])

  return { items, loading, loadingMore, error, hasMore, loadMore, reload, patch, prepend }
}
