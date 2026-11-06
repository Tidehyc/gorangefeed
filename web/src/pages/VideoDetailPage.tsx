import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '@/api/client'
import { feedApi } from '@/api/feed'
import { interactionApi } from '@/api/interaction'
import { relationApi } from '@/api/relation'
import { videoApi } from '@/api/video'
import { fromVideoDetail } from '@/lib/normalize'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { VideoViewModel } from '@/types/api'
import { CommentSheet } from '@/components/feed/CommentSheet'
import { VideoSlide } from '@/components/feed/VideoSlide'
import { BackIcon } from '@/components/icons'
import { Spinner } from '@/components/ui/Spinner'
import { ErrorState } from '@/components/ui/States'

export function VideoDetailPage() {
  const { id } = useParams<{ id: string }>()
  const videoId = Number(id)
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)

  const [video, setVideo] = useState<VideoViewModel | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [muted, setMuted] = useState(true)
  const [commentsOpen, setCommentsOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const detail = await videoApi.detail(videoId)
      setVideo(fromVideoDetail(detail))
    } catch (err) {
      setError(err instanceof ApiError ? err.friendlyMessage : '视频不存在或已被删除')
    } finally {
      setLoading(false)
    }
  }, [videoId])

  useEffect(() => {
    if (!Number.isFinite(videoId) || videoId <= 0) {
      setError('视频不存在')
      setLoading(false)
      return
    }
    void load()
  }, [videoId, load])

  const patch = (updater: (current: VideoViewModel) => VideoViewModel) => {
    setVideo((current) => (current ? updater(current) : current))
  }

  const requireLogin = () => {
    if (status === 'authed') return true
    toast.info('登录后才能互动')
    navigate(`/login?redirect=/video/${videoId}`)
    return false
  }

  const toggleLike = () => {
    if (!video || !requireLogin()) return
    const next = !video.isLiked
    patch((current) => ({
      ...current,
      isLiked: next,
      likeCount: Math.max(0, current.likeCount + (next ? 1 : -1)),
    }))
    const request = next ? interactionApi.like(video.id) : interactionApi.unlike(video.id)
    request.catch(() => {
      patch((current) => ({ ...current, isLiked: !next, likeCount: video.likeCount }))
      toast.error('操作失败，请重试')
    })
  }

  const toggleFavorite = () => {
    if (!video || !requireLogin()) return
    const next = !video.isFavorited
    patch((current) => ({
      ...current,
      isFavorited: next,
      favoriteCount: Math.max(0, current.favoriteCount + (next ? 1 : -1)),
    }))
    const request = next ? interactionApi.favorite(video.id) : interactionApi.unfavorite(video.id)
    request.catch(() => {
      patch((current) => ({ ...current, isFavorited: !next, favoriteCount: video.favoriteCount }))
      toast.error('操作失败，请重试')
    })
  }

  const toggleFollow = () => {
    if (!video || !requireLogin()) return
    const next = !video.isFollowed
    patch((current) => ({ ...current, isFollowed: next }))
    const request = next ? relationApi.follow(video.authorId) : relationApi.unfollow(video.authorId)
    request.catch(() => {
      patch((current) => ({ ...current, isFollowed: !next }))
      toast.error('操作失败，请重试')
    })
  }

  const share = async () => {
    if (!video) return
    const url = window.location.href
    try {
      if (navigator.share) {
        await navigator.share({ title: video.title || 'GoRangeFeed', url })
        return
      }
      await navigator.clipboard.writeText(url)
      toast.success('链接已复制')
    } catch {
      toast.info(url)
    }
  }

  if (loading) {
    return (
      <div className="flex h-[100dvh] items-center justify-center bg-black text-white">
        <Spinner size={24} />
      </div>
    )
  }

  if (error || !video) {
    return (
      <div className="flex h-[100dvh] flex-col items-center justify-center gap-4 bg-black">
        <ErrorState message={error ?? '视频不存在'} onRetry={() => void load()} />
        <button type="button" onClick={() => navigate('/')} className="text-[13px] text-mist-500">
          回首页
        </button>
      </div>
    )
  }

  return (
    <div className="relative mx-auto flex h-[100dvh] w-full max-w-[520px] justify-center bg-black lg:my-0">
      <button
        type="button"
        onClick={() => (window.history.length > 1 ? navigate(-1) : navigate('/'))}
        aria-label="返回"
        className="absolute left-3 top-4 z-30 rounded-full bg-black/40 p-2 text-white/90 backdrop-blur"
      >
        <BackIcon size={20} />
      </button>

      <VideoSlide
        video={video}
        active
        muted={muted}
        onToggleMute={() => setMuted((prev) => !prev)}
        onLike={toggleLike}
        onComment={() => setCommentsOpen(true)}
        onFavorite={toggleFavorite}
        onShare={() => void share()}
        onFollow={toggleFollow}
        onOpenAuthor={() => navigate(`/user/${video.authorId}`)}
        onOpenTag={(tag) => navigate(`/tag/${encodeURIComponent(tag)}`)}
        onFirstPlay={(target) => void feedApi.reportView(target.id, 'play', 0)}
      />

      <CommentSheet
        videoId={video.id}
        open={commentsOpen}
        onClose={() => setCommentsOpen(false)}
        onCountChange={(delta) =>
          patch((current) => ({ ...current, commentCount: Math.max(0, current.commentCount + delta) }))
        }
      />
    </div>
  )
}
