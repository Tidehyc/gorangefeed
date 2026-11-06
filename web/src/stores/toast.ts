import { create } from 'zustand'

export type ToastTone = 'info' | 'success' | 'error'

export interface ToastItem {
  id: number
  text: string
  tone: ToastTone
}

interface ToastState {
  items: ToastItem[]
  show: (text: string, tone?: ToastTone) => void
  dismiss: (id: number) => void
}

let nextId = 1

export const useToastStore = create<ToastState>((set) => ({
  items: [],
  show(text, tone = 'info') {
    const id = nextId++
    set((state) => ({ items: [...state.items, { id, text, tone }] }))
    window.setTimeout(() => {
      set((state) => ({ items: state.items.filter((item) => item.id !== id) }))
    }, 2600)
  },
  dismiss(id) {
    set((state) => ({ items: state.items.filter((item) => item.id !== id) }))
  },
}))

export const toast = {
  info: (text: string) => useToastStore.getState().show(text, 'info'),
  success: (text: string) => useToastStore.getState().show(text, 'success'),
  error: (text: string) => useToastStore.getState().show(text, 'error'),
}
