import { useCallback } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { feedApi } from '@/api/feed'
import { useCursorList } from '@/hooks/useCursorList'
import { fromFeedItem } from '@/lib/normalize'
import type { FeedPage } from '@/types/api'
import { VideoStream } from '@/components/feed/VideoStream'
import { BackIcon } from '@/components/icons'
import { FeedSkeleton } from '@/components/ui/Skeleton'
import { ErrorState } from '@/components/ui/States'

export function TagPage() {
  const { tag = '' } = useParams<{ tag: string }>()
  const navigate = useNavigate()

  const fetcher = useCallback(
    async (cursor: number | string | null): Promise<FeedPage> => feedApi.byTag(tag, cursor, 6),
    [tag],
  )

  const { items, loading, error, hasMore, loadMore, patch, reload } = useCursorList(
    fetcher,
    { select: (page) => page.items.map(fromFeedItem), dedupeKey: (video) => video.id },
  )

  return (
    <div className="relative h-[100dvh] w-full bg-black">
      <header className="pointer-events-none absolute inset-x-0 top-0 z-30 flex items-center gap-2 px-3 pt-3">
        <button
          type="button"
          onClick={() => (window.history.length > 1 ? navigate(-1) : navigate('/'))}
          aria-label="返回"
          className="pointer-events-auto rounded-full bg-black/40 p-2 text-white/90 backdrop-blur"
        >
          <BackIcon size={20} />
        </button>
        <div className="pointer-events-auto truncate rounded-full bg-black/40 px-4 py-1.5 text-[14px] font-medium text-white backdrop-blur">
          #{tag}
        </div>
      </header>

      {loading && (
        <div className="feed-scroller">
          <FeedSkeleton />
        </div>
      )}

      {!loading && error && items.length === 0 && (
        <div className="flex h-[100dvh] flex-col items-center justify-center gap-4 px-8">
          <ErrorState message={error} onRetry={() => void reload()} />
        </div>
      )}

      {!loading && !error && items.length === 0 && (
        <div className="flex h-[100dvh] flex-col items-center justify-center gap-3 px-10 text-center">
          <p className="text-[15px] font-medium text-mist-100">这个话题下还没有视频</p>
          <p className="text-[13px] text-mist-500">发布作品时带上 #{tag}，它会出现在这里</p>
          <button
            type="button"
            onClick={() => navigate('/publish')}
            className="mt-2 rounded-full bg-brand px-5 py-2 text-[13px] font-medium text-white"
          >
            发布作品
          </button>
        </div>
      )}

      {!loading && items.length > 0 && (
        <VideoStream
          items={items}
          patch={patch}
          hasMore={hasMore}
          onLoadMore={loadMore}
          storageKey={`grf.tag.index.${tag}`}
        />
      )}
    </div>
  )
}
