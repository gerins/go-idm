package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"go-idm/internal/engine"
	"go-idm/internal/store"
)

const (
	eventUpdate    = "downloads:update"
	eventRemoved   = "downloads:removed"
	eventClipboard = "clipboard:url"
)

// App is the Wails-bound facade over the engine. Every exported method is
// callable from the frontend.
type App struct {
	ctx context.Context
	st  *store.Store
	mgr *engine.Manager
}

func NewApp() *App { return &App{} }

// dataDir returns where the database lives. GOIDM_DATA_DIR overrides it,
// which is handy for portable installs and for testing.
func dataDir() (string, error) {
	if d := os.Getenv("GOIDM_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "GoIDM"), nil
}

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
	dir, err := dataDir()
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

	go a.watchClipboard(ctx)
	return nil
}

func (a *App) shutdown(context.Context) {
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
func (a *App) ProbeURL(url string) (*engine.ProbeResult, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	return a.mgr.Probe(ctx, url, nil)
}

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
