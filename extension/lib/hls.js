// Reading just enough of an HLS playlist to decide whether GoIDM can download
// it and how to list it. GoIDM's engine does the real parsing when it downloads.

function resolve(ref, base) {
  try {
    return new URL(ref, base).href
  } catch {
    return ''
  }
}

/**
 * Returns null if text is not an HLS playlist worth listing, otherwise
 * { master, variants, live, drm, duration }:
 *  - master: lists several qualities (variants holds their playlist URLs)
 *  - live: still being written, so there is nothing finite to download
 *  - drm: encrypted in a way other than plain AES-128, which GoIDM can't open
 *  - duration: seconds, for a single-quality playlist
 */
export function analyzePlaylist(text, baseUrl) {
  const lines = String(text).replace(/^﻿/, '').split(/\r?\n/).map((l) => l.trim())
  if (lines[0] !== '#EXTM3U') return null

  let master = false
  let endList = false
  let vod = false
  let drm = false
  let segments = 0
  let duration = 0
  const variants = []

  for (let i = 1; i < lines.length; i++) {
    const line = lines[i]
    if (line.startsWith('#EXT-X-STREAM-INF')) {
      master = true
      const uri = lines.slice(i + 1).find((l) => l && !l.startsWith('#'))
      if (uri) variants.push(resolve(uri, baseUrl))
    } else if (line === '#EXT-X-ENDLIST') {
      endList = true
    } else if (line.startsWith('#EXT-X-PLAYLIST-TYPE:VOD')) {
      vod = true
    } else if (line.startsWith('#EXTINF:')) {
      segments++
      duration += parseFloat(line.slice(8)) || 0
    } else if (line.startsWith('#EXT-X-KEY:')) {
      const method = /METHOD=([^,\s]+)/.exec(line)?.[1]
      if (method && method !== 'NONE' && method !== 'AES-128') drm = true
    }
  }

  if (!master && segments === 0) return null // e.g. a plain .m3u music playlist
  return {
    master,
    variants: variants.filter(Boolean),
    live: !master && !endList && !vod,
    drm,
    duration: master ? 0 : duration,
  }
}
