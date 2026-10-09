package store

import (
	"path/filepath"
	"testing"
	"time"

	"go-idm/internal/engine"
)

func TestRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sub", "idm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	d := &engine.Download{
		ID: "a", URL: "https://x/y.zip", Size: 10, CreatedAt: time.Now(),
		Status:   engine.StatusPaused,
		Segments: []engine.Segment{{Start: 0, End: 4, Done: 3}, {Start: 5, End: 9}},
		Headers:  map[string]string{"Cookie": "a=b"},
	}
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	d.Status = engine.StatusCompleted // upsert
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}

	got, err := s.List()
	if err != nil || len(got) != 1 {
		t.Fatalf("List = %v, %v", got, err)
	}
	if got[0].Status != engine.StatusCompleted || got[0].Downloaded() != 3 || got[0].Headers["Cookie"] != "a=b" {
		t.Fatalf("unexpected row: %+v", got[0])
	}

	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.List(); len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}
}

func TestConfig(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "idm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	def := engine.DefaultConfig()
	cfg, err := s.LoadConfig(def)
	if err != nil || cfg != def {
		t.Fatalf("expected defaults, got %+v, %v", cfg, err)
	}
	def.Connections = 16
	if err := s.SaveConfig(def); err != nil {
		t.Fatal(err)
	}
	cfg, err = s.LoadConfig(engine.DefaultConfig())
	if err != nil || cfg.Connections != 16 {
		t.Fatalf("config not persisted: %+v, %v", cfg, err)
	}
}
