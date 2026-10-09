// Package store persists downloads and settings in SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go driver, no CGO

	"go-idm/internal/engine"
)

const schema = `
CREATE TABLE IF NOT EXISTS downloads (
	id         TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL,
	data       TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);`

const configKey = "config"

// Store is a SQLite-backed engine.Store.
type Store struct {
	db *sql.DB
}

var _ engine.Store = (*Store)(nil)

// Open opens or creates the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection keeps writes serialized and per-connection pragmas valid.
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA synchronous = NORMAL`,
		schema,
	} {
		if _, err := db.Exec(q); err != nil {
			db.Close()
			return nil, fmt.Errorf("init sqlite: %w", err)
		}
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(d *engine.Download) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO downloads (id, created_at, data) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		d.ID, d.CreatedAt.UnixNano(), string(data))
	return err
}

func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM downloads WHERE id = ?`, id)
	return err
}

func (s *Store) List() ([]*engine.Download, error) {
	rows, err := s.db.Query(`SELECT data FROM downloads ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*engine.Download
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		d := new(engine.Download)
		if err := json.Unmarshal([]byte(data), d); err != nil {
			return nil, fmt.Errorf("decode download: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// LoadConfig returns the saved config, or def when none has been saved.
func (s *Store) LoadConfig(def engine.Config) (engine.Config, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, configKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	if err != nil {
		return def, err
	}
	cfg := def // fields missing from older saves keep their defaults
	if err := json.Unmarshal([]byte(v), &cfg); err != nil {
		return def, fmt.Errorf("decode config: %w", err)
	}
	return cfg, nil
}

// SaveConfig persists cfg.
func (s *Store) SaveConfig(cfg engine.Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		configKey, string(data))
	return err
}
