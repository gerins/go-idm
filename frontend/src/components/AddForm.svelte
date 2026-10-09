<script lang="ts">
  import { api, errMsg } from '../lib/api'
  import { categoryFor, metaFor } from '../lib/category'
  import { downloads } from '../lib/downloads.svelte'
  import { bytes, extractUrls } from '../lib/format'
  import { settings } from '../lib/settings.svelte'
  import { toasts } from '../lib/toasts.svelte'
  import type { ProbeResult } from '../lib/types'
  import { ui } from '../lib/ui.svelte'
  import Icon from './Icon.svelte'

  let { onclose }: { onclose: () => void } = $props()

  type Probe =
    | { state: 'idle' }
    | { state: 'loading' }
    | { state: 'ok'; r: ProbeResult }
    | { state: 'error'; msg: string }

  const cfg = settings.config
  let text = $state(ui.addPrefill)
  let probe = $state<Probe>({ state: 'idle' })
  let nameEdit = $state<string | null>(null)
  let dirOverride = $state('')
  let connections = $state(cfg?.connections ?? 8)
  let limitValue = $state('')
  let limitUnit = $state<'KB' | 'MB'>('KB')
  let startNow = $state(true)
  let advancedOpen = $state(false)
  // Headers captured from the browser (Referer, Cookie, User-Agent) prefill Advanced.
  const preset = ui.addHeaders
  const fromBrowser = Object.keys(preset).length > 0
  let referer = $state(preset.Referer ?? '')
  let cookie = $state(preset.Cookie ?? '')
  let extraHeaders = $state(
    Object.entries(preset)
      .filter(([k]) => k !== 'Referer' && k !== 'Cookie')
      .map(([k, v]) => `${k}: ${v}`)
      .join('\n'),
  )
  let submitting = $state(false)
  let error = $state('')

  const urls = $derived(extractUrls(text))
  const single = $derived(urls.length === 1)
  const duplicate = $derived(urls.some((u) => downloads.has(u)))
  const resumable = $derived(probe.state !== 'ok' || probe.r.resumable)
  const suggestedName = $derived(probe.state === 'ok' ? probe.r.fileName : '')
  const fileName = $derived(nameEdit ?? suggestedName)
  const connOptions = $derived([...new Set([1, 2, 4, 8, 16, 32, cfg?.connections ?? 8])].sort((a, b) => a - b))

  const destination = $derived.by(() => {
    if (dirOverride) return dirOverride
    const base = cfg?.downloadDir ?? ''
    if (!cfg?.categorize || !fileName) return base
    const sep = base.includes('\\') ? '\\' : '/'
    return `${base}${sep}${categoryFor(fileName)}`
  })

  // Browser context some hosts require: Referer, Cookie and "Name: value" lines.
  const headers = $derived.by(() => {
    const h: Record<string, string> = {}
    if (referer.trim()) h.Referer = referer.trim()
    if (cookie.trim()) h.Cookie = cookie.trim()
    for (const line of extraHeaders.split('\n')) {
      const i = line.indexOf(':')
      if (i <= 0) continue
      const k = line.slice(0, i).trim()
      const v = line.slice(i + 1).trim()
      if (k && v) h[k] = v
    }
    return h
  })

  const needsBrowserContext = $derived(probe.state === 'error' && /\b(401|403)\b/.test(probe.msg))

  $effect(() => {
    if (needsBrowserContext) advancedOpen = true
  })

  let seq = 0
  $effect(() => {
    if (urls.length !== 1) {
      probe = { state: 'idle' }
      return
    }
    const url = urls[0]
    const hdrs = headers
    const mine = ++seq
    probe = { state: 'loading' }
    const timer = setTimeout(async () => {
      try {
        const r = await api.probe(url, hdrs)
        if (mine === seq) probe = { state: 'ok', r }
      } catch (e) {
        if (mine === seq) probe = { state: 'error', msg: errMsg(e) }
      }
    }, 450)
    return () => clearTimeout(timer)
  })

  async function browse() {
    try {
      const dir = await api.chooseFolder(dirOverride || cfg?.downloadDir || '')
      if (dir) dirOverride = dir
    } catch (e) {
      error = errMsg(e)
    }
  }

  async function submit(e: Event) {
    e.preventDefault()
    if (urls.length === 0 || submitting) return
    submitting = true
    error = ''
    const unit = limitUnit === 'MB' ? 1024 * 1024 : 1024
    const limit = Math.max(0, Math.round((parseFloat(limitValue) || 0) * unit))
    let added = 0
    try {
      for (const url of urls) {
        await api.add({
          url,
          dir: dirOverride,
          fileName: single && nameEdit ? nameEdit : '',
          connections: resumable ? connections : 1,
          speedLimit: limit,
          headers,
          pageUrl: ui.addPageUrl,
          startPaused: !startNow,
        })
        added++
      }
      toasts.success(added === 1 ? 'Download added' : `${added} downloads added`)
      onclose()
    } catch (err) {
      error = (added > 0 ? `${added} added, then failed: ` : '') + errMsg(err)
    } finally {
      submitting = false
    }
  }
</script>

