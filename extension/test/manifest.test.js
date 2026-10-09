import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const load = (name) => JSON.parse(readFileSync(new URL(`../${name}`, import.meta.url), 'utf8'))
const chromium = load('manifest.json')
const firefox = load('manifest.firefox.json')

test('both manifests ask for the same permissions and UI', () => {
  for (const key of ['manifest_version', 'version', 'name', 'permissions', 'host_permissions', 'action', 'options_ui', 'icons']) {
    assert.deepEqual(firefox[key], chromium[key], key)
  }
})

test('Firefox uses background scripts, not a service worker', () => {
  assert.deepEqual(firefox.background.scripts, ['background.js'])
  assert.equal(firefox.background.service_worker, undefined)
  assert.equal(chromium.background.service_worker, 'background.js')
})

test('Chromium-only keys stay out of the Firefox manifest', () => {
  assert.equal(firefox.key, undefined)
  assert.equal(firefox.minimum_chrome_version, undefined)
  assert.match(firefox.browser_specific_settings.gecko.id, /^[\w.-]*@[\w.-]+$/)
})
