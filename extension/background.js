import { send, describeError } from './lib/native.js'
import { buildAddMessage, cookieHeader, knownSize, pickPageUrl, shouldCapture } from './lib/capture.js'
import { getSettings } from './lib/settings.js'
import { ext } from './lib/api.js'
import { classifyMedia } from './lib/media.js'
import { clearMedia, getMedia, recordMedia } from './lib/mediastore.js'

const MENU_ID = 'goidm-download'

async function cookiesFor(url) {
  try {
    return cookieHeader(await ext.cookies.getAll({ url }))
  } catch {
    return ''
  }
}

async function openTabs() {
  try {
    return await ext.tabs.query({})
  } catch {
    return []
  }
}

async function handOff({ url, referrer, pageUrl, mime, size }) {
  return send(
    buildAddMessage({
      url,
      referrer,
      pageUrl,
      mime,
      size,
      cookies: await cookiesFor(url),
      userAgent: navigator.userAgent,
    }),
  )
}

async function setBadge(text, color = '#d64545') {
  await ext.action.setBadgeBackgroundColor({ color })
  await ext.action.setBadgeText({ text })
}

async function refreshBadge() {
  const { enabled } = await getSettings()
  await setBadge(enabled ? '' : 'OFF', '#6b7280')
}

// The browser has already started the download by the time we hear about it.
// Pause it, hand the URL (with the browser's cookies and referrer) to GoIDM,
// and only cancel the browser's copy once GoIDM has accepted it. If the hand-off
// fails the download simply resumes in the browser, so nothing is ever lost.
async function onDownloadCreated(item) {
  const settings = await getSettings()
  if (!shouldCapture(item, settings).capture) return

  try {
    await ext.downloads.pause(item.id)
  } catch {
    // Not pausable (for example it already finished); checked below.
  }
  const [current] = await ext.downloads.search({ id: item.id })
  if (!current || current.state !== 'in_progress') {
    if (current?.paused) await ext.downloads.resume(item.id).catch(() => {})
    return
  }

  const res = await handOff({
    url: item.url,
    referrer: item.referrer,
    pageUrl: pickPageUrl(item.referrer, await openTabs()),
    mime: item.mime,
    size: knownSize(item),
  })

  if (res.ok) {
    await ext.downloads.cancel(item.id).catch(() => {})
    await ext.downloads.erase({ id: item.id }).catch(() => {})
    await refreshBadge()
  } else {
    await ext.downloads.resume(item.id).catch(() => {})
    await setBadge('!')
    console.warn('GoIDM hand-off failed, download continues in the browser:', describeError(res.error))
  }
}

ext.downloads.onCreated.addListener((item) => {
  onDownloadCreated(item).catch((e) => console.error('GoIDM capture failed', e))
})

ext.runtime.onInstalled.addListener(async () => {
  await ext.contextMenus.removeAll()
  ext.contextMenus.create({
    id: MENU_ID,
    title: 'Download with GoIDM',
    contexts: ['link', 'video', 'audio', 'image'],
  })
  refreshBadge()
})

ext.runtime.onStartup.addListener(refreshBadge)

ext.storage.onChanged.addListener((changes, area) => {
  if (area === 'local' && 'enabled' in changes) refreshBadge()
})

ext.contextMenus.onClicked.addListener(async (info, tab) => {
  if (info.menuItemId !== MENU_ID) return
  const url = info.linkUrl || info.srcUrl
  if (!url) return
  const res = await handOff({
    url,
    referrer: info.frameUrl || info.pageUrl || tab?.url,
    pageUrl: pickPageUrl(info.pageUrl || tab?.url),
  })
  if (res.ok) await refreshBadge()
  else {
    await setBadge('!')
    console.warn('GoIDM hand-off failed:', describeError(res.error))
  }
})

// Video and audio files the page loads are remembered per tab and listed in the
// popup, with their count on the toolbar icon. Classifying is cheap and
// synchronous, so the setting is only read for responses that match.
ext.webRequest.onHeadersReceived.addListener(
  (details) => {
    if (details.tabId < 0) return
    const item = classifyMedia({ url: details.url, status: details.statusCode, headers: details.responseHeaders })
    if (item) onMediaFound(details.tabId, item).catch((e) => console.error('GoIDM media detection failed', e))
  },
  { urls: ['http://*/*', 'https://*/*'], types: ['media', 'xmlhttprequest', 'other'] },
  ['responseHeaders'],
)

async function onMediaFound(tabId, item) {
  if (!(await getSettings()).detectMedia) return
  const count = await recordMedia(tabId, item)
  await ext.action.setBadgeBackgroundColor({ tabId, color: '#3a66f0' })
  await ext.action.setBadgeText({ tabId, text: String(count) })
}

// A new page starts a new list.
ext.tabs.onUpdated.addListener((tabId, change) => {
  if (change.status !== 'loading') return
  clearMedia(tabId).catch(() => {})
  ext.action.setBadgeText({ tabId, text: '' }).catch(() => {})
})
ext.tabs.onRemoved.addListener((tabId) => clearMedia(tabId).catch(() => {}))

// The popup's Download button. Only files we detected on that tab are accepted.
async function downloadMedia({ tabId, url }) {
  const entry = (await getMedia(tabId)).find((m) => m.url === url)
  if (!entry) return { ok: false, error: 'media_gone' }
  const page = (await ext.tabs.get(tabId).catch(() => null))?.url ?? ''
  return handOff({ url: entry.url, referrer: page, pageUrl: pickPageUrl(page), mime: entry.mime, size: entry.size })
}

ext.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg?.type !== 'download-media' || typeof msg.tabId !== 'number' || typeof msg.url !== 'string') return
  downloadMedia(msg).then(sendResponse, (e) => sendResponse({ ok: false, error: String(e?.message ?? e) }))
  return true // the response is sent asynchronously
})
