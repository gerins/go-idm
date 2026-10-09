package engine

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"go-idm/internal/netx"
)

const (
	tickInterval = 500 * time.Millisecond
	minBurst     = 2 * bufSize
)

// Store persists downloads. Implementations must be safe for concurrent use.
type Store interface {
	Save(d *Download) error
	Delete(id string) error
	List() ([]*Download, error)
}

type entry struct {
	d      Download
	job    *job
	cancel context.CancelCauseFunc

	lastBytes int64
	lastAt    time.Time
	speed     float64
}

// Manager owns the download queue and schedules jobs.
type Manager struct {
	store Store
	hooks Hooks

	mu      sync.Mutex // guards everything below
	cfg     Config
	client  *http.Client
	entries map[string]*entry
	order   []string
	closed  bool
	tickN   int

	// outMu serializes persistence and hook delivery so that snapshots built
	// under mu reach the store and the UI in the order they were taken.
	outMu sync.Mutex

	global *rate.Limiter
	root   context.Context
	stop   context.CancelCauseFunc
	wg     sync.WaitGroup
}

// NewManager creates a manager. Call Load to restore saved downloads and
// Start to begin progress reporting.
func NewManager(store Store, cfg Config, hooks Hooks) (*Manager, error) {
	cfg = cfg.normalized()
	client, err := netx.NewClient(cfg.Proxy)
	if err != nil {
		return nil, err
	}
	root, stop := context.WithCancelCause(context.Background())
	m := &Manager{
		store:   store,
		hooks:   hooks,
		cfg:     cfg,
		client:  client,
		entries: make(map[string]*entry),
		global:  rate.NewLimiter(rate.Inf, minBurst),
		root:    root,
		stop:    stop,
	}
	m.applyGlobalLimit(cfg.SpeedLimit)
	return m, nil
}

func (m *Manager) applyGlobalLimit(bps int64) {
	if bps <= 0 {
		m.global.SetLimit(rate.Inf)
		m.global.SetBurst(minBurst)
		return
	}
	m.global.SetLimit(rate.Limit(bps))
	m.global.SetBurst(int(max(bps/4, minBurst)))
}

// Load restores persisted downloads. Anything that was running or queued when
// the app last exited comes back paused so startup never surprises the user
// with bandwidth use.
func (m *Manager) Load() error {
	ds, err := m.store.List()
	if err != nil {
		return err
	}
	slices.SortStableFunc(ds, func(a, b *Download) int {
		return cmp.Or(cmp.Compare(a.Order, b.Order), a.CreatedAt.Compare(b.CreatedAt))
	})
	// Downloads saved before queue order existed all have Order 0. Number
	// them in creation order, and save so every row agrees from now on.
	if !isStrictlyOrdered(ds) {
		for i, d := range ds {
			d.Order = float64(i + 1)
			if err := m.store.Save(d); err != nil {
				return err
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range ds {
		if d.Status == StatusActive || d.Status == StatusQueued {
			d.Status = StatusPaused
		}
		m.entries[d.ID] = &entry{d: *d}
		m.order = append(m.order, d.ID)
	}
	return nil
}

// Start launches the progress ticker.
func (m *Manager) Start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(tickInterval)
		defer t.Stop()
		for {
			select {
			case <-m.root.Done():
				return
			case <-t.C:
				m.tick()
			}
		}
	}()
}

// Close pauses every running download, persists state and waits for workers.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	var jobs []*job
	for _, e := range m.entries {
		if e.job != nil {
			jobs = append(jobs, e.job)
		}
	}
	m.mu.Unlock()

	m.stop(errShutdown)
	deadline := time.After(10 * time.Second)
	for _, j := range jobs {
		select {
		case <-j.finished:
		case <-deadline:
		}
	}
	m.wg.Wait()
}

// Config returns the active configuration.
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// SetConfig applies a new configuration. Running downloads keep their
// connection settings; new and resumed ones pick up the change.
func (m *Manager) SetConfig(c Config) error {
	c = c.normalized()
	client, err := netx.NewClient(c.Proxy)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = c
	m.client = client
	m.applyGlobalLimit(c.SpeedLimit)
	var f flush
	m.scheduleLocked(&f)
	m.release(&f)
	return nil
}

