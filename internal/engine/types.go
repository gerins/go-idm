// Package engine implements the segmented, resumable download engine.
package engine

import (
	"errors"
	"path/filepath"
	"time"
)

// Status is the lifecycle state of a download.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Segment is one contiguous byte range of the target file. End is inclusive
// and is -1 when the total size is unknown.
type Segment struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
}

// Download is the persisted state of a single download.
type Download struct {
	ID           string            `json:"id"`
	URL          string            `json:"url"`
	FileName     string            `json:"fileName"`
	Dir          string            `json:"dir"`
	Category     string            `json:"category"`
	Claimed      bool              `json:"claimed"` // Dir and FileName are final and reserved
	Size         int64             `json:"size"`    // -1 when unknown
	Resumable    bool              `json:"resumable"`
	ETag         string            `json:"etag"`
	LastModified string            `json:"lastModified"`
	Connections  int               `json:"connections"`
	SpeedLimit   int64             `json:"speedLimit"` // bytes/s, 0 = unlimited
	Headers      map[string]string `json:"headers,omitempty"`
	Segments     []Segment         `json:"segments,omitempty"`
	Status       Status            `json:"status"`
	Error        string            `json:"error"`
	CreatedAt    time.Time         `json:"createdAt"`
	CompletedAt  time.Time         `json:"completedAt"`
}

// Downloaded sums the bytes written across all segments.
func (d *Download) Downloaded() int64 {
	var n int64
	for _, s := range d.Segments {
		n += s.Done
	}
	return n
}

func (d *Download) finalPath() string { return filepath.Join(d.Dir, d.FileName) }
func (d *Download) partPath() string  { return d.finalPath() + partSuffix }

const partSuffix = ".part"

// SegmentInfo is the UI view of a Segment.
type SegmentInfo struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
}

// Info is the read-only, UI-facing snapshot of a download, including live
// speed and ETA.
type Info struct {
	ID          string        `json:"id"`
	URL         string        `json:"url"`
	FileName    string        `json:"fileName"`
	Dir         string        `json:"dir"`
	Path        string        `json:"path"`
	Category    string        `json:"category"`
	Size        int64         `json:"size"`
	Downloaded  int64         `json:"downloaded"`
	Speed       int64         `json:"speed"` // bytes/s
	ETA         int64         `json:"eta"`   // seconds, -1 unknown
	Status      Status        `json:"status"`
	Error       string        `json:"error"`
	Resumable   bool          `json:"resumable"`
	Connections int           `json:"connections"`
	SpeedLimit  int64         `json:"speedLimit"`
	Segments    []SegmentInfo `json:"segments"`
	CreatedAt   time.Time     `json:"createdAt"`
	CompletedAt time.Time     `json:"completedAt"`
}

// AddRequest describes a new download.
type AddRequest struct {
	URL         string            `json:"url"`
	Dir         string            `json:"dir"` // empty = default directory
	FileName    string            `json:"fileName"`
	Connections int               `json:"connections"` // 0 = default
	SpeedLimit  int64             `json:"speedLimit"`
	Headers     map[string]string `json:"headers"`
	StartPaused bool              `json:"startPaused"`
}

// Hooks receive engine notifications. Calls are serialized and ordered.
// Either field may be nil.
type Hooks struct {
	OnUpdate func([]Info)
	OnRemove func(id string)
}

var (
	// ErrNotFound is returned for unknown download IDs.
	ErrNotFound = errors.New("download not found")
	// ErrClosed is returned after the manager has been closed.
	ErrClosed = errors.New("manager closed")

	errPaused   = errors.New("paused")
	errRemoved  = errors.New("removed")
	errShutdown = errors.New("shutdown")
)
