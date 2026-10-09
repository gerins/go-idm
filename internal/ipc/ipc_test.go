package ipc

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

type fakeHandler struct {
	mu    sync.Mutex
	added []AddRequest
	shown int
	err   error
}

func (f *fakeHandler) Add(r AddRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.added = append(f.added, r)
	return nil
}

func (f *fakeHandler) Show() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown++
	return nil
}

func startServer(t *testing.T, h Handler) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	s := NewServer(dir, "1.2.3", h)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, dir
}

func TestRoundTrip(t *testing.T) {
	h := &fakeHandler{}
	_, dir := startServer(t, h)
	c := NewClient(dir)
	ctx := context.Background()

	p, err := c.Ping(ctx)
	if err != nil || !p.OK || p.App != "goidm" || p.Version != "1.2.3" {
		t.Fatalf("ping = %+v, %v", p, err)
	}
	want := AddRequest{URL: "https://x/y.zip", Cookies: "a=b", Referrer: "https://x/", PageURL: "https://x/page", UserAgent: "UA", Size: 5}
	if err := c.Add(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := c.Show(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.added) != 1 || h.added[0] != want || h.shown != 1 {
		t.Fatalf("handler saw %+v, shown=%d", h.added, h.shown)
	}
}

func TestHandlerErrorIsReported(t *testing.T) {
	h := &fakeHandler{err: errors.New("unsupported URL scheme")}
	_, dir := startServer(t, h)
	err := NewClient(dir).Add(context.Background(), AddRequest{URL: "ftp://x"})
	if err == nil || err.Error() != "unsupported URL scheme" {
		t.Fatalf("err = %v", err)
	}
}

func TestNotRunning(t *testing.T) {
	c := NewClient(t.TempDir())
	if _, err := c.Ping(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("missing info file: err = %v", err)
	}

	// Stale file pointing at a closed port.
	s, dir := startServer(t, &fakeHandler{})
	s.Close()
	if err := writeInfo(dir, Info{Port: 1, Token: "t", PID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(dir).Ping(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("stale info: err = %v", err)
	}
}

func TestCloseRemovesInfoFile(t *testing.T) {
	s, dir := startServer(t, &fakeHandler{})
	if _, err := os.Stat(filepath.Join(dir, infoFile)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(filepath.Join(dir, infoFile)); !os.IsNotExist(err) {
		t.Fatalf("info file should be removed: %v", err)
	}
}

func TestGuard(t *testing.T) {
	_, dir := startServer(t, &fakeHandler{})
	info, err := readInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "http://127.0.0.1:" + strconv.Itoa(info.Port)

	do := func(name, method, path string, mutate func(*http.Request)) int {
		t.Helper()
		var body *bytes.Reader
		if method == http.MethodPost {
			body = bytes.NewReader([]byte(`{"url":"https://x/a.zip"}`))
		} else {
			body = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, base+path, body)
		req.Header.Set("Authorization", "Bearer "+info.Token)
		if mutate != nil {
			mutate(req)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if got := do("valid", http.MethodPost, "/v1/add", nil); got != http.StatusOK {
		t.Errorf("valid request = %d", got)
	}
	if got := do("no token", http.MethodGet, "/v1/ping", func(r *http.Request) { r.Header.Del("Authorization") }); got != http.StatusUnauthorized {
		t.Errorf("missing token = %d", got)
	}
	if got := do("wrong token", http.MethodGet, "/v1/ping", func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }); got != http.StatusUnauthorized {
		t.Errorf("wrong token = %d", got)
	}
	if got := do("browser origin", http.MethodGet, "/v1/ping", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); got != http.StatusForbidden {
		t.Errorf("Origin header = %d", got)
	}
	if got := do("rebinding", http.MethodGet, "/v1/ping", func(r *http.Request) { r.Host = "evil.example:" + strconv.Itoa(info.Port) }); got != http.StatusForbidden {
		t.Errorf("foreign Host = %d", got)
	}
}
