package engine

import (
	"os"
	"path/filepath"
)

// Config holds user-tunable engine settings.
type Config struct {
	DownloadDir    string `json:"downloadDir"`
	MaxActive      int    `json:"maxActive"`
	Connections    int    `json:"connections"`
	SpeedLimit     int64  `json:"speedLimit"` // global, bytes/s, 0 = unlimited
	Proxy          string `json:"proxy"`
	UserAgent      string `json:"userAgent"`
	Categorize     bool   `json:"categorize"`
	WatchClipboard bool   `json:"watchClipboard"`
	MaxRetries     int    `json:"maxRetries"`
}

// DefaultConfig returns sensible defaults, using ~/Downloads as the target.
func DefaultConfig() Config {
	dir := "."
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, "Downloads")
	}
	return Config{
		DownloadDir:    dir,
		MaxActive:      3,
		Connections:    8,
		UserAgent:      "Mozilla/5.0 (compatible; GoIDM/0.1)",
		Categorize:     true,
		WatchClipboard: true,
		MaxRetries:     5,
	}
}

func (c Config) normalized() Config {
	def := DefaultConfig()
	if c.DownloadDir == "" {
		c.DownloadDir = def.DownloadDir
	}
	if c.UserAgent == "" {
		c.UserAgent = def.UserAgent
	}
	c.MaxActive = clamp(c.MaxActive, 1, 10)
	c.Connections = clamp(c.Connections, 1, 32)
	c.MaxRetries = clamp(c.MaxRetries, 0, 20)
	if c.SpeedLimit < 0 {
		c.SpeedLimit = 0
	}
	return c
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}