// Probe inspects a URL without creating a download.
func (m *Manager) Probe(ctx context.Context, rawURL string, headers map[string]string) (*ProbeResult, error) {
	m.mu.Lock()
	client := m.client
	h := m.requestHeaders(headers)
	m.mu.Unlock()
	res, err := Probe(ctx, client, rawURL, h)
	if err == nil && looksLikeHLS(res) {
		if err := enrichHLS(ctx, client, res, h, ""); err != nil {
			return nil, err
		}
	}
	return res, err
}

// CheckURL reports whether raw is a URL the engine can download.
func CheckURL(raw string) error {
	_, err := parseURL(raw)
	return err
}

// Add registers a download and schedules it.
func (m *Manager) Add(req AddRequest) (Info, error) {
	u, err := parseURL(req.URL)
	if err != nil {
		return Info{}, err
	}
	dir := ""
	if req.Dir != "" {
		if !filepath.IsAbs(req.Dir) {
			return Info{}, fmt.Errorf("download directory must be an absolute path")
		}
		dir = filepath.Clean(req.Dir)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Info{}, ErrClosed
	}
	conns := req.Connections
	if conns <= 0 {
		conns = m.cfg.Connections
	}
	e := &entry{d: Download{
		ID:          newID(),
		URL:         u.String(),
		FileName:    netx.SanitizeFileName(req.FileName),
		Dir:         dir,
		Size:        -1,
		Connections: clamp(conns, 1, 32),
		SpeedLimit:  max(req.SpeedLimit, 0),
		Headers:     maps.Clone(req.Headers),
		PageURL:     webURL(req.PageURL),
		Variant:     webURL(req.Variant),
		Order:       m.nextOrderLocked(),
		Status:      StatusQueued,
		CreatedAt:   time.Now(),
	}}
	if req.StartPaused {
		e.d.Status = StatusPaused
	}
	m.entries[e.d.ID] = e
	m.order = append(m.order, e.d.ID)

	var f flush
	f.touch(e, true)
	m.scheduleLocked(&f)
	info := m.infoLocked(e)
	m.release(&f)
	return info, nil
}

// nextOrderLocked is the queue position for a download added at the end.
func (m *Manager) nextOrderLocked() float64 {
	if n := len(m.order); n > 0 {
		return m.entries[m.order[n-1]].d.Order + 1
	}
	return 1
}

func isStrictlyOrdered(ds []*Download) bool {
	for i := 1; i < len(ds); i++ {
		if ds[i].Order <= ds[i-1].Order {
			return false
		}
	}
	return true
}

// Move places a download immediately before beforeID in the queue, or at the
// end when beforeID is empty. Queued downloads start in queue order; running
// ones are not interrupted.
func (m *Manager) Move(id, beforeID string) error {
	m.mu.Lock()
	e, ok := m.entries[id]
	if !ok || (beforeID != "" && m.entries[beforeID] == nil) {
		m.mu.Unlock()
		return ErrNotFound
	}
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	if id == beforeID {
		m.mu.Unlock()
		return nil
	}

	rest := slices.DeleteFunc(slices.Clone(m.order), func(s string) bool { return s == id })
	at := len(rest)
	if beforeID != "" {
		at = slices.Index(rest, beforeID)
	}
	moved := slices.Insert(slices.Clone(rest), at, id)
	if slices.Equal(moved, m.order) {
		m.mu.Unlock()
		return nil
	}
	m.order = moved

	// Give the download a position between its new neighbours, so only its own
	// row has to be saved. When the gap has run out, renumber everything.
	var f flush
	switch {
	case at == len(rest):
		e.d.Order = m.entries[rest[at-1]].d.Order + 1
	case at == 0:
		e.d.Order = m.entries[rest[0]].d.Order - 1
	default:
		lo, hi := m.entries[rest[at-1]].d.Order, m.entries[rest[at]].d.Order
		e.d.Order = (lo + hi) / 2
		if e.d.Order <= lo || e.d.Order >= hi {
			for i, oid := range m.order {
				m.entries[oid].d.Order = float64(i + 1)
				f.touch(m.entries[oid], true)
			}
		}
	}
	f.touch(e, true)
	m.release(&f)
	return nil
}

