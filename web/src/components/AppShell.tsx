import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { notificationApi } from '@/api/notification'
import { useAuthStore } from '@/stores/auth'
import { Avatar } from './ui/Avatar'
import { ToastHost } from './ui/ToastHost'
import { BellIcon, HomeIcon, PlusIcon, UserIcon } from './icons'

function useUnreadCount() {
  const status = useAuthStore((s) => s.status)
  const [count, setCount] = useState(0)

  useEffect(() => {
    if (status !== 'authed') {
      setCount(0)
      return
    }
    let alive = true
    const load = async () => {
      try {
        const res = await notificationApi.unreadCount()
        if (alive) setCount(res.unread_count)
      } catch {
        // A badge is decoration; polling failures stay silent.
      }
    }
    void load()
    const timer = window.setInterval(load, 30_000)
    return () => {
      alive = false
      window.clearInterval(timer)
    }
  }, [status])

  return count
}

function Badge({ count }: { count: number }) {
  if (count <= 0) return null
  return (
    <span className="absolute -right-1.5 -top-1 min-w-[16px] rounded-full bg-brand px-1 text-center text-[10px] font-semibold leading-4 text-white">
      {count > 99 ? '99+' : count}
    </span>
  )
}

export function AppShell() {
  const unread = useUnreadCount()
  const status = useAuthStore((s) => s.status)
  const user = useAuthStore((s) => s.user)
  const location = useLocation()
  const navigate = useNavigate()

  const hideChrome = location.pathname === '/' || location.pathname.startsWith('/video/')

  return (
    <div className="min-h-[100dvh] bg-ink-900">
      {/* Desktop: fixed rail. Mobile keeps the bottom bar so the video owns the screen. */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[232px] flex-col border-r border-white/5 bg-ink-800 px-3 py-5 lg:flex">
        <Link to="/" className="mb-6 flex items-center gap-2 px-3">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand text-sm font-black text-white">
            G
          </span>
          <span className="text-[15px] font-semibold tracking-tight">GoRangeFeed</span>
        </Link>

        <nav className="flex flex-1 flex-col gap-1">
          <SideLink to="/" icon={<HomeIcon size={20} />} label="首页" end />
          <SideLink to="/notifications" icon={<BellIcon size={20} />} label="消息" badge={unread} />
          <SideLink to="/me" icon={<UserIcon size={20} />} label="我的" />
          <SideLink to="/publish" icon={<PlusIcon size={20} />} label="发布作品" />
        </nav>

        <Link
          to={status === 'authed' ? '/me' : '/login'}
          className="mt-4 flex items-center gap-2 rounded-xl px-2 py-2 hover:bg-white/5"
        >
          <Avatar src={user?.avatar_url} name={user?.nickname} size={32} />
          <span className="truncate text-[13px] text-mist-300">
            {user?.nickname ?? (status === 'authed' ? '我的账号' : '未登录')}
          </span>
        </Link>
      </aside>

      <div className="lg:pl-[232px]">
        <Outlet />
      </div>

      {!hideChrome && (
        <nav className="safe-bottom fixed inset-x-0 bottom-0 z-30 flex items-stretch justify-around border-t border-white/5 bg-ink-800/95 px-2 pt-1 backdrop-blur lg:hidden">
          <NavItem to="/" icon={<HomeIcon size={21} />} label="首页" end />
          <button
            type="button"
            onClick={() => navigate(status === 'authed' ? '/publish' : '/login?redirect=/publish')}
            className="flex flex-1 flex-col items-center gap-0.5 py-1.5 text-mist-500"
            aria-label="发布"
          >
            <span className="flex h-7 w-9 items-center justify-center rounded-lg bg-brand text-white">
              <PlusIcon size={18} />
            </span>
            <span className="text-[10px]">发布</span>
          </button>
          <NavItem
            to="/notifications"
            icon={
              <span className="relative">
                <BellIcon size={21} />
                <Badge count={unread} />
              </span>
            }
            label="消息"
          />
          <NavItem to="/me" icon={<UserIcon size={21} />} label="我的" />
        </nav>
      )}

      <ToastHost />
    </div>
  )
}

function SideLink({
  to,
  icon,
  label,
  badge = 0,
  end = false,
}: {
  to: string
  icon: JSX.Element
  label: string
  badge?: number
  end?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        `relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-[14px] transition-colors ${
          isActive ? 'bg-white/10 text-white' : 'text-mist-400 hover:bg-white/5 hover:text-mist-100'
        }`
      }
    >
      {icon}
      <span>{label}</span>
      {badge > 0 && <Badge count={badge} />}
    </NavLink>
  )
}

function NavItem({
  to,
  icon,
  label,
  end = false,
}: {
  to: string
  icon: JSX.Element
  label: string
  end?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        `flex flex-1 flex-col items-center gap-0.5 py-1.5 text-[10px] ${
          isActive ? 'text-white' : 'text-mist-500'
        }`
      }
    >
      {icon}
      <span>{label}</span>
    </NavLink>
  )
}
