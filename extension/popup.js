import { send, describeError } from './lib/native.js'
import { getSettings, saveSettings } from './lib/settings.js'
import { ext } from './lib/api.js'
import { formatSize } from './lib/media.js'
import { getMedia } from './lib/mediastore.js'

const $ = (id) => document.getElementById(id)

function show(kind, title, detail = '') {
  $('dot').className = `dot ${kind}`
  $('status').textContent = title
  $('detail').textContent = detail
}

async function check() {
  const res = await send({ type: 'ping' })
  if (res.ok) return show('ok', 'Connected', `GoIDM ${res.version}`)
  if (res.error === 'app_not_running') {
    return show('warn', 'GoIDM is not running', 'It starts automatically the next time a download is captured.')
  }
  show('bad', 'Not connected', describeError(res.error))
}

async function sendMedia(tabId, item, button) {
  button.disabled = true
  button.textContent = 'Sending…'
  const res = await ext.runtime.sendMessage({ type: 'download-media', tabId, url: item.url })
  if (res?.ok) {
    button.textContent = 'Sent'
    return
  }
  button.disabled = false
  button.textContent = 'Download'
  show('bad', 'Could not send the video', describeError(res?.error ?? 'unknown_error'))
}

// Names come from web pages, so everything is inserted as text, never as HTML.
async function renderMedia(settings) {
  const [tab] = await ext.tabs.query({ active: true, currentWindow: true })
  const items = settings.detectMedia && tab ? await getMedia(tab.id) : []
  $('media').hidden = items.length === 0
  const list = $('mediaList')
  list.replaceChildren(
    ...items.map((item) => {
      const li = document.createElement('li')
      const info = document.createElement('div')
      info.className = 'info'
      const name = document.createElement('div')
      name.className = 'name'
      name.textContent = item.name
      name.title = item.url
      const meta = document.createElement('div')
      meta.className = 'meta'
      meta.textContent = [item.kind === 'audio' ? 'Audio' : 'Video', formatSize(item.size), item.mime].filter(Boolean).join(' · ')
      info.append(name, meta)
      const button = document.createElement('button')
      button.className = 'primary'
      button.textContent = 'Download'
      button.addEventListener('click', () => sendMedia(tab.id, item, button))
      li.append(info, button)
      return li
    }),
  )
}

async function init() {
  const settings = await getSettings()
  $('enabled').checked = settings.enabled
  $('enabled').addEventListener('change', (e) => saveSettings({ enabled: e.target.checked }))

  $('options').addEventListener('click', () => ext.runtime.openOptionsPage())
  $('open').addEventListener('click', async () => {
    const res = await send({ type: 'show' })
    if (res.ok) window.close()
    else show('bad', 'Could not open GoIDM', describeError(res.error))
  })

  // Opening the popup acknowledges any earlier hand-off error.
  if ((await ext.action.getBadgeText({})) === '!') {
    await ext.action.setBadgeText({ text: settings.enabled ? '' : 'OFF' })
  }

  await renderMedia(settings)
  await check()
}

init()
