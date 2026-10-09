// Command idm-host is the native messaging host the GoIDM browser extension
// talks to. The browser starts it on demand and speaks length-prefixed JSON
// over stdin/stdout; it relays requests to the running app and can start the
// app if it is not running.
//
// stdout carries only protocol frames, so nothing else may write to it.
package main

import (
	"fmt"
	"os"

	"go-idm/internal/appdir"
	"go-idm/internal/ipc"
	"go-idm/internal/nativehost"
)

func main() {
	dir, err := appdir.Dir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "idm-host:", err)
		os.Exit(1)
	}
	if err := nativehost.Serve(os.Stdin, os.Stdout, ipc.NewClient(dir), launchApp); err != nil {
		fmt.Fprintln(os.Stderr, "idm-host:", err)
		os.Exit(1)
	}
}
