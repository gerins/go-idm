// Package ipc is the local channel between the native messaging host (which
// the browser launches) and the running GoIDM app.
//
// The app listens on a random loopback port and publishes the port and a
// secret token in a 0600 file in the data directory. Only processes that can
// read that file can talk to the app, so web pages cannot.
package ipc

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	infoFile    = "ipc.json"
	maxBodySize = 1 << 20
)

// ErrNotRunning means no app is listening (missing or stale info file).
var ErrNotRunning = errors.New("GoIDM is not running")

// Info is what the app publishes for clients.
type Info struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// AddRequest describes a download captured by the browser.
type AddRequest struct {
	URL       string `json:"url"`
	FileName  string `json:"fileName,omitempty"`
	Referrer  string `json:"referrer,omitempty"`
	PageURL   string `json:"pageUrl,omitempty"` // the page the download was started from
	Cookies   string `json:"cookies,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	MIME      string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// Headers returns the HTTP request headers that reproduce the browser's
// context for this download.
func (r AddRequest) Headers() map[string]string {
	h := map[string]string{}
	if r.Referrer != "" {
		h["Referer"] = r.Referrer
	}
	if r.Cookies != "" {
		h["Cookie"] = r.Cookies
	}
	if r.UserAgent != "" {
		h["User-Agent"] = r.UserAgent
	}
	return h
}

// PingResponse identifies the app.
type PingResponse struct {
	OK      bool   `json:"ok"`
	App     string `json:"app"`
	Version string `json:"version"`
}

// Handler is implemented by the app.
type Handler interface {
	Add(AddRequest) error
	Show() error
}

// Server exposes a Handler on loopback.
type Server struct {
	dir     string
	version string
	h       Handler
	token   string
	srv     *http.Server
	ln      net.Listener
}

func NewServer(dir, version string, h Handler) *Server {
	return &Server{dir: dir, version: version, h: h}
}

// Start begins listening and publishes the connection info.
func (s *Server) Start() error {
	var tok [32]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return err
	}
	s.token = hex.EncodeToString(tok[:])

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.ln = ln
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/ping", s.ping)
	mux.HandleFunc("POST /v1/add", s.add)
	mux.HandleFunc("POST /v1/show", s.show)
	s.srv = &http.Server{
		Handler:           s.guard(mux, port),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
	}

	if err := writeInfo(s.dir, Info{Port: port, Token: s.token, PID: os.Getpid()}); err != nil {
		ln.Close()
		return err
	}
	go func() { _ = s.srv.Serve(ln) }()
	return nil
}

// Close stops the server and withdraws the info file if it is still ours.
func (s *Server) Close() {
	if s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
	if info, err := readInfo(s.dir); err == nil && info.Token == s.token {
		_ = os.Remove(filepath.Join(s.dir, infoFile))
	}
}

// guard rejects anything that does not look like our own host process:
// wrong Host header (DNS rebinding), any Origin header (browsers always send
// one on cross-site requests), or a bad token.
func (s *Server) guard(next http.Handler, port int) http.Handler {
	p := strconv.Itoa(port)
	okHosts := map[string]bool{"127.0.0.1:" + p: true, "localhost:" + p: true}
	want := []byte("Bearer " + s.token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !okHosts[r.Host] || r.Header.Get("Origin") != "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) ping(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, PingResponse{OK: true, App: "goidm", Version: s.version})
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	var req AddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if err := s.h.Add(req); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) show(w http.ResponseWriter, _ *http.Request) {
	if err := s.h.Show(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeInfo(dir string, info Info) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	// CreateTemp makes the file 0600 on Unix; on Windows the per-user profile
	// directory ACL provides the equivalent protection.
	tmp, err := os.CreateTemp(dir, "ipc-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, infoFile))
}

func readInfo(dir string) (Info, error) {
	data, err := os.ReadFile(filepath.Join(dir, infoFile))
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, err
	}
	if info.Port == 0 || info.Token == "" {
		return Info{}, errors.New("malformed ipc info")
	}
	return info, nil
}

// Client talks to a running app.
type Client struct {
	dir  string
	http *http.Client
}

func NewClient(dir string) *Client {
	return &Client{dir: dir, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	info, err := readInfo(c.dir)
	if err != nil {
		return ErrNotRunning
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:"+strconv.Itoa(info.Port)+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Connection refused: the info file is stale.
		return ErrNotRunning
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("app returned %d", resp.StatusCode)
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) (PingResponse, error) {
	var p PingResponse
	err := c.do(ctx, http.MethodGet, "/v1/ping", nil, &p)
	return p, err
}

func (c *Client) Add(ctx context.Context, req AddRequest) error {
	return c.do(ctx, http.MethodPost, "/v1/add", req, nil)
}

func (c *Client) Show(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/v1/show", struct{}{}, nil)
}
