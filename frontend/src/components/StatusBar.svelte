<script lang="ts">
  import { bytes, speed } from '../lib/format'
  import { downloads } from '../lib/downloads.svelte'
  import { settings } from '../lib/settings.svelte'

  const active = $derived(downloads.counts.active)
  const limit = $derived(settings.config?.speedLimit ?? 0)
</script>

<footer class="flex h-8 shrink-0 items-center gap-4 border-t border-border bg-surface px-4 text-xs text-muted">
  <span class="flex items-center gap-2">
    <span class="size-2 rounded-full {active > 0 ? 'bg-success' : 'bg-faint'}"></span>
    {#if active > 0}
      Downloading {active} {active === 1 ? 'file' : 'files'}
    {:else}
      Idle
    {/if}
  </span>
  {#if downloads.counts.waiting > 0}
    <span>{downloads.counts.waiting} waiting</span>
  {/if}
  <span class="ml-auto flex items-center gap-4 tabular-nums">
    {#if limit > 0}<span>Limit {speed(limit)}</span>{/if}
    <span class="text-text">↓ {bytes(downloads.totalSpeed)}/s</span>
  </span>
</footer>
