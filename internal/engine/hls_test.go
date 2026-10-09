package engine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func encryptCBC(t *testing.T, key, iv, plain []byte) []byte {
	t.Helper()
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	data := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(data, data)
	return data
}

func noFFmpeg(t *testing.T) {
	t.Helper()
	old := findFFmpeg
	findFFmpeg = func() string { return "" }
	t.Cleanup(func() { findFFmpeg = old })
}

// stubFFmpeg installs a fake ffmpeg that concatenates its -i inputs into the
// last argument, which is enough to see that it was run with the right files.
func stubFFmpeg(t *testing.T, fail bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	script := `#!/bin/sh
out=""; ins=""
while [ $# -gt 0 ]; do
  case "$1" in
    -i) ins="$ins $2"; shift 2;;
    -loglevel|-map|-c|-f) shift 2;;
    -y|-hide_banner) shift;;
    *) out="$1"; shift;;
  esac
done
`
	if fail {
		script += "echo 'moov atom not found' >&2\nexit 1\n"
	} else {
		script += "cat $ins > \"$out\"\n"
	}
	p := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := findFFmpeg
	findFFmpeg = func() string { return p }
	t.Cleanup(func() { findFFmpeg = old })
}

// hlsServer serves playlists and pieces from memory and counts requests.
type hlsServer struct {
	*httptest.Server
	mu     sync.Mutex
	files  map[string][]byte
	ctype  map[string]string
	hits   map[string]int
	broken map[string]bool // paths that answer 404
	delay  time.Duration
}

