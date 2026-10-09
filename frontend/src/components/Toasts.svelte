<script lang="ts">
  import { fly } from 'svelte/transition'
  import { toasts } from '../lib/toasts.svelte'
  import Icon from './Icon.svelte'

  const tone = {
    info: 'text-accent',
    success: 'text-success',
    error: 'text-danger',
  } as const
</script>

<div class="pointer-events-none fixed right-4 bottom-12 z-50 flex w-80 flex-col gap-2" aria-live="polite">
  {#each toasts.items as t (t.id)}
    <div
      class="pointer-events-auto rounded-xl border border-border bg-surface-2 p-3 shadow-[var(--shadow)]"
      transition:fly={{ y: 12, duration: 160 }}
      role="status"
    >
      <div class="flex items-start gap-2.5">
        <span class="mt-0.5 {tone[t.kind]}">
          <Icon name={t.kind === 'error' ? 'alert' : t.kind === 'success' ? 'check' : 'link'} size={16} />
        </span>
        <div class="min-w-0 flex-1">
          <div class="font-medium">{t.title}</div>
          {#if t.detail}<div class="mt-0.5 text-xs break-words text-muted">{t.detail}</div>{/if}
          {#if t.actions}
            <div class="mt-2.5 flex gap-2">
              {#each t.actions as a, i}
                <button
                  class="btn h-7 px-2.5 text-xs {i === 0 ? 'btn-primary' : ''}"
                  onclick={() => {
                    toasts.dismiss(t.id)
                    a.run()
                  }}>{a.label}</button
                >
              {/each}
            </div>
          {/if}
        </div>
        <button class="icon-btn -mt-1 -mr-1 size-6" aria-label="Dismiss" onclick={() => toasts.dismiss(t.id)}>
          <Icon name="x" size={13} />
        </button>
      </div>
    </div>
  {/each}
</div>
