<script lang="ts">
  import { api, errMsg } from '../lib/api'
  import { downloads, type Filter } from '../lib/downloads.svelte'
  import { toasts } from '../lib/toasts.svelte'
  import { ui } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  const titles: Record<string, string> = {
    all: 'All downloads',
    active: 'Downloading',
    waiting: 'Queued & paused',
    completed: 'Completed',
    failed: 'Failed',
  }

  function title(f: Filter): string {
    return f.startsWith('cat:') ? f.slice(4) : titles[f]
  }

  const count = $derived(downloads.filtered(ui.filter, ui.query).length)

  async function run(fn: () => Promise<unknown>) {
    try {
      await fn()
    } catch (e) {
      toasts.error('Action failed', errMsg(e))
    }
  }
</script>

<header class="flex h-14 shrink-0 items-center gap-3 border-b border-border px-4">
  <h1 class="text-[15px] font-semibold tracking-tight">{title(ui.filter)}</h1>
  <span class="rounded-full bg-surface-2 px-2 py-0.5 text-xs text-muted tabular-nums">{count}</span>

  <div class="ml-auto flex items-center gap-2">
    <label class="relative">
      <span class="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-faint">
        <Icon name="search" size={15} />
      </span>
      <input class="field w-56 pl-8" type="search" placeholder="Search downloads" bind:value={ui.query} />
    </label>
    <button
      class="btn"
      disabled={downloads.counts.waiting === 0 && downloads.counts.failed === 0}
      onclick={() => run(api.resumeAll)}
      title="Resume all paused and failed downloads"
    >
      <Icon name="play" size={15} /> Resume all
    </button>
    <button
      class="btn"
      disabled={downloads.counts.active === 0 && downloads.counts.waiting === 0}
      onclick={() => run(api.pauseAll)}
      title="Pause everything"
    >
      <Icon name="pause" size={15} /> Pause all
    </button>
    <button class="btn btn-primary" onclick={() => ui.openAdd()}>
      <Icon name="plus" size={16} /> Add URL
    </button>
  </div>
</header>
