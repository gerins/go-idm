<script lang="ts">
  import { theme, type ThemePref } from '../lib/theme.svelte'
  import Icon from './Icon.svelte'
  import type { IconName } from './icons'

  const options: { value: ThemePref; label: string; icon: IconName }[] = [
    { value: 'system', label: 'Match system', icon: 'monitor' },
    { value: 'light', label: 'Light', icon: 'sun' },
    { value: 'dark', label: 'Dark', icon: 'moon' },
  ]
</script>

<div class="flex rounded-lg bg-surface-2 p-0.5" role="radiogroup" aria-label="Theme">
  {#each options as o (o.value)}
    <button
      type="button"
      role="radio"
      aria-checked={theme.pref === o.value}
      aria-label={o.label}
      title={o.label}
      class="grid h-7 flex-1 place-items-center rounded-md transition-colors {theme.pref === o.value
        ? 'bg-surface-3 text-text shadow-sm'
        : 'text-faint hover:text-text'}"
      onclick={() => theme.set(o.value)}
    >
      <Icon name={o.icon} size={15} />
    </button>
  {/each}
</div>
