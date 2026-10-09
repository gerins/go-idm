# GoIDM

A desktop download manager written in Go (engine) with a Svelte UI, packaged with [Wails v2](https://wails.io).

## Features (phase 1)

- Segmented downloads over HTTP `Range` (up to 32 connections), with a one-stream fallback for servers without range support
- Pause, resume and crash-safe resume (progress is persisted per segment in SQLite; resume re-validates size and ETag)
- Queue with a concurrency limit, global and per-download speed limits
- Retries with backoff, stall detection, proxy support (http, https, socks5)
- Filename detection (Content-Disposition, URL, MIME), safe on Windows, collision-free names
- Sorting into Video, Music, Documents and other folders
- Clipboard link watcher, paste (Ctrl+V) and drag-and-drop of links
- Light and dark themes following the OS
- Chrome/Edge/Brave/Firefox extension that captures browser downloads (with cookies and referrer) and hands them to the app, see [extension/README.md](extension/README.md)

## Layout

```
internal/engine   download engine: probe, segmenter, jobs, queue, limiter
internal/netx     HTTP client and filename helpers
internal/store    SQLite persistence (pure Go driver, no CGO)
internal/ipc      authenticated loopback channel between the browser host and the app
internal/nativehost  native messaging protocol and browser registration
cmd/idm-cli       headless CLI built on the same engine
cmd/idm-host      native messaging host the browser launches
extension/        Manifest V3 browser extension (Chromium and Firefox)
app.go, main.go   Wails bindings and window setup
frontend/         Svelte 5 + TypeScript + Tailwind v4 UI
```

## Develop

Requires Go 1.26+, Node 20+ and the Wails CLI (`make deps` installs the CLI and the frontend packages).

```bash
make help            # list all targets
make dev             # live-reload app; also serves the UI at http://localhost:34115
make test            # Go tests with -race (local httptest servers, no network)
make check           # gofmt, vet (mac + windows), frontend type-check, tests
make build           # production build for the current OS
make build-windows   # cross-compile build/bin/goidm.exe
make build-cli       # headless CLI into build/bin/idm-cli
make build-host      # native host into build/bin (make dev does this for you)
make build-firefox-extension  # Firefox extension folder into build/bin/extension-firefox
make firefox-zip     # zip it for signing at addons.mozilla.org
make ext-test        # browser extension unit tests
```

Set `GOIDM_DATA_DIR` to keep the database somewhere other than the default
(`%AppData%\GoIDM` on Windows) while testing.

CLI: `make cli ARGS="-o ./out -c 8 https://example.com/file.zip"`

## Not yet implemented

System tray and native notifications (needs Wails v3 or a tray library), scheduler,
HLS/DASH, checksum verification, queue reordering.
