export function formatCount(value: number | undefined | null): string {
  const n = Number(value ?? 0)
  if (!Number.isFinite(n) || n <= 0) return '0'
  if (n < 10000) return String(n)
  const wan = n / 10000
  // 抖音风格: one decimal below 100万, integer above.
  return wan >= 100 ? `${Math.round(wan)}万` : `${wan.toFixed(1).replace(/\.0$/, '')}万`
}

export function formatDuration(ms: number | undefined | null): string {
  const totalSeconds = Math.max(0, Math.round((ms ?? 0) / 1000))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}:${String(seconds).padStart(2, '0')}`
}

export function relativeTime(iso: string | undefined | null): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diff = Date.now() - then
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)} 天前`
  const d = new Date(then)
  const sameYear = d.getFullYear() === new Date().getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return sameYear ? `${month}-${day}` : `${d.getFullYear()}-${month}-${day}`
}

/** Stable colour per name/id so avatars without an image never look like broken assets. */
export function gradientFor(seed: string | number): string {
  const palettes = [
    'from-orange-500 to-rose-500',
    'from-sky-500 to-indigo-500',
    'from-emerald-500 to-teal-500',
    'from-fuchsia-500 to-purple-500',
    'from-amber-500 to-orange-600',
    'from-cyan-500 to-blue-600',
  ]
  const key = String(seed)
  let hash = 0
  for (let i = 0; i < key.length; i += 1) hash = (hash * 31 + key.charCodeAt(i)) % 9973
  return palettes[hash % palettes.length]
}

export function initialOf(name: string | undefined | null): string {
  const trimmed = (name ?? '').trim()
  if (!trimmed) return '?'
  // Chinese nicknames read better with the first character than with an upper-cased letter.
  return /[\u4e00-\u9fa5]/.test(trimmed[0]) ? trimmed[0] : trimmed[0].toUpperCase()
}

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}
