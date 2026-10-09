package nativehost

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// BrowserStatus reports registration for one browser.
type BrowserStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"` // some registration exists
	Current   bool   `json:"current"`   // and it points at the current host binary
}

// manifest is the native messaging host manifest browsers read.
type manifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`    // Chromium
	AllowedExts    []string `json:"allowed_extensions,omitempty"` // Firefox
}

// Manifest renders the host manifest for Chromium-based browsers, allowing
// the given extension IDs.
func Manifest(hostPath string, extensionIDs ...string) ([]byte, error) {
	if len(extensionIDs) == 0 {
		extensionIDs = []string{ExtensionID}
	}
	origins := make([]string, len(extensionIDs))
	for i, id := range extensionIDs {
		origins[i] = "chrome-extension://" + id + "/"
	}
	return render(hostPath, manifest{AllowedOrigins: origins})
}

// FirefoxManifest renders the host manifest for Firefox, which names the
// allowed add-ons by their gecko ID instead of by origin.
func FirefoxManifest(hostPath string, addonIDs ...string) ([]byte, error) {
	if len(addonIDs) == 0 {
		addonIDs = []string{FirefoxExtensionID}
	}
	return render(hostPath, manifest{AllowedExts: addonIDs})
}

func render(hostPath string, m manifest) ([]byte, error) {
	if !filepath.IsAbs(hostPath) {
		return nil, fmt.Errorf("host path must be absolute: %q", hostPath)
	}
	m.Name = HostName
	m.Description = "GoIDM native messaging host"
	m.Path = hostPath
	m.Type = "stdio"
	return json.MarshalIndent(m, "", "  ")
}

// readManifestPath returns the host path recorded in a manifest, or "".
func readManifestPath(data []byte) string {
	var m manifest
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return m.Path
}
