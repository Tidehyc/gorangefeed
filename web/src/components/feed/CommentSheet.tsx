import { useEffect, useRef, useState } from 'react'
import { ApiError } from '@/api/client'
import { videoApi } from '@/api/video'
import { useCursorList } from '@/hooks/useCursorList'
import { newIdempotencyKey, relativeTime } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { CommentDetail } from '@/types/api'
import { Avatar } from '../ui/Avatar'
import { EmptyState, ErrorState } from '../ui/States'
import { Sheet } from '../ui/Sheet'
import { Spinner } from '../ui/Spinner'
import { HeartIcon } from '../icons'

interface Props {
  videoId: number | null
  open: boolean
  onClose: () => void
  onCountChange: (delta: number) => void
}

export function CommentSheet({ videoId, open, onClose, onCountChange }: Props) {
  const user = useAuthStore((state) => state.user)
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const list = useCursorList<CommentDetail>(
    (cursor) =>
      videoId
        ? videoApi.comments(videoId, typeof cursor === 'string' ? cursor : null)
        : Promise.resolve({ items: [], next_cursor: '', has_more: false }),
    { enabled: open && videoId !== null },
  )

  useEffect(() => {
    if (open) {
      setDraft('')
      // Focus after the slide-up animation so mobile keyboards do not fight the layout.
      window.setTimeout(() => inputRef.current?.focus(), 260)
    }
  }, [open])

  const send = async () => {
    const content = draft.trim()
    if (!content || !videoId || sending) return
    if (!user) {
      toast.error('请先登录再评论')
      return
    }
    setSending(true)
    try {
      const created = await videoApi.createComment(videoId, content, newIdempotencyKey())
      list.prepend({
        comment: created,
        author: {
          id: user.id,
          username: user.username,
          nickname: user.nickname,
          avatar_url: user.avatar_url,
        },
        is_liked: false,
      })
      setDraft('')
      onCountChange(1)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.friendlyMessage : '评论失败，请重试')
    } finally {
      setSending(false)
    }
  }

  const toggleCommentLike = async (item: CommentDetail) => {
    if (!user) {
      toast.error('请先登录')
      return
    }
    const liked = item.is_liked
    // Optimistic: the like button is the highest-frequency action in this sheet.
    list.patch(
      (row) => row.comment.id === item.comment.id,
      (row) => ({
        ...row,
        is_liked: !liked,
        comment: { ...row.comment, like_count: Math.max(0, row.comment.like_count + (liked ? -1 : 1)) },
      }),
    )
    try {
      if (liked) await videoApi.unlikeComment(item.comment.id)
      else await videoApi.likeComment(item.comment.id)
    } catch {
      list.patch(
        (row) => row.comment.id === item.comment.id,
        (row) => ({ ...row, is_liked: liked, comment: { ...row.comment, like_count: item.comment.like_count } }),
      )
      toast.error('操作失败，请重试')
    }
  }

  return (
    <Sheet open={open} onClose={onClose} title={`${list.items.length} 条评论`} maxHeightClass="h-[78dvh]">
      <div className="min-h-full pb-2">
        {list.loading && (
          <div className="flex justify-center py-10">
            <Spinner />
          </div>
        )}

        {!list.loading && list.error && <ErrorState message={list.error} onRetry={() => void list.reload()} />}

        {!list.loading && !list.error && list.items.length === 0 && (
          <EmptyState title="还没有评论" hint="说点什么，让作者看到你的想法" />
        )}

        <ul className="divide-y divide-white/5">
          {list.items.map((item) => (
            <li key={item.comment.id} className="flex gap-3 px-4 py-3">
              <Avatar src={item.author?.avatar_url} name={item.author?.nickname} size={34} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-[13px] font-medium text-mist-300">
                    {item.author?.nickname || item.author?.username || '已注销用户'}
                  </span>
                  {item.comment.parent_id > 0 && (
                    <span className="rounded bg-white/10 px-1 text-[10px] text-mist-400">回复</span>
                  )}
                </div>
                <p className="mt-0.5 break-words text-[14px] leading-5 text-mist-100">{item.comment.content}</p>
                <div className="mt-1 flex items-center gap-3 text-[11px] text-mist-500">
                  <span>{relativeTime(item.comment.created_at)}</span>
                </div>
              </div>
              <button
                type="button"
                onClick={() => void toggleCommentLike(item)}
                className={`flex flex-col items-center gap-0.5 self-start pt-1 text-[11px] ${
                  item.is_liked ? 'text-brand' : 'text-mist-500'
                }`}
                aria-label={item.is_liked ? '取消点赞' : '点赞'}
              >
                <HeartIcon size={16} filled={item.is_liked} />
                <span>{item.comment.like_count > 0 ? item.comment.like_count : ''}</span>
              </button>
            </li>
          ))}
        </ul>

        {!list.loading && list.hasMore && (
          <div className="py-3 text-center">
            <button
              type="button"
              onClick={() => void list.loadMore()}
              disabled={list.loadingMore}
              className="text-[13px] text-mist-400 hover:text-mist-100"
            >
              {list.loadingMore ? '加载中…' : '查看更多评论'}
            </button>
          </div>
        )}
      </div>

      <div className="safe-bottom sticky bottom-0 flex items-center gap-2 border-t border-white/10 bg-ink-700 px-3 py-2">
        <Avatar src={user?.avatar_url} name={user?.nickname} size={30} />
        <input
          ref={inputRef}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') void send()
          }}
          maxLength={500}
          placeholder={user ? '说点什么…' : '登录后参与评论'}
          className="h-9 flex-1 rounded-full bg-white/10 px-3 text-[14px] text-mist-100 placeholder:text-mist-700 focus:ring-1 focus:ring-brand/60"
        />
        <button
          type="button"
          onClick={() => void send()}
          disabled={!draft.trim() || sending}
          className="h-9 rounded-full bg-brand px-4 text-[13px] font-medium text-white disabled:opacity-40"
        >
          {sending ? '发送中' : '发送'}
        </button>
      </div>
    </Sheet>
  )
}
