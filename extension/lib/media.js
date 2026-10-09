// Spotting downloadable video and audio files in a page's network traffic.
// Pure functions, kept free of browser APIs so they can be unit tested.

/** Files smaller than this are ads, sound effects and probes, not what the user wants. */
export const MIN_MEDIA_BYTES = 256 * 1024
/** Most items remembered per tab; the oldest are dropped. */
export const MAX_PER_TAB = 20

const AUDIO_EXT = /\.(mp3|m4a|aac|ogg|oga|opus|wav|flac)$/i
const VIDEO_EXT = /\.(mp4|m4v|webm|mkv|mov|avi|flv|wmv|mpg|mpeg|3gp)$/i
// Streaming formats are many small pieces, not one file GoIDM can fetch.
const STREAMING_MIME = /mpegurl|dash\+xml|mp2t|iso\.segment|ms-sstr/
const STREAMING_PATH = /\.(m3u8|mpd|ts|m4s)$|videoplayback/i
const CHUNK_PARAMS = ['range', 'bytestart', 'byterange']

function headerValue(headers, name) {
  const h = (headers ?? []).find((x) => x.name.toLowerCase() === name)
  return h?.value ?? ''
}

/** Total size of the file from a response, or -1 when it can't be told. */
function totalSize(status, headers) {
  const range = /\/(\d+)\s*$/.exec(headerValue(headers, 'content-range'))
  if (range) return Number(range[1])
  // On a 206 Content-Length is the length of the piece, not of the file.
  if (status === 200) {
    const n = Number(headerValue(headers, 'content-length'))
    if (Number.isFinite(n) && n > 0) return n
  }
  return -1
}

function fileName(u) {
  const last = u.pathname.split('/').filter(Boolean).pop() ?? ''
  try {
    return decodeURIComponent(last) || u.hostname
  } catch {
    return last || u.hostname
  }
}

/**
 * Decides whether an HTTP response is a downloadable media file.
 * Returns { url, kind: 'video' | 'audio', mime, size, name } or null.
 */
export function classifyMedia({ url, status, headers }) {
  let u
  try {
    u = new URL(url)
  } catch {
    return null
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
  if (status !== 200 && status !== 206) return null

  const mime = headerValue(headers, 'content-type').split(';')[0].trim().toLowerCase()
  if (STREAMING_MIME.test(mime) || STREAMING_PATH.test(u.pathname)) return null
  if (CHUNK_PARAMS.some((p) => u.searchParams.has(p))) return null

  let kind = null
  if (mime.startsWith('video/')) kind = 'video'
  else if (mime.startsWith('audio/') || mime === 'application/ogg') kind = 'audio'
  else if (mime === '' || mime === 'application/octet-stream' || mime === 'binary/octet-stream') {
    // Servers often label media as a generic download; the file name tells.
    if (AUDIO_EXT.test(u.pathname)) kind = 'audio'
    else if (VIDEO_EXT.test(u.pathname)) kind = 'video'
  }
  if (!kind) return null

  const size = totalSize(status, headers)
  if (size >= 0 && size < MIN_MEDIA_BYTES) return null

  u.hash = ''
  return { url: u.href, kind, mime, size, name: fileName(u) }
}

/**
 * Adds an item to a tab's list: newest first, one entry per URL, capped.
 * A later partial response that doesn't reveal the size keeps the known one.
 */
export function addMedia(list, item) {
  const old = list.find((m) => m.url === item.url)
  const merged = old && item.size < 0 ? { ...item, size: old.size } : item
  return [merged, ...list.filter((m) => m.url !== item.url)].slice(0, MAX_PER_TAB)
}

/** "148 MB", or "" when the size is unknown. */
export function formatSize(bytes) {
  if (!(bytes > 0)) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let n = bytes
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1)} ${units[i]}`
}
