<script lang="ts">
  import type { Info } from '../lib/types'

  let { d }: { d: Info } = $props()

  const color = $derived(
    {
      active: 'var(--accent)',
      queued: 'var(--faint)',
      paused: 'var(--warn)',
      failed: 'var(--danger)',
      completed: 'var(--success)',
    }[d.status],
  )

  // A stream has a segment per few seconds, hundreds in all: one bar then.
  const manySegments = $derived(d.segments.length > 32)

  // Unknown size (or still connecting): nothing meaningful to fill.
  const indeterminate = $derived(d.status === 'active' && (d.size <= 0 || d.segments.length === 0))
</script>

{#if indeterminate}
  <div class="indeterminate h-1.5 rounded-full opacity-80"></div>
{:else if d.size > 0 && manySegments}
  <div class="relative h-1.5 overflow-hidden rounded-full bg-surface-3" role="progressbar" aria-valuenow={d.downloaded} aria-valuemax={d.size}>
    <div
      class="absolute inset-y-0 left-0 rounded-full transition-[width] duration-500 ease-linear"
      style="width: {Math.min(100, (d.downloaded / d.size) * 100)}%; background: {color}"
    ></div>
  </div>
{:else if d.size > 0 && d.segments.length > 0}
  <div class="flex h-1.5 gap-[2px]" role="progressbar" aria-valuenow={d.downloaded} aria-valuemax={d.size}>
    {#each d.segments as s}
      {@const len = Math.max(s.end - s.start + 1, 1)}
      <div class="relative overflow-hidden rounded-full bg-surface-3" style="flex: {len} 1 0">
        <div
          class="absolute inset-y-0 left-0 rounded-full transition-[width] duration-500 ease-linear"
          style="width: {Math.min(100, (s.done / len) * 100)}%; background: {color}"
        ></div>
      </div>
    {/each}
  </div>
{:else}
  <div class="h-1.5 rounded-full bg-surface-3"></div>
{/if}
