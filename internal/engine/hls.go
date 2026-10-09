package engine

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"go-idm/internal/netx"
)

// HLS (HTTP Live Streaming) downloads fetch every media segment of a playlist
// into a parts folder next to the target file, then join them. Segments are
// the unit of resume: ones already on disk are not fetched again. Live streams,
// and DRM (SAMPLE-AES and friends) are not supported.

const (
	partsSuffix    = ".parts"
	maxPlaylist    = 16 << 20
	maxEncSegment  = 128 << 20 // an encrypted segment is decrypted in memory
	unknownSegSize = 1 << 20   // progress estimate before any segment has finished
)

// findFFmpeg locates ffmpeg, which turns the MPEG-TS pieces into an .mp4 and
// merges separate audio. A variable so tests can substitute it.
var findFFmpeg = lookFFmpeg

func lookFFmpeg() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	if self, err := os.Executable(); err == nil {
		for _, n := range []string{"ffmpeg", "ffmpeg.exe"} {
			if p := filepath.Join(filepath.Dir(self), n); isRegular(p) {
				return p
			}
		}
	}
	return ""
}

func isRegular(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// looksLikeHLS reports whether a probed URL is an HLS playlist.
func looksLikeHLS(res *ProbeResult) bool {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(res.ContentType, ";", 2)[0]))
	switch ct {
	case "application/vnd.apple.mpegurl", "application/x-mpegurl", "application/mpegurl",
		"audio/mpegurl", "audio/x-mpegurl":
		return true
	}
	u, err := url.Parse(res.FinalURL)
	return err == nil && strings.HasSuffix(strings.ToLower(u.Path), ".m3u8")
}

// hlsTrackPlan is one playlist of a stream: the video (or only) track, and
// optionally a separate audio track.
type hlsTrackPlan struct {
	name string // file-name safe: "video" or "audio"
	pl   *mediaPlaylist
}

// hlsPlan is everything learned from a stream's playlists.
type hlsPlan struct {
	url      string // the playlist the stream was requested with, after redirects
	variants []HLSVariant
	variant  HLSVariant // the chosen one; zero when the URL was a media playlist
	tracks   []hlsTrackPlan
	duration float64 // seconds
}

type hlsFetcher func(ctx context.Context, uri string) (text, finalURL string, err error)

// loadHLS reads the playlists for a stream. variantURL picks a quality from a
// master playlist; empty means the best one.
func loadHLS(ctx context.Context, fetch hlsFetcher, uri, variantURL string) (*hlsPlan, error) {
	text, final, err := fetch(ctx, uri)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(final)
	if err != nil {
		return nil, &fatalError{err}
	}
	master, media, err := parsePlaylist(base, text)
	if err != nil {
		return nil, &fatalError{err}
	}
	plan := &hlsPlan{url: final}

	var audioURI string
	if master != nil {
		plan.variants = master.variants
		plan.variant = master.pick(variantURL)
		audioURI = master.audioFor(plan.variant)
		mtext, mfinal, err := fetch(ctx, plan.variant.URL)
		if err != nil {
			return nil, err
		}
		mbase, err := url.Parse(mfinal)
		if err != nil {
			return nil, &fatalError{err}
		}
		m2, md, err := parsePlaylist(mbase, mtext)
		if err != nil {
			return nil, &fatalError{err}
		}
		if m2 != nil {
			return nil, fatalf("nested master playlists are not supported")
		}
		media = md
	}
	plan.tracks = []hlsTrackPlan{{name: "video", pl: media}}

	if audioURI != "" {
		atext, afinal, err := fetch(ctx, audioURI)
		if err != nil {
			return nil, err
		}
		abase, err := url.Parse(afinal)
		if err != nil {
			return nil, &fatalError{err}
		}
		m2, md, err := parsePlaylist(abase, atext)
		if err != nil || m2 != nil {
			return nil, fatalf("the audio playlist is not valid")
		}
		plan.tracks = append(plan.tracks, hlsTrackPlan{name: "audio", pl: md})
	}

	for _, t := range plan.tracks {
		if err := validateTrack(t.pl); err != nil {
			return nil, err
		}
	}
	for _, s := range plan.tracks[0].pl.segments {
		plan.duration += s.duration
	}
	return plan, nil
}

