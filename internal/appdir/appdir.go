// Package appdir locates the per-user data directory shared by the app and
// the native messaging host.
package appdir

import (
	"os"
	"path/filepath"
)

// Dir returns the data directory. GOIDM_DATA_DIR overrides it, which is useful
// for portable installs and testing; both the app and the host must see the
// same value.
func Dir() (string, error) {
	if d := os.Getenv("GOIDM_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "GoIDM"), nil
}
