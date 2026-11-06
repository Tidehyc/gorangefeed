import type { ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { BackIcon } from './icons'

interface PageHeaderProps {
  title: ReactNode
  /** Pass false to render a title-only header (tab roots such as /me). */
  back?: boolean
  right?: ReactNode
  /** Transparent headers sit on top of media instead of owning a bar. */
  transparent?: boolean
}

export function PageHeader({ title, back = true, right, transparent = false }: PageHeaderProps) {
  const navigate = useNavigate()
  return (
    <header
      className={`sticky top-0 z-30 flex h-14 items-center gap-2 px-3 ${
        transparent ? 'bg-transparent' : 'border-b border-white/5 bg-ink-900/90 backdrop-blur'
      }`}
    >
      {back && (
        <button
          type="button"
          onClick={() => (window.history.length > 1 ? navigate(-1) : navigate('/'))}
          aria-label="返回"
          className="-ml-1 rounded-full p-2 text-mist-300 hover:bg-white/5"
        >
          <BackIcon size={20} />
        </button>
      )}
      <h1 className="flex-1 truncate text-[16px] font-semibold text-mist-100">{title}</h1>
      {right}
    </header>
  )
}