// Pause stops a running download, keeping its progress, or dequeues a queued one.
func (m *Manager) Pause(id string) error {
	m.mu.Lock()
	e, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	var f flush
	m.pauseLocked(e, &f)
	m.scheduleLocked(&f)
	m.release(&f)
	return nil
}

func (m *Manager) pauseLocked(e *entry, f *flush) {
	switch e.d.Status {
	case StatusActive:
		e.d.Status = StatusPaused
		if e.cancel != nil {
			e.cancel(errPaused)
		}
	case StatusQueued:
		e.d.Status = StatusPaused
	default:
		return
	}
	f.touch(e, true)
}

// Resume queues a paused or failed download.
func (m *Manager) Resume(id string) error {
	m.mu.Lock()
	e, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	var f flush
	m.resumeLocked(e, &f)
	m.scheduleLocked(&f)
	m.release(&f)
	return nil
}

func (m *Manager) resumeLocked(e *entry, f *flush) {
	if e.d.Status != StatusPaused && e.d.Status != StatusFailed {
		return
	}
	e.d.Status = StatusQueued
	e.d.Error = ""
	f.touch(e, true)
}

// PauseAll pauses every active and queued download.
func (m *Manager) PauseAll() {
	m.mu.Lock()
	var f flush
	for _, id := range m.order {
		m.pauseLocked(m.entries[id], &f)
	}
	m.scheduleLocked(&f)
	m.release(&f)
}

// ResumeAll queues every paused or failed download.
func (m *Manager) ResumeAll() {
	m.mu.Lock()
	var f flush
	for _, id := range m.order {
		m.resumeLocked(m.entries[id], &f)
	}
	m.scheduleLocked(&f)
	m.release(&f)
}

// Remove stops and forgets a download. With deleteFiles it also deletes the
// partial and completed files from disk.
func (m *Manager) Remove(id string, deleteFiles bool) error {
	m.mu.Lock()
	e, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	delete(m.entries, id)
	m.order = slices.DeleteFunc(m.order, func(s string) bool { return s == id })
	j := e.job
	if j != nil && e.cancel != nil {
		e.cancel(errRemoved)
	}
	d := e.d

	f := flush{removed: []string{id}}
	m.scheduleLocked(&f)
	m.release(&f)

	if j != nil {
		select {
		case <-j.finished:
		case <-time.After(5 * time.Second):
		}
	}
	if deleteFiles && d.Claimed {
		_ = os.Remove(d.partPath())
		_ = os.Remove(d.finalPath())
		_ = os.RemoveAll(d.finalPath() + partsSuffix)
	}
	return nil
}

// Get returns one download's snapshot.
func (m *Manager) Get(id string) (Info, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return Info{}, false
	}
	return m.infoLocked(e), true
}

// List returns all downloads in queue order.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.infoLocked(m.entries[id]))
	}
	return out
}

// scheduleLocked starts queued downloads while slots are free.
func (m *Manager) scheduleLocked(f *flush) {
	if m.closed {
		return
	}
	active := 0
	for _, e := range m.entries {
		if e.job != nil {
			active++
		}
	}
	for _, id := range m.order {
		if active >= m.cfg.MaxActive {
			return
		}
		e := m.entries[id]
		// A job that was just paused may still be winding down; its
		// finish() reschedules once the slot is really free.
		if e.d.Status != StatusQueued || e.job != nil {
			continue
		}
		m.startLocked(e)
		f.touch(e, true)
		active++
	}
}

func (m *Manager) startLocked(e *entry) {
	ctx, cancel := context.WithCancelCause(m.root)
	j := &job{
		m:        m,
		e:        e,
		d:        cloneDownload(&e.d),
		client:   m.client,
		headers:  m.requestHeaders(e.d.Headers),
		retries:  m.cfg.MaxRetries,
		global:   m.global,
		finished: make(chan struct{}),
	}
	if lim := e.d.SpeedLimit; lim > 0 {
		j.limiter = rate.NewLimiter(rate.Limit(lim), int(max(lim/4, minBurst)))
	}
	e.job, e.cancel = j, cancel
	e.d.Status = StatusActive
	e.d.Error = ""
	e.speed, e.lastBytes, e.lastAt = 0, e.d.Downloaded(), time.Now()
	go m.execute(ctx, j)
}

