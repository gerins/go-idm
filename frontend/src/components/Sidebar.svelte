<script lang="ts">
  import { metaFor } from '../lib/category'
  import { downloads, type Filter } from '../lib/downloads.svelte'
  import { CATEGORIES } from '../lib/types'
  import { ui } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  const nav: { id: Filter; label: string; icon: IconName; count: () => number }[] = [
    { id: 'all', label: 'All downloads', icon: 'list', count: () => downloads.counts.all },
    { id: 'active', label: 'Downloading', icon: 'arrowDown', count: () => downloads.counts.active },
    { id: 'waiting', label: 'Queued & paused', icon: 'clock', count: () => downloads.counts.waiting },
    { id: 'completed', label: 'Completed', icon: 'check', count: () => downloads.counts.completed },
    { id: 'failed', label: 'Failed', icon: 'alert', count: () => downloads.counts.failed },
  ]
</script>

{#snippet item(id: Filter, label: string, icon: IconName, count: number, color?: string)}
  <button
    class="flex h-8 w-full items-center gap-2.5 rounded-lg px-2.5 text-left transition-colors {ui.filter === id
      ? 'bg-accent-soft text-text'
      : 'text-muted hover:bg-surface-2 hover:text-text'}"
    aria-current={ui.filter === id}
    onclick={() => (ui.filter = id)}
  >
    <span style:color={ui.filter === id ? (color ?? 'var(--accent)') : color}><Icon name={icon} size={16} /></span>
    <span class="flex-1 truncate">{label}</span>
    {#if count > 0}<span class="text-xs tabular-nums text-faint">{count}</span>{/if}
  </button>
{/snippet}

<aside class="flex w-[228px] shrink-0 flex-col border-r border-border bg-surface">
  <div class="flex items-center gap-2.5 px-4 pt-4 pb-5">
    <div
      class="grid size-8 place-items-center rounded-[10px] text-accent-fg shadow-sm"
      style="background: linear-gradient(135deg, var(--accent), color-mix(in srgb, var(--accent) 55%, #9b6bff))"
    >
      <Icon name="download" size={18} />
    </div>
    <div class="leading-tight">
      <div class="text-[15px] font-semibold tracking-tight">GoIDM</div>
      <div class="text-[11px] text-faint">Download manager</div>
    </div>
  </div>

  <nav class="flex-1 space-y-0.5 overflow-y-auto px-2.5" aria-label="Filters">
    {#each nav as n (n.id)}
      {@render item(n.id, n.label, n.icon, n.count())}
    {/each}

    <div class="px-2.5 pt-5 pb-1.5 text-[11px] font-semibold tracking-wider text-faint uppercase">Categories</div>
    {#each CATEGORIES as c (c)}
      {@const m = metaFor(c)}
      {@render item(`cat:${c}`, c, m.icon, downloads.counts.cats[c] ?? 0, m.color)}
    {/each}
  </nav>

  <div class="border-t border-border p-2.5">
    <button class="btn btn-ghost w-full justify-start" onclick={() => (ui.settingsOpen = true)}>
      <Icon name="sliders" size={16} /> Settings
    </button>
  </div>
</aside>
