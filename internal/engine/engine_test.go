package engine

import (
	"bytes"
	"crypto/sha256"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memStore struct {
	mu sync.Mutex
	m  map[string]Download
}

func newMemStore() *memStore { return &memStore{m: map[string]Download{}} }

func (s *memStore) Save(d *Download) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[d.ID] = cloneDownload(d)
	return nil
}

func (s *memStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}

func (s *memStore) List() ([]*Download, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Download
	for _, d := range s.m {
		c := cloneDownload(&d)
		out = append(out, &c)
	}
	return out, nil
}

// recorder collects hook calls.
type recorder struct {
	mu      sync.Mutex
	updates []Info
	removed []string
}

func (r *recorder) hooks() Hooks {
	return Hooks{
		OnUpdate: func(in []Info) {
			r.mu.Lock()
			r.updates = append(r.updates, in...)
			r.mu.Unlock()
		},
		OnRemove: func(id string) {
			r.mu.Lock()
			r.removed = append(r.removed, id)
			r.mu.Unlock()
		},
	}
}

// assertTerminalStable fails if an update for id arrives after Completed
// that is not Completed (i.e. hook ordering broke).
func (r *recorder) assertTerminalStable(t *testing.T, id string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := false
	for _, u := range r.updates {
		if u.ID != id {
			continue
		}
		if seen && u.Status != StatusCompleted {
			t.Fatalf("stale update after completion: %s", u.Status)
		}
		if u.Status == StatusCompleted {
			seen = true
		}
	}
}

func init() {
	minSegmentSize = 64 << 10
	retryBase = 5 * time.Millisecond
}

func randomData(n int) []byte {
	r := rand.NewChaCha8([32]byte{1, 2, 3})
	b := make([]byte, n)
	_, _ = r.Read(b)
	return b
}

// slowReader throttles reads to simulate a slow origin and counts bytes sent.
type slowReader struct {
	*bytes.Reader
	delay time.Duration
	sent  *atomic.Int64
}

func (s slowReader) Read(p []byte) (int, error) {
	if len(p) > 32<<10 {
		p = p[:32<<10]
	}
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	n, err := s.Reader.Read(p)
	s.sent.Add(int64(n))
	return n, err
}

type testServer struct {
	*httptest.Server
	data     []byte
	delay    time.Duration
	sent     atomic.Int64
	requests atomic.Int64
	ranged   atomic.Int64
	failNext atomic.Int64 // number of non-probe requests to fail with 503
}

func newTestServer(t *testing.T, data []byte, delay time.Duration) *testServer {
	t.Helper()
	ts := &testServer{data: data, delay: delay}
	mux := http.NewServeMux()
	mux.HandleFunc("/file.bin", func(w http.ResponseWriter, r *http.Request) {
		ts.requests.Add(1)
		if rg := r.Header.Get("Range"); rg != "" {
			ts.ranged.Add(1)
			if rg != "bytes=0-0" && ts.failNext.Add(-1) >= 0 {
				http.Error(w, "boom", http.StatusServiceUnavailable)
				return
			}
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Disposition", `attachment; filename="report final.bin"`)
		http.ServeContent(w, r, "file.bin", time.Unix(1700000000, 0),
			slowReader{bytes.NewReader(data), delay, &ts.sent})
	})
	mux.HandleFunc("/norange", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/chunked", func(w http.ResponseWriter, r *http.Request) {
		for off := 0; off < len(data); off += 10000 {
			_, _ = w.Write(data[off:min(off+10000, len(data))])
			w.(http.Flusher).Flush()
		}
	})
	mux.HandleFunc("/missing", http.NotFound)
	ts.Server = httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func newTestManager(t *testing.T, mutate func(*Config)) (*Manager, *recorder, string) {
	t.Helper()
	return newTestManagerWith(t, newMemStore(), t.TempDir(), mutate)
}

func newTestManagerWith(t *testing.T, st Store, dir string, mutate func(*Config)) (*Manager, *recorder, string) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.DownloadDir = dir
	cfg.Categorize = false
	cfg.Connections = 4
	if mutate != nil {
		mutate(&cfg)
	}
	rec := &recorder{}
	m, err := NewManager(st, cfg, rec.hooks())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	m.Start()
	t.Cleanup(m.Close)
	return m, rec, dir
}

