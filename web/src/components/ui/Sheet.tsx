import { useEffect, type ReactNode } from 'react'

interface SheetProps {
  open: boolean
  onClose: () => void
  title?: ReactNode
  children: ReactNode
  /** Height cap; comments need more room than the share sheet. */
  maxHeightClass?: string
  footer?: ReactNode
}

export function Sheet({
  open,
  onClose,
  title,
  children,
  maxHeightClass = 'max-h-[78dvh]',
  footer,
}: SheetProps) {
  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = previous
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex flex-col justify-end" role="dialog" aria-modal="true">
      <button
        type="button"
        aria-label="关闭"
        onClick={onClose}
        className="absolute inset-0 animate-fade-in bg-black/55"
      />
      <div
        className={`relative flex ${maxHeightClass} animate-slide-up flex-col rounded-t-2xl bg-ink-700 pb-[env(safe-area-inset-bottom)] shadow-float`}
      >
        <div className="flex items-center justify-between px-4 py-3">
          <div className="text-[15px] font-medium text-mist-100">{title}</div>
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭"
            className="rounded-full p-1.5 text-mist-500 hover:bg-white/5 hover:text-mist-300"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
            </svg>
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
        {footer}
      </div>
    </div>
  )
}
