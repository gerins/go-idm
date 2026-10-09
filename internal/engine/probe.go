package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"go-idm/internal/netx"
)

// ProbeResult describes a remote file as seen by a ranged GET.
type ProbeResult struct {
	FinalURL     string `json:"finalUrl"`
	FileName     string `json:"fileName"`
	Size         int64  `json:"size"` // -1 when unknown
	Resumable    bool   `json:"resumable"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified"`
	ContentType  string `json:"contentType"`

	// Set for HLS streams (see hls.go).
	HLS      bool         `json:"hls"`
	Variants []HLSVariant `json:"variants"` // qualities, best first; empty for a single-quality stream
	Duration float64      `json:"duration"` // seconds
}

// HTTPError is returned for unexpected HTTP status codes.
type HTTPError struct {
	Status int
	Detail string // the server's own explanation, when it gave a readable one
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("server returned %d %s", e.Status, http.StatusText(e.Status))
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

// newHTTPError builds an HTTPError and reads a short, human-readable reason
// from the response body (JSON message fields or plain text).
func newHTTPError(resp *http.Response) *HTTPError {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return &HTTPError{Status: resp.StatusCode, Detail: errorDetail(resp.Header.Get("Content-Type"), b)}
}

func errorDetail(contentType string, body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" || !utf8.ValidString(text) {
		return ""
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "just a moment") || strings.Contains(lower, "cf-chl") {
		return "blocked by a browser check; add your browser's Cookie under Advanced"
	}
	if strings.HasPrefix(text, "{") {
		var m map[string]any
		if json.Unmarshal(body, &m) == nil {
			for _, k := range []string{"message", "error", "value", "reason"} {
				if s, ok := m[k].(string); ok && s != "" {
					return clip(s)
				}
			}
		}
		return ""
	}
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(text, "<") || !(strings.HasPrefix(ct, "text/plain") || ct == "") {
		return "" // an HTML error page is not useful to show
	}
	return clip(text)
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

func (e *HTTPError) retryable() bool {
	return e.Status >= 500 || e.Status == http.StatusTooManyRequests || e.Status == http.StatusRequestTimeout
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q (only http and https)", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid URL: missing host")
	}
	return u, nil
}

func newRequest(ctx context.Context, rawURL string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Byte ranges only make sense on the identity encoding.
	req.Header.Set("Accept-Encoding", "identity")
	// Browsers send a Referer, and some hosts reject requests without a
	// same-site one. Default to the site's own origin; callers can override.
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", req.URL.Scheme+"://"+req.URL.Host+"/")
	}
	return req, nil
}

// Probe issues a GET for the first byte and reports size, range support and
// the suggested file name. A ranged GET is used instead of HEAD because many
// servers reject or mishandle HEAD.
func Probe(ctx context.Context, c *http.Client, rawURL string, headers map[string]string) (*ProbeResult, error) {
	if _, err := parseURL(rawURL); err != nil {
		return nil, &fatalError{err}
	}
	req, err := newRequest(ctx, rawURL, headers)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res := &ProbeResult{
		FinalURL:     resp.Request.URL.String(),
		FileName:     netx.FileName(resp.Header, resp.Request.URL),
		Size:         -1,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		ContentType:  resp.Header.Get("Content-Type"),
	}

	switch resp.StatusCode {
	case http.StatusPartialContent:
		if _, _, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok && total >= 0 {
			res.Size = total
			res.Resumable = true
		}
	case http.StatusOK:
		// Range was ignored; the whole body would follow.
		res.Size = resp.ContentLength
	case http.StatusRequestedRangeNotSatisfiable:
		// Empty file: "bytes */0".
		if _, _, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok && total >= 0 {
			res.Size = total
			res.Resumable = true
		}
	default:
		return nil, newHTTPError(resp)
	}
	return res, nil
}

// parseContentRange parses "bytes start-end/total" and "bytes */total".
// total is -1 for "*".
func parseContentRange(v string) (start, end, total int64, ok bool) {
	v = strings.TrimSpace(v)
	rest, found := strings.CutPrefix(v, "bytes ")
	if !found {
		return 0, 0, 0, false
	}
	rng, tot, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, 0, false
	}
	total = -1
	if tot != "*" {
		n, err := strconv.ParseInt(tot, 10, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		total = n
	}
	if rng == "*" {
		return 0, 0, total, true
	}
	s, e, found := strings.Cut(rng, "-")
	if !found {
		return 0, 0, 0, false
	}
	start, err1 := strconv.ParseInt(s, 10, 64)
	end, err2 := strconv.ParseInt(e, 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, 0, false
	}
	return start, end, total, true
}