func newHLSServer(t *testing.T) *hlsServer {
	t.Helper()
	s := &hlsServer{files: map[string][]byte{}, ctype: map[string]string{}, hits: map[string]int{}, broken: map[string]bool{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		data, ok := s.files[r.URL.Path]
		bad := s.broken[r.URL.Path]
		ct := s.ctype[r.URL.Path]
		s.mu.Unlock()
		if !ok || bad {
			http.NotFound(w, r)
			return
		}
		if s.delay > 0 {
			time.Sleep(s.delay)
		}
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *hlsServer) put(path string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = data
	if strings.HasSuffix(path, ".m3u8") {
		s.ctype[path] = "application/vnd.apple.mpegurl"
	}
}

func (s *hlsServer) hitCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

func (s *hlsServer) setBroken(path string, v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broken[path] = v
}

// addMedia stores n random pieces under prefix and a VOD playlist for them. It
// returns the pieces joined, which is what the download should produce.
func (s *hlsServer) addMedia(prefix string, n int, ext string, extra ...string) []byte {
	var all []byte
	var pl strings.Builder
	pl.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n")
	for _, e := range extra {
		pl.WriteString(e + "\n")
	}
	for i := range n {
		seg := make([]byte, 20_000+i*100)
		_, _ = rand.Read(seg)
		path := fmt.Sprintf("%s/seg%d.%s", prefix, i, ext)
		s.put(path, seg)
		all = append(all, seg...)
		fmt.Fprintf(&pl, "#EXTINF:4.0,\n%s\n", path)
	}
	pl.WriteString("#EXT-X-ENDLIST\n")
	s.put(prefix+"/index.m3u8", []byte(pl.String()))
	return all
}

func addHLS(t *testing.T, m *Manager, url string, mutate ...func(*AddRequest)) Info {
	t.Helper()
	req := AddRequest{URL: url}
	for _, f := range mutate {
		f(&req)
	}
	return mustAdd(t, m, req)
}

func TestHLSDownloadJoinsSegments(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	want := srv.addMedia("/movie", 8, "ts")
	m, _, dir := newTestManager(t, func(c *Config) { c.Connections = 4 })

	info := addHLS(t, m, srv.URL+"/movie/index.m3u8")
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")

	if done.FileName != "movie.ts" { // "index" is generic, so the folder names it
		t.Errorf("file name = %q", done.FileName)
	}
	assertFile(t, filepath.Join(dir, "movie.ts"), want)
	if done.Size != int64(len(want)) {
		t.Errorf("size = %d, want %d", done.Size, len(want))
	}
	if _, err := os.Stat(filepath.Join(dir, "movie.ts"+partsSuffix)); !os.IsNotExist(err) {
		t.Error("the parts folder should be removed")
	}
}

func TestHLSMasterPicksBestQualityOrTheChosenOne(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	low := srv.addMedia("/low", 3, "ts")
	high := srv.addMedia("/high", 3, "ts")
	srv.put("/show/master.m3u8", []byte("#EXTM3U\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=500000,RESOLUTION=640x360\n/low/index.m3u8\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1920x1080\n/high/index.m3u8\n"))
	m, _, dir := newTestManager(t, nil)

	best := addHLS(t, m, srv.URL+"/show/master.m3u8")
	waitFor(t, m, best.ID, status(StatusCompleted), "best")
	assertFile(t, filepath.Join(dir, "show.ts"), high)

	chosen := addHLS(t, m, srv.URL+"/show/master.m3u8", func(r *AddRequest) { r.Variant = srv.URL + "/low/index.m3u8" })
	done := waitFor(t, m, chosen.ID, status(StatusCompleted), "chosen")
	assertFile(t, filepath.Join(dir, done.FileName), low)
}

func TestHLSProbeListsQualities(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	srv.addMedia("/low", 3, "ts")
	srv.addMedia("/high", 5, "ts")
	srv.put("/show/master.m3u8", []byte("#EXTM3U\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=500000,RESOLUTION=640x360\n/low/index.m3u8\n"+
		"#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1920x1080\n/high/index.m3u8\n"))
	m, _, _ := newTestManager(t, nil)

	res, err := m.Probe(t.Context(), srv.URL+"/show/master.m3u8", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.HLS || len(res.Variants) != 2 || res.Variants[0].Height != 1080 || res.Variants[1].Height != 360 {
		t.Fatalf("%+v", res)
	}
	if res.FileName != "show.ts" || res.Size != -1 || !res.Resumable || res.Duration != 20 {
		t.Errorf("%+v", res)
	}

	live := newHLSServer(t)
	live.put("/live.m3u8", []byte("#EXTM3U\n#EXTINF:4,\n/s.ts\n"))
	if _, err := m.Probe(t.Context(), live.URL+"/live.m3u8", nil); err == nil || !strings.Contains(err.Error(), "live") {
		t.Errorf("live probe error = %v", err)
	}
}

func TestHLSAES128(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	key := []byte("sixteen byte key")
	srv.put("/enc/key.bin", key)

	var want []byte
	var pl strings.Builder
	pl.WriteString("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:5\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n")
	for i := range 4 { // IV from the sequence number (5, 6, 7, 8)
		plain := make([]byte, 30_000+i)
		_, _ = rand.Read(plain)
		want = append(want, plain...)
		iv := make([]byte, 16)
		iv[15] = byte(5 + i)
		srv.put(fmt.Sprintf("/enc/s%d.ts", i), encryptCBC(t, key, iv, plain))
		fmt.Fprintf(&pl, "#EXTINF:4,\ns%d.ts\n", i)
	}
	// Then two with an explicit IV.
	iv := bytes.Repeat([]byte{0xAB}, 16)
	pl.WriteString("#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\",IV=0xABABABABABABABABABABABABABABABAB\n")
	for i := 4; i < 6; i++ {
		plain := make([]byte, 25_000+i)
		_, _ = rand.Read(plain)
		want = append(want, plain...)
		srv.put(fmt.Sprintf("/enc/s%d.ts", i), encryptCBC(t, key, iv, plain))
		fmt.Fprintf(&pl, "#EXTINF:4,\ns%d.ts\n", i)
	}
	pl.WriteString("#EXT-X-ENDLIST\n")
	srv.put("/enc/index.m3u8", []byte(pl.String()))

	m, _, dir := newTestManager(t, func(c *Config) { c.Connections = 3 })
	info := addHLS(t, m, srv.URL+"/enc/index.m3u8")
	waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	assertFile(t, filepath.Join(dir, "enc.ts"), want)
	if n := srv.hitCount("/enc/key.bin"); n != 1 {
		t.Errorf("the key was fetched %d times, want once", n)
	}
}

func TestHLSFragmentedMP4(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	initSeg := []byte("ftyp-moov-init")
	srv.put("/f/init.mp4", initSeg)
	body := srv.addMedia("/f", 3, "m4s", `#EXT-X-MAP:URI="/f/init.mp4"`)
	m, _, dir := newTestManager(t, nil)

	info := addHLS(t, m, srv.URL+"/f/index.m3u8")
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	if done.FileName != "f.mp4" {
		t.Errorf("name = %q", done.FileName)
	}
	assertFile(t, filepath.Join(dir, "f.mp4"), append(append([]byte{}, initSeg...), body...))
}

func TestHLSByteRanges(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	whole := make([]byte, 90_000)
	_, _ = rand.Read(whole)
	srv.put("/br/all.ts", whole)
	srv.put("/br/index.m3u8", []byte("#EXTM3U\n"+
		"#EXTINF:4,\n#EXT-X-BYTERANGE:30000@0\nall.ts\n"+
		"#EXTINF:4,\n#EXT-X-BYTERANGE:30000\nall.ts\n"+
		"#EXTINF:4,\n#EXT-X-BYTERANGE:30000\nall.ts\n#EXT-X-ENDLIST\n"))
	m, _, dir := newTestManager(t, nil)

	info := addHLS(t, m, srv.URL+"/br/index.m3u8")
	waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	assertFile(t, filepath.Join(dir, "br.ts"), whole)
}

func TestHLSUnsupportedStreamsFailClearly(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	srv.put("/live.m3u8", []byte("#EXTM3U\n#EXTINF:4,\n/s.ts\n"))
	srv.put("/drm.m3u8", []byte("#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"k\"\n#EXTINF:4,\n/s.ts\n#EXT-X-ENDLIST\n"))
	srv.put("/empty.m3u8", []byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	m, _, _ := newTestManager(t, nil)

	for path, want := range map[string]string{"/live.m3u8": "live", "/drm.m3u8": "SAMPLE-AES", "/empty.m3u8": "no media segments"} {
		info := addHLS(t, m, srv.URL+path)
		failed := waitFor(t, m, info.ID, status(StatusFailed), path)
		if !strings.Contains(failed.Error, want) {
			t.Errorf("%s: error %q should mention %q", path, failed.Error, want)
		}
	}
}

// A piece that cannot be fetched fails the download but keeps the others;
// resuming fetches only what is missing.
func TestHLSResumeKeepsFinishedPieces(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	want := srv.addMedia("/r", 6, "ts")
	srv.setBroken("/r/seg3.ts", true)
	m, _, dir := newTestManager(t, func(c *Config) { c.Connections = 1; c.MaxRetries = 0 })

	info := addHLS(t, m, srv.URL+"/r/index.m3u8")
	failed := waitFor(t, m, info.ID, status(StatusFailed), "failure")
	if !strings.Contains(failed.Error, "404") {
		t.Errorf("error = %q", failed.Error)
	}
	if failed.Downloaded == 0 {
		t.Error("progress of the finished pieces should be kept")
	}

	srv.setBroken("/r/seg3.ts", false)
	if err := m.Resume(info.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	assertFile(t, filepath.Join(dir, "r.ts"), want)

	for i := range 6 {
		wantHits := 1
		if i == 3 {
			wantHits = 2
		}
		if got := srv.hitCount(fmt.Sprintf("/r/seg%d.ts", i)); got != wantHits {
			t.Errorf("seg%d fetched %d times, want %d", i, got, wantHits)
		}
	}
}

func TestHLSRemoveDeletesParts(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	srv.addMedia("/x", 4, "ts")
	srv.setBroken("/x/seg2.ts", true)
	m, _, dir := newTestManager(t, func(c *Config) { c.Connections = 1; c.MaxRetries = 0 })

	info := addHLS(t, m, srv.URL+"/x/index.m3u8")
	waitFor(t, m, info.ID, status(StatusFailed), "failure")
	parts := filepath.Join(dir, "x.ts"+partsSuffix)
	if _, err := os.Stat(parts); err != nil {
		t.Fatalf("parts folder missing: %v", err)
	}
	if err := m.Remove(info.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parts); !os.IsNotExist(err) {
		t.Errorf("parts folder should be gone: %v", err)
	}
}

func TestHLSSeparateAudioNeedsFFmpeg(t *testing.T) {
	srv := newHLSServer(t)
	video := srv.addMedia("/v", 3, "ts")
	audio := srv.addMedia("/a", 3, "ts")
	srv.put("/show/master.m3u8", []byte("#EXTM3U\n"+
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="en",DEFAULT=YES,URI="/a/index.m3u8"`+"\n"+
		`#EXT-X-STREAM-INF:BANDWIDTH=1000000,AUDIO="aud"`+"\n/v/index.m3u8\n"))

	t.Run("without ffmpeg it says so", func(t *testing.T) {
		noFFmpeg(t)
		m, _, _ := newTestManager(t, nil)
		info := addHLS(t, m, srv.URL+"/show/master.m3u8")
		failed := waitFor(t, m, info.ID, status(StatusFailed), "failure")
		if !strings.Contains(failed.Error, "ffmpeg") {
			t.Errorf("error = %q", failed.Error)
		}
	})

	t.Run("with ffmpeg the tracks are merged", func(t *testing.T) {
		stubFFmpeg(t, false)
		m, _, dir := newTestManager(t, nil)
		info := addHLS(t, m, srv.URL+"/show/master.m3u8")
		done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
		if done.FileName != "show.mp4" {
			t.Errorf("name = %q", done.FileName)
		}
		assertFile(t, filepath.Join(dir, "show.mp4"), append(append([]byte{}, video...), audio...))
	})
}

func TestHLSTransportStreamBecomesMP4WithFFmpeg(t *testing.T) {
	srv := newHLSServer(t)
	want := srv.addMedia("/clip", 3, "ts")

	t.Run("ffmpeg converts", func(t *testing.T) {
		stubFFmpeg(t, false)
		m, _, dir := newTestManager(t, nil)
		info := addHLS(t, m, srv.URL+"/clip/index.m3u8")
		done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
		if done.FileName != "clip.mp4" {
			t.Errorf("name = %q", done.FileName)
		}
		assertFile(t, filepath.Join(dir, "clip.mp4"), want)
	})

	t.Run("a failing ffmpeg falls back to the .ts file", func(t *testing.T) {
		stubFFmpeg(t, true)
		m, _, dir := newTestManager(t, nil)
		info := addHLS(t, m, srv.URL+"/clip/index.m3u8")
		done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
		if done.FileName != "clip.ts" {
			t.Errorf("name = %q", done.FileName)
		}
		assertFile(t, filepath.Join(dir, "clip.ts"), want)
		if _, err := os.Stat(filepath.Join(dir, "clip.mp4.part")); !os.IsNotExist(err) {
			t.Error("no stray .part file should remain")
		}
	})
}

func TestHLSProgressEstimateGrowsAccurate(t *testing.T) {
	noFFmpeg(t)
	srv := newHLSServer(t)
	srv.delay = 30 * time.Millisecond
	want := srv.addMedia("/p", 10, "ts")
	m, _, _ := newTestManager(t, func(c *Config) { c.Connections = 1 })

	info := addHLS(t, m, srv.URL+"/p/index.m3u8")
	mid := waitFor(t, m, info.ID, func(i Info) bool { return i.Downloaded > 100_000 }, "some progress")
	if mid.Size <= 0 || len(mid.Segments) != 10 {
		t.Fatalf("an estimated total and one bar piece per segment are expected: %+v", mid)
	}
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	if done.Size != int64(len(want)) || done.Downloaded != done.Size {
		t.Errorf("final size/downloaded = %d/%d, want %d", done.Size, done.Downloaded, len(want))
	}
}

// Converting to MP4 changes the size; a finished download must still read as
// exactly complete rather than "112%".
func TestHLSCompletedSizeMatchesDownloaded(t *testing.T) {
	srv := newHLSServer(t)
	srv.addMedia("/c", 3, "ts")
	// A fake ffmpeg that writes a smaller file than its input.
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	script := "#!/bin/sh\nfor last; do :; done\nhead -c 1000 \"$(echo \"$@\" | sed 's/.*-i \\([^ ]*\\) .*/\\1/')\" > \"$last\"\n"
	p := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := findFFmpeg
	findFFmpeg = func() string { return p }
	t.Cleanup(func() { findFFmpeg = old })

	m, _, _ := newTestManager(t, nil)
	info := addHLS(t, m, srv.URL+"/c/index.m3u8")
	done := waitFor(t, m, info.ID, status(StatusCompleted), "completion")
	if done.Size != 1000 || done.Downloaded != done.Size {
		t.Errorf("size/downloaded = %d/%d, want 1000/1000", done.Size, done.Downloaded)
	}
}