func waitFor(t *testing.T, m *Manager, id string, cond func(Info) bool, what string) Info {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if info, ok := m.Get(id); ok && cond(info) {
			return info
		}
		time.Sleep(10 * time.Millisecond)
	}
	info, _ := m.Get(id)
	t.Fatalf("timed out waiting for %s; last state: %+v", what, info)
	return Info{}
}

func status(s Status) func(Info) bool { return func(i Info) bool { return i.Status == s } }

func mustAdd(t *testing.T, m *Manager, req AddRequest) Info {
	t.Helper()
	info, err := m.Add(req)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func assertFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(got) != sha256.Sum256(want) {
		t.Fatalf("content mismatch for %s: got %d bytes, want %d", path, len(got), len(want))
	}
}

func TestSegmentedDownload(t *testing.T) {
	data := randomData(3<<20 + 123)
	srv := newTestServer(t, data, 0)
	m, rec, dir := newTestManager(t, nil)

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")

	if done.FileName != "report final.bin" {
		t.Errorf("FileName = %q", done.FileName)
	}
	if !done.Resumable || done.Size != int64(len(data)) || len(done.Segments) != 4 {
		t.Errorf("unexpected info: resumable=%v size=%d segments=%d", done.Resumable, done.Size, len(done.Segments))
	}
	assertFile(t, filepath.Join(dir, "report final.bin"), data)
	if _, err := os.Stat(filepath.Join(dir, "report final.bin"+partSuffix)); !os.IsNotExist(err) {
		t.Errorf(".part file should be gone: %v", err)
	}
	st, _ := os.Stat(filepath.Join(dir, "report final.bin"))
	if st.ModTime().Unix() != 1700000000 {
		t.Errorf("mtime not restored: %v", st.ModTime())
	}
	if srv.ranged.Load() < 5 { // probe + 4 segments
		t.Errorf("expected ranged requests, got %d", srv.ranged.Load())
	}
	rec.assertTerminalStable(t, info.ID)
}

func TestNoRangeSupport(t *testing.T) {
	data := randomData(300 << 10)
	srv := newTestServer(t, data, 0)
	m, _, dir := newTestManager(t, nil)

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/norange"})
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	if done.Resumable || len(done.Segments) != 1 {
		t.Errorf("expected single non-resumable segment, got %+v", done)
	}
	assertFile(t, filepath.Join(dir, "norange"), data)
}

func TestUnknownSize(t *testing.T) {
	data := randomData(95 << 10)
	srv := newTestServer(t, data, 0)
	m, _, dir := newTestManager(t, nil)

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/chunked"})
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	if done.Size != int64(len(data)) {
		t.Errorf("size should be settled after completion, got %d", done.Size)
	}
	assertFile(t, filepath.Join(dir, "chunked"), data)
}

func TestPauseResume(t *testing.T) {
	data := randomData(4 << 20)
	srv := newTestServer(t, data, 15*time.Millisecond)
	m, rec, dir := newTestManager(t, func(c *Config) { c.Connections = 2 })

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	waitFor(t, m, info.ID, func(i Info) bool { return i.Downloaded > 256<<10 }, "some progress")

	if err := m.Pause(info.ID); err != nil {
		t.Fatal(err)
	}
	paused := waitFor(t, m, info.ID, func(i Info) bool { return i.Status == StatusPaused }, "pause")
	time.Sleep(100 * time.Millisecond)
	paused, _ = m.Get(info.ID)
	if paused.Downloaded <= 0 || paused.Downloaded >= int64(len(data)) {
		t.Fatalf("expected partial progress, got %d", paused.Downloaded)
	}
	if _, err := os.Stat(paused.Path); err != nil {
		t.Fatalf("part file missing while paused: %v", err)
	}

	if err := m.Resume(info.ID); err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	assertFile(t, done.Path, data)
	if !strings.HasPrefix(done.Path, dir) {
		t.Errorf("unexpected path %s", done.Path)
	}
	// Resuming must not re-download what we already had (allow slack for
	// bytes in flight when the connection was cut).
	if sent := srv.sent.Load(); sent > int64(len(data))+512<<10 {
		t.Errorf("server sent %d bytes for a %d byte file: resume re-downloaded data", sent, len(data))
	}
	rec.assertTerminalStable(t, info.ID)
}