func validateTrack(pl *mediaPlaylist) error {
	if pl.live {
		return fatalf("live streams are not supported")
	}
	if len(pl.segments) == 0 {
		return fatalf("the playlist has no media segments")
	}
	check := func(k *hlsKey) error {
		if k == nil {
			return nil
		}
		if k.method != "AES-128" {
			return fatalf("%s encrypted streams (DRM) are not supported", k.method)
		}
		if k.uri == "" {
			return fatalf("an encrypted segment has no key URI")
		}
		return nil
	}
	if pl.init != nil {
		if err := check(pl.init.key); err != nil {
			return err
		}
	}
	for i := range pl.segments {
		if err := check(pl.segments[i].key); err != nil {
			return err
		}
	}
	return nil
}

// container is what the joined segments of a track are.
func (t hlsTrackPlan) container() string {
	if t.pl.init != nil {
		return ".mp4" // fragmented MP4: header plus fragments is a playable file
	}
	u, err := url.Parse(t.pl.segments[0].uri)
	if err == nil {
		switch ext := strings.ToLower(path.Ext(u.Path)); ext {
		case ".aac", ".mp3", ".ac3", ".mp4", ".m4a":
			return ext
		}
	}
	return ".ts"
}

var genericStems = map[string]bool{
	"index": true, "master": true, "playlist": true, "chunklist": true, "stream": true,
	"video": true, "manifest": true, "main": true, "hls": true, "prog_index": true, "media": true,
}

// hlsFileName suggests a name for a stream: from the playlist's URL, ending in
// .mp4 when the pieces can be put in an MP4 (fragmented MP4, or MPEG-TS with
// ffmpeg), else in the pieces' own format.
func hlsFileName(playlistURL string, plan *hlsPlan, haveFFmpeg bool) string {
	stem := ""
	if u, err := url.Parse(playlistURL); err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := len(parts) - 1; i >= 0 && stem == ""; i-- {
			s := strings.TrimSuffix(parts[i], path.Ext(parts[i]))
			if s != "" && !genericStems[strings.ToLower(s)] {
				stem = s
			}
		}
		if stem == "" {
			stem = u.Hostname()
		}
	}
	if stem == "" {
		stem = "video"
	}
	ext := plan.tracks[0].container()
	if len(plan.tracks) > 1 || (ext == ".ts" && haveFFmpeg) {
		ext = ".mp4"
	}
	return netx.SanitizeFileName(stem + ext)
}

// hlsSeg is one fetch: a media segment or a track's initialization segment.
type hlsSeg struct {
	seg    hlsSegment
	track  int
	dur    float64
	path   string       // the finished piece on disk
	done   atomic.Int64 // bytes received in the current attempt
	size   atomic.Int64 // exact size once finished, 0 before
	isInit bool
}

type hlsRun struct {
	plan      *hlsPlan
	dir       string
	segs      []*hlsSeg
	bandwidth int // bits per second of the chosen variant, for estimates

	keyMu sync.Mutex
	keys  map[string][]byte
}

func newHLSRun(plan *hlsPlan, dir string) *hlsRun {
	r := &hlsRun{plan: plan, dir: dir, bandwidth: plan.variant.Bandwidth, keys: map[string][]byte{}}
	for ti, t := range plan.tracks {
		if t.pl.init != nil {
			r.segs = append(r.segs, &hlsSeg{seg: *t.pl.init, track: ti, isInit: true,
				path: filepath.Join(dir, fmt.Sprintf("%s-init.seg", t.name))})
		}
		for i, s := range t.pl.segments {
			r.segs = append(r.segs, &hlsSeg{seg: s, track: ti, dur: s.duration,
				path: filepath.Join(dir, fmt.Sprintf("%s-%06d.seg", t.name, i))})
		}
	}
	return r
}

