import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { Spinner } from './Spinner'

type Variant = 'primary' | 'ghost' | 'outline' | 'danger'
type Size = 'sm' | 'md' | 'lg'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  loading?: boolean
  block?: boolean
  children: ReactNode
}

const variants: Record<Variant, string> = {
  primary: 'bg-brand text-white hover:bg-brand-soft active:bg-brand-deep',
  ghost: 'bg-white/10 text-mist-100 hover:bg-white/15',
  outline: 'border border-white/25 text-mist-100 hover:border-white/45 hover:bg-white/5',
  danger: 'bg-red-500 text-white hover:bg-red-400',
}

const sizes: Record<Size, string> = {
  sm: 'h-8 px-3 text-[13px] rounded-lg',
  md: 'h-10 px-4 text-sm rounded-xl',
  lg: 'h-12 px-5 text-[15px] rounded-xl',
}

export function Button({
  variant = 'primary',
  size = 'md',
  loading = false,
  block = false,
  disabled,
  className = '',
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      className={`inline-flex items-center justify-center gap-2 font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 ${
        variants[variant]
      } ${sizes[size]} ${block ? 'w-full' : ''} ${className}`}
    >
      {loading && <Spinner size={14} />}
      {children}
    </button>
  )
}
