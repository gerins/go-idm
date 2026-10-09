// Package nativehost implements the browser side of GoIDM: the native
// messaging protocol spoken by the host process, and registration of that
// host with Chromium-based browsers and Firefox.
package nativehost

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"go-idm/internal/ipc"
)

// HostName is the native messaging host name the extension connects to.
const HostName = "com.goidm.host"

// ExtensionID is the ID of the bundled extension. It is derived from the
// public key in extension/manifest.json (see TestExtensionIDMatchesManifest).
const ExtensionID = "bopmpbnmogfmiheanjbjeiiddkobbmng"

// FirefoxExtensionID is the gecko ID of the Firefox build of the extension. It
// must match browser_specific_settings.gecko.id in extension/manifest.firefox.json
// (see TestFirefoxExtensionIDMatchesManifest).
const FirefoxExtensionID = "goidm@go-idm"

// Error codes returned to the extension in Response.Error.
const (
	ErrAppNotRunning = "app_not_running"
	ErrLaunchFailed  = "app_launch_failed"
	ErrUnknownType   = "unknown_type"
)

// maxFrame bounds messages from the browser; Chrome's own limit is far larger.
const maxFrame = 1 << 20

// Request is a message from the extension.
type Request struct {
	Type string `json:"type"` // "ping", "add" or "show"
	ipc.AddRequest
}

// Response is the reply sent back to the extension.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	App     string `json:"app,omitempty"`
	Version string `json:"version,omitempty"`
}

// App is the subset of ipc.Client the host needs.
type App interface {
	Ping(ctx context.Context) (ipc.PingResponse, error)
	Add(ctx context.Context, req ipc.AddRequest) error
	Show(ctx context.Context) error
}

// Launcher starts the app. It may return before the app is ready.
type Launcher func() error

// Serve reads framed requests from in and writes framed responses to out
// until in is closed. If the app is not running when an add or show request
// arrives, launch (if non-nil) is used to start it.
func Serve(in io.Reader, out io.Writer, app App, launch Launcher) error {
	for {
		raw, err := readFrame(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var req Request
		var resp Response
		if err := json.Unmarshal(raw, &req); err != nil {
			resp = Response{Error: "invalid_request"}
		} else {
			resp = handle(req, app, launch)
		}
		if err := writeFrame(out, resp); err != nil {
			return err
		}
	}
}

func handle(req Request, app App, launch Launcher) Response {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch req.Type {
	case "ping":
		p, err := app.Ping(ctx)
		if err != nil {
			return failure(err)
		}
		return Response{OK: true, App: p.App, Version: p.Version}
	case "add":
		return withLaunch(ctx, app, launch, func() error { return app.Add(ctx, req.AddRequest) })
	case "show":
		return withLaunch(ctx, app, launch, func() error { return app.Show(ctx) })
	default:
		return Response{Error: ErrUnknownType}
	}
}

// withLaunch runs op, starting the app first if it is not running.
func withLaunch(ctx context.Context, app App, launch Launcher, op func() error) Response {
	err := op()
	if errors.Is(err, ipc.ErrNotRunning) && launch != nil {
		if lerr := launch(); lerr != nil {
			return Response{Error: ErrLaunchFailed}
		}
		if !waitForApp(ctx, app, 20*time.Second) {
			return Response{Error: ErrAppNotRunning}
		}
		err = op()
	}
	if err != nil {
		return failure(err)
	}
	return Response{OK: true}
}

func waitForApp(ctx context.Context, app App, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if _, err := app.Ping(ctx); err == nil {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func failure(err error) Response {
	if errors.Is(err, ipc.ErrNotRunning) {
		return Response{Error: ErrAppNotRunning}
	}
	return Response{Error: err.Error()}
}

// Frames are a 4-byte length in native byte order followed by JSON.

func readFrame(r io.Reader) ([]byte, error) {
	var n uint32
	if err := binary.Read(r, binary.NativeEndian, &n); err != nil {
		return nil, err
	}
	if n > maxFrame {
		return nil, fmt.Errorf("message too large: %d bytes", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeFrame(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := binary.Write(w, binary.NativeEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
