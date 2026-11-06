import { useEffect, useState } from 'react'
import { gradientFor, initialOf } from '@/lib/format'

interface AvatarProps {
  src?: string | null
  name?: string | null
  size?: number
  className?: string
  ring?: boolean
}

/**
 * Avatar with a deterministic gradient fallback. A missing or 404 image must never show the
 * browser's broken image icon (FR-UX-05).
 */
export function Avatar({ src, name, size = 40, className = '', ring = false }: AvatarProps) {
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setFailed(false)
  }, [src])

  const dimension = { width: size, height: size }
  const ringClass = ring ? 'ring-2 ring-white/90' : ''
  const base = `shrink-0 rounded-full object-cover ${ringClass} ${className}`

  if (src && !failed) {
    return (
      <img
        src={src}
        alt={name ?? '用户头像'}
        style={dimension}
        loading="lazy"
        className={`${base} bg-ink-600`}
        onError={() => setFailed(true)}
      />
    )
  }

  return (
    <div
      style={dimension}
      aria-label={name ?? '用户头像'}
      className={`flex items-center justify-center rounded-full bg-gradient-to-br ${gradientFor(
        name ?? '?',
      )} font-semibold text-white ${ringClass} ${className}`}
    >
      <span style={{ fontSize: Math.max(11, size * 0.38) }}>{initialOf(name)}</span>
    </div>
  )
}