<form class="flex min-h-0 flex-col" onsubmit={submit}>
  <div class="space-y-4 overflow-y-auto px-5 pb-1">
    <div>
      <label class="label" for="add-url">Link{urls.length > 1 ? `s (${urls.length})` : ''}</label>
      <textarea
        id="add-url"
        data-autofocus
        class="field selectable"
        rows={Math.min(Math.max(text.split('\n').length, 2), 5)}
        placeholder="https://example.com/file.zip — one link per line"
        bind:value={text}
        spellcheck="false"
      ></textarea>
      {#if fromBrowser}
        <p class="mt-1.5 flex items-center gap-1.5 text-xs text-success">
          <Icon name="check" size={13} /> Captured from your browser, including its session
        </p>
      {/if}
      {#if duplicate}
        <p class="mt-1.5 text-xs text-warn">This link is already in your list. It will be downloaded again.</p>
      {/if}
    </div>

    {#if single}
      <div class="rounded-xl border border-border bg-bg p-3">
        {#if probe.state === 'loading'}
          <div class="flex items-center gap-2 text-muted">
            <Icon name="spinner" size={15} class="animate-spin" /> Checking link…
          </div>
        {:else if probe.state === 'ok'}
          {@const meta = metaFor(categoryFor(fileName || probe.r.fileName))}
          <div class="flex items-center gap-3">
            <div
              class="grid size-9 shrink-0 place-items-center rounded-lg"
              style="background: color-mix(in srgb, {meta.color} 16%, transparent); color: {meta.color}"
            >
              <Icon name={meta.icon} size={18} />
            </div>
            <div class="min-w-0 flex-1">
              <div class="truncate font-medium">{fileName || probe.r.fileName}</div>
              <div class="text-xs text-muted">
                {probe.r.size >= 0 ? bytes(probe.r.size) : 'Unknown size'} ·
                {#if probe.r.resumable}
                  <span class="text-success">Supports pause &amp; resume</span>
                {:else}
                  <span class="text-warn">No resume support — single connection</span>
                {/if}
              </div>
            </div>
          </div>
        {:else if probe.state === 'error'}
          <div class="flex items-start gap-2 text-danger">
            <Icon name="alert" size={16} class="mt-0.5 shrink-0" />
            <div class="text-xs">
              <div class="font-medium">Can't reach this link</div>
              <div class="text-muted">{probe.msg}.</div>
              {#if needsBrowserContext}
                <div class="mt-1 text-muted">
                  Some sites check what a browser sends. Add the Referer or Cookie from your browser under Advanced
                  below.
                </div>
              {:else}
                <div class="text-muted">You can still add it anyway.</div>
              {/if}
            </div>
          </div>
        {:else}
          <div class="text-muted">Paste a link to see details.</div>
        {/if}
      </div>

      <div>
        <label class="label" for="add-name">Save as</label>
        <input
          id="add-name"
          class="field selectable"
          placeholder={suggestedName || 'Detected automatically'}
          value={fileName}
          oninput={(e) => (nameEdit = e.currentTarget.value)}
          spellcheck="false"
        />
      </div>
    {/if}

    <div>
      <span class="label">Save to</span>
      <div class="flex gap-2">
        <input class="field selectable min-w-0 flex-1" readonly value={destination} title={destination} />
        <button type="button" class="btn shrink-0" onclick={browse}>
          <Icon name="folder" size={15} /> Browse
        </button>
        {#if dirOverride}
          <button type="button" class="btn btn-ghost shrink-0" onclick={() => (dirOverride = '')}>Reset</button>
        {/if}
      </div>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <div>
        <label class="label" for="add-conn">Connections</label>
        <select id="add-conn" class="field" bind:value={connections} disabled={!resumable}>
          {#each connOptions as n}<option value={n}>{n}</option>{/each}
        </select>
      </div>
      <div>
        <label class="label" for="add-limit">Speed limit</label>
        <div class="flex gap-2">
          <input
            id="add-limit"
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
    </div>

    <details class="group rounded-xl border border-border" bind:open={advancedOpen}>
      <summary class="flex cursor-default list-none items-center justify-between px-3 py-2 text-muted select-none">
        <span class="font-medium">Advanced</span>
        <span class="text-xs text-faint">Referer, Cookie, headers</span>
      </summary>
      <div class="space-y-3 border-t border-border p-3">
        <div>
          <label class="label" for="add-ref">Referer</label>
          <input
            id="add-ref"
            class="field selectable"
            placeholder="Defaults to the link's own site"
            bind:value={referer}
            spellcheck="false"
          />
        </div>
        <div>
          <label class="label" for="add-cookie">Cookie</label>
          <input
            id="add-cookie"
            class="field selectable"
            placeholder="name=value; other=value"
            bind:value={cookie}
            spellcheck="false"
            autocomplete="off"
          />
        </div>
        <div>
          <label class="label" for="add-hdrs">Other headers</label>
          <textarea
            id="add-hdrs"
            class="field selectable"
            rows="2"
            placeholder="Authorization: Bearer …"
            bind:value={extraHeaders}
            spellcheck="false"
          ></textarea>
        </div>
        <p class="text-xs text-faint">
          Copy these from your browser's developer tools (Network tab, request headers). They are stored on this
          computer with the download.
        </p>
      </div>
    </details>

    {#if error}
      <p class="rounded-lg bg-danger/10 px-3 py-2 text-xs text-danger" role="alert">{error}</p>
    {/if}
  </div>

  <footer class="mt-4 flex items-center gap-3 border-t border-border px-5 py-3">
    <label class="flex items-center gap-2 text-muted">
      <input type="checkbox" class="accent-[var(--accent)]" bind:checked={startNow} /> Start immediately
    </label>
    <div class="ml-auto flex gap-2">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={urls.length === 0 || submitting}>
        <Icon name="download" size={15} />
        {urls.length > 1 ? `Download ${urls.length} files` : 'Download'}
      </button>
    </div>
  </footer>
</form>
