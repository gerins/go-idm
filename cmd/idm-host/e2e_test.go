package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go-idm/internal/engine"
	"go-idm/internal/ipc"
	"go-idm/internal/store"
)

// appHandler is what the real app does for a captured download, minus the UI.
type appHandler struct{ mgr *engine.Manager }

func (h appHandler) Add(r ipc.AddRequest) error {
	if err := engine.CheckURL(r.URL); err != nil {
		return err
	}
	_, err := h.mgr.Add(engine.AddRequest{URL: r.URL, FileName: r.FileName, Headers: r.Headers()})
	return err
}
func (h appHandler) Show() error { return nil }

func buildHost(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "idm-host")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build host: %v\n%s", err, out)
	}
	return bin
}

// callHost runs the host binary like a browser would: one framed request on
// stdin, one framed response on stdout.
func callHost(t *testing.T, bin, dataDir string, req any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(req)
	var in bytes.Buffer
	_ = binary.Write(&in, binary.NativeEndian, uint32(len(body)))
	in.Write(body)

	cmd := exec.Command(bin, "chrome-extension://bopmpbnmogfmiheanjbjeiiddkobbmng/")
	cmd.Env = append(os.Environ(), "GOIDM_DATA_DIR="+dataDir)
	cmd.Stdin = &in
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("host: %v\nstderr: %s", err, stderr.String())
	}
	var n uint32
	if err := binary.Read(&out, binary.NativeEndian, &n); err != nil {
		t.Fatalf("no response frame: %v (stderr %s)", err, stderr.String())
	}
	raw := make([]byte, n)
	if _, err := io.ReadFull(&out, raw); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected extra output on stdout: %q", out.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("bad response %q: %v", raw, err)
	}
	return resp
}

func TestBrowserToDownloadEndToEnd(t *testing.T) {
	data := make([]byte, 3<<20+17)
	r := rand.NewChaCha8([32]byte{7})
	_, _ = r.Read(data)

	// A host that, like many file sites, refuses requests lacking the browser's session.
	var mu sync.Mutex
	var seenUA, seenReferer []string
	file := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Cookie") != "sid=secret" || !strings.HasPrefix(req.Header.Get("Referer"), "https://files.example/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"session required"}`))
			return
		}
		mu.Lock()
		seenUA = append(seenUA, req.Header.Get("User-Agent"))
		seenReferer = append(seenReferer, req.Header.Get("Referer"))
		mu.Unlock()
		w.Header().Set("Content-Disposition", `attachment; filename="captured.bin"`)
		http.ServeContent(w, req, "captured.bin", time.Time{}, bytes.NewReader(data))
	}))
	defer file.Close()

	dataDir, dlDir := t.TempDir(), t.TempDir()
	st, err := store.Open(filepath.Join(dataDir, "goidm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := engine.DefaultConfig()
	cfg.DownloadDir, cfg.Categorize = dlDir, false
	mgr, err := engine.NewManager(st, cfg, engine.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	mgr.Start()
	defer mgr.Close()

	srv := ipc.NewServer(dataDir, "test", appHandler{mgr})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	bin := buildHost(t)

	if resp := callHost(t, bin, dataDir, map[string]any{"type": "ping"}); resp["ok"] != true || resp["app"] != "goidm" {
		t.Fatalf("ping = %v", resp)
	}

	resp := callHost(t, bin, dataDir, map[string]any{
		"type":      "add",
		"url":       file.URL + "/download/abc",
		"referrer":  "https://files.example/page",
		"cookies":   "sid=secret",
		"userAgent": "Mozilla/5.0 BrowserUA",
		"size":      len(data),
	})
	if resp["ok"] != true {
		t.Fatalf("add = %v", resp)
	}

	var final engine.Info
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		list := mgr.List()
		if len(list) == 1 && (list[0].Status == engine.StatusCompleted || list[0].Status == engine.StatusFailed) {
			final = list[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final.Status != engine.StatusCompleted {
		t.Fatalf("download did not complete: %+v", final)
	}
	got, err := os.ReadFile(filepath.Join(dlDir, "captured.bin"))
	if err != nil || sha256.Sum256(got) != sha256.Sum256(data) {
		t.Fatalf("content mismatch (err=%v, %d bytes)", err, len(got))
	}
	mu.Lock()
	defer mu.Unlock()
	for _, ua := range seenUA {
		if ua != "Mozilla/5.0 BrowserUA" {
			t.Errorf("server saw User-Agent %q, want the browser's", ua)
		}
	}
	if len(seenReferer) == 0 {
		t.Error("server never saw an authorized request")
	}

	// Without the browser's context the same URL is refused, so the headers really mattered.
	if _, err := mgr.Probe(t.Context(), file.URL+"/download/abc", nil); err == nil || !strings.Contains(err.Error(), "session required") {
		t.Errorf("unauthenticated probe error = %v", err)
	}
}

func TestHostReportsAppNotRunning(t *testing.T) {
	bin := buildHost(t)
	resp := callHost(t, bin, t.TempDir(), map[string]any{"type": "ping"})
	if resp["ok"] == true || resp["error"] != "app_not_running" {
		t.Fatalf("resp = %v", resp)
	}
}
