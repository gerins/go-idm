// Mirrors the JSON shapes emitted by the Go engine (internal/engine).

export type Status = 'queued' | 'active' | 'paused' | 'completed' | 'failed'

export interface SegmentInfo {
  start: number
  end: number
  done: number
}

export interface Info {
  id: string
  url: string
  fileName: string
  dir: string
  path: string
  category: string
  size: number // -1 when unknown
  downloaded: number
  speed: number // bytes/s
  eta: number // seconds, -1 unknown
  status: Status
  error: string
  resumable: boolean
  connections: number
  speedLimit: number
  segments: SegmentInfo[]
  createdAt: string
  completedAt: string
}

export interface Config {
  downloadDir: string
  maxActive: number
  connections: number
  speedLimit: number
  proxy: string
  userAgent: string
  categorize: boolean
  watchClipboard: boolean
  confirmCaptured: boolean
  maxRetries: number
}

export interface ProbeResult {
  finalUrl: string
  fileName: string
  size: number
  resumable: boolean
  etag: string
  lastModified: string
  contentType: string
}

export interface AddRequest {
  url: string
  dir: string
  fileName: string
  connections: number
  speedLimit: number
  headers: Record<string, string>
  startPaused: boolean
}

export const CATEGORIES = ['Video', 'Music', 'Images', 'Documents', 'Compressed', 'Programs', 'General'] as const

export interface BrowserStatus {
  name: string
  installed: boolean
  current: boolean
}

export interface Integration {
  hostPath: string
  hostFound: boolean
  extensionId: string
  extensionDir: string
  extensionFound: boolean
  firefoxExtensionId: string
  firefoxExtensionDir: string
  firefoxExtensionFound: boolean
  browsers: BrowserStatus[]
}

/** A download captured by the browser extension. */
export interface ExternalAdd {
  url: string
  fileName: string
  headers: Record<string, string> | null
}
