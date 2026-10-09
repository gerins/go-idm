# GoIDM browser extension

Chrome, Edge, Brave and other Chromium browsers, and Firefox 128+ (Manifest V3). It hands downloads to the GoIDM desktop app, together with the browser's cookies, referrer and user agent, so links that only work in a logged-in browser still download.

```
browser ── extension ──native messaging──▶ idm-host ──local HTTP + token──▶ GoIDM app
```

## Install

1. Build or install GoIDM (`make build`, which also produces `idm-host` and this folder next to the app).
2. In GoIDM open **Settings, Browser integration** and click **Install**. This registers the native host with your browsers.
3. In the browser open `chrome://extensions`, enable **Developer mode**, click **Load unpacked** and pick this folder (GoIDM's settings page has a link that reveals it).

The extension ID is fixed by the `key` in `manifest.json` (`bopmpbnmogfmiheanjbjeiiddkobbmng`), which is what the native host manifest allows. Reloading or moving the folder does not change it.

### Firefox

Firefox needs a different manifest (`manifest.firefox.json`: background scripts instead of a service worker, and a gecko ID), so the build produces a second folder, `extension-firefox/`, next to `extension/`.

1. Build GoIDM (`make build`, or `make build-firefox-extension` for just the folder) and click **Install** in **Settings, Browser integration**. This also registers the native host with Firefox (`~/.mozilla/native-messaging-hosts` on Linux, `~/Library/Application Support/Mozilla/NativeMessagingHosts` on macOS, `HKCU\Software\Mozilla\NativeMessagingHosts` on Windows).
2. In Firefox open `about:debugging#/runtime/this-firefox`, click **Load Temporary Add-on…** and pick `manifest.json` in `extension-firefox/`.

A temporary add-on is removed when Firefox closes. To keep it permanently, run `make firefox-zip` and submit `build/bin/goidm-firefox.zip` at [addons.mozilla.org](https://addons.mozilla.org/developers/) (choose **On your own** to get a signed `.xpi` without listing it), then install the signed file. Firefox Developer Edition and Nightly can instead load unsigned add-ons with `xpinstall.signatures.required` set to `false`.

The add-on ID `goidm@go-idm` is fixed in `manifest.firefox.json` and is what the native host manifest allows. It is not supported in Snap or Flatpak builds of Firefox, whose sandbox cannot reach the native host.

## What it does

- **Captures downloads.** When the browser starts a download it is paused, sent to GoIDM, and cancelled in the browser only after GoIDM accepted it. If GoIDM can't be reached the download just resumes in the browser, so nothing is lost.
- **Remembers the download page.** The page the download came from is saved with it, and the link button on a download in GoIDM reopens that page in your browser (to fetch a fresh link or find the next file). The browser often trims the referrer to just the site, so the active tab's address is used when it is on the same site. Downloads from before this feature fall back to their referrer.
- **Right-click, Download with GoIDM** on links, images, video and audio.
- **Starts GoIDM** if it isn't running when a download is captured.
- **Popup** shows connection status and has the on/off switch. **Options** sets a minimum size (default 1 MB) and a list of sites to leave alone.

By default GoIDM shows its Add dialog for each captured download (turn off **Ask before downloading** in its settings to start them immediately).

## Privacy

Cookies are read only for the URL being downloaded and are sent only to the GoIDM app on your own computer, over a loopback connection that needs a secret token stored in your user profile. They are saved with the download in GoIDM's local database. Nothing is sent anywhere else.

## Limits

- Downloads that cannot be replayed with a plain GET (POST forms, `blob:` URLs) stay in the browser.
- Firefox: cookies are read from the default cookie store, so downloads from a container tab are sent without that container's cookies.
- If you publish the Chromium extension to a store, the store assigns its own ID. Pass that ID to `nativehost.Install` so the host allows it. The Firefox ID is the one in `manifest.firefox.json`, so signing keeps it.

## Develop

```bash
make ext-test        # unit tests for the capture logic and manifests (node --test)
cd extension && go run icons/gen.go   # regenerate icons
```

After editing, press the reload button on `chrome://extensions` (or **Reload** under the add-on in `about:debugging`; run `make build-firefox-extension` first to refresh the Firefox folder). Service worker logs are under **Inspect views: service worker**, or **Inspect** in `about:debugging` for Firefox.

The code calls the browser through `lib/api.js` (`browser` on Firefox, `chrome` elsewhere) and only uses promises, so the same files run on both. Keep `manifest.firefox.json` in step with `manifest.json`; `make ext-test` checks they agree on permissions and UI.
