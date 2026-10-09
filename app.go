package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"go-idm/internal/appdir"
	"go-idm/internal/engine"
	"go-idm/internal/ipc"
	"go-idm/internal/store"
)

const appVersion = "0.1.0"

const (
	eventUpdate    = "downloads:update"
	eventRemoved   = "downloads:removed"
	eventClipboard = "clipboard:url"
	eventExternal  = "external:add"
)

// App is the Wails-bound facade over the engine. Every exported method is
// callable from the frontend.
type App struct {
	ctx context.Context
	st  *store.Store
	mgr *engine.Manager
	ipc *ipc.Server

	// Captured downloads that arrive before the UI has loaded (the browser
	// extension can start the app) are held here until it asks for them.
	extMu   sync.Mutex
	uiReady bool
	pending []ExternalAdd
}

// ExternalAdd is a download captured by the browser extension, offered to the
// user in the Add dialog.
type ExternalAdd struct {
	URL      string            `json:"url"`
	FileName string            `json:"fileName"`
	Headers  map[string]string `json:"headers"`
	PageURL  string            `json:"pageUrl"`
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.init(ctx); err != nil {
		slog.Error("startup failed", "err", err)
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "GoIDM cannot start",
			Message: err.Error(),
		})
		runtime.Quit(ctx)
	}
}

func (a *App) init(ctx context.Context) error {
	dir, err := appdir.Dir()
	if err != nil {
		return fmt.Errorf("locate data directory: %w", err)
	}
	st, err := store.Open(filepath.Join(dir, "goidm.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	cfg, err := st.LoadConfig(engine.DefaultConfig())
	if err != nil {
		slog.Warn("could not load saved settings, using defaults", "err", err)
	}
	mgr, err := engine.NewManager(st, cfg, engine.Hooks{
		OnUpdate: func(in []engine.Info) { runtime.EventsEmit(ctx, eventUpdate, in) },
		OnRemove: func(id string) { runtime.EventsEmit(ctx, eventRemoved, id) },
	})
	if err != nil {
		// A bad saved proxy must not brick the app.
		slog.Warn("saved settings rejected, using defaults", "err", err)
		cfg = engine.DefaultConfig()
		if mgr, err = engine.NewManager(st, cfg, engine.Hooks{
			OnUpdate: func(in []engine.Info) { runtime.EventsEmit(ctx, eventUpdate, in) },
			OnRemove: func(id string) { runtime.EventsEmit(ctx, eventRemoved, id) },
		}); err != nil {
			return err
		}
	}
	if err := mgr.Load(); err != nil {
		return fmt.Errorf("load downloads: %w", err)
	}
	mgr.Start()
	a.st, a.mgr = st, mgr

	// Browser integration is optional: the app works without it.
	a.ipc = ipc.NewServer(dir, appVersion, ipcHandler{a})
	if err := a.ipc.Start(); err != nil {
		slog.Warn("browser integration unavailable", "err", err)
		a.ipc = nil
	}

	go a.watchClipboard(ctx)
	return nil
}

func (a *App) shutdown(context.Context) {
	if a.ipc != nil {
		a.ipc.Close()
	}
	if a.mgr != nil {
		a.mgr.Close()
	}
	if a.st != nil {
		_ = a.st.Close()
	}
}

// watchClipboard emits eventClipboard when the user copies a file-like URL.
// Whatever is on the clipboard at launch is ignored.
func (a *App) watchClipboard(ctx context.Context) {
	last, _ := runtime.ClipboardGetText(ctx)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !a.mgr.Config().WatchClipboard {
			continue
		}
		text, err := runtime.ClipboardGetText(ctx)
		if err != nil || text == last {
			continue
		}
		last = text
		if u := downloadableURL(text); u != "" {
			runtime.EventsEmit(ctx, eventClipboard, u)
		}
	}
}

// ListDownloads returns every download in queue order.
func (a *App) ListDownloads() []engine.Info { return a.mgr.List() }

// AddDownload queues a new download.
func (a *App) AddDownload(req engine.AddRequest) (engine.Info, error) { return a.mgr.Add(req) }

// ProbeURL fetches size, resumability and file name for the Add dialog.
// headers carries browser context (Referer, Cookie, ...) the user supplied.
func (a *App) ProbeURL(url string, headers map[string]string) (*engine.ProbeResult, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.mgr.Probe(ctx, url, headers)
}

// MoveDownload puts a download just before beforeID in the queue, or last when
// beforeID is empty. Waiting downloads start in this order.
func (a *App) MoveDownload(id, beforeID string) error { return a.mgr.Move(id, beforeID) }

func (a *App) PauseDownload(id string) error  { return a.mgr.Pause(id) }
func (a *App) ResumeDownload(id string) error { return a.mgr.Resume(id) }
func (a *App) PauseAll()                      { a.mgr.PauseAll() }
func (a *App) ResumeAll()                     { a.mgr.ResumeAll() }

// RemoveDownload forgets a download and optionally deletes its files.
func (a *App) RemoveDownload(id string, deleteFiles bool) error {
	return a.mgr.Remove(id, deleteFiles)
}

// OpenFile opens a completed download with the default application.
func (a *App) OpenFile(id string) error {
	info, ok := a.mgr.Get(id)
	if !ok {
		return engine.ErrNotFound
	}
	if info.Status != engine.StatusCompleted {
		return fmt.Errorf("download is not complete")
	}
	return openPath(info.Path)
}

// ShowInFolder reveals the download's file, or its folder if the file is gone.
func (a *App) ShowInFolder(id string) error {
	info, ok := a.mgr.Get(id)
	if !ok {
		return engine.ErrNotFound
	}
	if info.Path == "" {
		return fmt.Errorf("download has no location yet")
	}
	return revealPath(info.Path)
}

// OpenPage opens the web page a download was started from in the default browser.
func (a *App) OpenPage(id string) error {
	info, ok := a.mgr.Get(id)
	if !ok {
		return engine.ErrNotFound
	}
	if info.PageURL == "" {
		return fmt.Errorf("this download has no page link")
	}
	runtime.BrowserOpenURL(a.ctx, info.PageURL)
	return nil
}

func (a *App) GetConfig() engine.Config { return a.mgr.Config() }

// SaveConfig validates, applies and persists settings, returning the
// normalized result.
func (a *App) SaveConfig(cfg engine.Config) (engine.Config, error) {
	if err := a.mgr.SetConfig(cfg); err != nil {
		return a.mgr.Config(), err
	}
	applied := a.mgr.Config()
	if err := a.st.SaveConfig(applied); err != nil {
		return applied, fmt.Errorf("settings applied but not saved: %w", err)
	}
	return applied, nil
}

// ChooseFolder shows a native folder picker. It returns "" if cancelled.
func (a *App) ChooseFolder(current string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose download folder",
		DefaultDirectory:     current,
		CanCreateDirectories: true,
	})
}

