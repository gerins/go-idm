import test from 'node:test'
import assert from 'node:assert/strict'

// An in-memory stand-in for storage.session whose calls take a moment, so two
// unqueued read-modify-write cycles would overlap and lose an update.
const data = new Map()
const tick = () => new Promise((r) => setTimeout(r, 5))
globalThis.chrome = {
  storage: {
    session: {
      async get(k) {
        await tick()
        return data.has(k) ? { [k]: structuredClone(data.get(k)) } : {}
      },
      async set(obj) {
        await tick()
        for (const [k, v] of Object.entries(obj)) data.set(k, structuredClone(v))
      },
      async remove(k) {
        await tick()
        data.delete(k)
      },
    },
  },
}
const { recordMedia, getMedia, clearMedia } = await import('../lib/mediastore.js')

const item = (n) => ({ url: `https://x.example/${n}.mp4`, kind: 'video', mime: 'video/mp4', size: 1e6, name: `${n}.mp4` })

test('responses arriving together are all recorded', async () => {
  await Promise.all([1, 2, 3, 4, 5].map((n) => recordMedia(7, item(n))))
  assert.equal((await getMedia(7)).length, 5)
})

test('tabs are separate and a tab can be cleared', async () => {
  await recordMedia(8, item(1))
  await clearMedia(7)
  assert.deepEqual(await getMedia(7), [])
  assert.equal((await getMedia(8)).length, 1)
})
