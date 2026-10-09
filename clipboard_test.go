package main

import "testing"

func TestDownloadableURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/files/a.zip":        "https://example.com/files/a.zip",
		"  http://h/x.ISO?token=1  ":             "http://h/x.ISO?token=1",
		"https://example.com/":                   "",
		"https://example.com/page.html":          "",
		"https://example.com/about":              "",
		"ftp://example.com/a.zip":                "",
		"hello world a.zip":                      "",
		"https://example.com/a.zip\nsecond line": "",
		"":                                       "",
		"javascript:alert(1)//a.zip":             "",
	}
	for in, want := range cases {
		if got := downloadableURL(in); got != want {
			t.Errorf("downloadableURL(%q) = %q, want %q", in, got, want)
		}
	}
}
