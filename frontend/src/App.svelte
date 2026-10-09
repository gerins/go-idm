<script lang="ts">
  import { onMount } from 'svelte'
  import { EventsOn } from '../wailsjs/runtime/runtime'
  import AddDialog from './components/AddDialog.svelte'
  import DownloadRow from './components/DownloadRow.svelte'
  import EmptyState from './components/EmptyState.svelte'
  import Icon from './components/Icon.svelte'
  import RemoveDialog from './components/RemoveDialog.svelte'
  import SettingsDialog from './components/SettingsDialog.svelte'
  import Sidebar from './components/Sidebar.svelte'
  import StatusBar from './components/StatusBar.svelte'
  import Toasts from './components/Toasts.svelte'
  import Toolbar from './components/Toolbar.svelte'
  import { downloads } from './lib/downloads.svelte'
  import { extractUrls } from './lib/format'
  import { errMsg } from './lib/api'
  import { settings } from './lib/settings.svelte'
  import { toasts } from './lib/toasts.svelte'
  import { ui } from './lib/ui.svelte'

  const visible = $derived(downloads.filtered(ui.filter, ui.query))
  let dragging = $state(false)
  let dragDepth = 0

  const typing = (t: EventTarget | null) =>
    t instanceof HTMLElement && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT')

  onMount(() => {
    Promise.all([downloads.init(), settings.load()]).catch((e) => toasts.error('Failed to start', errMsg(e)))

    const off = EventsOn('clipboard:url', (url: string) => {
      if (ui.addOpen || downloads.has(url)) return
      let name = url
      try {
        name = decodeURIComponent(new URL(url).pathname.split('/').pop() || url)
      } catch {
        // keep the raw URL
      }
      toasts.push('info', 'Download this link?', {
        detail: name,
        actions: [
          { label: 'Download', run: () => ui.openAdd(url) },
          { label: 'Dismiss', run: () => {} },
        ],
      })
    })
    return off
  })

  function onPaste(e: ClipboardEvent) {
    if (typing(e.target) || ui.addOpen || ui.settingsOpen) return
    const urls = extractUrls(e.clipboardData?.getData('text') ?? '')
    if (urls.length) {
      e.preventDefault()
      ui.openAdd(urls.join('\n'))
    }
  }

  function onKey(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n' && !ui.addOpen) {
      e.preventDefault()
      ui.openAdd()
    }
  }

  function hasDroppable(e: DragEvent) {
    const t = e.dataTransfer?.types ?? []
    return t.includes('text/uri-list') || t.includes('text/plain')
  }

  function onDragEnter(e: DragEvent) {
    if (!hasDroppable(e)) return
    dragDepth++
    dragging = true
  }

  function onDragLeave() {
    dragDepth = Math.max(0, dragDepth - 1)
    if (dragDepth === 0) dragging = false
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    dragDepth = 0
    dragging = false
    const text = e.dataTransfer?.getData('text/uri-list') || e.dataTransfer?.getData('text/plain') || ''
    const urls = extractUrls(text)
    if (urls.length) ui.openAdd(urls.join('\n'))
    else toasts.info('Nothing to download', 'Drop an http or https link.')
  }
</script>

<svelte:window
  onpaste={onPaste}
  onkeydown={onKey}
  ondragenter={onDragEnter}
  ondragleave={onDragLeave}
  ondragover={(e) => hasDroppable(e) && e.preventDefault()}
  ondrop={onDrop}
/>

<div class="flex h-full">
  <Sidebar />
  <main class="flex min-w-0 flex-1 flex-col">
    <Toolbar />
    <div class="min-h-0 flex-1 overflow-y-auto" role="table" aria-label="Downloads">
      {#if !downloads.loaded}
        <div class="grid h-full place-items-center text-muted">
          <Icon name="spinner" size={22} class="animate-spin" />
        </div>
      {:else if visible.length === 0}
        <EmptyState filtered={downloads.counts.all > 0} />
      {:else}
        {#each visible as d (d.id)}
          <DownloadRow {d} />
        {/each}
      {/if}
    </div>
    <StatusBar />
  </main>
</div>

{#if dragging}
  <div class="pointer-events-none fixed inset-3 z-40 grid place-items-center rounded-2xl border-2 border-dashed border-accent bg-accent-soft backdrop-blur-sm">
    <div class="flex items-center gap-3 text-[15px] font-medium text-accent">
      <Icon name="download" size={22} /> Drop link to download
    </div>
  </div>
{/if}

<AddDialog />
<SettingsDialog />
<RemoveDialog />
<Toasts />
