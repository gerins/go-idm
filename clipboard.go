package main

import (
	"net/url"
	"path"
	"strings"
)

// Extensions that mean "a web page", not a file worth downloading.
var pageExts = map[string]bool{
	".html": true, ".htm": true, ".php": true, ".asp": true, ".aspx": true,
	".jsp": true, ".cgi": true, ".xhtml": true, ".shtml": true,
}

// downloadableURL returns the cleaned URL if text is a single http(s) link
// whose path looks like a file, or "" otherwise. Plain page links are ignored
// so that copying an address from the browser bar does not prompt.
func downloadableURL(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 2048 || strings.ContainsAny(text, " \t\r\n") {
		return ""
	}
	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	ext := strings.ToLower(path.Ext(u.Path))
	if ext == "" || len(ext) > 8 || pageExts[ext] {
		return ""
	}
	return u.String()
}
