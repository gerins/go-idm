import type { Filter } from './downloads.svelte'
import type { Info } from './types'

class UI {
  filter = $state<Filter>('all')
  query = $state('')
  addOpen = $state(false)
  addPrefill = $state('')
  settingsOpen = $state(false)
  removeTarget = $state.raw<Info | null>(null)

  openAdd(prefill = '') {
    this.addPrefill = prefill
    this.addOpen = true
  }
}

export const ui = new UI()
