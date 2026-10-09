// Pure decision and message-building logic, kept free of chrome.* so it can be
// unit tested with node --test.

/** Size of a DownloadItem in bytes, or -1 when the browser does not know yet. */
export function knownSize(item) {
  if (item.totalBytes > 0) return item.totalBytes
  if (item.fileSize > 0) return item.fileSize
  return -1
}

/** Normalizes one line of the exclusion list to a bare lowercase hostname. */
export function normalizeHost(line) {
  let s = String(line).trim().toLowerCase()
  if (!s) return ''
  s = s.replace(/^[a-z][a-z0-9+.-]*:\/\//, '') // scheme
  s = s.split(/[/?#]/)[0] // path
  s = s.replace(/:\d+$/, '') // port
  s = s.replace(/^\*\./, '').replace(/^\./, '')
  return s
}

export function parseHostList(text) {
  return [...new Set(String(text).split(/[\n,]/).map(normalizeHost).filter(Boolean))]
}

/** True if hostname equals rule or is a subdomain of it. */
export function hostMatches(hostname, rule) {
  const h = hostname.toLowerCase()
  return h === rule || h.endsWith('.' + rule)
}

/**
 * Decides whether GoIDM should take over a download.
 * Returns { capture: boolean, reason: string }.
 */
export function shouldCapture(item, settings) {
  if (!settings.enabled) return { capture: false, reason: 'disabled' }
  // Downloads started by an extension (including a fallback of ours) are left alone.
  if (item.byExtensionId) return { capture: false, reason: 'started by an extension' }
  if (item.state && item.state !== 'in_progress') return { capture: false, reason: 'not in progress' }

  let url
  try {
    url = new URL(item.url)
  } catch {
    return { capture: false, reason: 'invalid url' }
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return { capture: false, reason: 'unsupported scheme' }
  }
  if ((settings.excludedHosts ?? []).some((rule) => hostMatches(url.hostname, rule))) {
    return { capture: false, reason: 'excluded site' }
  }
  const size = knownSize(item)
  const min = (settings.minSizeMB ?? 0) * 1024 * 1024
  if (size >= 0 && size < min) return { capture: false, reason: 'below minimum size' }
  return { capture: true, reason: 'ok' }
}

/** "a=1; b=2" from chrome.cookies.Cookie objects. */
export function cookieHeader(cookies) {
  return (cookies ?? []).map((c) => `${c.name}=${c.value}`).join('; ')
}

export function buildAddMessage({ url, referrer, cookies, userAgent, mime, size }) {
  const msg = { type: 'add', url }
  if (referrer) msg.referrer = referrer
  if (cookies) msg.cookies = cookies
  if (userAgent) msg.userAgent = userAgent
  if (mime) msg.mime = mime
  if (size > 0) msg.size = size
  return msg
}
