<script lang="ts">
  import { onMount } from 'svelte'
  import { api, errMsg } from '../lib/api'
  import { toasts } from '../lib/toasts.svelte'
  import type { Integration } from '../lib/types'
  import Icon from './Icon.svelte'

  let info = $state.raw<Integration | null>(null)
  let busy = $state(false)
  let browser = $state<'chromium' | 'firefox'>('chromium')

  const registered = $derived(info?.browsers.some((b) => b.installed && b.current) ?? false)
  const stale = $derived(info?.browsers.some((b) => b.installed && !b.current) ?? false)

  onMount(async () => {
    try {
      info = await api.integration()
    } catch (e) {
      toasts.error('Could not read browser integration', errMsg(e))
    }
  })

  async function run(fn: () => Promise<Integration>, ok: string) {
    busy = true
    try {
      info = await fn()
      toasts.success(ok)
    } catch (e) {
      toasts.error('Browser integration', errMsg(e))
    } finally {
      busy = false
    }
  }
</script>

{#if info}
  <div class="space-y-3 rounded-xl border border-border bg-bg p-3">
    <div class="flex items-start gap-3">
      <div class="min-w-0 flex-1">
        <div class="font-medium">
          {#if registered}Native host registered{:else if stale}Registration is out of date{:else}Not set up yet{/if}
        </div>
        <div class="text-xs text-muted">
          {#if !info.hostFound}
            <span class="text-warn">{info.hostPath.split(/[\\/]/).pop()} was not found next to GoIDM.</span>
            Build it with <code class="selectable">make build-host</code>.
          {:else}
            Lets the browser extension hand downloads to GoIDM.
          {/if}
        </div>
      </div>
      <button
        type="button"
        class="btn btn-primary shrink-0"
        disabled={busy || !info.hostFound}
        onclick={() => run(api.installIntegration, 'Browser integration installed')}
      >
        {registered ? 'Reinstall' : stale ? 'Repair' : 'Install'}
      </button>
    </div>

    <ul class="space-y-1 text-xs">
      {#each info.browsers as b (b.name)}
        <li class="flex items-center gap-2 text-muted">
          <span
            class="size-2 rounded-full {b.installed && b.current ? 'bg-success' : b.installed ? 'bg-warn' : 'bg-faint'}"
          ></span>
          <span class="flex-1">{b.name}</span>
          <span>{b.installed ? (b.current ? 'Registered' : 'Out of date') : 'Not registered'}</span>
        </li>
      {/each}
    </ul>

    <div class="space-y-2 border-t border-border pt-3">
      <div class="flex gap-1">
        <button
          type="button"
          class="btn h-7 px-2 text-xs {browser === 'chromium' ? 'btn-primary' : 'btn-ghost'}"
          onclick={() => (browser = 'chromium')}>Chrome, Edge, Brave</button
        >
        <button
          type="button"
          class="btn h-7 px-2 text-xs {browser === 'firefox' ? 'btn-primary' : 'btn-ghost'}"
          onclick={() => (browser = 'firefox')}>Firefox</button
        >
      </div>

      {#if browser === 'chromium'}
        <ol class="list-decimal space-y-1 pl-5 text-xs text-muted">
          <li>Click <strong class="text-text">Install</strong> above.</li>
          <li>
            In your browser open <code class="selectable">chrome://extensions</code> and turn on Developer mode.
          </li>
          <li>
            Choose <strong class="text-text">Load unpacked</strong> and select the
            <button
              type="button"
              class="text-accent underline underline-offset-2"
              disabled={!info.extensionFound}
              onclick={() => api.revealExtension().catch((e) => toasts.error('Could not open folder', errMsg(e)))}
              >extension folder</button
            >.
          </li>
        </ol>
      {:else}
        <ol class="list-decimal space-y-1 pl-5 text-xs text-muted">
          <li>Click <strong class="text-text">Install</strong> above.</li>
          <li>In Firefox open <code class="selectable">about:debugging#/runtime/this-firefox</code>.</li>
          <li>
            Choose <strong class="text-text">Load Temporary Add-on</strong> and select
            <code class="selectable">manifest.json</code> in the
            <button
              type="button"
              class="text-accent underline underline-offset-2"
              disabled={!info.firefoxExtensionFound}
              onclick={() =>
                api.revealFirefoxExtension().catch((e) => toasts.error('Could not open folder', errMsg(e)))}
              >Firefox extension folder</button
            >.
          </li>
        </ol>
        <p class="pl-5 text-xs text-faint">
          A temporary add-on is removed when Firefox closes. For a permanent install the extension must be signed at
          addons.mozilla.org (see extension/README.md).
        </p>
      {/if}
    </div>

    {#if registered || stale}
      <div class="flex justify-end">
        <button
          type="button"
          class="btn btn-ghost h-7 px-2 text-xs"
          disabled={busy}
          onclick={() => run(api.removeIntegration, 'Browser integration removed')}
        >
          <Icon name="trash" size={13} /> Remove
        </button>
      </div>
    {/if}
  </div>
{/if}