func TestResumeAcrossRestart(t *testing.T) {
	data := randomData(3 << 20)
	srv := newTestServer(t, data, 15*time.Millisecond)
	st, dir := newMemStore(), t.TempDir()

	m1, _, _ := newTestManagerWith(t, st, dir, func(c *Config) { c.Connections = 2 })
	info := mustAdd(t, m1, AddRequest{URL: srv.URL + "/file.bin"})
	waitFor(t, m1, info.ID, func(i Info) bool { return i.Downloaded > 256<<10 }, "some progress")
	m1.Close()

	m2, _, _ := newTestManagerWith(t, st, dir, func(c *Config) { c.Connections = 2 })
	reloaded, ok := m2.Get(info.ID)
	if !ok || reloaded.Status != StatusPaused || reloaded.Downloaded == 0 {
		t.Fatalf("expected paused download with progress after restart, got %+v (ok=%v)", reloaded, ok)
	}
	if err := m2.Resume(info.ID); err != nil {
		t.Fatal(err)
	}
	done := waitFor(t, m2, info.ID, status(StatusCompleted), "completion")
	assertFile(t, done.Path, data)
}

func TestRetriesTransientErrors(t *testing.T) {
	data := randomData(1 << 20)
	srv := newTestServer(t, data, 0)
	srv.failNext.Store(3)
	m, _, dir := newTestManager(t, nil)

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	waitFor(t, m, info.ID, status(StatusCompleted), "completion despite 503s")
	assertFile(t, filepath.Join(dir, "report final.bin"), data)
}

func TestGivesUpOnNotFound(t *testing.T) {
	srv := newTestServer(t, nil, 0)
	m, _, _ := newTestManager(t, nil)

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/missing"})
	failed := waitFor(t, m, info.ID, status(StatusFailed), "failure")
	if !strings.Contains(failed.Error, "404") {
		t.Errorf("error = %q", failed.Error)
	}
}

func TestSpeedLimit(t *testing.T) {
	data := randomData(400 << 10)
	srv := newTestServer(t, data, 0)
	m, _, _ := newTestManager(t, nil)

	start := time.Now()
	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", SpeedLimit: 200 << 10})
	waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	// 400 KiB at 200 KiB/s with a 64 KiB burst is about 1.7s.
	if el := time.Since(start); el < 1200*time.Millisecond {
		t.Errorf("finished in %v; limiter not applied", el)
	}
}

func TestGlobalSpeedLimit(t *testing.T) {
	data := randomData(400 << 10)
	srv := newTestServer(t, data, 0)
	m, _, _ := newTestManager(t, func(c *Config) { c.SpeedLimit = 200 << 10 })

	start := time.Now()
	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	if el := time.Since(start); el < 1200*time.Millisecond {
		t.Errorf("finished in %v; global limiter not applied", el)
	}
}

func TestQueueRespectsMaxActive(t *testing.T) {
	data := randomData(1 << 20)
	srv := newTestServer(t, data, 10*time.Millisecond)
	m, _, _ := newTestManager(t, func(c *Config) { c.MaxActive = 1; c.Connections = 1 })

	a := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", FileName: "a.bin"})
	b := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", FileName: "b.bin"})

	waitFor(t, m, a.ID, func(i Info) bool { return i.Downloaded > 0 }, "a progress")
	if bi, _ := m.Get(b.ID); bi.Status != StatusQueued {
		t.Fatalf("second download should be queued, got %s", bi.Status)
	}
	waitFor(t, m, a.ID, status(StatusCompleted), "a completion")
	waitFor(t, m, b.ID, status(StatusCompleted), "b completion")
}

