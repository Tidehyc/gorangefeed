interface SpinnerProps {
  size?: number
  className?: string
}

export function Spinner({ size = 18, className = '' }: SpinnerProps) {
  return (
    <span
      role="status"
      aria-label="加载中"
      className={`inline-block animate-spin rounded-full border-2 border-current border-t-transparent ${className}`}
      style={{ width: size, height: size }}
    />
  )
}