// fingerprint identifies the plan, so pieces left from a different stream
// (or the same URL serving something new) are never joined into this one.
func (r *hlsRun) fingerprint() string {
	h := sha256.New()
	for _, s := range r.segs {
		fmt.Fprintf(h, "%d|%s|%d|%d\n", s.track, s.seg.uri, s.seg.offset, s.seg.length)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// snapshot reports progress as byte ranges laid end to end. Finished pieces
// have their real size; others are estimated from the average so far, so the
// total grows more accurate as the download goes.
func (r *hlsRun) snapshot() []Segment {
	var doneBytes int64
	var doneDur float64
	for _, s := range r.segs {
		if n := s.size.Load(); n > 0 && s.dur > 0 {
			doneBytes += n
			doneDur += s.dur
		}
	}
	perSec := 0.0
	switch {
	case doneDur > 0:
		perSec = float64(doneBytes) / doneDur
	case r.bandwidth > 0:
		perSec = float64(r.bandwidth) / 8
	}

	out := make([]Segment, len(r.segs))
	var pos int64
	for i, s := range r.segs {
		size, done := s.size.Load(), s.done.Load()
		length := size
		if size > 0 {
			done = size
		} else {
			est := int64(perSec * s.dur)
			switch {
			case est <= 0 && perSec == 0:
				est = unknownSegSize
			case est < 16<<10:
				est = 16 << 10
			}
			length = max(est, done)
		}
		out[i] = Segment{Start: pos, End: pos + length - 1, Done: done}
		pos += length
	}
	return out
}

func segmentsSize(segs []Segment) int64 {
	if len(segs) == 0 {
		return -1
	}
	return segs[len(segs)-1].End + 1
}

// fetchText downloads a playlist, retrying transient failures.
func (j *job) fetchText(ctx context.Context, uri string) (string, string, error) {
	for attempt := 0; ; attempt++ {
		text, final, err := j.fetchTextOnce(ctx, uri)
		if err == nil {
			return text, final, nil
		}
		if ctx.Err() != nil || !retryable(err) || attempt >= j.retries {
			return "", "", err
		}
		if err := sleepCtx(ctx, backoff(attempt)); err != nil {
			return "", "", err
		}
	}
}

func (j *job) fetchTextOnce(ctx context.Context, uri string) (string, string, error) {
	return fetchPlaylist(ctx, j.client, uri, headersFor(j.headers, j.d.URL, uri))
}

func fetchPlaylist(ctx context.Context, c *http.Client, uri string, headers map[string]string) (string, string, error) {
	req, err := newRequest(ctx, uri, headers)
	if err != nil {
		return "", "", &fatalError{err}
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", newHTTPError(resp)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPlaylist+1))
	if err != nil {
		return "", "", err
	}
	if len(b) > maxPlaylist {
		return "", "", fatalf("the playlist is too large")
	}
	return string(b), resp.Request.URL.String(), nil
}

// enrichHLS fills a probe result for a playlist URL: quality choices, a file
// name and duration. It fails for streams that cannot be downloaded, so the
// Add dialog can say so before anything is queued.
func enrichHLS(ctx context.Context, c *http.Client, res *ProbeResult, headers map[string]string, variantURL string) error {
	fetch := func(ctx context.Context, uri string) (string, string, error) {
		return fetchPlaylist(ctx, c, uri, headersFor(headers, res.FinalURL, uri))
	}
	plan, err := loadHLS(ctx, fetch, res.FinalURL, variantURL)
	if err != nil {
		return err
	}
	res.HLS = true
	res.Variants = plan.variants
	res.Duration = plan.duration
	res.Size = -1
	res.Resumable = true
	res.FileName = hlsFileName(res.FinalURL, plan, findFFmpeg() != "")
	return nil
}

// runHLS downloads a stream whose playlist was probed as res.
func (j *job) runHLS(ctx context.Context, res *ProbeResult) error {
	d := &j.d
	plan, err := loadHLS(ctx, j.fetchText, res.FinalURL, d.Variant)
	if err != nil {
		return err
	}
	ffmpeg := findFFmpeg()
	if len(plan.tracks) > 1 && ffmpeg == "" {
		return fatalf("this stream keeps its audio in a separate track; install ffmpeg and add it to PATH to download it")
	}

	if err := j.m.claimPath(j, hlsFileName(res.FinalURL, plan, ffmpeg != "")); err != nil {
		return &fatalError{err}
	}
	d.Resumable, d.LastModified, d.ETag = true, "", ""
	j.url = res.FinalURL
	j.headers = headersFor(j.headers, d.URL, res.FinalURL)

	run := newHLSRun(plan, d.finalPath()+partsSuffix)
	if err := prepareParts(run); err != nil {
		return &fatalError{err}
	}
	j.mu.Lock()
	j.hls = run
	j.mu.Unlock()
	d.Size = segmentsSize(run.snapshot())
	j.m.commit(j, false)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(max(d.Connections, 1))
	for _, s := range run.segs {
		if s.size.Load() > 0 {
			continue
		}
		g.Go(func() error { return j.hlsSegment(gctx, run, s) })
	}
	if err := g.Wait(); err != nil {
		return err
	}

	if err := j.assembleHLS(ctx, run, ffmpeg); err != nil {
		return err
	}
	st, err := os.Stat(d.partPath())
	if err != nil {
		return &fatalError{err}
	}
	d.Size = st.Size()
	j.m.commit(j, false)
	if err := j.finalize(); err != nil {
		return err
	}
	_ = os.RemoveAll(run.dir)
	return nil
}

// prepareParts readies the parts folder and marks pieces already downloaded.
func prepareParts(r *hlsRun) error {
	fp := r.fingerprint()
	planFile := filepath.Join(r.dir, "plan")
	if old, err := os.ReadFile(planFile); err != nil || string(old) != fp {
		if err := os.RemoveAll(r.dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(planFile, []byte(fp), 0o644); err != nil {
		return err
	}
	for _, s := range r.segs {
		if st, err := os.Stat(s.path); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
			s.size.Store(st.Size())
		}
	}
	return nil
}

func (j *job) hlsSegment(ctx context.Context, r *hlsRun, s *hlsSeg) error {
	attempt := 0
	for {
		err := j.hlsFetch(ctx, r, s)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryable(err) {
			return err
		}
		attempt++
		if attempt > j.retries {
			return err
		}
		if err := sleepCtx(ctx, backoff(attempt-1)); err != nil {
			return err
		}
	}
}

// hlsFetch makes one attempt at one piece. It writes to a temporary file and
// renames it, so a piece on disk is always complete.
func (j *job) hlsFetch(ctx context.Context, r *hlsRun, s *hlsSeg) (err error) {
	s.done.Store(0)
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(idleTimeout, cancel)
	defer idle.Stop()
	defer func() {
		if err != nil && ctx.Err() == nil && reqCtx.Err() != nil {
			err = errIdle
		}
	}()

	var key []byte
	if k := s.seg.key; k != nil {
		if key, err = j.hlsKey(reqCtx, r, k.uri); err != nil {
			return err
		}
	}

	req, err := newRequest(reqCtx, s.seg.uri, headersFor(j.headers, j.url, s.seg.uri))
	if err != nil {
		return &fatalError{err}
	}
	ranged := s.seg.length > 0
	if ranged {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", s.seg.offset, s.seg.offset+s.seg.length-1))
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent && ranged, resp.StatusCode == http.StatusOK && !ranged:
	case resp.StatusCode == http.StatusOK:
		return fatalf("server ignored the byte range of a stream segment")
	default:
		return newHTTPError(resp)
	}

	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return &fatalError{err}
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(tmp)
		}
	}()

	// Reads feed progress and the speed limits; encrypted pieces are held in
	// memory so they can be decrypted as a whole.
	src := &meteredReader{r: resp.Body, onRead: func(n int) error {
		idle.Reset(idleTimeout)
		s.done.Add(int64(n))
		return j.throttle(ctx, n)
	}}
	var written int64
	if key == nil {
		written, err = io.Copy(f, src)
		if err != nil {
			return err
		}
	} else {
		data, rerr := io.ReadAll(io.LimitReader(src, maxEncSegment+1))
		if rerr != nil {
			return rerr
		}
		if len(data) > maxEncSegment {
			return fatalf("an encrypted segment is too large")
		}
		plain, derr := decryptSegment(data, key, ivFor(s.seg))
		if derr != nil {
			return &fatalError{derr}
		}
		n, werr := f.Write(plain)
		if werr != nil {
			return &fatalError{werr}
		}
		written = int64(n)
	}
	if written == 0 {
		return io.ErrUnexpectedEOF
	}
	if err := f.Close(); err != nil {
		return &fatalError{err}
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return &fatalError{err}
	}
	s.size.Store(written)
	return nil
}

type meteredReader struct {
	r      io.Reader
	onRead func(n int) error
}

func (m *meteredReader) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	if n > 0 {
		if cerr := m.onRead(n); cerr != nil {
			return n, cerr
		}
	}
	return n, err
}

func (j *job) hlsKey(ctx context.Context, r *hlsRun, uri string) ([]byte, error) {
	r.keyMu.Lock()
	defer r.keyMu.Unlock()
	if k, ok := r.keys[uri]; ok {
		return k, nil
	}
	req, err := newRequest(ctx, uri, headersFor(j.headers, j.url, uri))
	if err != nil {
		return nil, &fatalError{err}
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, newHTTPError(resp)
	}
	k, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return nil, err
	}
	if len(k) != 16 {
		return nil, fatalf("the encryption key is %d bytes, expected 16", len(k))
	}
	r.keys[uri] = k
	return k, nil
}

