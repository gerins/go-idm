package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// launchApp starts the GoIDM app that ships next to this binary.
func launchApp() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(self)

	// macOS: this binary lives in GoIDM.app/Contents/MacOS; start the bundle
	// through LaunchServices so it behaves like a normal app launch.
	if runtime.GOOS == "darwin" && strings.HasSuffix(dir, filepath.Join(".app", "Contents", "MacOS")) {
		bundle := filepath.Dir(filepath.Dir(dir))
		return exec.Command("open", bundle).Start()
	}

	name := "goidm"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	app := filepath.Join(dir, name)
	if _, err := os.Stat(app); err != nil {
		return fmt.Errorf("app not found next to host: %w", err)
	}
	cmd := exec.Command(app)
	detach(cmd)
	return cmd.Start()
}
