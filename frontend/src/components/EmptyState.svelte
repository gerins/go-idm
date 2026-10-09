<script lang="ts">
  import { ui } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  let { filtered = false }: { filtered?: boolean } = $props()
</script>

<div class="grid h-full place-items-center p-8">
  <div class="max-w-sm text-center">
    <div class="mx-auto mb-4 grid size-16 place-items-center rounded-2xl bg-surface-2 text-faint">
      <Icon name={filtered ? 'search' : 'inbox'} size={30} />
    </div>
    {#if filtered}
      <h2 class="text-[15px] font-semibold">Nothing here</h2>
      <p class="mt-1 text-muted">No downloads match this view.</p>
    {:else}
      <h2 class="text-[15px] font-semibold">No downloads yet</h2>
      <p class="mt-1 text-muted">
        Paste a link with <kbd class="rounded border border-border bg-surface-2 px-1.5 py-0.5 text-xs">Ctrl</kbd>
        + <kbd class="rounded border border-border bg-surface-2 px-1.5 py-0.5 text-xs">V</kbd>, drop one onto the
        window, or add it manually.
      </p>
      <button class="btn btn-primary mx-auto mt-5" onclick={() => ui.openAdd()}>
        <Icon name="plus" size={16} /> Add URL
      </button>
    {/if}
  </div>
</div>
