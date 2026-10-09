import { ext } from './api.js'

export const DEFAULTS = Object.freeze({
  enabled: true,
  minSizeMB: 1,
  excludedHosts: [],
})

export async function getSettings() {
  const stored = await ext.storage.local.get(DEFAULTS)
  return { ...DEFAULTS, ...stored }
}

export async function saveSettings(patch) {
  await ext.storage.local.set(patch)
}
