// Command idm-cli downloads URLs with the GoIDM engine, headless.
//
//	idm-cli [-o dir] [-c connections] [-limit KiB/s] URL...
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"go-idm/internal/engine"
)

// nopStore keeps the CLI stateless.
type nopStore struct{}

func (nopStore) Save(*engine.Download) error       { return nil }
func (nopStore) Delete(string) error               { return nil }
func (nopStore) List() ([]*engine.Download, error) { return nil, nil }

func main() {
	dir := flag.String("o", ".", "output directory")
	conns := flag.Int("c", 8, "connections per download")
	limit := flag.Int64("limit", 0, "global speed limit in KiB/s (0 = unlimited)")
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	cfg := engine.DefaultConfig()
	cfg.DownloadDir = *dir
	cfg.Categorize = false
	cfg.Connections = *conns
	cfg.SpeedLimit = *limit << 10
	cfg.MaxActive = flag.NArg()

	var (
		mu     sync.Mutex
		latest = map[string]engine.Info{}
	)
	m, err := engine.NewManager(nopStore{}, cfg, engine.Hooks{
		OnUpdate: func(in []engine.Info) {
			mu.Lock()
			for _, i := range in {
				latest[i.ID] = i
			}
			mu.Unlock()
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m.Start()

	ids := make([]string, 0, flag.NArg())
	for _, u := range flag.Args() {
		info, err := m.Add(engine.AddRequest{URL: u})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", u, err)
			os.Exit(1)
		}
		ids = append(ids, info.ID)
	}

	intr := make(chan os.Signal, 1)
	signal.Notify(intr, os.Interrupt)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	exit := 0
	for {
		select {
		case <-intr:
			fmt.Println("\ninterrupted; progress is discarded in CLI mode")
			m.Close()
			os.Exit(130)
		case <-tick.C:
		}

		mu.Lock()
		pending := 0
		for _, id := range ids {
			i := latest[id]
			fmt.Printf("\r%-40.40s %s %6.1f%% %10s/s", i.FileName, i.Status, pct(i), human(i.Speed))
			switch i.Status {
			case engine.StatusCompleted:
			case engine.StatusFailed:
				exit = 1
			default:
				pending++
			}
		}
		mu.Unlock()
		if pending == 0 {
			break
		}
	}
	fmt.Println()
	mu.Lock()
	for _, id := range ids {
		i := latest[id]
		if i.Status == engine.StatusFailed {
			fmt.Printf("failed: %s: %s\n", i.URL, i.Error)
		} else {
			fmt.Printf("saved:  %s\n", i.Path)
		}
	}
	mu.Unlock()
	m.Close()
	os.Exit(exit)
}

func pct(i engine.Info) float64 {
	if i.Size <= 0 {
		return 0
	}
	return float64(i.Downloaded) * 100 / float64(i.Size)
}

func human(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f, exp := float64(n), 0
	for f >= unit && exp < 4 {
		f /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", f, "KMGT"[exp-1])
}
