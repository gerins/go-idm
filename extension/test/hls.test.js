import test from 'node:test'
import assert from 'node:assert/strict'
import { analyzePlaylist } from '../lib/hls.js'

const base = 'https://cdn.example.com/a/master.m3u8'
const lines = (...l) => l.join('\n')

test('a master playlist lists its quality playlists, resolved against its URL', () => {
  const r = analyzePlaylist(
    lines(
      '#EXTM3U',
      '#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360',
      '',
      'low/index.m3u8',
      '#EXT-X-STREAM-INF:BANDWIDTH=2400000',
      'https://other.example.com/high.m3u8',
    ),
    base,
  )
  assert.equal(r.master, true)
  assert.deepEqual(r.variants, ['https://cdn.example.com/a/low/index.m3u8', 'https://other.example.com/high.m3u8'])
  assert.equal(r.live, false)
})

test('a finished single-quality playlist has a duration', () => {
  const r = analyzePlaylist(lines('#EXTM3U', '#EXTINF:4.5,', 's0.ts', '#EXTINF:3,', 's1.ts', '#EXT-X-ENDLIST'), base)
  assert.deepEqual(r, { master: false, variants: [], live: false, drm: false, duration: 7.5 })
})

test('live playlists are flagged, VOD ones are not', () => {
  assert.equal(analyzePlaylist(lines('#EXTM3U', '#EXTINF:4,', 's0.ts'), base).live, true)
  assert.equal(analyzePlaylist(lines('#EXTM3U', '#EXT-X-PLAYLIST-TYPE:VOD', '#EXTINF:4,', 's0.ts'), base).live, false)
})

test('encryption: AES-128 is fine, other methods are DRM', () => {
  const withKey = (method) =>
    analyzePlaylist(lines('#EXTM3U', `#EXT-X-KEY:METHOD=${method},URI="k"`, '#EXTINF:4,', 's0.ts', '#EXT-X-ENDLIST'), base)
  assert.equal(withKey('AES-128').drm, false)
  assert.equal(withKey('NONE').drm, false)
  assert.equal(withKey('SAMPLE-AES').drm, true)
  assert.equal(withKey('SAMPLE-AES-CTR').drm, true)
})

test('things that are not HLS playlists give null', () => {
  assert.equal(analyzePlaylist('', base), null)
  assert.equal(analyzePlaylist('<html>nope</html>', base), null)
  assert.equal(analyzePlaylist(lines('#EXTM3U', 'song1.mp3', 'song2.mp3'), base), null) // a music .m3u
  assert.equal(analyzePlaylist('﻿#EXTM3U\n#EXTINF:4,\ns.ts\n#EXT-X-ENDLIST', base).live, false) // BOM tolerated
})