func (m *Manager) execute(ctx context.Context, j *job) {
	defer close(j.finished)
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("internal error: %v", r)
				slog.Error("download job panicked", "id", j.d.ID, "panic", r)
			}
		}()
		err = j.run(ctx)
	}()
	m.finish(j, err, context.Cause(ctx))
}

func (m *Manager) finish(j *job, runErr, cause error) {
	m.mu.Lock()
	e := j.e
	e.job, e.cancel, e.speed = nil, nil, 0
	if m.entries[e.d.ID] != e { // removed while running
		m.mu.Unlock()
		return
	}
	if segs, ok := j.snapshot(); ok {
		e.d.Segments = segs
		if j.hls != nil {
			if runErr != nil {
				e.d.Size = segmentsSize(segs) // the estimate, for the paused or failed row
			} else {
				// The joined file can differ in size from the pieces (an MP4
				// made from MPEG-TS is smaller), so show it as one full piece.
				e.d.Segments = []Segment{{Start: 0, End: e.d.Size - 1, Done: e.d.Size}}
			}
		}
	}
	switch {
	case runErr == nil:
		e.d.Status = StatusCompleted
		e.d.Error = ""
		e.d.CompletedAt = time.Now()
	case cause == errPaused || cause == errShutdown:
		if e.d.Status == StatusActive {
			e.d.Status = StatusPaused
		}
	default:
		if e.d.Status == StatusActive {
			e.d.Status = StatusFailed
			e.d.Error = runErr.Error()
		}
	}
	var f flush
	f.touch(e, true)
	m.scheduleLocked(&f)
	m.release(&f)
}

// commit copies job-discovered metadata into the entry and persists it.
func (m *Manager) commit(j *job, withSegments bool) {
	m.mu.Lock()
	e := j.e
	if m.entries[e.d.ID] != e {
		m.mu.Unlock()
		return
	}
	d := &j.d
	e.d.Size, e.d.Resumable, e.d.ETag, e.d.LastModified = d.Size, d.Resumable, d.ETag, d.LastModified
	e.d.FileName, e.d.Dir, e.d.Category, e.d.Claimed = d.FileName, d.Dir, d.Category, d.Claimed
	if withSegments {
		e.d.Segments = slices.Clone(d.Segments)
	}
	var f flush
	f.touch(e, true)
	m.release(&f)
}

