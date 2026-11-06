import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return (
    <div className="flex min-h-[70dvh] flex-col items-center justify-center gap-3 px-8 text-center">
      <p className="text-[44px] font-black text-ink-500">404</p>
      <p className="text-[15px] font-medium text-mist-100">这个页面不存在</p>
      <p className="text-[13px] text-mist-500">链接可能已失效，或者作者删除了内容</p>
      <Link to="/" className="mt-3 rounded-full bg-brand px-5 py-2 text-[13px] font-medium text-white">
        回首页
      </Link>
    </div>
  )
}
