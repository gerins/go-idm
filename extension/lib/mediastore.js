// Per-tab list of detected media. The background worker can be stopped and
// restarted at any time, so the list lives in session storage (cleared when
// the browser closes) rather than in memory.

import { ext } from './api.js'
import { addMedia } from './media.js'

const key = (tabId) => `media:${tabId}`

// Reads and writes go through one queue so two responses arriving together
// can't overwrite each other's update.
let queue = Promise.resolve()
function enqueue(fn) {
  const run = queue.then(fn)
  queue = run.catch(() => {})
  return run
}

async function read(tabId) {
  return (await ext.storage.session.get(key(tabId)))[key(tabId)] ?? []
}

/** Records an item for a tab and resolves with how many the tab now has. */
export function recordMedia(tabId, item) {
  return enqueue(async () => {
    const next = addMedia(await read(tabId), item)
    await ext.storage.session.set({ [key(tabId)]: next })
    return next.length
  })
}

export function getMedia(tabId) {
  return enqueue(() => read(tabId))
}

export function clearMedia(tabId) {
  return enqueue(() => ext.storage.session.remove(key(tabId)))
}
