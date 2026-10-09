package netx

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxNameBytes = 200

var (
	dispositionRe = regexp.MustCompile(`(?i)filename\s*=\s*"?([^";]+)"?`)
	reservedNames = map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
		"COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
		"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
)

// FileName picks a file name for a response: Content-Disposition first, then
// the last URL path segment, then an extension guessed from Content-Type.
// The result is always safe to use as a single path element on Windows.
func FileName(h http.Header, u *url.URL) string {
	if name := fromDisposition(h.Get("Content-Disposition")); name != "" {
		if s := SanitizeFileName(name); s != "" {
			return s
		}
	}
	if u != nil {
		if s := SanitizeFileName(NameFromURL(u.String())); s != "" {
			return s
		}
	}
	ext := ""
	if ct, _, err := mime.ParseMediaType(h.Get("Content-Type")); err == nil && ct != "application/octet-stream" {
		if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
			ext = exts[0]
		}
	}
	return "download" + ext
}

// NameFromURL returns the unescaped last path segment of rawURL, or "".
func NameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	p := u.Path
	if p == "" || strings.HasSuffix(p, "/") {
		return ""
	}
	return path.Base(p)
}

func fromDisposition(cd string) string {
	if cd == "" {
		return ""
	}
	if _, params, err := mime.ParseMediaType(cd); err == nil {
		if n := params["filename"]; n != "" {
			return n
		}
	}
	if m := dispositionRe.FindStringSubmatch(cd); m != nil {
		if n, err := url.PathUnescape(strings.TrimSpace(m[1])); err == nil {
			return n
		}
		return strings.TrimSpace(m[1])
	}
	return ""
}

// SanitizeFileName reduces name to a single, Windows-safe path element.
// It returns "" if nothing usable remains.
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)

	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f, strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimLeft(strings.TrimRight(b.String(), " ."), " ")
	if s == "" {
		return ""
	}

	s = truncateName(s)

	stem := strings.ToUpper(strings.TrimSuffix(s, path.Ext(s)))
	if reservedNames[stem] {
		s = "_" + s
	}
	return s
}

func truncateName(s string) string {
	if len(s) <= maxNameBytes {
		return s
	}
	ext := path.Ext(s)
	if len(ext) > 20 {
		ext = ""
	}
	stem := strings.TrimSuffix(s, ext)
	for len(stem)+len(ext) > maxNameBytes {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	return stem + ext
}

// UniqueName returns name, or "stem (n).ext" for the first n >= 1 for which
// taken reports false.
func UniqueName(name string, taken func(string) bool) string {
	if !taken(name) {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if !taken(candidate) {
			return candidate
		}
	}
}
