package nativehost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go-idm/internal/ipc"
)

type fakeApp struct {
	running bool
	added   []ipc.AddRequest
	shown   int
	addErr  error
}

func (f *fakeApp) Ping(context.Context) (ipc.PingResponse, error) {
	if !f.running {
		return ipc.PingResponse{}, ipc.ErrNotRunning
	}
	return ipc.PingResponse{OK: true, App: "goidm", Version: "9.9"}, nil
}

func (f *fakeApp) Add(_ context.Context, r ipc.AddRequest) error {
	if !f.running {
		return ipc.ErrNotRunning
	}
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, r)
	return nil
}

func (f *fakeApp) Show(context.Context) error {
	if !f.running {
		return ipc.ErrNotRunning
	}
	f.shown++
	return nil
}

func frame(t *testing.T, v any) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := writeFrame(&b, v); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// roundTrip feeds requests through Serve and decodes every response.
func roundTrip(t *testing.T, app App, launch Launcher, reqs ...any) []Response {
	t.Helper()
	var in bytes.Buffer
	for _, r := range reqs {
		in.Write(frame(t, r))
	}
	var out bytes.Buffer
	if err := Serve(&in, &out, app, launch); err != nil {
		t.Fatal(err)
	}
	var resps []Response
	for out.Len() > 0 {
		raw, err := readFrame(&out)
		if err != nil {
			t.Fatal(err)
		}
		var r Response
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		resps = append(resps, r)
	}
	return resps
}

func TestPingAddShow(t *testing.T) {
	app := &fakeApp{running: true}
	got := roundTrip(t, app, nil,
		map[string]any{"type": "ping"},
		map[string]any{"type": "add", "url": "https://x/a.zip", "cookies": "a=b", "referrer": "https://x/", "userAgent": "UA", "size": 42},
		map[string]any{"type": "show"},
		map[string]any{"type": "bogus"},
	)
	if len(got) != 4 {
		t.Fatalf("got %d responses", len(got))
	}
	if !got[0].OK || got[0].App != "goidm" || got[0].Version != "9.9" {
		t.Errorf("ping = %+v", got[0])
	}
	want := ipc.AddRequest{URL: "https://x/a.zip", Cookies: "a=b", Referrer: "https://x/", UserAgent: "UA", Size: 42}
	if !got[1].OK || len(app.added) != 1 || app.added[0] != want {
		t.Errorf("add = %+v, app saw %+v", got[1], app.added)
	}
	if !got[2].OK || app.shown != 1 {
		t.Errorf("show = %+v", got[2])
	}
	if got[3].OK || got[3].Error != ErrUnknownType {
		t.Errorf("unknown type = %+v", got[3])
	}
}

func TestAppNotRunning(t *testing.T) {
	app := &fakeApp{}
	got := roundTrip(t, app, nil, map[string]any{"type": "ping"}, map[string]any{"type": "add", "url": "https://x/a"})
	for i, r := range got {
		if r.OK || r.Error != ErrAppNotRunning {
			t.Errorf("response %d = %+v", i, r)
		}
	}
}

func TestLaunchesAppOnAdd(t *testing.T) {
	app := &fakeApp{}
	launched := 0
	launch := func() error {
		launched++
		app.running = true // the app comes up
		return nil
	}
	got := roundTrip(t, app, launch, map[string]any{"type": "add", "url": "https://x/a.zip"})
	if !got[0].OK || launched != 1 || len(app.added) != 1 {
		t.Fatalf("resp=%+v launched=%d added=%v", got[0], launched, app.added)
	}
}

func TestPingNeverLaunches(t *testing.T) {
	app := &fakeApp{}
	launched := 0
	got := roundTrip(t, app, func() error { launched++; return nil }, map[string]any{"type": "ping"})
	if launched != 0 || got[0].Error != ErrAppNotRunning {
		t.Fatalf("ping must not start the app: launched=%d resp=%+v", launched, got[0])
	}
}

func TestLaunchFailure(t *testing.T) {
	got := roundTrip(t, &fakeApp{}, func() error { return errors.New("nope") }, map[string]any{"type": "show"})
	if got[0].OK || got[0].Error != ErrLaunchFailed {
		t.Fatalf("resp = %+v", got[0])
	}
}

func TestAppErrorIsRelayed(t *testing.T) {
	app := &fakeApp{running: true, addErr: errors.New("unsupported URL scheme")}
	got := roundTrip(t, app, nil, map[string]any{"type": "add", "url": "ftp://x"})
	if got[0].OK || got[0].Error != "unsupported URL scheme" {
		t.Fatalf("resp = %+v", got[0])
	}
}

func TestInvalidJSONKeepsServing(t *testing.T) {
	var in bytes.Buffer
	_ = binary.Write(&in, binary.NativeEndian, uint32(3))
	in.WriteString("{{{")
	in.Write(frame(t, map[string]any{"type": "ping"}))
	var out bytes.Buffer
	if err := Serve(&in, &out, &fakeApp{running: true}, nil); err != nil {
		t.Fatal(err)
	}
	r1, _ := readFrame(&out)
	r2, _ := readFrame(&out)
	if !bytes.Contains(r1, []byte("invalid_request")) || !bytes.Contains(r2, []byte(`"ok":true`)) {
		t.Fatalf("responses: %s | %s", r1, r2)
	}
}

func TestFrameLimits(t *testing.T) {
	var in bytes.Buffer
	_ = binary.Write(&in, binary.NativeEndian, uint32(maxFrame+1))
	if err := Serve(&in, io.Discard, &fakeApp{}, nil); err == nil {
		t.Error("oversized frame should be rejected")
	}
	// Truncated body.
	in.Reset()
	_ = binary.Write(&in, binary.NativeEndian, uint32(10))
	in.WriteString("abc")
	if err := Serve(&in, io.Discard, &fakeApp{}, nil); err == nil {
		t.Error("truncated frame should be an error")
	}
}

func TestManifest(t *testing.T) {
	data, err := Manifest("/opt/goidm/idm-host")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != HostName || m.Type != "stdio" || m.Path != "/opt/goidm/idm-host" ||
		len(m.AllowedOrigins) != 1 || m.AllowedOrigins[0] != "chrome-extension://"+ExtensionID+"/" {
		t.Errorf("manifest = %+v", m)
	}
	if _, err := Manifest("relative/host"); err == nil {
		t.Error("relative host path must be rejected")
	}
}

// The ID is derived from the public key in the extension manifest; fail if
// they ever drift apart.
func TestExtensionIDMatchesManifest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "extension", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Key == "" {
		t.Fatalf("manifest key: %v", err)
	}
	der, err := base64.StdEncoding.DecodeString(m.Key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	id := make([]byte, 32)
	for i := range id {
		nibble := sum[i/2] >> 4
		if i%2 == 1 {
			nibble = sum[i/2] & 0x0f
		}
		id[i] = 'a' + nibble
	}
	if string(id) != ExtensionID {
		t.Fatalf("ExtensionID = %s, but extension/manifest.json key yields %s", ExtensionID, id)
	}
}
