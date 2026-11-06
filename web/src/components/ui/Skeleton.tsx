interface SkeletonProps {
  className?: string
}

export function Skeleton({ className = '' }: SkeletonProps) {
  return (
    <div className={`relative overflow-hidden rounded-lg bg-ink-600 ${className}`}>
      <div className="absolute inset-0 -translate-x-full animate-shimmer bg-gradient-to-r from-transparent via-white/10 to-transparent" />
    </div>
  )
}

/** First-screen placeholder: mirrors the real feed layout so the swap does not shift content. */
export function FeedSkeleton() {
  return (
    <div className="feed-slide relative flex items-center justify-center bg-ink-900">
      <div className="absolute inset-0 flex flex-col justify-end gap-3 p-4 pb-32">
        <Skeleton className="h-4 w-28" />
        <Skeleton className="h-3 w-52" />
        <Skeleton className="h-3 w-40" />
      </div>
      <div className="absolute right-3 bottom-32 flex flex-col items-center gap-5">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-9 w-9 rounded-full" />
        ))}
      </div>
    </div>
  )
}

export function GridSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div className="grid grid-cols-3 gap-1">
      {Array.from({ length: count }).map((_, i) => (
        <Skeleton key={i} className="aspect-[3/4] w-full rounded-none" />
      ))}
    </div>
  )
}
