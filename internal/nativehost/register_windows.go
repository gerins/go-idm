//go:build windows

package nativehost

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

type target struct {
	name string
	key  string // under HKEY_CURRENT_USER
}

func targets() []target {
	return []target{
		{"Google Chrome", `Software\Google\Chrome\NativeMessagingHosts\` + HostName},
		{"Chromium", `Software\Chromium\NativeMessagingHosts\` + HostName},
		{"Microsoft Edge", `Software\Microsoft\Edge\NativeMessagingHosts\` + HostName},
		{"Brave", `Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\` + HostName},
	}
}

func manifestFile(dataDir string) string {
	return filepath.Join(dataDir, "native-messaging", HostName+".json")
}

// Install writes the manifest into dataDir and points each browser's
// per-user registry key at it. Registry keys for browsers that are not
// installed are harmless.
func Install(hostPath, dataDir string, extensionIDs ...string) ([]string, error) {
	data, err := Manifest(hostPath, extensionIDs...)
	if err != nil {
		return nil, err
	}
	file := manifestFile(dataDir)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return nil, err
	}
	var names []string
	for _, t := range targets() {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, t.key, registry.SET_VALUE)
		if err != nil {
			return names, err
		}
		err = k.SetStringValue("", file)
		k.Close()
		if err != nil {
			return names, err
		}
		names = append(names, t.name)
	}
	return names, nil
}

// Uninstall removes the registry keys and the manifest file.
func Uninstall(dataDir string) error {
	for _, t := range targets() {
		if err := registry.DeleteKey(registry.CURRENT_USER, t.key); err != nil && err != registry.ErrNotExist {
			return err
		}
	}
	if err := os.Remove(manifestFile(dataDir)); err != nil && !os.IsNotExist(err) {
		return err
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
