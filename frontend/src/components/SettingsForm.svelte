<script lang="ts">
  import { api, errMsg } from '../lib/api'
  import { settings } from '../lib/settings.svelte'
  import { toasts } from '../lib/toasts.svelte'
  import type { Config } from '../lib/types'
  import Icon from './Icon.svelte'
  import Toggle from './Toggle.svelte'

  let { onclose }: { onclose: () => void } = $props()

  const initial = settings.config as Config
  let downloadDir = $state(initial.downloadDir)
  let categorize = $state(initial.categorize)
  let maxActive = $state(initial.maxActive)
  let connections = $state(initial.connections)
  let maxRetries = $state(initial.maxRetries)
  let proxy = $state(initial.proxy)
  let userAgent = $state(initial.userAgent)
  let watchClipboard = $state(initial.watchClipboard)

  const mb = initial.speedLimit >= 1024 * 1024 && initial.speedLimit % (1024 * 1024) === 0
  let limitUnit = $state<'KB' | 'MB'>(mb ? 'MB' : 'KB')
  let limitValue = $state(
    initial.speedLimit > 0 ? String(initial.speedLimit / (mb ? 1024 * 1024 : 1024)) : '',
  )

  let saving = $state(false)
  let error = $state('')

  async function browse() {
    try {
      const dir = await api.chooseFolder(downloadDir)
      if (dir) downloadDir = dir
    } catch (e) {
      error = errMsg(e)
    }
  }

  async function save(e: Event) {
    e.preventDefault()
    saving = true
    error = ''
    const unit = limitUnit === 'MB' ? 1024 * 1024 : 1024
    try {
      await settings.save({
        downloadDir,
        categorize,
        maxActive: Number(maxActive) || 1,
        connections: Number(connections) || 1,
        maxRetries: Number(maxRetries) || 0,
        proxy: proxy.trim(),
        userAgent: userAgent.trim(),
        watchClipboard,
        speedLimit: Math.max(0, Math.round((parseFloat(limitValue) || 0) * unit)),
      })
      toasts.success('Settings saved')
      onclose()
    } catch (err) {
      error = errMsg(err)
    } finally {
      saving = false
    }
  }
</script>

{#snippet row(title: string, hint: string)}
  <div class="min-w-0 flex-1">
    <div class="font-medium">{title}</div>
    <div class="text-xs text-muted">{hint}</div>
  </div>
{/snippet}

<form class="flex min-h-0 flex-col" onsubmit={save}>
  <div class="space-y-5 overflow-y-auto px-5 pb-1">
    <section class="space-y-3">
      <h3 class="text-[11px] font-semibold tracking-wider text-faint uppercase">Downloads</h3>
      <div>
        <label class="label" for="set-dir">Default download folder</label>
        <div class="flex gap-2">
          <input id="set-dir" class="field selectable min-w-0 flex-1" bind:value={downloadDir} spellcheck="false" />
          <button type="button" class="btn shrink-0" onclick={browse}><Icon name="folder" size={15} /> Browse</button>
        </div>
      </div>
      <div class="flex items-center gap-4">
        {@render row('Sort into folders by type', 'Video, Music, Documents and so on')}
        <Toggle bind:checked={categorize} label="Sort into folders by type" />
      </div>
      <div class="grid grid-cols-3 gap-3">
        <div>
          <label class="label" for="set-active">At once</label>
          <input id="set-active" class="field" type="number" min="1" max="10" bind:value={maxActive} />
        </div>
        <div>
          <label class="label" for="set-conn">Connections</label>
          <input id="set-conn" class="field" type="number" min="1" max="32" bind:value={connections} />
        </div>
        <div>
          <label class="label" for="set-retry">Retries</label>
          <input id="set-retry" class="field" type="number" min="0" max="20" bind:value={maxRetries} />
        </div>
      </div>
    </section>

    <section class="space-y-3">
      <h3 class="text-[11px] font-semibold tracking-wider text-faint uppercase">Network</h3>
      <div>
        <label class="label" for="set-limit">Global speed limit</label>
        <div class="flex gap-2">
          <input
            id="set-limit"
            class="field min-w-0 flex-1"
            type="number"
            min="0"
            step="any"
            placeholder="Unlimited"
            bind:value={limitValue}
          />
          <select class="field w-[84px] shrink-0" bind:value={limitUnit} aria-label="Speed limit unit">
            <option value="KB">KB/s</option>
            <option value="MB">MB/s</option>
          </select>
        </div>
      </div>
      <div>
        <label class="label" for="set-proxy">Proxy</label>
        <input
          id="set-proxy"
          class="field selectable"
          placeholder="Use system settings — or http://host:port, socks5://host:port"
          bind:value={proxy}
          spellcheck="false"
        />
      </div>
      <div>
        <label class="label" for="set-ua">User agent</label>
        <input id="set-ua" class="field selectable" bind:value={userAgent} spellcheck="false" />
      </div>
    </section>

    <section class="space-y-3">
      <h3 class="text-[11px] font-semibold tracking-wider text-faint uppercase">Behavior</h3>
      <div class="flex items-center gap-4">
        {@render row('Watch clipboard for links', 'Offer to download file links you copy')}
        <Toggle bind:checked={watchClipboard} label="Watch clipboard for links" />
      </div>
    </section>

    {#if error}
      <p class="rounded-lg bg-danger/10 px-3 py-2 text-xs text-danger" role="alert">{error}</p>
    {/if}
  </div>

  <footer class="mt-4 flex justify-end gap-2 border-t border-border px-5 py-3">
    <button type="button" class="btn" onclick={onclose}>Cancel</button>
    <button type="submit" class="btn btn-primary" disabled={saving}>Save</button>
  </footer>
</form>
