//go:build windows

package nativehost

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

type target struct {
	name    string
	key     string // under HKEY_CURRENT_USER
	firefox bool   // uses Firefox's manifest format
}

func targets() []target {
	return []target{
		{name: "Google Chrome", key: `Software\Google\Chrome\NativeMessagingHosts\` + HostName},
		{name: "Chromium", key: `Software\Chromium\NativeMessagingHosts\` + HostName},
		{name: "Microsoft Edge", key: `Software\Microsoft\Edge\NativeMessagingHosts\` + HostName},
		{name: "Brave", key: `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + HostName},
		{name: "Firefox", key: `Software\Mozilla\NativeMessagingHosts\` + HostName, firefox: true},
	}
}

// manifestFile is where a browser family's manifest lives. Firefox needs its
// own file because its manifest format differs from Chromium's.
func manifestFile(dataDir string, firefox bool) string {
	name := HostName + ".json"
	if firefox {
		name = HostName + ".firefox.json"
	}
	return filepath.Join(dataDir, "native-messaging", name)
}

// Install writes the manifests into dataDir and points each browser's
// per-user registry key at the right one. Registry keys for browsers that are
// not installed are harmless. extensionIDs are the Chromium extension IDs to
// allow; Firefox always allows FirefoxExtensionID.
func Install(hostPath, dataDir string, extensionIDs ...string) ([]string, error) {
	chromium, err := Manifest(hostPath, extensionIDs...)
	if err != nil {
		return nil, err
	}
	firefox, err := FirefoxManifest(hostPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "native-messaging"), 0o755); err != nil {
		return nil, err
	}
	for isFirefox, data := range map[bool][]byte{false: chromium, true: firefox} {
		if err := os.WriteFile(manifestFile(dataDir, isFirefox), data, 0o644); err != nil {
			return nil, err
		}
	}
	var names []string
	for _, t := range targets() {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, t.key, registry.SET_VALUE)
		if err != nil {
			return names, err
		}
		err = k.SetStringValue("", manifestFile(dataDir, t.firefox))
		k.Close()
		if err != nil {
			return names, err
		}
		names = append(names, t.name)
	}
	return names, nil
}

// Uninstall removes the registry keys and the manifest files.
func Uninstall(dataDir string) error {
	for _, t := range targets() {
		if err := registry.DeleteKey(registry.CURRENT_USER, t.key); err != nil && err != registry.ErrNotExist {
			return err
		}
	}
	for _, firefox := range []bool{false, true} {
		if err := os.Remove(manifestFile(dataDir, firefox)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Status reports registration per browser.
func Status(hostPath string) []BrowserStatus {
	var out []BrowserStatus
	for _, t := range targets() {
		st := BrowserStatus{Name: t.name}
		if k, err := registry.OpenKey(registry.CURRENT_USER, t.key, registry.QUERY_VALUE); err == nil {
			if file, _, err := k.GetStringValue(""); err == nil {
				st.Installed = true
				if data, err := os.ReadFile(file); err == nil {
					st.Current = readManifestPath(data) == hostPath
				}
			}
			k.Close()
		}
		out = append(out, st)
	}
	return out
}
