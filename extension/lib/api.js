// The WebExtension API object. Firefox's native namespace is `browser`; Chrome
// and other Chromium browsers only have `chrome`. With Manifest V3 both return
// promises, so the rest of the extension uses this and never takes callbacks.
export const ext = globalThis.browser ?? globalThis.chrome
