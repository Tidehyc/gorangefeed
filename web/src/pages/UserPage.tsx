import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '@/api/client'
import { relationApi } from '@/api/relation'
import { userApi } from '@/api/user'
import { videoApi } from '@/api/video'
import { useCursorList } from '@/hooks/useCursorList'
import { fromVideoDetail } from '@/lib/normalize'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { PublicUserProfile } from '@/types/api'
import { PageHeader } from '@/components/PageHeader'
import { ProfileHeader } from '@/components/ProfileHeader'
import { VideoGrid } from '@/components/VideoGrid'
import { ErrorState } from '@/components/ui/States'
import { Skeleton } from '@/components/ui/Skeleton'

export function UserPage() {
  const { id } = useParams<{ id: string }>()
  const userId = Number(id)
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)

  const [profile, setProfile] = useState<PublicUserProfile | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [isFollowing, setIsFollowing] = useState(false)
  const [followPending, setFollowPending] = useState(false)
  const [followerCount, setFollowerCount] = useState(0)

  useEffect(() => {
    if (!Number.isFinite(userId) || userId <= 0) {
      setError('用户不存在')
      setLoading(false)
      return
    }
    let alive = true
    setLoading(true)
    setError(null)

    userApi
      .publicProfile(userId)
      .then((data) => {
        if (!alive) return
        setProfile(data)
        setFollowerCount(data.stats.follower_count)
      })
      .catch((err: unknown) => {
        if (!alive) return
        setError(err instanceof ApiError ? err.friendlyMessage : '加载失败')
      })
      .finally(() => {
        if (alive) setLoading(false)
      })

    return () => {
      alive = false
    }
  }, [userId])

  // Public profile does not carry follow state (G4), so an authenticated viewer asks /relation.
  useEffect(() => {
    if (status !== 'authed' || !Number.isFinite(userId) || userId <= 0) return
    let alive = true
    relationApi
      .relation(userId)
      .then((res) => {
        if (alive) setIsFollowing(res.is_following)
      })
      .catch(() => undefined)
    return () => {
      alive = false
    }
  }, [status, userId])

  const toggleFollow = async () => {
    if (status !== 'authed') {
      toast.info('登录后才能关注')
      navigate(`/login?redirect=/user/${userId}`)
      return
    }
    const next = !isFollowing
    setFollowPending(true)
    setIsFollowing(next)
    setFollowerCount((count) => Math.max(0, count + (next ? 1 : -1)))
    try {
      if (next) await relationApi.follow(userId)
      else await relationApi.unfollow(userId)
      toast.success(next ? '已关注' : '已取消关注')
    } catch (err) {
      setIsFollowing(!next)
      setFollowerCount((count) => Math.max(0, count + (next ? -1 : 1)))
      toast.error(err instanceof ApiError ? err.friendlyMessage : '操作失败，请重试')
    } finally {
      setFollowPending(false)
    }
  }

  const list = useCursorList(
    (cursor) => videoApi.authorVideos(userId, typeof cursor === 'number' ? cursor : null, 12),
    {
      enabled: Number.isFinite(userId) && userId > 0,
      select: (page) => page.videos.map(fromVideoDetail),
      dedupeKey: (video) => video.id,
    },
  )

  if (loading) {
    return (
      <div className="mx-auto max-w-[720px]">
        <PageHeader title="用户主页" />
        <div className="flex items-center gap-4 px-4 py-4">
          <Skeleton className="h-[76px] w-[76px] rounded-full" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-5 w-32" />
            <Skeleton className="h-3 w-24" />
          </div>
        </div>
      </div>
    )
  }

  if (error || !profile) {
    return (
      <div className="mx-auto max-w-[720px]">
        <PageHeader title="用户主页" />
        <ErrorState message={error ?? '用户不存在'} onRetry={() => window.location.reload()} />
      </div>
    )
  }

  return (
    <div className="mx-auto min-h-[100dvh] w-full max-w-[720px] pb-20">
      <PageHeader title={profile.nickname} />
      <ProfileHeader
        userId={userId}
        nickname={profile.nickname}
        avatarUrl={profile.avatar_url}
        bio={profile.bio}
        stats={{
          ...profile.stats,
          follower_count: followerCount,
        }}
        isSelf={false}
        isFollowing={isFollowing}
        followPending={followPending}
        onToggleFollow={() => void toggleFollow()}
      />

      <div className="border-b border-white/5 px-4 pb-2 pt-1">
        <span className="text-[14px] font-semibold text-mist-100">作品</span>
      </div>

      <VideoGrid
        items={list.items}
        loading={list.loading}
        error={list.error}
        hasMore={list.hasMore}
        loadingMore={list.loadingMore}
        onLoadMore={() => void list.loadMore()}
        onRetry={() => void list.reload()}
        emptyTitle="TA 还没有作品"
        emptyHint="等 TA 发布第一条视频吧"
      />
    </div>
  )
}