func TestUniqueFileNames(t *testing.T) {
	data := randomData(200 << 10)
	srv := newTestServer(t, data, 0)
	m, _, dir := newTestManager(t, nil)

	a := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	b := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	waitFor(t, m, a.ID, status(StatusCompleted), "a")
	waitFor(t, m, b.ID, status(StatusCompleted), "b")

	assertFile(t, filepath.Join(dir, "report final.bin"), data)
	assertFile(t, filepath.Join(dir, "report final (1).bin"), data)
}

func TestCategoryFolders(t *testing.T) {
	data := randomData(50 << 10)
	srv := newTestServer(t, data, 0)
	m, _, dir := newTestManager(t, func(c *Config) { c.Categorize = true })

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin", FileName: "clip.mp4"})
	waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	assertFile(t, filepath.Join(dir, "Video", "clip.mp4"), data)
}

func TestRemoveDeletesFiles(t *testing.T) {
	data := randomData(2 << 20)
	srv := newTestServer(t, data, 15*time.Millisecond)
	m, rec, _ := newTestManager(t, func(c *Config) { c.Connections = 2 })

	info := mustAdd(t, m, AddRequest{URL: srv.URL + "/file.bin"})
	running := waitFor(t, m, info.ID, func(i Info) bool { return i.Downloaded > 0 }, "progress")
	if err := m.Remove(info.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(running.Path); !os.IsNotExist(err) {
		t.Errorf("part file should be deleted: %v", err)
	}
	if _, ok := m.Get(info.ID); ok {
		t.Error("download still listed")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.removed) != 1 || rec.removed[0] != info.ID {
		t.Errorf("OnRemove calls = %v", rec.removed)
	}
}

func TestAddValidation(t *testing.T) {
	m, _, _ := newTestManager(t, nil)
	for _, bad := range []string{"", "ftp://x/y", "http://", "not a url", "file:///etc/passwd"} {
		if _, err := m.Add(AddRequest{URL: bad}); err == nil {
			t.Errorf("Add(%q) should fail", bad)
		}
	}
	if _, err := m.Add(AddRequest{URL: "http://x/y", Dir: "relative/dir"}); err == nil {
		t.Error("relative dir should be rejected")
	}
}

func TestProbe(t *testing.T) {
	data := randomData(10 << 10)
	srv := newTestServer(t, data, 0)
	m, _, _ := newTestManager(t, nil)

	res, err := m.Probe(t.Context(), srv.URL+"/file.bin", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Size != int64(len(data)) || !res.Resumable || res.FileName != "report final.bin" || res.ETag != `"v1"` {
		t.Errorf("unexpected probe: %+v", res)
	}
	res, err = m.Probe(t.Context(), srv.URL+"/norange", nil)
	if err != nil || res.Resumable || res.Size != int64(len(data)) {
		t.Errorf("norange probe = %+v, %v", res, err)
	}
	if _, err := m.Probe(t.Context(), srv.URL+"/missing", nil); err == nil {
		t.Error("expected 404 error")
	}
}

func TestHTTPErrorShowsServerReason(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"value":"file_rate_limited_captcha_required","message":"Captcha required to download this file"}`))
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><body>Forbidden</body></html>`))
	})
	mux.HandleFunc("/challenge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><title>Just a moment...</title></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	m, _, _ := newTestManager(t, nil)

	cases := map[string]string{
		"/json":      "server returned 403 Forbidden: Captcha required to download this file",
		"/html":      "server returned 403 Forbidden",
		"/challenge": "blocked by a browser check",
	}
	for path, want := range cases {
		_, err := m.Probe(t.Context(), srv.URL+path, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want it to contain %q", path, err, want)
		}
	}
}

