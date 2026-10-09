import test from 'node:test'
import assert from 'node:assert/strict'
import { MAX_PER_TAB, MIN_MEDIA_BYTES, addMedia, classifyMedia, formatSize } from '../lib/media.js'

const h = (obj) => Object.entries(obj).map(([name, value]) => ({ name, value }))
const mp4 = (over = {}) => ({
  url: 'https://cdn.example.com/videos/My%20Clip.mp4?token=abc#t=5',
  status: 200,
  headers: h({ 'Content-Type': 'video/mp4', 'Content-Length': '52428800' }),
  ...over,
})

test('a video response is detected with its name, type and size', () => {
  assert.deepEqual(classifyMedia(mp4()), {
    url: 'https://cdn.example.com/videos/My%20Clip.mp4?token=abc',
    kind: 'video',
    mime: 'video/mp4',
    size: 52428800,
    name: 'My Clip.mp4',
  })
})

test('audio and generic downloads with a media extension are detected', () => {
  const song = classifyMedia({ url: 'https://x.example/a/song.mp3', status: 200, headers: h({ 'content-type': 'audio/mpeg', 'content-length': '4000000' }) })
  assert.equal(song.kind, 'audio')
  const generic = classifyMedia({ url: 'https://x.example/dl/movie.mkv', status: 200, headers: h({ 'content-type': 'application/octet-stream', 'content-length': '9000000' }) })
  assert.equal(generic.kind, 'video')
  assert.equal(classifyMedia({ url: 'https://x.example/dl/data.bin', status: 200, headers: h({ 'content-type': 'application/octet-stream' }) }), null)
})

test('size comes from Content-Range on a partial response', () => {
  const r = classifyMedia(mp4({ status: 206, headers: h({ 'content-type': 'video/mp4', 'content-range': 'bytes 0-1048575/73400320', 'content-length': '1048576' }) }))
  assert.equal(r.size, 73400320)
  // A 206 without a total must not report the piece length as the file size.
  const unknown = classifyMedia(mp4({ status: 206, headers: h({ 'content-type': 'video/mp4', 'content-range': 'bytes 0-1048575/*', 'content-length': '1048576' }) }))
  assert.equal(unknown.size, -1)
})

test('tiny files are ignored, unknown sizes are kept', () => {
  assert.equal(classifyMedia(mp4({ headers: h({ 'content-type': 'video/mp4', 'content-length': String(MIN_MEDIA_BYTES - 1) }) })), null)
  assert.equal(classifyMedia(mp4({ headers: h({ 'content-type': 'video/mp4' }) })).size, -1)
})

test('streaming formats and chunked requests are ignored', () => {
  const big = { 'content-length': '9000000' }
  for (const [url, type] of [
    ['https://x.example/live/index.m3u8', 'application/vnd.apple.mpegurl'],
    ['https://x.example/live/seg1.ts', 'video/mp2t'],
    ['https://x.example/dash/manifest.mpd', 'application/dash+xml'],
    ['https://x.example/dash/chunk.m4s', 'video/iso.segment'],
    ['https://x.example/dash/chunk0.mp4?range=0-999999', 'video/mp4'],
    ['https://r1.googlevideo.com/videoplayback?id=1', 'video/mp4'],
  ]) {
    assert.equal(classifyMedia({ url, status: 200, headers: h({ 'content-type': type, ...big }) }), null, url)
  }
})

test('errors, other content and odd urls are ignored', () => {
  assert.equal(classifyMedia(mp4({ status: 404 })), null)
  assert.equal(classifyMedia(mp4({ status: 304 })), null)
  assert.equal(classifyMedia(mp4({ headers: h({ 'content-type': 'text/html' }) })), null)
  assert.equal(classifyMedia(mp4({ url: 'blob:https://x.example/uuid' })), null)
  assert.equal(classifyMedia(mp4({ url: 'not a url' })), null)
})

test('addMedia keeps one entry per url, newest first, and caps the list', () => {
  const item = (n, size = 1e6) => ({ url: `https://x.example/${n}.mp4`, kind: 'video', mime: 'video/mp4', size, name: `${n}.mp4` })
  let list = []
  for (let i = 0; i < MAX_PER_TAB + 5; i++) list = addMedia(list, item(i))
  assert.equal(list.length, MAX_PER_TAB)
  assert.equal(list[0].name, `${MAX_PER_TAB + 4}.mp4`)

  list = addMedia(list, item(MAX_PER_TAB + 2))
  assert.equal(list.length, MAX_PER_TAB)
  assert.equal(list[0].name, `${MAX_PER_TAB + 2}.mp4`)
  assert.equal(list.filter((m) => m.name === `${MAX_PER_TAB + 2}.mp4`).length, 1)
})

test('a later response without a size keeps the size already known', () => {
  const base = { url: 'https://x.example/a.mp4', kind: 'video', mime: 'video/mp4', name: 'a.mp4' }
  const list = addMedia(addMedia([], { ...base, size: 5e6 }), { ...base, size: -1 })
  assert.equal(list[0].size, 5e6)
})

test('formatSize', () => {
  assert.equal(formatSize(-1), '')
  assert.equal(formatSize(0), '')
  assert.equal(formatSize(512), '512 B')
  assert.equal(formatSize(1536), '1.5 KB')
  assert.equal(formatSize(52428800), '50.0 MB')
  assert.equal(formatSize(5 * 1024 ** 3), '5.0 GB')
  assert.equal(formatSize(150 * 1024 ** 2), '150 MB')
})
