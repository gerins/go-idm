import { send, describeError } from './lib/native.js'
import { buildAddMessage, cookieHeader, knownSize, shouldCapture } from './lib/capture.js'
import { getSettings } from './lib/settings.js'
import { ext } from './lib/api.js'

const MENU_ID = 'goidm-download'

async function cookiesFor(url) {
  try {
    return cookieHeader(await ext.cookies.getAll({ url }))
  } catch {
    return ''
  }
}

async function handOff({ url, referrer, mime, size }) {
  return send(
    buildAddMessage({
      url,
      referrer,
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
  const res = await handOff({ url, referrer: info.frameUrl || info.pageUrl || tab?.url })
  if (res.ok) await refreshBadge()
  else {
    await setBadge('!')
    console.warn('GoIDM hand-off failed:', describeError(res.error))
  }
})
