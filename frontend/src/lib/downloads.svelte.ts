import { EventsOn } from '../../wailsjs/runtime/runtime'
import { api } from './api'
import type { Info } from './types'

export type Filter = 'all' | 'active' | 'waiting' | 'completed' | 'failed' | `cat:${string}`

class Downloads {
  items = $state.raw<Record<string, Info>>({})
  loaded = $state(false)

  all = $derived(Object.values(this.items).sort((a, b) => b.createdAt.localeCompare(a.createdAt)))

  counts = $derived.by(() => {
    const c = { all: 0, active: 0, waiting: 0, completed: 0, failed: 0 }
    const cats: Record<string, number> = {}
    for (const d of this.all) {
      c.all++
      if (d.status === 'active') c.active++
      else if (d.status === 'queued' || d.status === 'paused') c.waiting++
      else if (d.status === 'completed') c.completed++
      else if (d.status === 'failed') c.failed++
      if (d.category) cats[d.category] = (cats[d.category] ?? 0) + 1
    }
    return { ...c, cats }
  })

  totalSpeed = $derived(this.all.reduce((n, d) => (d.status === 'active' ? n + d.speed : n), 0))

  async init() {
    EventsOn('downloads:update', (infos: Info[]) => this.apply(infos))
    EventsOn('downloads:removed', (id: string) => {
      const { [id]: _gone, ...rest } = this.items
      this.items = rest
    })
    // Events registered first win over the snapshot for ids they already touched.
    const list = await api.list()
    const next = { ...this.items }
    for (const d of list) next[d.id] ??= d
    this.items = next
    this.loaded = true
  }

  apply(infos: Info[]) {
    const next = { ...this.items }
    for (const i of infos) next[i.id] = i
    this.items = next
  }

  has(url: string): boolean {
    return this.all.some((d) => d.url === url)
  }

  filtered(filter: Filter, query: string): Info[] {
    const q = query.trim().toLowerCase()
    return this.all.filter((d) => {
      if (q && !d.fileName.toLowerCase().includes(q) && !d.url.toLowerCase().includes(q)) return false
      switch (filter) {
        case 'all':
          return true
        case 'active':
          return d.status === 'active'
        case 'waiting':
          return d.status === 'queued' || d.status === 'paused'
        case 'completed':
          return d.status === 'completed'
        case 'failed':
          return d.status === 'failed'
        default:
          return d.category === filter.slice(4)
      }
    })
  }
}

export const downloads = new Downloads()
