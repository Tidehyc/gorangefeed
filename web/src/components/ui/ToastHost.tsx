import { useToastStore } from '@/stores/toast'

export function ToastHost() {
  const items = useToastStore((state) => state.items)
  const dismiss = useToastStore((state) => state.dismiss)

  return (
    <div className="pointer-events-none fixed inset-x-0 top-3 z-[60] flex flex-col items-center gap-2 px-4">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          onClick={() => dismiss(item.id)}
          className={`pointer-events-auto max-w-[90vw] animate-fade-in rounded-full px-4 py-2 text-[13px] shadow-float ${
            item.tone === 'error'
              ? 'bg-red-500 text-white'
              : item.tone === 'success'
                ? 'bg-emerald-500 text-white'
                : 'bg-ink-600 text-mist-100'
          }`}
        >
          {item.text}
        </button>
      ))}
    </div>
  )
}
