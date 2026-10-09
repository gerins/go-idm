<script lang="ts">
  import type { Snippet } from 'svelte'
  import Icon from './Icon.svelte'

  let {
    open = $bindable(false),
    title,
    width = 520,
    children,
  }: { open: boolean; title: string; width?: number; children: Snippet } = $props()

  let dlg: HTMLDialogElement

  $effect(() => {
    if (open && !dlg.open) {
      dlg.showModal()
      queueMicrotask(() => dlg.querySelector<HTMLElement>('[data-autofocus]')?.focus())
    } else if (!open && dlg.open) {
      dlg.close()
    }
  })
</script>

<dialog
  bind:this={dlg}
  class="m-auto max-h-[92vh] max-w-[94vw] overflow-hidden rounded-2xl border border-border bg-surface p-0 shadow-[var(--shadow)]"
  style="width: {width}px"
  onclose={() => (open = false)}
  onclick={(e) => {
    if (e.target === dlg) open = false
  }}
>
  {#if open}
    <div class="flex max-h-[92vh] flex-col">
      <header class="flex items-center justify-between px-5 pt-4 pb-3">
        <h2 class="text-[15px] font-semibold">{title}</h2>
        <button class="icon-btn -mr-2" onclick={() => (open = false)} aria-label="Close">
          <Icon name="x" size={16} />
        </button>
      </header>
      {@render children()}
    </div>
  {/if}
</dialog>
