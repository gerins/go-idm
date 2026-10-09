import { parseHostList } from './lib/capture.js'
import { getSettings, saveSettings } from './lib/settings.js'
import { ext } from './lib/api.js'

const $ = (id) => document.getElementById(id)
let timer

function flashSaved() {
  $('saved').classList.add('on')
  clearTimeout(timer)
  timer = setTimeout(() => $('saved').classList.remove('on'), 1200)
}

async function persist() {
  const minSizeMB = Math.max(0, parseFloat($('minSize').value) || 0)
  await saveSettings({ minSizeMB, excludedHosts: parseHostList($('excluded').value) })
  flashSaved()
}

async function init() {
  const s = await getSettings()
  $('minSize').value = s.minSizeMB
  $('excluded').value = s.excludedHosts.join('\n')
  $('extId').textContent = ext.runtime.id

  let debounce
  const later = () => {
    clearTimeout(debounce)
    debounce = setTimeout(persist, 400)
  }
  $('minSize').addEventListener('input', later)
  $('excluded').addEventListener('input', later)
}

init()