// FrontendReady is called once by the UI after it has subscribed to events.
// It returns downloads captured while the UI was still loading.
func (a *App) FrontendReady() []ExternalAdd {
	a.extMu.Lock()
	defer a.extMu.Unlock()
	a.uiReady = true
	out := a.pending
	a.pending = nil
	return out
}

func (a *App) deliverExternal(e ExternalAdd) {
	a.extMu.Lock()
	if !a.uiReady {
		a.pending = append(a.pending, e)
		a.extMu.Unlock()
		return
	}
	a.extMu.Unlock()
	runtime.EventsEmit(a.ctx, eventExternal, e)
}

func (a *App) bringToFront() {
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
	// Windows refuses to steal focus; a brief always-on-top toggle raises the window.
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

// ipcHandler adapts the App to the browser-facing IPC without exposing these
// methods to the frontend bindings.
type ipcHandler struct{ a *App }

func (h ipcHandler) Add(req ipc.AddRequest) error {
	if err := engine.CheckURL(req.URL); err != nil {
		return err
	}
	if h.a.mgr.Config().ConfirmCaptured {
		h.a.deliverExternal(ExternalAdd{URL: req.URL, FileName: req.FileName, Headers: req.Headers(), PageURL: req.PageURL})
		h.a.bringToFront()
		return nil
	}
	_, err := h.a.mgr.Add(engine.AddRequest{URL: req.URL, FileName: req.FileName, Headers: req.Headers(), PageURL: req.PageURL})
	return err
}

func (h ipcHandler) Show() error {
	h.a.bringToFront()
	return nil
}
