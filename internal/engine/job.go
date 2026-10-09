package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

const (
	bufSize     = 32 << 10
	idleTimeout = 60 * time.Second
)

// Variables so tests can shrink them.
var (
	minSegmentSize int64 = 1 << 20
	retryBase            = time.Second
)

var errIdle = errors.New("connection stalled")

// fatalError marks an error that must not be retried.
type fatalError struct{ err error }

func (e *fatalError) Error() string { return e.err.Error() }
func (e *fatalError) Unwrap() error { return e.err }

func fatalf(format string, args ...any) error {
	return &fatalError{fmt.Errorf(format, args...)}
}

func retryable(err error) bool {
	var fe *fatalError
	if errors.As(err, &fe) {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		return he.retryable()
	}
	return true
}

func backoff(attempt int) time.Duration {
	d := retryBase << min(attempt, 5)
	return d + rand.N(d/4+1)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// job runs one download from probe to rename. It is owned by the Manager.
type job struct {
	m        *Manager
	e        *entry
	d        Download // working copy; written back through Manager.commit
	client   *http.Client
	headers  map[string]string
	retries  int
	global   *rate.Limiter
	limiter  *rate.Limiter // per-download, nil when unlimited
	finished chan struct{}

	mu   sync.RWMutex // guards the slice headers below, not the elements
	segs []Segment    // immutable plan: Start and End only
	done []atomic.Int64

	url    string
	file   *os.File
	resume bool
	etag   string
}

// snapshot returns the live segment state, or ok=false before the plan exists.
func (j *job) snapshot() (segs []Segment, ok bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.segs == nil {
		return nil, false
	}
	out := make([]Segment, len(j.segs))
	for i, s := range j.segs {
		out[i] = Segment{Start: s.Start, End: s.End, Done: j.done[i].Load()}
	}
	return out, true
}

func (j *job) setPlan(segs []Segment) {
	done := make([]atomic.Int64, len(segs))
	plan := make([]Segment, len(segs))
	for i, s := range segs {
		done[i].Store(s.Done)
		plan[i] = Segment{Start: s.Start, End: s.End}
	}
	j.mu.Lock()
	j.segs, j.done = plan, done
	j.mu.Unlock()
}

func (j *job) downloaded() int64 {
	var n int64
	for i := range j.done {
		n += j.done[i].Load()
	}
	return n
}

func (j *job) run(ctx context.Context) error {
	res, err := j.probe(ctx)
	if err != nil {
		return err
	}

	d := &j.d
	fresh := len(d.Segments) == 0 ||
		!d.Resumable || !res.Resumable ||
		res.Size != d.Size ||
		(d.ETag != "" && res.ETag != "" && res.ETag != d.ETag)
	if !fresh {
		if _, err := os.Stat(d.partPath()); err != nil {
			fresh = true
		}
	}

	if fresh {
		if d.Claimed {
			_ = os.Remove(d.partPath())
		}
		d.Size, d.Resumable, d.ETag, d.LastModified = res.Size, res.Resumable, res.ETag, res.LastModified
		d.Segments = planSegments(res.Size, d.Connections, res.Resumable)
		if err := j.m.claimPath(j, res.FileName); err != nil {
			return &fatalError{err}
		}
	}
	j.m.commit(j, fresh)

	j.url = res.FinalURL
	j.resume = d.Resumable
	j.etag = d.ETag
	j.headers = headersFor(j.headers, d.URL, res.FinalURL)
	j.setPlan(d.Segments)

	f, err := os.OpenFile(d.partPath(), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return &fatalError{err}
	}
	j.file = f
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
	}()
	if d.Size > 0 {
		if err := f.Truncate(d.Size); err != nil {
			return &fatalError{err}
		}
	}

	g, gctx := errgroup.WithContext(ctx)
	for i := range j.segs {
		if s := j.segs[i]; s.End >= 0 && s.Start+j.done[i].Load() > s.End {
			continue
		}
		g.Go(func() error { return j.runSegment(gctx, i) })
	}
	if err := g.Wait(); err != nil {
		return err
	}

	total := j.downloaded()
	if d.Size >= 0 && total != d.Size {
		return fatalf("size mismatch: got %d bytes, expected %d", total, d.Size)
	}
	if d.Size < 0 {
		d.Size = total
		j.m.commit(j, false)
	}

	if err := f.Sync(); err != nil {
		return &fatalError{err}
	}
	closed = true
	if err := f.Close(); err != nil {
		return &fatalError{err}
	}
	return j.finalize()
}

func (j *job) probe(ctx context.Context) (*ProbeResult, error) {
	for attempt := 0; ; attempt++ {
		res, err := Probe(ctx, j.client, j.d.URL, j.headers)
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil || !retryable(err) || attempt >= j.retries {
			return nil, err
		}
		if err := sleepCtx(ctx, backoff(attempt)); err != nil {
			return nil, err
		}
	}
}

