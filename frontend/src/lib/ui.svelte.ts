import type { Filter } from './downloads.svelte'
import type { Info } from './types'

class UI {
  filter = $state<Filter>('all')
  query = $state('')
  addOpen = $state(false)
  addPrefill = $state('')
  addHeaders = $state.raw<Record<string, string>>({})
  addPageUrl = $state('')
  settingsOpen = $state(false)
  removeTarget = $state.raw<Info | null>(null)
  // Queue reordering: the row being dragged, and where it would land.
  dragId = $state<string | null>(null)
  dropAt = $state.raw<{ id: string; after: boolean } | null>(null)

  openAdd(prefill = '', headers: Record<string, string> = {}, pageUrl = '') {
    this.addPrefill = prefill
    this.addHeaders = headers
    this.addPageUrl = pageUrl
    this.addOpen = true
  }
}

export const ui = new UI()
