<script lang="ts">
  import Modal from './Modal.svelte'
  import RemoveForm from './RemoveForm.svelte'
  import { ui } from '../lib/ui.svelte'

  let open = $state(false)

  // Open whenever a target is set; closing clears it.
  $effect(() => {
    open = ui.removeTarget !== null
  })
  $effect(() => {
    if (!open) ui.removeTarget = null
  })
</script>

<Modal bind:open title="Remove download" width={440}>
  {#if ui.removeTarget}
    <RemoveForm d={ui.removeTarget} onclose={() => (open = false)} />
  {/if}
</Modal>
