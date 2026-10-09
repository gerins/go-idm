import type { Filter } from './downloads.svelte'
import type { Info } from './types'

class UI {
  filter = $state<Filter>('all')
  query = $state('')
  addOpen = $state(false)
  addPrefill = $state('')
  addHeaders = $state.raw<Record<string, string>>({})
  settingsOpen = $state(false)
  removeTarget = $state.raw<Info | null>(null)

  openAdd(prefill = '', headers: Record<string, string> = {}) {
    this.addPrefill = prefill
    this.addHeaders = headers
    this.addOpen = true
  }
}

export const ui = new UI()
