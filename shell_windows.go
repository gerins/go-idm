//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func openPath(p string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}

// revealPath selects the file in Explorer. The command line is built by hand
// because Explorer needs /select,"path" with the quotes around the path only.
func revealPath(p string) error {
	if _, err := os.Stat(p); err != nil {
		p = filepath.Dir(p)
		cmd := exec.Command("explorer.exe", p)
		return cmd.Start()
	}
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
	return cmd.Start()
}