// ivFor returns the CBC initialization vector of a segment: the one in its
// key tag, else its media sequence number.
func ivFor(s hlsSegment) []byte {
	if s.key != nil && s.key.iv != nil {
		return s.key.iv
	}
	iv := make([]byte, 16)
	binary.BigEndian.PutUint64(iv[8:], uint64(s.seq))
	return iv
}

func decryptSegment(data, key, iv []byte) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("encrypted segment is not a whole number of blocks")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(data, data)
	pad := int(data[len(data)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(data) ||
		!bytes.Equal(data[len(data)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, errors.New("could not decrypt a segment (wrong key?)")
	}
	return data[:len(data)-pad], nil
}

// assembleHLS joins the pieces of each track, then turns the result into the
// final .part file: copied as is, or through ffmpeg when MPEG-TS should become
// an MP4 or when audio and video are separate.
func (j *job) assembleHLS(ctx context.Context, r *hlsRun, ffmpeg string) error {
	d := &j.d
	joined := make([]string, len(r.plan.tracks))
	for ti, t := range r.plan.tracks {
		joined[ti] = filepath.Join(r.dir, t.name+".joined")
		if err := joinPieces(joined[ti], r.segs, ti); err != nil {
			return &fatalError{err}
		}
	}

	wantMP4 := strings.EqualFold(filepath.Ext(d.FileName), ".mp4")
	switch {
	case len(joined) > 1:
		if ffmpeg == "" {
			return fatalf("ffmpeg is needed to merge the audio and video tracks")
		}
		args := []string{"-i", joined[0], "-i", joined[1], "-map", "0:v?", "-map", "1:a?", "-c", "copy"}
		if err := runFFmpeg(ctx, ffmpeg, args, d.partPath()); err != nil {
			return fatalf("ffmpeg could not merge the tracks: %v", err)
		}
	case wantMP4 && r.plan.tracks[0].container() == ".ts":
		if ffmpeg != "" {
			if err := runFFmpeg(ctx, ffmpeg, []string{"-i", joined[0], "-c", "copy"}, d.partPath()); err == nil {
				return nil
			} else if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		// No usable ffmpeg: keep the stream as it is, under a name that says so.
		if err := j.renameToTS(); err != nil {
			return &fatalError{err}
		}
		fallthrough
	default:
		if err := os.Rename(joined[0], d.partPath()); err != nil {
			return &fatalError{err}
		}
	}
	return nil
}

// renameToTS changes the claimed name from .mp4 to .ts.
func (j *job) renameToTS() error {
	d := &j.d
	stem := strings.TrimSuffix(d.FileName, filepath.Ext(d.FileName))
	name := netx.UniqueName(stem+".ts", func(n string) bool {
		return n != d.FileName && (exists(filepath.Join(d.Dir, n)) || exists(filepath.Join(d.Dir, n)+partSuffix))
	})
	// The part file is created under the new name; drop the one for the old.
	_ = os.Remove(d.partPath())
	d.FileName = name
	d.Category = CategoryFor(name)
	j.m.commit(j, false)
	return nil
}

func joinPieces(dst string, segs []*hlsSeg, track int) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	for _, s := range segs {
		if s.track != track {
			continue
		}
		in, err := os.Open(s.path)
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			out.Close()
			return err
		}
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func runFFmpeg(ctx context.Context, ffmpeg string, inputs []string, out string) error {
	args := append([]string{"-y", "-hide_banner", "-loglevel", "error"}, inputs...)
	args = append(args, "-f", "mp4", out)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(out)
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(clip(msg))
	}
	return nil
}
