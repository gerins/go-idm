export type ToastKind = 'info' | 'success' | 'error'

export interface ToastAction {
  label: string
  run: () => void
}

export interface Toast {
  id: number
  kind: ToastKind
  title: string
  detail?: string
  actions?: ToastAction[]
}

class Toasts {
  items = $state.raw<Toast[]>([])
  #next = 1

  push(kind: ToastKind, title: string, opts: { detail?: string; actions?: ToastAction[]; ms?: number } = {}) {
    const id = this.#next++
    this.items = [...this.items, { id, kind, title, detail: opts.detail, actions: opts.actions }]
    const ms = opts.ms ?? (kind === 'error' ? 7000 : opts.actions ? 12000 : 3500)
    setTimeout(() => this.dismiss(id), ms)
    return id
  }

  dismiss(id: number) {
    this.items = this.items.filter((t) => t.id !== id)
  }

  info(title: string, detail?: string) {
    return this.push('info', title, { detail })
  }
  success(title: string, detail?: string) {
    return this.push('success', title, { detail })
  }
  error(title: string, detail?: string) {
    return this.push('error', title, { detail })
  }
}

export const toasts = new Toasts()
