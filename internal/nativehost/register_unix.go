//go:build !windows

package nativehost

import (
	"os"
	"path/filepath"
	"runtime"
)

type target struct {
	name    string
	root    string // the browser's profile root; its existence means the browser is installed
	dir     string // where it looks for host manifests; defaults to root/NativeMessagingHosts
	firefox bool   // uses Firefox's manifest format
}

func (t target) hostDir() string {
	if t.dir != "" {
		return t.dir
	}
	return filepath.Join(t.root, "NativeMessagingHosts")
}

func (t target) manifestPath() string { return filepath.Join(t.hostDir(), HostName+".json") }

func (t target) manifest(hostPath string, extensionIDs []string) ([]byte, error) {
	if t.firefox {
		return FirefoxManifest(hostPath)
	}
	return Manifest(hostPath, extensionIDs...)
}

func targets() []target {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if runtime.GOOS == "darwin" {
		base := filepath.Join(home, "Library", "Application Support")
		return []target{
			{name: "Google Chrome", root: filepath.Join(base, "Google", "Chrome")},
			{name: "Chromium", root: filepath.Join(base, "Chromium")},
			{name: "Microsoft Edge", root: filepath.Join(base, "Microsoft Edge")},
			{name: "Brave", root: filepath.Join(base, "BraveSoftware", "Brave-Browser")},
			{name: "Firefox", root: filepath.Join(base, "Firefox"), dir: filepath.Join(base, "Mozilla", "NativeMessagingHosts"), firefox: true},
		}
	}
	base := filepath.Join(home, ".config")
	return []target{
		{name: "Google Chrome", root: filepath.Join(base, "google-chrome")},
		{name: "Chromium", root: filepath.Join(base, "chromium")},
		{name: "Microsoft Edge", root: filepath.Join(base, "microsoft-edge")},
		{name: "Brave", root: filepath.Join(base, "BraveSoftware", "Brave-Browser")},
		{name: "Firefox", root: filepath.Join(home, ".mozilla"), dir: filepath.Join(home, ".mozilla", "native-messaging-hosts"), firefox: true},
	}
}

// Install registers the host with every installed browser (or with Chrome if
// none is found yet) and returns their names. extensionIDs are the Chromium
// extension IDs to allow; Firefox always allows FirefoxExtensionID. dataDir is
// unused on this platform.
func Install(hostPath, dataDir string, extensionIDs ...string) ([]string, error) {
	all := targets()
	var chosen []target
	for _, t := range all {
		if st, err := os.Stat(t.root); err == nil && st.IsDir() {
			chosen = append(chosen, t)
		}
	}
	if len(chosen) == 0 && len(all) > 0 {
		chosen = all[:1]
	}
	var names []string
	for _, t := range chosen {
		data, err := t.manifest(hostPath, extensionIDs)
		if err != nil {
			return names, err
		}
		if err := os.MkdirAll(t.hostDir(), 0o755); err != nil {
			return names, err
		}
		if err := os.WriteFile(t.manifestPath(), data, 0o644); err != nil {
			return names, err
		}
		names = append(names, t.name)
	}
	return names, nil
}

// Uninstall removes every registration made by Install.
func Uninstall(dataDir string) error {
	for _, t := range targets() {
		if err := os.Remove(t.manifestPath()); err != nil && !os.IsNotExist(err) {
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
		if data, err := os.ReadFile(t.manifestPath()); err == nil {
			st.Installed = true
			st.Current = readManifestPath(data) == hostPath
		}
		out = append(out, st)
	}
	return out
}
