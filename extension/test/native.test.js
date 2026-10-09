import test from 'node:test'
import assert from 'node:assert/strict'

// lib/api.js binds the extension API at import time, so install one stub first
// and let each test swap what it delegates to.
let handler
globalThis.chrome = { runtime: { sendNativeMessage: (...args) => handler(...args) } }
const { send } = await import('../lib/native.js')

test('send resolves with the host response', async () => {
  handler = async (host, msg) => ({ ok: true, host, type: msg.type })
  assert.deepEqual(await send({ type: 'ping' }), { ok: true, host: 'com.goidm.host', type: 'ping' })
})

test('send turns a rejection into an error code instead of throwing', async () => {
  handler = async () => {
    throw new Error('No such native application com.goidm.host')
  }
  assert.deepEqual(await send({ type: 'ping' }), { ok: false, error: 'host_not_installed' })
})

test('send reports an empty response', async () => {
  handler = async () => undefined
  assert.deepEqual(await send({ type: 'ping' }), { ok: false, error: 'empty_response' })
})
