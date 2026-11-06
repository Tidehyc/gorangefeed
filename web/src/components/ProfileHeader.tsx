import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import type { UserStats } from '@/types/api'
import { formatCount } from '@/lib/format'
import { Avatar } from './ui/Avatar'
import { Button } from './ui/Button'

interface Props {
  userId: number
  nickname: string
  username?: string
  avatarUrl?: string
  bio?: string
  stats?: UserStats
  isSelf: boolean
  isFollowing: boolean
  followPending?: boolean
  onToggleFollow?: () => void
  actions?: ReactNode
}

export function ProfileHeader({
  userId,
  nickname,
  username,
  avatarUrl,
  bio,
  stats,
  isSelf,
  isFollowing,
  followPending = false,
  onToggleFollow,
  actions,
}: Props) {
  return (
    <section className="px-4 pb-4 pt-2">
      <div className="flex items-center gap-4">
        <Avatar src={avatarUrl} name={nickname} size={76} />
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-[19px] font-semibold text-mist-100">{nickname || '未命名用户'}</h1>
          {username && <p className="mt-0.5 truncate text-[12px] text-mist-500">@{username}</p>}
          <div className="mt-3 flex items-center gap-5">
            <Stat label="关注" value={stats?.following_count} to={`/user/${userId}/followings`} />
            <Stat label="粉丝" value={stats?.follower_count} to={`/user/${userId}/followers`} />
            <Stat label="作品" value={stats?.work_count} />
            <Stat label="获赞" value={stats?.total_likes} />
          </div>
        </div>
      </div>

      {bio && <p className="mt-3 whitespace-pre-wrap text-[13px] leading-5 text-mist-400">{bio}</p>}

      <div className="mt-4 flex gap-2">
        {isSelf ? (
          actions
        ) : (
          <Button
            variant={isFollowing ? 'ghost' : 'primary'}
            size="md"
            loading={followPending}
            onClick={onToggleFollow}
            className="min-w-[104px]"
          >
            {isFollowing ? '已关注' : '关注'}
          </Button>
        )}
      </div>
    </section>
  )
}

function Stat({ label, value, to }: { label: string; value?: number; to?: string }) {
  const content = (
    <>
      <span className="text-[15px] font-semibold text-mist-100">{formatCount(value ?? 0)}</span>
      <span className="text-[11px] text-mist-500">{label}</span>
    </>
  )
  if (to) {
    return (
      <Link to={to} className="flex flex-col items-center leading-tight hover:opacity-80">
        {content}
      </Link>
    )
  }
  return <div className="flex flex-col items-center leading-tight">{content}</div>
}
