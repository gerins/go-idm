package engine

import "net/url"

// webURL returns raw if it is an http(s) URL, else "". Page links are opened
// in the user's browser, so nothing else (file:, javascript:, ...) is kept.
func webURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

// pageURL is the page a download was started from. Downloads saved before the
// page was recorded fall back to the Referer the browser sent.
func pageURL(d *Download) string {
	if d.PageURL != "" {
		return d.PageURL
	}
	return webURL(d.Headers["Referer"])
}
