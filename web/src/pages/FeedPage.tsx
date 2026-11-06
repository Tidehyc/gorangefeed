import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { feedApi } from '@/api/feed'
import { useCursorList } from '@/hooks/useCursorList'
import { fromFeedItem } from '@/lib/normalize'
import { useAuthStore } from '@/stores/auth'
import type { FeedPage as FeedPageData, VideoViewModel } from '@/types/api'
import { VideoStream } from '@/components/feed/VideoStream'
import { FeedSkeleton } from '@/components/ui/Skeleton'
import { ErrorState } from '@/components/ui/States'

type Tab = 'following' | 'recommend'

const TAB_STORAGE = 'grf.feed.tab'

export function FeedPage() {
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)
  const [tab, setTab] = useState<Tab>(() =>
    window.sessionStorage.getItem(TAB_STORAGE) === 'following' ? 'following' : 'recommend',
  )

  useEffect(() => {
    window.sessionStorage.setItem(TAB_STORAGE, tab)
  }, [tab])

  const switchTab = (next: Tab) => {
    if (next === 'following' && status !== 'authed') {
      // /feed/following is hard-auth; a guest must be sent to login, not shown an empty list.
      navigate('/login?redirect=/')
      return
    }
    setTab(next)
  }

  return (
    <div className="relative h-[100dvh] w-full bg-black">
      <header className="pointer-events-none absolute inset-x-0 top-0 z-30 flex justify-center pt-3">
        <div className="pointer-events-auto flex items-center gap-5 rounded-full bg-black/25 px-5 py-1.5 backdrop-blur">
          <TabButton active={tab === 'following'} onClick={() => switchTab('following')}>
            关注
          </TabButton>
          <span className="h-4 w-px bg-white/20" />
          <TabButton active={tab === 'recommend'} onClick={() => switchTab('recommend')}>
            推荐
          </TabButton>
        </div>
      </header>

      {/* key remounts the stream so switching tabs resets scroll, cursor and active slide. */}
      <FeedStream key={tab} tab={tab} />
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`relative py-1 text-[15px] transition-colors ${
        active ? 'font-semibold text-white' : 'text-white/55'
      }`}
    >
      {children}
      {active && (
        <span className="absolute -bottom-0.5 left-1/2 h-[3px] w-5 -translate-x-1/2 rounded-full bg-white" />
      )}
    </button>
  )
}

function FeedStream({ tab }: { tab: Tab }) {
  const navigate = useNavigate()

  const fetcher = useCallback(
    async (cursor: number | string | null): Promise<FeedPageData> =>
      tab === 'recommend' ? feedApi.recommend(cursor, 6) : feedApi.following(cursor, 6),
    [tab],
  )

  const { items, loading, error, hasMore, loadMore, patch, reload } = useCursorList(
    fetcher,
    useMemo(
      () => ({ select: (page: FeedPageData) => page.items.map(fromFeedItem), dedupeKey: (v: VideoViewModel) => v.id }),
      [],
    ),
  )

  if (loading) {
    return (
      <div className="feed-scroller">
        <FeedSkeleton />
        <FeedSkeleton />
        <FeedSkeleton />
      </div>
    )
  }

  if (error && items.length === 0) {
    return (
      <div className="flex h-[100dvh] flex-col items-center justify-center gap-4 px-8">
        <ErrorState message={error} onRetry={() => void reload()} />
        {tab === 'following' && (
          <button type="button" onClick={() => navigate('/')} className="text-[13px] text-mist-500">
            去推荐看看
          </button>
        )}
      </div>
    )
  }

  if (items.length === 0) {
    return (
      <div className="flex h-[100dvh] flex-col items-center justify-center gap-3 px-10 text-center">
        <p className="text-[15px] font-medium text-mist-100">
          {tab === 'following' ? '关注的人还没有作品' : '还没有可播放的内容'}
        </p>
        <p className="text-[13px] leading-5 text-mist-500">
          {tab === 'following'
            ? '去推荐页关注几位创作者，这里就会出现他们的新作品'
            : '数据库里还没有已发布的视频，发布一条试试'}
        </p>
        <div className="mt-2 flex gap-2">
          <button
            type="button"
            onClick={() => navigate('/')}
            className="rounded-full bg-brand px-5 py-2 text-[13px] font-medium text-white"
          >
            去发现
          </button>
          <button
            type="button"
            onClick={() => navigate('/publish')}
            className="rounded-full border border-white/20 px-5 py-2 text-[13px] text-mist-200"
          >
            发布作品
          </button>
        </div>
      </div>
    )
  }

  return (
    <VideoStream
      items={items}
      patch={patch}
      hasMore={hasMore}
      onLoadMore={loadMore}
      storageKey={`grf.feed.index.${tab}`}
    />
  )
}
