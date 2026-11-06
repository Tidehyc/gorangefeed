import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ApiError } from '@/api/client'
import { feedApi } from '@/api/feed'
import { interactionApi } from '@/api/interaction'
import { relationApi } from '@/api/relation'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { VideoViewModel } from '@/types/api'
import { CommentSheet } from './CommentSheet'
import { VideoSlide } from './VideoSlide'

interface Props {
  items: VideoViewModel[]
  patch: (predicate: (item: VideoViewModel) => boolean, updater: (item: VideoViewModel) => VideoViewModel) => void
  hasMore: boolean
  onLoadMore: () => Promise<void>
  /** sessionStorage key so returning to the feed resumes the same slide. */
  storageKey: string
}

/**
 * The scroll-snapping stream plus every interaction handler. Shared by the home feed and the
 * topic page so the like/favorite/follow rules exist in exactly one place.
 */
export function VideoStream({ items, patch, hasMore, onLoadMore, storageKey }: Props) {
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)
  const scrollerRef = useRef<HTMLDivElement>(null)
  const [activeIndex, setActiveIndex] = useState(0)
  const [muted, setMuted] = useState(true)
  const [commentTarget, setCommentTarget] = useState<VideoViewModel | null>(null)
  const restored = useRef(false)

  useEffect(() => {
    if (restored.current || items.length === 0) return
    restored.current = true
    const saved = Number(window.sessionStorage.getItem(storageKey) ?? '0')
    if (saved > 0 && saved < items.length) {
      const scroller = scrollerRef.current
      if (scroller) {
        scroller.scrollTop = saved * scroller.clientHeight
        setActiveIndex(saved)
      }
    }
  }, [items.length, storageKey])

  const handleScroll = useCallback(() => {
    const scroller = scrollerRef.current
    if (!scroller) return
    const height = scroller.clientHeight || 1
    const index = Math.min(items.length - 1, Math.max(0, Math.round(scroller.scrollTop / height)))
    setActiveIndex((prev) => {
      if (prev === index) return prev
      window.sessionStorage.setItem(storageKey, String(index))
      return index
    })
  }, [items.length, storageKey])

  useEffect(() => {
    if (items.length === 0 || !hasMore) return
    if (activeIndex >= items.length - 2) void onLoadMore()
  }, [activeIndex, items.length, hasMore, onLoadMore])

  const requireLogin = (): boolean => {
    if (status === 'authed') return true
    toast.info('登录后才能互动')
    navigate('/login?redirect=/')
    return false
  }

  const toggleLike = (video: VideoViewModel) => {
    if (!requireLogin()) return
    const next = !video.isLiked
    patch(
      (item) => item.id === video.id,
      (item) => ({ ...item, isLiked: next, likeCount: Math.max(0, item.likeCount + (next ? 1 : -1)) }),
    )
    const request = next ? interactionApi.like(video.id) : interactionApi.unlike(video.id)
    request.catch((err: unknown) => {
      patch(
        (item) => item.id === video.id,
        (item) => ({ ...item, isLiked: video.isLiked, likeCount: video.likeCount }),
      )
      toast.error(err instanceof ApiError ? err.friendlyMessage : '操作失败，请重试')
    })
  }

  const toggleFavorite = (video: VideoViewModel) => {
    if (!requireLogin()) return
    const next = !video.isFavorited
    patch(
      (item) => item.id === video.id,
      (item) => ({
        ...item,
        isFavorited: next,
        favoriteCount: Math.max(0, item.favoriteCount + (next ? 1 : -1)),
      }),
    )
    const request = next ? interactionApi.favorite(video.id) : interactionApi.unfavorite(video.id)
    request.catch((err: unknown) => {
      patch(
        (item) => item.id === video.id,
        (item) => ({ ...item, isFavorited: video.isFavorited, favoriteCount: video.favoriteCount }),
      )
      toast.error(err instanceof ApiError ? err.friendlyMessage : '收藏失败，请重试')
    })
  }

  const toggleFollow = (video: VideoViewModel) => {
    if (!requireLogin()) return
    const next = !video.isFollowed
    patch(
      (item) => item.authorId === video.authorId,
      (item) => ({ ...item, isFollowed: next }),
    )
    const request = next ? relationApi.follow(video.authorId) : relationApi.unfollow(video.authorId)
    request.then(
      () => toast.success(next ? '已关注' : '已取消关注'),
      (err: unknown) => {
        patch(
          (item) => item.authorId === video.authorId,
          (item) => ({ ...item, isFollowed: video.isFollowed }),
        )
        toast.error(err instanceof ApiError ? err.friendlyMessage : '关注失败，请重试')
      },
    )
  }

  const share = async (video: VideoViewModel) => {
    const url = `${window.location.origin}/video/${video.id}`
    try {
      if (navigator.share) {
        await navigator.share({ title: video.title || 'GoRangeFeed', text: video.description, url })
        return
      }
      await navigator.clipboard.writeText(url)
      toast.success('链接已复制')
    } catch {
      toast.info(url)
    }
  }

  return (
    <>
      <div ref={scrollerRef} onScroll={handleScroll} className="feed-scroller">
        {items.map((video, index) => (
          <VideoSlide
            key={`${video.id}-${index}`}
            video={video}
            active={index === activeIndex}
            muted={muted}
            onToggleMute={() => setMuted((prev) => !prev)}
            onLike={toggleLike}
            onComment={setCommentTarget}
            onFavorite={toggleFavorite}
            onShare={(target) => void share(target)}
            onFollow={toggleFollow}
            onOpenAuthor={(target) => navigate(`/user/${target.authorId}`)}
            onOpenTag={(tag) => navigate(`/tag/${encodeURIComponent(tag)}`)}
            onFirstPlay={(target) => void feedApi.reportView(target.id, 'play', 0)}
          />
        ))}

        <div className="feed-slide flex items-center justify-center bg-ink-900">
          {hasMore ? (
            <div className="flex flex-col items-center gap-2 text-mist-500">
              <span className="h-6 w-6 animate-spin rounded-full border-2 border-current border-t-transparent" />
              <span className="text-[12px]">正在加载更多…</span>
            </div>
          ) : (
            <p className="text-[13px] text-mist-600">已经到底了</p>
          )}
        </div>
      </div>

      <CommentSheet
        videoId={commentTarget?.id ?? null}
        open={commentTarget !== null}
        onClose={() => setCommentTarget(null)}
        onCountChange={(delta) => {
          if (!commentTarget) return
          patch(
            (item) => item.id === commentTarget.id,
            (item) => ({ ...item, commentCount: Math.max(0, item.commentCount + delta) }),
          )
        }}
      />
    </>
  )
}