func TestRequestHeadersReachServer(t *testing.T) {
	var gotReferer, gotCookie atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer.Store(r.Header.Get("Referer"))
		gotCookie.Store(r.Header.Get("Cookie"))
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	m, _, _ := newTestManager(t, nil)

	if _, err := m.Probe(t.Context(), srv.URL+"/f.bin", nil); err != nil {
		t.Fatal(err)
	}
	if got := gotReferer.Load(); got != srv.URL+"/" {
		t.Errorf("default Referer = %v, want %s/", got, srv.URL)
	}

	h := map[string]string{"referer": "https://example.org/page", "Cookie": "sid=abc"}
	if _, err := m.Probe(t.Context(), srv.URL+"/f.bin", h); err != nil {
		t.Fatal(err)
	}
	if gotReferer.Load() != "https://example.org/page" || gotCookie.Load() != "sid=abc" {
		t.Errorf("custom headers not sent: referer=%v cookie=%v", gotReferer.Load(), gotCookie.Load())
	}
}

func TestPlanSegments(t *testing.T) {
	cases := []struct {
		size      int64
		conns     int
		resumable bool
		want      int
	}{
		{-1, 8, true, 1},
		{0, 8, true, 0},
		{1000, 8, false, 1},
		{1000, 8, true, 1},         // below min segment size
		{64 << 10 * 3, 8, true, 3}, // capped by size / minSegmentSize
		{64 << 10 * 100, 8, true, 8},
	}
	for _, c := range cases {
		segs := planSegments(c.size, c.conns, c.resumable)
		if len(segs) != c.want {
			t.Errorf("planSegments(%d,%d,%v) = %d segments, want %d", c.size, c.conns, c.resumable, len(segs), c.want)
			continue
		}
		if c.size > 0 && len(segs) > 0 {
			var covered, prevEnd int64 = 0, -1
			for _, s := range segs {
				if s.Start != prevEnd+1 {
					t.Errorf("gap or overlap at %+v", s)
				}
				prevEnd = s.End
				covered += s.End - s.Start + 1
			}
			if covered != c.size {
				t.Errorf("segments cover %d of %d bytes", covered, c.size)
			}
		}
	}
}

func TestParseContentRange(t *testing.T) {
	s, e, total, ok := parseContentRange("bytes 5-9/100")
	if !ok || s != 5 || e != 9 || total != 100 {
		t.Errorf("got %d %d %d %v", s, e, total, ok)
	}
	if _, _, total, ok := parseContentRange("bytes */0"); !ok || total != 0 {
		t.Errorf("bytes */0: %d %v", total, ok)
	}
	if _, _, total, ok := parseContentRange("bytes 0-0/*"); !ok || total != -1 {
		t.Errorf("unknown total: %d %v", total, ok)
	}
	for _, bad := range []string{"", "items 0-1/2", "bytes 0-1", "bytes a-b/c"} {
		if _, _, _, ok := parseContentRange(bad); ok {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestHeadersForStripsCredentialsAcrossHosts(t *testing.T) {
	h := map[string]string{"Cookie": "s=1", "Authorization": "Bearer x", "Referer": "https://a/"}
	same := headersFor(h, "https://a.example/f", "https://a.example/g")
	if same["Cookie"] == "" || same["Authorization"] == "" {
		t.Error("same-host redirect must keep credentials")
	}
	cross := headersFor(h, "https://a.example/f", "https://cdn.other/g")
	if cross["Cookie"] != "" || cross["Authorization"] != "" || cross["Referer"] == "" {
		t.Errorf("cross-host headers = %v", cross)
	}
}

func TestCategoryFor(t *testing.T) {
	for name, want := range map[string]string{
		"a.MP4": "Video", "b.pdf": "Documents", "c.tar": "Compressed", "d": "General", "e.exe": "Programs",
	} {
		if got := CategoryFor(name); got != want {
			t.Errorf("CategoryFor(%q) = %q, want %q", name, got, want)
		}
	}
}
