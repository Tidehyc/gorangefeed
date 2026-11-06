import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { ApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { toast } from '@/stores/toast'
import { Button } from '@/components/ui/Button'

type Mode = 'login' | 'register'

const DEMO_ACCOUNT = { username: 'alice', password: 'Passw0rd!' }

export function LoginPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const status = useAuthStore((state) => state.status)
  const pending = useAuthStore((state) => state.pending)
  const login = useAuthStore((state) => state.login)
  const register = useAuthStore((state) => state.register)

  const redirect = params.get('redirect') || '/'
  const [mode, setMode] = useState<Mode>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (status === 'authed') navigate(redirect, { replace: true })
  }, [status, navigate, redirect])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setError(null)

    if (mode === 'register') {
      // Mirror the server rules so the user is not sent a round trip for a fixable mistake.
      if (!/^[a-zA-Z0-9_]{3,32}$/.test(username)) {
        setError('用户名需为 3-32 位字母、数字或下划线')
        return
      }
      if (password.length < 8 || password.length > 32 || !/[A-Za-z]/.test(password) || !/\d/.test(password)) {
        setError('密码需 8-32 位，且同时包含字母和数字')
        return
      }
    }

    try {
      if (mode === 'login') await login(username.trim(), password)
      else await register(username.trim(), password)
      toast.success(mode === 'login' ? '登录成功' : '注册成功，已自动登录')
      navigate(redirect, { replace: true })
    } catch (err) {
      setError(err instanceof ApiError ? err.friendlyMessage : '操作失败，请稍后重试')
    }
  }

  return (
    <div className="relative flex min-h-[100dvh] flex-col items-center justify-center px-6 py-10">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-64 bg-gradient-to-b from-brand/20 to-transparent" />

      <div className="relative w-full max-w-[360px]">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-brand text-xl font-black text-white">
            G
          </div>
          <h1 className="text-[22px] font-semibold tracking-tight">
            {mode === 'login' ? '登录 GoRangeFeed' : '创建账号'}
          </h1>
          <p className="mt-1 text-[13px] text-mist-500">
            {mode === 'login' ? '登录后可点赞、评论、发布自己的作品' : '注册即可获得属于你的推荐流'}
          </p>
        </div>

        <div className="mb-6 flex rounded-xl bg-white/5 p-1">
          {(['login', 'register'] as Mode[]).map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => {
                setMode(item)
                setError(null)
              }}
              className={`flex-1 rounded-lg py-2 text-[13px] font-medium transition-colors ${
                mode === item ? 'bg-white/10 text-white' : 'text-mist-500'
              }`}
            >
              {item === 'login' ? '登录' : '注册'}
            </button>
          ))}
        </div>

        <form onSubmit={submit} className="flex flex-col gap-3">
          <label className="flex flex-col gap-1.5">
            <span className="text-[12px] text-mist-500">用户名</span>
            <input
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              placeholder="3-32 位字母、数字或下划线"
              className="h-11 rounded-xl border border-white/10 bg-white/5 px-3 text-[15px] text-mist-100 placeholder:text-mist-700 focus:border-brand/60"
            />
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-[12px] text-mist-500">密码</span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              placeholder="至少 8 位，含字母与数字"
              className="h-11 rounded-xl border border-white/10 bg-white/5 px-3 text-[15px] text-mist-100 placeholder:text-mist-700 focus:border-brand/60"
            />
          </label>

          {error && <p className="rounded-lg bg-red-500/10 px-3 py-2 text-[13px] text-red-300">{error}</p>}

          <Button type="submit" size="lg" block loading={pending}>
            {mode === 'login' ? '登录' : '注册并登录'}
          </Button>
        </form>

        <div className="mt-6 rounded-xl border border-white/10 bg-white/[0.03] p-3 text-[12px] leading-5 text-mist-500">
          <p className="mb-1 font-medium text-mist-300">本地演示账号</p>
          <p>
            用户名 <span className="text-mist-100">{DEMO_ACCOUNT.username}</span> · 密码{' '}
            <span className="text-mist-100">{DEMO_ACCOUNT.password}</span>
          </p>
          <button
            type="button"
            onClick={() => {
              setMode('login')
              setUsername(DEMO_ACCOUNT.username)
              setPassword(DEMO_ACCOUNT.password)
            }}
            className="mt-2 text-brand hover:underline"
          >
            填入演示账号
          </button>
        </div>

        <Link to="/" className="mt-6 block text-center text-[13px] text-mist-500 hover:text-mist-300">
          先随便逛逛 →
        </Link>
      </div>
    </div>
  )
}
