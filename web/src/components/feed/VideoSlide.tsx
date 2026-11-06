import { useEffect, useRef, useState } from 'react'
import type { VideoViewModel } from '@/types/api'
import { formatCount, formatDuration, relativeTime } from '@/lib/format'
import { Avatar } from '../ui/Avatar'
import {
  CommentIcon,
  HeartIcon,
  PlayIcon,
  ShareIcon,
  StarIcon,
  VolumeOffIcon,
  VolumeOnIcon,
} from '../icons'

interface Props {
  video: VideoViewModel
  active: boolean
  muted: boolean
  onToggleMute: () => void
  onLike: (video: VideoViewModel) => void
  onComment: (video: VideoViewModel) => void
  onFavorite: (video: VideoViewModel) => void
  onShare: (video: VideoViewModel) => void
  onFollow: (video: VideoViewModel) => void
  onOpenAuthor: (video: VideoViewModel) => void
  onOpenTag: (tag: string) => void
  onFirstPlay: (video: VideoViewModel) => void
}

const HEART_LIFETIME = 900

export function VideoSlide({
  video,
  active,
  muted,
  onToggleMute,
  onLike,
  onComment,
  onFavorite,
  onShare,
  onFollow,
  onOpenAuthor,
  onOpenTag,
  onFirstPlay,
}: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const clickTimer = useRef<number | null>(null)
  const playedOnce = useRef(false)
  const [paused, setPaused] = useState(false)
  const [waiting, setWaiting] = useState(true)
  const [hearts, setHearts] = useState<{ id: number; x: number; y: number }[]>([])

  // Playback follows the active slide: exactly one element plays, the previous one is paused and
  // rewound. This is what keeps decode instances at 1-2 and scroll switching instant.
  useEffect(() => {
    const el = videoRef.current
    if (!el) return
    if (active) {
      setWaiting(true)
      void el.play().catch(() => {
        // Autoplay can be refused (data saver, battery saver). The poster and play button remain.
        setPaused(true)
        setWaiting(false)
      })
    } else {
      el.pause()
      el.currentTime = 0
      setPaused(false)
      setWaiting(false)
    }
  }, [active])

  // The muted attribute is not reliably applied through the JSX prop on some browsers.
  useEffect(() => {
    if (videoRef.current) videoRef.current.muted = muted
  }, [muted])

  useEffect(() => {
    return () => {
      if (clickTimer.current !== null) window.clearTimeout(clickTimer.current)
    }
  }, [])

  const spawnHeart = (clientX: number, clientY: number) => {
    const rect = videoRef.current?.getBoundingClientRect()
    const x = rect ? clientX - rect.left : clientX
    const y = rect ? clientY - rect.top : clientY
    const id = Date.now() + Math.random()
    setHearts((prev) => [...prev, { id, x, y }])
    window.setTimeout(() => {
      setHearts((prev) => prev.filter((heart) => heart.id !== id))
    }, HEART_LIFETIME)
  }

  const togglePlay = () => {
    const el = videoRef.current
    if (!el) return
    if (el.paused) {
      void el.play().catch(() => setPaused(true))
      setPaused(false)
    } else {
      el.pause()
      setPaused(true)
    }
  }

  // One handler covers both gestures: a single tap toggles playback, a second tap inside the
  // window upgrades to a like instead of toggling twice.
  const handleTap = (event: React.MouseEvent<HTMLDivElement>) => {
    if (clickTimer.current !== null) {
      window.clearTimeout(clickTimer.current)
      clickTimer.current = null
      spawnHeart(event.clientX, event.clientY)
      if (!video.isLiked) onLike(video)
      return
    }
    clickTimer.current = window.setTimeout(() => {
      clickTimer.current = null
      togglePlay()
    }, 220)
  }

  return (
    <section className="feed-slide relative w-full overflow-hidden bg-black">
      <video
        ref={videoRef}
        src={video.playUrl}
        poster={video.coverUrl || undefined}
        className="h-full w-full object-contain"
        playsInline
        loop
        muted={muted}
        preload={active ? 'auto' : 'metadata'}
        onWaiting={() => setWaiting(true)}
        onPlaying={() => {
          setWaiting(false)
          setPaused(false)
        }}
        onPause={() => setPaused(true)}
        onTimeUpdate={(event) => {
          if (!active || playedOnce.current) return
          if (event.currentTarget.currentTime > 0.2) {
            playedOnce.current = true
            onFirstPlay(video)
          }
        }}
        aria-label={video.title || '短视频'}
      />

      {/* Tap layer: owns play/pause and the double-tap heart so the <video> keeps native semantics. */}
      <div className="absolute inset-0" onMouseUp={handleTap} role="presentation" />

      {waiting && active && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          <span className="h-8 w-8 animate-spin rounded-full border-2 border-white/70 border-t-transparent" />
        </div>
      )}

      {paused && active && !waiting && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          <PlayIcon size={64} className="text-white/70 drop-shadow" />
        </div>
      )}

      {hearts.map((heart) => (
        <div
          key={heart.id}
          className="pointer-events-none absolute -translate-x-1/2 -translate-y-1/2 animate-heart-pop text-[110px] leading-none text-brand"
          style={{ left: heart.x, top: heart.y }}
        >
          ♥
        </div>
      ))}

      <button
        type="button"
        onClick={onToggleMute}
        className="absolute right-3 top-3 z-20 rounded-full bg-black/40 p-2 text-white/90 backdrop-blur"
        aria-label={muted ? '开启声音' : '静音'}
      >
        {muted ? <VolumeOffIcon size={18} /> : <VolumeOnIcon size={18} />}
      </button>

      <div className="absolute bottom-28 right-2 z-10 flex flex-col items-center gap-5 text-white">
        <button type="button" onClick={() => onOpenAuthor(video)} className="relative" aria-label="作者主页">
          <Avatar src={video.authorAvatar} name={video.authorName} size={44} ring />
          {!video.isFollowed && (
            <span
              onClick={(event) => {
                event.stopPropagation()
                onFollow(video)
              }}
              className="absolute -bottom-1.5 left-1/2 flex h-5 w-5 -translate-x-1/2 items-center justify-center rounded-full bg-brand text-[13px] font-bold leading-none"
              aria-label="关注"
            >
              +
            </span>
          )}
        </button>

        <ActionButton
          label={formatCount(video.likeCount)}
          onClick={() => onLike(video)}
          activeClass={video.isLiked ? 'text-brand' : ''}
          icon={<HeartIcon size={30} filled={video.isLiked} />}
        />
        <ActionButton
          label={formatCount(video.commentCount)}
          onClick={() => onComment(video)}
          icon={<CommentIcon size={29} />}
        />
        <ActionButton
          label={formatCount(video.favoriteCount)}
          onClick={() => onFavorite(video)}
          activeClass={video.isFavorited ? 'text-amber-400' : ''}
          icon={<StarIcon size={29} filled={video.isFavorited} />}
        />
        <ActionButton label="分享" onClick={() => onShare(video)} icon={<ShareIcon size={27} />} />
      </div>

      <div className="absolute inset-x-0 bottom-0 z-10 bg-gradient-to-t from-black/85 via-black/35 to-transparent px-4 pb-20 pt-16 text-white lg:pb-8">
        <button
          type="button"
          onClick={() => onOpenAuthor(video)}
          className="text-[15px] font-semibold text-shadow-hero"
        >
          @{video.authorName}
        </button>
        {video.title && (
          <h2 className="mt-1.5 text-[15px] font-medium leading-6 text-shadow-hero">{video.title}</h2>
        )}
        <p className="mt-1 line-clamp-2 max-w-[78%] text-[13px] leading-5 text-white/85">
          {video.description}
        </p>

        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {video.tags.slice(0, 5).map((tag) => (
            <button
              key={tag}
              type="button"
              onClick={() => onOpenTag(tag)}
              className="rounded-full bg-white/15 px-2.5 py-0.5 text-[12px] text-white/90 backdrop-blur hover:bg-white/25"
            >
              #{tag}
            </button>
          ))}
        </div>

        <div className="mt-2 flex items-center gap-3 text-[11px] text-white/60">
          <span>{relativeTime(video.publishedAt)}</span>
          <span>{formatCount(video.viewCount)} 次播放</span>
          <span>{formatDuration(video.durationMs)}</span>
        </div>
      </div>
    </section>
  )
}

function ActionButton({
  icon,
  label,
  onClick,
  activeClass = '',
}: {
  icon: JSX.Element
  label: string
  onClick: () => void
  activeClass?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex flex-col items-center gap-1 text-white transition-transform active:scale-90 ${activeClass}`}
      aria-label={label}
    >
      {icon}
      <span className="text-[11px] font-medium text-shadow-hero">{label}</span>
    </button>
  )
}