// finalize renames the .part file into place and restores the remote mtime.
func (j *job) finalize() error {
	d := &j.d
	var err error
	// Antivirus scanners on Windows may briefly hold the file open.
	for range 5 {
		if err = os.Rename(d.partPath(), d.finalPath()); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		return &fatalError{err}
	}
	if t, perr := http.ParseTime(d.LastModified); perr == nil {
		_ = os.Chtimes(d.finalPath(), t, t)
	}
	return nil
}

func (j *job) runSegment(ctx context.Context, i int) error {
	attempt := 0
	for {
		before := j.done[i].Load()
		err := j.fetch(ctx, i)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryable(err) {
			return err
		}
		if j.done[i].Load() > before {
			attempt = 0 // made progress; the link is flaky, not dead
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

// fetch downloads the remainder of segment i over a single request.
func (j *job) fetch(ctx context.Context, i int) (err error) {
	seg := j.segs[i]
	bounded := seg.End >= 0

	if !j.resume && j.done[i].Load() > 0 {
		// Without range support a retry has to start over.
		j.done[i].Store(0)
	}
	pos := seg.Start + j.done[i].Load()
	if bounded && pos > seg.End {
		return nil
	}

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(idleTimeout, cancel)
	defer idle.Stop()
	defer func() {
		if err != nil && ctx.Err() == nil && reqCtx.Err() != nil {
			err = errIdle
		}
	}()

	req, err := newRequest(reqCtx, j.url, j.headers)
	if err != nil {
		return &fatalError{err}
	}
	if j.resume {
		if bounded {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", pos, seg.End))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", pos))
		}
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if j.resume {
		switch resp.StatusCode {
		case http.StatusPartialContent:
			if start, _, _, ok := parseContentRange(resp.Header.Get("Content-Range")); !ok || start != pos {
				return fatalf("server returned an unexpected byte range")
			}
		case http.StatusOK:
			return fatalf("server stopped honoring Range requests")
		default:
			return newHTTPError(resp)
		}
	} else if resp.StatusCode != http.StatusOK {
		return newHTTPError(resp)
	}
	if e := resp.Header.Get("ETag"); j.etag != "" && e != "" && e != j.etag {
		return fatalf("remote file changed during download")
	}

	buf := make([]byte, bufSize)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			idle.Stop()
			chunk := buf[:n]
			if bounded {
				if rem := seg.End - pos + 1; int64(len(chunk)) > rem {
					chunk = chunk[:max(rem, 0)]
				}
			}
			if len(chunk) > 0 {
				if err := j.throttle(ctx, len(chunk)); err != nil {
					return err
				}
				if _, err := j.file.WriteAt(chunk, pos); err != nil {
					return &fatalError{err}
				}
				pos += int64(len(chunk))
				j.done[i].Add(int64(len(chunk)))
			}
			idle.Reset(idleTimeout)
		}
		if bounded && pos > seg.End {
			return nil
		}
		if rerr != nil {
			if rerr == io.EOF {
				if bounded {
					return io.ErrUnexpectedEOF
				}
				return nil
			}
			return rerr
		}
	}
}

func (j *job) throttle(ctx context.Context, n int) error {
	if err := j.global.WaitN(ctx, n); err != nil {
		return err
	}
	if j.limiter != nil {
		return j.limiter.WaitN(ctx, n)
	}
	return nil
}

// planSegments splits size bytes over up to conns ranges of at least
// minSegmentSize each. Unknown sizes and non-resumable servers get one
// segment; empty files get none.
func planSegments(size int64, conns int, resumable bool) []Segment {
	switch {
	case size < 0:
		return []Segment{{Start: 0, End: -1}}
	case size == 0:
		return nil
	case !resumable || conns <= 1:
		return []Segment{{Start: 0, End: size - 1}}
	}
	n := min(int64(conns), (size+minSegmentSize-1)/minSegmentSize)
	n = max(n, 1)
	chunk := size / n
	segs := make([]Segment, n)
	for i := range segs {
		start := int64(i) * chunk
		end := start + chunk - 1
		if int64(i) == n-1 {
			end = size - 1
		}
		segs[i] = Segment{Start: start, End: end}
	}
	return segs
}

// headersFor drops credentials when the resolved URL is on another host,
// mirroring what net/http does when following a cross-host redirect.
func headersFor(h map[string]string, origURL, finalURL string) map[string]string {
	a, errA := url.Parse(origURL)
	b, errB := url.Parse(finalURL)
	if errA != nil || errB != nil || strings.EqualFold(a.Host, b.Host) {
		return h
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		switch strings.ToLower(k) {
		case "authorization", "cookie", "cookie2", "proxy-authorization", "www-authenticate":
			continue
		}
		out[k] = v
	}
	return out
}
