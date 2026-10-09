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
	AllowedOrigins []string `json:"allowed_origins"`
}

// Manifest renders the host manifest allowing the given extension IDs.
func Manifest(hostPath string, extensionIDs ...string) ([]byte, error) {
	if !filepath.IsAbs(hostPath) {
		return nil, fmt.Errorf("host path must be absolute: %q", hostPath)
	}
	if len(extensionIDs) == 0 {
		extensionIDs = []string{ExtensionID}
	}
	origins := make([]string, len(extensionIDs))
	for i, id := range extensionIDs {
		origins[i] = "chrome-extension://" + id + "/"
	}
	return json.MarshalIndent(manifest{
		Name:           HostName,
		Description:    "GoIDM native messaging host",
		Path:           hostPath,
		Type:           "stdio",
		AllowedOrigins: origins,
	}, "", "  ")
}

// readManifestPath returns the host path recorded in a manifest, or "".
func readManifestPath(data []byte) string {
	var m manifest
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	return m.Path
}
