import { send, describeError } from './lib/native.js'
import { getSettings, saveSettings } from './lib/settings.js'
import { ext } from './lib/api.js'

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

  await check()
}

init()
