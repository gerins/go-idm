# GoIDM browser extension

Chrome, Edge, Brave and other Chromium browsers (Manifest V3). It hands downloads to the GoIDM desktop app, together with the browser's cookies, referrer and user agent, so links that only work in a logged-in browser still download.

```
browser ── extension ──native messaging──▶ idm-host ──local HTTP + token──▶ GoIDM app
```

## Install

1. Build or install GoIDM (`make build`, which also produces `idm-host` and this folder next to the app).
2. In GoIDM open **Settings, Browser integration** and click **Install**. This registers the native host with your browsers.
3. In the browser open `chrome://extensions`, enable **Developer mode**, click **Load unpacked** and pick this folder (GoIDM's settings page has a link that reveals it).

The extension ID is fixed by the `key` in `manifest.json` (`bopmpbnmogfmiheanjbjeiiddkobbmng`), which is what the native host manifest allows. Reloading or moving the folder does not change it.

## What it does

- **Captures downloads.** When the browser starts a download it is paused, sent to GoIDM, and cancelled in the browser only after GoIDM accepted it. If GoIDM can't be reached the download just resumes in the browser, so nothing is lost.
- **Right-click, Download with GoIDM** on links, images, video and audio.
- **Starts GoIDM** if it isn't running when a download is captured.
- **Popup** shows connection status and has the on/off switch. **Options** sets a minimum size (default 1 MB) and a list of sites to leave alone.

By default GoIDM shows its Add dialog for each captured download (turn off **Ask before downloading** in its settings to start them immediately).

## Privacy

Cookies are read only for the URL being downloaded and are sent only to the GoIDM app on your own computer, over a loopback connection that needs a secret token stored in your user profile. They are saved with the download in GoIDM's local database. Nothing is sent anywhere else.

## Limits

- Downloads that cannot be replayed with a plain GET (POST forms, `blob:` URLs) stay in the browser.
- Firefox is not supported (it needs a different host manifest).
- If you publish the extension to a store, the store assigns its own ID. Pass that ID to `nativehost.Install` so the host allows it.

## Develop

```bash
make ext-test        # unit tests for the capture logic (node --test)
cd extension && go run icons/gen.go   # regenerate icons
```

After editing, press the reload button on `chrome://extensions`. Service worker logs are under **Inspect views: service worker**.
