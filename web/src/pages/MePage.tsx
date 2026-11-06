import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { authApi } from '@/api/auth'
import { ApiError } from '@/api/client'
import { videoApi } from '@/api/video'
import { useCursorList } from '@/hooks/useCursorList'
import { fromFeedItem, fromVideoDetail } from '@/lib/normalize'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { UserProfile } from '@/types/api'
import { PageHeader } from '@/components/PageHeader'
import { ProfileHeader } from '@/components/ProfileHeader'
import { VideoGrid } from '@/components/VideoGrid'
import { Button } from '@/components/ui/Button'
import { SettingsIcon } from '@/components/icons'

type Tab = 'works' | 'likes'

export function MePage() {
  const navigate = useNavigate()
  const storeUser = useAuthStore((state) => state.user)
  const patchUser = useAuthStore((state) => state.patchUser)
  const [profile, setProfile] = useState<UserProfile | null>(storeUser)
  const [tab, setTab] = useState<Tab>('works')

  // The cached profile from login is enough to paint, but stats only become real through a fresh
  // /users/me call.
  useEffect(() => {
    let alive = true
    authApi
      .me()
      .then((fresh) => {
        if (!alive) return
        setProfile(fresh)
        patchUser(fresh)
      })
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.code !== 'unauthorized') {
          toast.error(err.friendlyMessage)
        }
      })
    return () => {
      alive = false
    }
  }, [patchUser])

  const userId = profile?.id ?? storeUser?.id ?? 0

  return (
    <div className="mx-auto min-h-[100dvh] w-full max-w-[720px] pb-20">
      <PageHeader
        title="我的主页"
        back={false}
        right={
          <Link
            to="/settings"
            className="rounded-full p-2 text-mist-300 hover:bg-white/5"
            aria-label="设置"
          >
            <SettingsIcon size={20} />
          </Link>
        }
      />

      <ProfileHeader
        userId={userId}
        nickname={profile?.nickname ?? ''}
        username={profile?.username}
        avatarUrl={profile?.avatar_url}
        bio={profile?.bio}
        stats={profile?.stats}
        isSelf
        isFollowing={false}
        actions={
          <>
            <Button variant="ghost" size="md" onClick={() => navigate('/settings')}>
              编辑资料
            </Button>
            <Button variant="outline" size="md" onClick={() => navigate('/publish')}>
              发布作品
            </Button>
          </>
        }
      />

      <ProfileTabs tab={tab} onChange={setTab} />

      {tab === 'works' ? (
        <WorksTab userId={userId} />
      ) : (
        <LikesTab />
      )}
    </div>
  )
}

function ProfileTabs({ tab, onChange }: { tab: Tab; onChange: (tab: Tab) => void }) {
  return (
    <div className="sticky top-14 z-20 flex border-b border-white/5 bg-ink-900/95 backdrop-blur">
      {(
        [
          ['works', '作品'],
          ['likes', '喜欢'],
        ] as [Tab, string][]
      ).map(([key, label]) => (
        <button
          key={key}
          type="button"
          onClick={() => onChange(key)}
          className={`relative flex-1 py-3 text-[14px] transition-colors ${
            tab === key ? 'font-semibold text-mist-100' : 'text-mist-500'
          }`}
        >
          {label}
          {tab === key && (
            <span className="absolute bottom-0 left-1/2 h-[2px] w-8 -translate-x-1/2 rounded-full bg-brand" />
          )}
        </button>
      ))}
    </div>
  )
}

function WorksTab({ userId }: { userId: number }) {
  const list = useCursorList(
    (cursor) => videoApi.authorVideos(userId, typeof cursor === 'number' ? cursor : null, 12),
    {
      enabled: userId > 0,
      select: (page) => page.videos.map(fromVideoDetail),
      dedupeKey: (video) => video.id,
    },
  )

  return (
    <VideoGrid
      items={list.items}
      loading={list.loading}
      error={list.error}
      hasMore={list.hasMore}
      loadingMore={list.loadingMore}
      onLoadMore={() => void list.loadMore()}
      onRetry={() => void list.reload()}
      emptyTitle="还没有作品"
      emptyHint="点「发布作品」上传第一条视频，它会出现在这里"
    />
  )
}

function LikesTab() {
  const list = useCursorList(
    (cursor) => videoApi.myLikes(typeof cursor === 'number' ? cursor : null, 12),
    {
      select: (page) => page.items.map(fromFeedItem),
      dedupeKey: (video) => video.id,
    },
  )

  return (
    <VideoGrid
      items={list.items}
      loading={list.loading}
      error={list.error}
      hasMore={list.hasMore}
      loadingMore={list.loadingMore}
      onLoadMore={() => void list.loadMore()}
      onRetry={() => void list.reload()}
      emptyTitle="还没有喜欢的作品"
      emptyHint="在推荐流里双击视频，喜欢的内容会出现在这里"
    />
  )
}
