//go:build !windows

package nativehost

import (
	"os"
	"path/filepath"
	"runtime"
)

type target struct {
	name string
	root string // the browser's profile root; its existence means the browser is installed
}

func (t target) hostDir() string      { return filepath.Join(t.root, "NativeMessagingHosts") }
func (t target) manifestPath() string { return filepath.Join(t.hostDir(), HostName+".json") }

func targets() []target {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if runtime.GOOS == "darwin" {
		base := filepath.Join(home, "Library", "Application Support")
		return []target{
			{"Google Chrome", filepath.Join(base, "Google", "Chrome")},
			{"Chromium", filepath.Join(base, "Chromium")},
			{"Microsoft Edge", filepath.Join(base, "Microsoft Edge")},
			{"Brave", filepath.Join(base, "BraveSoftware", "Brave-Browser")},
		}
	}
	base := filepath.Join(home, ".config")
	return []target{
		{"Google Chrome", filepath.Join(base, "google-chrome")},
		{"Chromium", filepath.Join(base, "chromium")},
		{"Microsoft Edge", filepath.Join(base, "microsoft-edge")},
		{"Brave", filepath.Join(base, "BraveSoftware", "Brave-Browser")},
	}
}

// Install registers the host with every installed Chromium-based browser (or
// with Chrome if none is found yet) and returns their names. dataDir is
// unused on this platform.
func Install(hostPath, dataDir string, extensionIDs ...string) ([]string, error) {
	data, err := Manifest(hostPath, extensionIDs...)
	if err != nil {
		return nil, err
	}
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
