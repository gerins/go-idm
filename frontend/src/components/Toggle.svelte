<script lang="ts">
  let { checked = $bindable(false), label }: { checked: boolean; label: string } = $props()
</script>

<button
  type="button"
  role="switch"
  aria-checked={checked}
  aria-label={label}
  class="relative h-5 w-9 shrink-0 rounded-full border border-transparent transition-colors {checked
    ? 'bg-accent'
    : 'bg-surface-3'}"
  onclick={() => (checked = !checked)}
>
  <span
    class="absolute top-0.5 left-0.5 size-4 rounded-full bg-white shadow transition-transform {checked
      ? 'translate-x-4'
      : ''}"
  ></span>
</button>
