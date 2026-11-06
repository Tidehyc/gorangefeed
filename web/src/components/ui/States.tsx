import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

interface EmptyStateProps {
  title: string
  hint?: string
  actionText?: string
  actionTo?: string
  onAction?: () => void
  icon?: ReactNode
}

export function EmptyState({ title, hint, actionText, actionTo, onAction, icon }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 px-8 py-16 text-center">
      {icon && <div className="text-mist-700">{icon}</div>}
      <p className="text-[15px] font-medium text-mist-100">{title}</p>
      {hint && <p className="max-w-[260px] text-[13px] leading-5 text-mist-500">{hint}</p>}
      {actionText && actionTo && (
        <Link
          to={actionTo}
          className="mt-2 rounded-full bg-brand px-5 py-2 text-[13px] font-medium text-white"
        >
          {actionText}
        </Link>
      )}
      {actionText && onAction && !actionTo && (
        <button
          type="button"
          onClick={onAction}
          className="mt-2 rounded-full bg-brand px-5 py-2 text-[13px] font-medium text-white"
        >
          {actionText}
        </button>
      )}
    </div>
  )
}

interface ErrorStateProps {
  message?: string
  onRetry?: () => void
  compact?: boolean
}

export function ErrorState({ message = '加载失败，请稍后重试', onRetry, compact = false }: ErrorStateProps) {
  return (
    <div
      className={`flex flex-col items-center justify-center gap-3 text-center ${
        compact ? 'py-8' : 'py-16'
      }`}
    >
      <span className="text-2xl" aria-hidden>
        ⚠️
      </span>
      <p className="max-w-[280px] text-[13px] leading-5 text-mist-400">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="rounded-full border border-white/20 px-4 py-1.5 text-[13px] text-mist-100 hover:bg-white/5"
        >
          点击重试
        </button>
      )}
    </div>
  )
}
