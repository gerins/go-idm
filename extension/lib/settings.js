export const DEFAULTS = Object.freeze({
  enabled: true,
  minSizeMB: 1,
  excludedHosts: [],
})

export async function getSettings() {
  const stored = await chrome.storage.local.get(DEFAULTS)
  return { ...DEFAULTS, ...stored }
}

export async function saveSettings(patch) {
  await chrome.storage.local.set(patch)
}
