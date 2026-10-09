package engine

import (
	"testing"
	"time"
)

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/downloads/page?id=3#top": "https://example.com/downloads/page?id=3#top",
		"http://example.com/":                         "http://example.com/",
		"":                                            "",
		"javascript:alert(1)":                         "",
		"file:///etc/passwd":                          "",
		"chrome://extensions":                         "",
		"https:///nohost":                             "",
		"not a url":                                   "",
	} {
		if got := webURL(in); got != want {
			t.Errorf("webURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPageURLFallsBackToReferer(t *testing.T) {
	d := &Download{Headers: map[string]string{"Referer": "https://example.com/get"}}
	if got := pageURL(d); got != "https://example.com/get" {
		t.Errorf("fallback = %q", got)
	}
	d.PageURL = "https://example.com/real-page"
	if got := pageURL(d); got != d.PageURL {
		t.Errorf("explicit page = %q", got)
	}
	if got := pageURL(&Download{}); got != "" {
		t.Errorf("empty = %q", got)
	}
}

// The page link survives a restart so the download page can be reopened later.
func TestPageURLPersists(t *testing.T) {
	srv := newTestServer(t, randomData(1<<20), 15*time.Millisecond)
	st, dir := newMemStore(), t.TempDir()

	m1, _, _ := newTestManagerWith(t, st, dir, nil)
	page := "https://example.com/software/downloads"
	info := mustAdd(t, m1, AddRequest{URL: srv.URL + "/a.bin", PageURL: page, StartPaused: true})
	bad := mustAdd(t, m1, AddRequest{URL: srv.URL + "/b.bin", PageURL: "javascript:alert(1)", StartPaused: true})
	if info.PageURL != page || bad.PageURL != "" {
		t.Fatalf("page urls = %q, %q", info.PageURL, bad.PageURL)
	}
	m1.Close()

	m2, _, _ := newTestManagerWith(t, st, dir, nil)
	got, ok := m2.Get(info.ID)
	if !ok || got.PageURL != page {
		t.Fatalf("after restart: %+v (ok=%v)", got, ok)
	}
}