// claimPath chooses and reserves the final directory and file name.
func (m *Manager) claimPath(j *job, suggested string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := &j.d
	if d.Claimed {
		return nil
	}
	name := netx.SanitizeFileName(d.FileName)
	if name == "" {
		name = netx.SanitizeFileName(suggested)
	}
	if name == "" {
		name = "download"
	}
	category := CategoryFor(name)
	dir := d.Dir
	if dir == "" {
		dir = m.cfg.DownloadDir
		if m.cfg.Categorize {
			dir = filepath.Join(dir, category)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name = netx.UniqueName(name, func(n string) bool {
		p := filepath.Join(dir, n)
		if exists(p) || exists(p+partSuffix) || exists(p+partsSuffix) {
			return true
		}
		for _, o := range m.entries {
			if o != j.e && o.d.Claimed && o.d.Status != StatusCompleted &&
				o.d.Dir == dir && strings.EqualFold(o.d.FileName, n) {
				return true
			}
		}
		return false
	})
	d.Dir, d.FileName, d.Category, d.Claimed = dir, name, category, true
	// Publish the claim to the shared entry now, under the same lock. Until it
	// is there another download picking a name would not see this one and could
	// choose the same file.
	e := j.e
	e.d.Dir, e.d.FileName, e.d.Category, e.d.Claimed = dir, name, category, true
	return nil
}

func (m *Manager) requestHeaders(extra map[string]string) map[string]string {
	h := map[string]string{"User-Agent": m.cfg.UserAgent}
	for k, v := range extra {
		h[http.CanonicalHeaderKey(k)] = v
	}
	// The engine owns these.
	delete(h, "Range")
	delete(h, "Accept-Encoding")
	return h
}

func (m *Manager) tick() {
	m.mu.Lock()
	m.tickN++
	persist := m.tickN%2 == 0
	now := time.Now()
	var f flush
	for _, id := range m.order {
		e := m.entries[id]
		if e.job == nil {
			continue
		}
		var total int64
		if segs, ok := e.job.snapshot(); ok {
			for _, s := range segs {
				total += s.Done
			}
		} else {
			total = e.d.Downloaded()
		}
		if dt := now.Sub(e.lastAt).Seconds(); dt > 0 {
			inst := float64(total-e.lastBytes) / dt
			if e.speed == 0 {
				e.speed = inst
			} else {
				e.speed = 0.7*e.speed + 0.3*inst
			}
		}
		e.lastBytes, e.lastAt = total, now
		f.touch(e, persist)
	}
	m.release(&f)
}

type touchItem struct {
	e       *entry
	persist bool
}

// flush collects the changes a locked section wants delivered.
type flush struct {
	items   []touchItem
	removed []string
}

func (f *flush) touch(e *entry, persist bool) {
	for i := range f.items {
		if f.items[i].e == e {
			f.items[i].persist = f.items[i].persist || persist
			return
		}
	}
	f.items = append(f.items, touchItem{e, persist})
}

// release must be called with m.mu held. It snapshots the touched entries,
// unlocks, then persists and notifies in snapshot order.
func (m *Manager) release(f *flush) {
	var infos []Info
	var saves []*Download
	for _, it := range f.items {
		if m.entries[it.e.d.ID] != it.e {
			continue
		}
		infos = append(infos, m.infoLocked(it.e))
		if it.persist {
			saves = append(saves, m.snapshotLocked(it.e))
		}
	}
	if len(infos) == 0 && len(f.removed) == 0 {
		m.mu.Unlock()
		return
	}
	m.outMu.Lock()
	m.mu.Unlock()
	defer m.outMu.Unlock()

	for _, id := range f.removed {
		if err := m.store.Delete(id); err != nil {
			slog.Error("delete download", "id", id, "err", err)
		}
		if m.hooks.OnRemove != nil {
			m.hooks.OnRemove(id)
		}
	}
	for _, d := range saves {
		if err := m.store.Save(d); err != nil {
			slog.Error("save download", "id", d.ID, "err", err)
		}
	}
	if len(infos) > 0 && m.hooks.OnUpdate != nil {
		m.hooks.OnUpdate(infos)
	}
}

// snapshotLocked copies an entry's persistent state with live segment progress.
func (m *Manager) snapshotLocked(e *entry) *Download {
	d := cloneDownload(&e.d)
	if e.job != nil {
		if segs, ok := e.job.snapshot(); ok {
			d.Segments = segs
			if e.job.hls != nil {
				d.Size = segmentsSize(segs)
			}
		}
	}
	return &d
}

func (m *Manager) infoLocked(e *entry) Info {
	d := m.snapshotLocked(e)
	downloaded := d.Downloaded()
	name := d.FileName
	if name == "" {
		name = netx.SanitizeFileName(netx.NameFromURL(d.URL))
	}
	info := Info{
		ID:          d.ID,
		URL:         d.URL,
		FileName:    name,
		Dir:         d.Dir,
		Category:    d.Category,
		Size:        d.Size,
		Downloaded:  downloaded,
		Speed:       int64(e.speed),
		ETA:         -1,
		Status:      d.Status,
		Error:       d.Error,
		Resumable:   d.Resumable,
		Connections: d.Connections,
		SpeedLimit:  d.SpeedLimit,
		PageURL:     pageURL(d),
		Order:       d.Order,
		CreatedAt:   d.CreatedAt,
		CompletedAt: d.CompletedAt,
		Segments:    make([]SegmentInfo, len(d.Segments)),
	}
	for i, s := range d.Segments {
		info.Segments[i] = SegmentInfo(s)
	}
	if d.Claimed {
		if d.Status == StatusCompleted {
			info.Path = d.finalPath()
		} else {
			info.Path = d.partPath()
		}
	}
	if d.Status == StatusCompleted {
		info.Downloaded = max(d.Size, downloaded)
	}
	if e.speed > 1 && d.Size > 0 && d.Status == StatusActive {
		info.ETA = int64(float64(d.Size-downloaded) / e.speed)
	}
	return info
}

func cloneDownload(d *Download) Download {
	c := *d
	c.Segments = slices.Clone(d.Segments)
	c.Headers = maps.Clone(d.Headers)
	return c
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
