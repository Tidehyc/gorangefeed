interface IconProps {
  size?: number
  className?: string
  filled?: boolean
  strokeWidth?: number
}

type Icon = (props: IconProps) => JSX.Element

function base(size: number, className: string) {
  return {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    className,
    'aria-hidden': true as const,
  }
}

export const HomeIcon: Icon = ({ size = 22, className = '', filled = false, strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-4.5v-6h-5v6H5a1 1 0 0 1-1-1z" strokeLinejoin="round" />
  </svg>
)

export const PlusIcon: Icon = ({ size = 22, className = '', strokeWidth = 2 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M12 5v14M5 12h14" strokeLinecap="round" />
  </svg>
)

export const BellIcon: Icon = ({ size = 22, className = '', filled = false, strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M6 9a6 6 0 1 1 12 0c0 4 1.2 5.4 2 6H4c.8-.6 2-2 2-6Z" strokeLinejoin="round" />
    <path d="M10 19a2 2 0 0 0 4 0" strokeLinecap="round" />
  </svg>
)

export const UserIcon: Icon = ({ size = 22, className = '', filled = false, strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={strokeWidth}>
    <circle cx="12" cy="8.5" r="3.6" />
    <path d="M4.8 20c.7-3.6 3.6-5.6 7.2-5.6s6.5 2 7.2 5.6" strokeLinecap="round" />
  </svg>
)

export const HeartIcon: Icon = ({ size = 22, className = '', filled = false, strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={strokeWidth}>
    <path
      d="M12 20s-7.2-4.4-8.6-8.6C2.4 8.2 4.3 5.5 7 5.5c1.9 0 3.4 1 5 3 1.6-2 3.1-3 5-3 2.7 0 4.6 2.7 3.6 5.9C19.2 15.6 12 20 12 20Z"
      strokeLinejoin="round"
    />
  </svg>
)

export const CommentIcon: Icon = ({ size = 22, className = '', filled = false, strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M20 12.2c0 3.9-3.6 7-8 7-.9 0-1.8-.1-2.6-.4L5 20l1.1-3.2C4.8 15.5 4 13.9 4 12.2c0-3.9 3.6-7 8-7s8 3.1 8 7Z" strokeLinejoin="round" />
  </svg>
)

export const StarIcon: Icon = ({ size = 22, className = '', filled = false, strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M6 4h12a1 1 0 0 1 1 1v15l-7-4.2L5 20V5a1 1 0 0 1 1-1Z" strokeLinejoin="round" />
  </svg>
)

export const ShareIcon: Icon = ({ size = 22, className = '', strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M4 12v7a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-7" strokeLinecap="round" />
    <path d="M12 15V4m0 0L8 8m4-4 4 4" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
)

export const PlayIcon: Icon = ({ size = 22, className = '' }) => (
  <svg {...base(size, className)} fill="currentColor">
    <path d="M8 5.2c0-.8.9-1.3 1.6-.9l9 6.8c.6.4.6 1.3 0 1.7l-9 6.8c-.7.4-1.6-.1-1.6-.9z" />
  </svg>
)

export const PauseIcon: Icon = ({ size = 22, className = '' }) => (
  <svg {...base(size, className)} fill="currentColor">
    <rect x="7" y="5" width="3.5" height="14" rx="1.2" />
    <rect x="13.5" y="5" width="3.5" height="14" rx="1.2" />
  </svg>
)

export const VolumeOffIcon: Icon = ({ size = 22, className = '', strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M11 6 6.5 10H4v4h2.5L11 18z" strokeLinejoin="round" />
    <path d="m16 9.5 4 5m0-5-4 5" strokeLinecap="round" />
  </svg>
)

export const VolumeOnIcon: Icon = ({ size = 22, className = '', strokeWidth = 1.8 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M11 6 6.5 10H4v4h2.5L11 18z" strokeLinejoin="round" />
    <path d="M15 9a4.2 4.2 0 0 1 0 6m2.4-8.4a7.5 7.5 0 0 1 0 10.8" strokeLinecap="round" />
  </svg>
)

export const BackIcon: Icon = ({ size = 22, className = '', strokeWidth = 2 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="M15 5l-7 7 7 7" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
)

export const MoreIcon: Icon = ({ size = 22, className = '' }) => (
  <svg {...base(size, className)} fill="currentColor">
    <circle cx="5" cy="12" r="1.7" />
    <circle cx="12" cy="12" r="1.7" />
    <circle cx="19" cy="12" r="1.7" />
  </svg>
)

export const SettingsIcon: Icon = ({ size = 20, className = '', strokeWidth = 1.7 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <circle cx="12" cy="12" r="3.1" />
    <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" />
  </svg>
)

export const ChevronRightIcon: Icon = ({ size = 18, className = '', strokeWidth = 2 }) => (
  <svg {...base(size, className)} fill="none" stroke="currentColor" strokeWidth={strokeWidth}>
    <path d="m9 5 7 7-7 7" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
)
