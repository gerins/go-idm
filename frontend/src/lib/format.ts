const UNITS = ['B', 'KB', 'MB', 'GB', 'TB']

export function bytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  let i = 0
  let v = n
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024
    i++
  }
  const digits = i === 0 || v >= 100 ? 0 : v >= 10 ? 1 : 2
  return `${v.toFixed(digits)} ${UNITS[i]}`
}

export function speed(n: number): string {
  return `${bytes(n)}/s`
}

export function eta(seconds: number): string {
  if (seconds < 0 || !Number.isFinite(seconds)) return ''
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))}s`
  const m = Math.floor(seconds / 60)
  if (m < 60) return `${m}m ${String(Math.round(seconds % 60)).padStart(2, '0')}s`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h}h ${String(m % 60).padStart(2, '0')}m`
  return `${Math.floor(h / 24)}d ${h % 24}h`
}

/** 75 -> "1:15", 3725 -> "1:02:05". */
export function clock(seconds: number): string {
  const s = Math.max(0, Math.round(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

export function percent(downloaded: number, size: number): number {
  if (size <= 0) return 0
  return Math.min(100, (downloaded / size) * 100)
}

export function when(iso: string): string {
  const t = Date.parse(iso)
  if (!Number.isFinite(t) || t <= 0 || iso.startsWith('0001')) return ''
  const diff = (Date.now() - t) / 1000
  if (diff < 45) return 'just now'
  if (diff < 3600) return `${Math.round(diff / 60)} min ago`
  const d = new Date(t)
  const today = new Date()
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  if (d.toDateString() === today.toDateString()) return `today, ${time}`
  return `${d.toLocaleDateString([], { month: 'short', day: 'numeric' })}, ${time}`
}

/** Parses "https://…" lines out of free text. */
export function extractUrls(text: string): string[] {
  const seen = new Set<string>()
  for (const raw of text.split(/\s+/)) {
    if (!/^https?:\/\//i.test(raw)) continue
    try {
      const u = new URL(raw)
      if (u.host) seen.add(u.toString())
    } catch {
      // ignore malformed
    }
  }
  return [...seen]
}
