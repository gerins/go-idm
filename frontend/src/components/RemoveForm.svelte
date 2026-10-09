<script lang="ts">
  import { untrack } from 'svelte'
  import { api, errMsg } from '../lib/api'
  import { toasts } from '../lib/toasts.svelte'
  import type { Info } from '../lib/types'

  let { d, onclose }: { d: Info; onclose: () => void } = $props()

  const completed = $derived(d.status === 'completed')
  // The form is recreated for each removal, so the initial value is enough.
  let deleteFiles = $state(untrack(() => d.status !== 'completed'))
  let busy = $state(false)

  async function confirm() {
    busy = true
    try {
      await api.remove(d.id, deleteFiles)
      onclose()
    } catch (e) {
      toasts.error('Could not remove download', errMsg(e))
      busy = false
    }
  }
</script>

<div class="px-5 pb-1">
  <p class="break-words text-muted">
    Remove <span class="font-medium text-text">{d.fileName || d.url}</span> from the list?
  </p>
  <label class="mt-4 flex items-center gap-2 text-muted">
    <input type="checkbox" class="accent-[var(--accent)]" bind:checked={deleteFiles} />
    {completed ? 'Also delete the file from disk' : 'Also delete the partial download'}
  </label>
</div>
<footer class="mt-4 flex justify-end gap-2 border-t border-border px-5 py-3">
  <button class="btn" onclick={onclose}>Cancel</button>
  <button class="btn btn-danger" data-autofocus disabled={busy} onclick={confirm}>Remove</button>
</footer>
