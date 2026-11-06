import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { notificationApi } from '@/api/notification'
import { relativeTime } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { NotificationItem } from '@/types/api'
import { PageHeader } from '@/components/PageHeader'
import { Avatar } from '@/components/ui/Avatar'
import { EmptyState, ErrorState } from '@/components/ui/States'
import { Skeleton } from '@/components/ui/Skeleton'

const TYPE_LABEL: Record<string, string> = {
  like: '赞了你',
  comment: '评论了你',
  follow: '关注了你',
  mention: '@了你',
  system: '系统通知',
}

export function NotificationsPage() {
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)
  const [items, setItems] = useState<NotificationItem[]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = async (reset: boolean) => {
    if (reset) setLoading(true)
    else setLoadingMore(true)
    try {
      const page = await notificationApi.list(reset ? null : cursor, 20)
      setItems((prev) => (reset ? page.items : [...prev, ...page.items]))
      setCursor(page.next_cursor)
      setHasMore(page.has_more)
      setError(null)
    } catch {
      setError('通知加载失败')
    } finally {
      setLoading(false)
      setLoadingMore(false)
    }
  }

  useEffect(() => {
    if (status !== 'authed') return
    void load(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status])

  const open = (item: NotificationItem) => {
    if (!item.is_read) {
      setItems((prev) => prev.map((row) => (row.id === item.id ? { ...row, is_read: true } : row)))
      void notificationApi.markRead(item.id).catch(() => undefined)
    }
    if (item.target_type === 'video' && item.target_id > 0) navigate(`/video/${item.target_id}`)
    else if (item.target_type === 'user' && item.actor_id > 0) navigate(`/user/${item.actor_id}`)
  }

  const markAll = async () => {
    try {
      await notificationApi.markAllRead()
      setItems((prev) => prev.map((row) => ({ ...row, is_read: true })))
      toast.success('已全部标记为已读')
    } catch {
      toast.error('操作失败，请重试')
    }
  }

  return (
    <div className="mx-auto min-h-[100dvh] w-full max-w-[640px] pb-24">
      <PageHeader
        title="消息"
        back={false}
        right={
          <button
            type="button"
            onClick={() => void markAll()}
            className="rounded-full px-3 py-1.5 text-[12px] text-mist-400 hover:bg-white/5"
          >
            全部已读
          </button>
        }
      />

      {loading && (
        <div className="space-y-3 px-4 py-4">
          {Array.from({ length: 6 }).map((_, index) => (
            <div key={index} className="flex items-center gap-3">
              <Skeleton className="h-10 w-10 rounded-full" />
              <div className="flex-1 space-y-2">
                <Skeleton className="h-3.5 w-40" />
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && error && <ErrorState message={error} onRetry={() => void load(true)} />}

      {!loading && !error && items.length === 0 && (
        <EmptyState
          title="还没有新消息"
          hint="有人点赞、评论或关注你时，会出现在这里"
          actionText="去刷视频"
          actionTo="/"
        />
      )}

      <ul className="divide-y divide-white/5">
        {items.map((item) => (
          <li key={item.id}>
            <button
              type="button"
              onClick={() => open(item)}
              className={`flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-white/[0.03] ${
                item.is_read ? '' : 'bg-brand/[0.06]'
              }`}
            >
              <Avatar src={item.actor?.avatar_url} name={item.actor?.nickname ?? '系统'} size={40} />
              <div className="min-w-0 flex-1">
                <p className="text-[13px] leading-5 text-mist-300">
                  <span className="font-medium text-mist-100">
                    {item.actor?.nickname || item.title || '系统'}
                  </span>{' '}
                  {TYPE_LABEL[item.type] ?? item.type}
                </p>
                {item.content && (
                  <p className="mt-0.5 line-clamp-2 text-[12px] leading-5 text-mist-500">{item.content}</p>
                )}
                <p className="mt-1 text-[11px] text-mist-700">{relativeTime(item.created_at)}</p>
              </div>
              {!item.is_read && <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-brand" />}
            </button>
          </li>
        ))}
      </ul>

      {!loading && hasMore && items.length > 0 && (
        <div className="py-4 text-center">
          <button
            type="button"
            onClick={() => void load(false)}
            disabled={loadingMore}
            className="text-[13px] text-mist-500 hover:text-mist-200"
          >
            {loadingMore ? '加载中…' : '加载更多'}
          </button>
        </div>
      )}

      {status !== 'authed' && (
        <EmptyState
          title="登录后查看消息"
          actionText="去登录"
          actionTo="/login?redirect=/notifications"
        />
      )}

      <p className="px-4 py-6 text-center text-[11px] text-mist-700">
        想让别人点赞你？<Link to="/publish" className="text-brand">发布一条作品</Link>
      </p>
    </div>
  )
}
