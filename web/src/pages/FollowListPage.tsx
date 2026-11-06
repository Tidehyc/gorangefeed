import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '@/api/client'
import { relationApi } from '@/api/relation'
import { useCursorList } from '@/hooks/useCursorList'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import type { RelationUser } from '@/types/api'
import { PageHeader } from '@/components/PageHeader'
import { Avatar } from '@/components/ui/Avatar'
import { Button } from '@/components/ui/Button'
import { EmptyState, ErrorState } from '@/components/ui/States'
import { Spinner } from '@/components/ui/Spinner'

type Kind = 'followers' | 'followings'

export function FollowListPage({ kind }: { kind: Kind }) {
  const { id } = useParams<{ id: string }>()
  const userId = Number(id)
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)
  const me = useAuthStore((state) => state.user)

  const list = useCursorList<RelationUser>(
    (cursor) =>
      kind === 'followers'
        ? relationApi.followers(userId, typeof cursor === 'number' ? cursor : null, 20)
        : relationApi.followings(userId, typeof cursor === 'number' ? cursor : null, 20),
    { enabled: Number.isFinite(userId) && userId > 0, dedupeKey: (user) => user.id },
  )

  // The list payload only carries is_friend, not "am I following this person", so the followed
  // set is resolved per visible page. 20 lightweight lookups beat rendering a wrong button state.
  const [following, setFollowing] = useState<Set<number>>(new Set())
  const [resolving, setResolving] = useState(false)

  useEffect(() => {
    if (status !== 'authed' || list.items.length === 0) return
    let alive = true
    const ids = list.items.map((user) => user.id)
    setResolving(true)

    void (async () => {
      const found = new Set<number>()
      // Small batches keep the connection pool happy on the server side.
      for (let i = 0; i < ids.length; i += 5) {
        const batch = ids.slice(i, i + 5)
        const results = await Promise.all(
          batch.map((target) =>
            relationApi
              .relation(target)
              .then((rel) => (rel.is_following ? target : null))
              .catch(() => null),
          ),
        )
        for (const target of results) if (target) found.add(target)
      }
      if (alive) {
        setFollowing((prev) => new Set([...prev, ...found]))
        setResolving(false)
      }
    })()

    return () => {
      alive = false
    }
  }, [list.items, status])

  const toggle = async (target: RelationUser) => {
    if (status !== 'authed') {
      toast.info('登录后才能关注')
      navigate(`/login?redirect=/user/${userId}/${kind}`)
      return
    }
    const isFollowing = following.has(target.id)
    setFollowing((prev) => {
      const next = new Set(prev)
      if (isFollowing) next.delete(target.id)
      else next.add(target.id)
      return next
    })
    try {
      if (isFollowing) await relationApi.unfollow(target.id)
      else await relationApi.follow(target.id)
    } catch (err) {
      setFollowing((prev) => {
        const next = new Set(prev)
        if (isFollowing) next.add(target.id)
        else next.delete(target.id)
        return next
      })
      toast.error(err instanceof ApiError ? err.friendlyMessage : '操作失败，请重试')
    }
  }

  return (
    <div className="mx-auto min-h-[100dvh] w-full max-w-[640px] pb-20">
      <PageHeader title={kind === 'followers' ? '粉丝' : '关注'} />

      {list.loading && (
        <div className="flex justify-center py-10 text-mist-500">
          <Spinner />
        </div>
      )}

      {!list.loading && list.error && (
        <ErrorState message={list.error} onRetry={() => void list.reload()} />
      )}

      {!list.loading && !list.error && list.items.length === 0 && (
        <EmptyState
          title={kind === 'followers' ? '还没有粉丝' : '还没有关注任何人'}
          hint={kind === 'followers' ? '发布作品后更容易被看见' : '去推荐页找几位感兴趣的创作者'}
          actionText={kind === 'followers' ? '去发布' : '去发现'}
          actionTo={kind === 'followers' ? '/publish' : '/'}
        />
      )}

      <ul className="divide-y divide-white/5">
        {list.items.map((user) => {
          const isFollowing = following.has(user.id)
          return (
            <li key={user.id} className="flex items-center gap-3 px-4 py-3">
              <Link to={`/user/${user.id}`}>
                <Avatar src={user.avatar_url} name={user.nickname} size={44} />
              </Link>
              <Link to={`/user/${user.id}`} className="min-w-0 flex-1">
                <p className="truncate text-[14px] font-medium text-mist-100">
                  {user.nickname || '未命名用户'}
                </p>
                <p className="truncate text-[12px] text-mist-500">@{user.username}</p>
                {user.bio && <p className="mt-0.5 truncate text-[12px] text-mist-700">{user.bio}</p>}
              </Link>
              {user.id !== me?.id && (
                <Button
                  size="sm"
                  variant={isFollowing ? 'ghost' : 'primary'}
                  onClick={() => void toggle(user)}
                  disabled={resolving && !isFollowing}
                >
                  {isFollowing ? '已关注' : '关注'}
                </Button>
              )}
            </li>
          )
        })}
      </ul>

      {list.hasMore && (
        <div className="py-4 text-center">
          <button
            type="button"
            onClick={() => void list.loadMore()}
            disabled={list.loadingMore}
            className="text-[13px] text-mist-500 hover:text-mist-200"
          >
            {list.loadingMore ? '加载中…' : '加载更多'}
          </button>
        </div>
      )}
    </div>
  )
}
