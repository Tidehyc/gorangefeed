import { Link } from 'react-router-dom'
import type { VideoViewModel } from '@/types/api'
import { formatCount, gradientFor } from '@/lib/format'
import { Skeleton } from './ui/Skeleton'
import { EmptyState, ErrorState } from './ui/States'

interface Props {
  items: VideoViewModel[]
  loading?: boolean
  error?: string | null
  hasMore?: boolean
  loadingMore?: boolean
  onLoadMore?: () => void
  onRetry?: () => void
  emptyTitle?: string
  emptyHint?: string
  /** The tag page shows a duration badge; the profile grid keeps it clean like Douyin. */
  showViews?: boolean
}

export function VideoGrid({
  items,
  loading = false,
  error = null,
  hasMore = false,
  loadingMore = false,
  onLoadMore,
  onRetry,
  emptyTitle = '还没有作品',
  emptyHint,
  showViews = true,
}: Props) {
  if (loading) {
    return (
      <div className="grid grid-cols-3 gap-0.5">
        {Array.from({ length: 9 }).map((_, index) => (
          <Skeleton key={index} className="aspect-[3/4] w-full rounded-none" />
        ))}
      </div>
    )
  }

  if (error) return <ErrorState message={error} onRetry={onRetry} />

  if (items.length === 0) return <EmptyState title={emptyTitle} hint={emptyHint} />

  return (
    <div>
      <div className="grid grid-cols-3 gap-0.5">
        {items.map((video) => (
          <Link
            key={video.id}
            to={`/video/${video.id}`}
            className="group relative aspect-[3/4] overflow-hidden bg-ink-700"
          >
            {video.coverUrl ? (
              <img
                src={video.coverUrl}
                alt={video.title || '视频封面'}
                loading="lazy"
                className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
              />
            ) : (
              <div
                className={`flex h-full w-full items-center justify-center bg-gradient-to-br ${gradientFor(
                  video.id,
                )} p-2 text-center`}
              >
                <span className="line-clamp-3 text-[12px] font-medium text-white/90">
                  {video.title || '无封面'}
                </span>
              </div>
            )}
            {showViews && (
              <span className="absolute bottom-1 left-1.5 flex items-center gap-0.5 text-[11px] text-white/90 text-shadow-hero">
                ▶ {formatCount(video.viewCount)}
              </span>
            )}
          </Link>
        ))}
      </div>

      {hasMore && (
        <div className="py-4 text-center">
          <button
            type="button"
            onClick={onLoadMore}
            disabled={loadingMore}
            className="text-[13px] text-mist-500 hover:text-mist-200"
          >
            {loadingMore ? '加载中…' : '加载更多'}
          </button>
        </div>
      )}

      {!hasMore && items.length > 0 && (
        <p className="py-5 text-center text-[12px] text-mist-700">没有更多了</p>
      )}
    </div>
  )
}
