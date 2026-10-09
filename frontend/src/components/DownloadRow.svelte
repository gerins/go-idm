<script lang="ts">
  import { api, errMsg } from '../lib/api'
  import { categoryFor, metaFor } from '../lib/category'
  import { bytes, eta, percent, speed, when } from '../lib/format'
  import { toasts } from '../lib/toasts.svelte'
  import type { Info } from '../lib/types'
  import { ui } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'
  import SegmentBar from './SegmentBar.svelte'

  let { d }: { d: Info } = $props()

  // Before the first probe the engine has no category; guess from the name.
  const category = $derived(d.category || categoryFor(d.fileName))
  const meta = $derived(metaFor(category))
  const pct = $derived(percent(d.downloaded, d.size))
  const connecting = $derived(d.status === 'active' && d.downloaded === 0 && d.segments.length === 0)

  const live = $derived.by(() => {
    const parts: string[] = []
    if (d.segments.length > 1) parts.push(`${d.segments.length} connections`)
    parts.push(speed(d.speed))
    if (d.eta >= 0) parts.push(`${eta(d.eta)} left`)
    return parts.join(' · ')
  })

  const statusLabel = $derived(
    {
      active: connecting ? 'Connecting…' : 'Downloading',
      queued: 'Waiting in queue',
      paused: 'Paused',
      failed: 'Failed',
      completed: 'Completed',
    }[d.status],
  )
  const statusColor = $derived(
    {
      active: 'text-accent',
      queued: 'text-muted',
      paused: 'text-warn',
      failed: 'text-danger',
      completed: 'text-success',
    }[d.status],
  )

  async function run(fn: () => Promise<unknown>) {
    try {
      await fn()
    } catch (e) {
      toasts.error('Action failed', errMsg(e))
    }
  }
</script>

<div
  class="group flex items-center gap-3 border-b border-border/60 px-4 py-3 transition-colors hover:bg-surface-2/60"
  role="row"
  tabindex="-1"
  ondblclick={() => d.status === 'completed' && run(() => api.openFile(d.id))}
>
  <div
    class="grid size-10 shrink-0 place-items-center rounded-xl"
    style="background: color-mix(in srgb, {meta.color} 16%, transparent); color: {meta.color}"
  >
    <Icon name={meta.icon} size={20} />
  </div>

  <div class="min-w-0 flex-1">
    <div class="flex items-baseline gap-3">
      <span class="truncate font-medium" title={d.fileName || d.url}>{d.fileName || d.url}</span>
      <span class="ml-auto shrink-0 text-xs font-medium {statusColor}">{statusLabel}</span>
    </div>

    {#if d.status !== 'completed'}
      <div class="mt-2"><SegmentBar {d} /></div>
    {/if}

    <div class="mt-1.5 flex items-center gap-3 text-xs text-muted tabular-nums">
      {#if d.status === 'completed'}
        <span>{bytes(d.size)} · {category} · {when(d.completedAt)}</span>
      {:else if d.status === 'failed'}
        <span class="truncate text-danger" title={d.error}>{d.error || 'Download failed'}</span>
      {:else if d.size > 0}
        <span>{bytes(d.downloaded)} of {bytes(d.size)} ({pct.toFixed(0)}%)</span>
      {:else if d.status === 'queued' || connecting}
        <span>{d.status === 'queued' ? 'Waiting to start' : 'Contacting server…'}</span>
      {:else}
        <span>{bytes(d.downloaded)} · size unknown</span>
      {/if}

      <span class="ml-auto shrink-0">
        {#if d.status === 'active' && !connecting}
          {live}
        {:else if d.status === 'paused' && !d.resumable && d.downloaded > 0}
          <span class="text-warn">Server can't resume — will restart</span>
        {/if}
      </span>
    </div>
  </div>

  <div class="flex shrink-0 items-center gap-0.5">
    {#if d.status === 'active' || d.status === 'queued'}
      <button class="icon-btn" title="Pause" aria-label="Pause" onclick={() => run(() => api.pause(d.id))}>
        <Icon name="pause" />
      </button>
    {:else if d.status === 'paused'}
      <button class="icon-btn" title="Resume" aria-label="Resume" onclick={() => run(() => api.resume(d.id))}>
        <Icon name="play" />
      </button>
    {:else if d.status === 'failed'}
      <button class="icon-btn" title="Retry" aria-label="Retry" onclick={() => run(() => api.resume(d.id))}>
        <Icon name="retry" />
      </button>
    {:else}
      <button class="icon-btn" title="Open file" aria-label="Open file" onclick={() => run(() => api.openFile(d.id))}>
        <Icon name="external" />
      </button>
    {/if}
    {#if d.path}
      <button
        class="icon-btn"
        title="Show in folder"
        aria-label="Show in folder"
        onclick={() => run(() => api.showInFolder(d.id))}
      >
        <Icon name="folder" />
      </button>
    {/if}
    <button class="icon-btn danger" title="Remove" aria-label="Remove" onclick={() => (ui.removeTarget = d)}>
      <Icon name="trash" />
    </button>
  </div>
</div>
