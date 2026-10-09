package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go-idm/internal/appdir"
	"go-idm/internal/nativehost"
)

// Integration describes the state of the browser extension setup.
type Integration struct {
	HostPath       string                     `json:"hostPath"`
	HostFound      bool                       `json:"hostFound"`
	ExtensionID    string                     `json:"extensionId"`
	ExtensionDir   string                     `json:"extensionDir"`
	ExtensionFound bool                       `json:"extensionFound"`
	Browsers       []nativehost.BrowserStatus `json:"browsers"`
}

func hostBinaryName() string {
	if runtime.GOOS == "windows" {
		return "idm-host.exe"
	}
	return "idm-host"
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// exeDirs returns the directory of the running binary and, in development,
// its ancestors, so build/bin and the source tree can be found.
func ancestors(start string, n int) []string {
	var out []string
	dir := start
	for range n {
		out = append(out, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return out
}

// findHost locates the native host binary: next to the app when installed,
// or under build/bin in a development checkout.
func findHost() (string, bool) {
	if p := os.Getenv("GOIDM_HOST_PATH"); p != "" {
		return p, isFile(p)
	}
	self, err := os.Executable()
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(self)
	if p := filepath.Join(dir, hostBinaryName()); isFile(p) {
		return p, true
	}
	for _, d := range ancestors(dir, 6) {
		if p := filepath.Join(d, "build", "bin", hostBinaryName()); isFile(p) {
			return p, true
		}
	}
	return filepath.Join(dir, hostBinaryName()), false
}

// findExtension locates the bundled extension folder.
func findExtension() (string, bool) {
	self, err := os.Executable()
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(self)
	candidates := []string{
		filepath.Join(dir, "extension"),
		filepath.Join(dir, "..", "Resources", "extension"), // macOS app bundle
	}
	for _, d := range ancestors(dir, 6) {
		candidates = append(candidates, filepath.Join(d, "extension"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "extension"))
	}
	for _, c := range candidates {
		if isFile(filepath.Join(c, "manifest.json")) {
			return filepath.Clean(c), true
		}
	}
	return filepath.Join(dir, "extension"), false
}

func integrationStatus() Integration {
	host, hostOK := findHost()
	ext, extOK := findExtension()
	return Integration{
		HostPath:       host,
		HostFound:      hostOK,
		ExtensionID:    nativehost.ExtensionID,
		ExtensionDir:   ext,
		ExtensionFound: extOK,
		Browsers:       nativehost.Status(host),
	}
}

// GetIntegration reports whether the browser extension setup is complete.
func (a *App) GetIntegration() Integration { return integrationStatus() }

// InstallIntegration registers the native host with the installed browsers.
func (a *App) InstallIntegration() (Integration, error) {
	host, ok := findHost()
	if !ok {
		return integrationStatus(), fmt.Errorf("native host %s was not found next to GoIDM", hostBinaryName())
	}
	dir, err := appdir.Dir()
	if err != nil {
		return integrationStatus(), err
	}
	if _, err := nativehost.Install(host, dir); err != nil {
		return integrationStatus(), fmt.Errorf("register native host: %w", err)
	}
	return integrationStatus(), nil
}

// RemoveIntegration unregisters the native host from all browsers.
func (a *App) RemoveIntegration() (Integration, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return integrationStatus(), err
	}
	if err := nativehost.Uninstall(dir); err != nil {
		return integrationStatus(), err
	}
	return integrationStatus(), nil
}

// RevealExtensionFolder shows the extension folder so it can be loaded into
// the browser with "Load unpacked".
func (a *App) RevealExtensionFolder() error {
	dir, ok := findExtension()
	if !ok {
		return fmt.Errorf("extension folder not found")
	}
	return revealPath(filepath.Join(dir, "manifest.json"))
}
