// Package netx holds HTTP client construction and response helpers shared by
// the download engine.
package netx

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewClient returns an HTTP client tuned for large, long-lived downloads.
// proxy may be empty (use environment settings) or an http, https, socks5 or
// socks5h URL.
func NewClient(proxy string) (*http.Client, error) {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   32,
		// Transparent gzip would break byte-range accounting.
		DisableCompression: true,
		// A non-nil empty map disables HTTP/2 so that every segment gets its
		// own TCP connection instead of being multiplexed over one.
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}

	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
		}
		if u.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL: missing host")
		}
		tr.Proxy = http.ProxyURL(u)
	}

	return &http.Client{Transport: tr}, nil
}
