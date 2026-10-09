package netx

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFileName(t *testing.T) {
	u := func(s string) *url.URL { x, _ := url.Parse(s); return x }
	cases := []struct {
		name string
		h    http.Header
		url  string
		want string
	}{
		{"disposition", http.Header{"Content-Disposition": {`attachment; filename="a b.zip"`}}, "http://x/y", "a b.zip"},
		{"rfc5987", http.Header{"Content-Disposition": {`attachment; filename*=UTF-8''na%C3%AFve%20file.txt`}}, "http://x/y", "naïve file.txt"},
		{"unquoted with spaces", http.Header{"Content-Disposition": {`attachment; filename=my file.pdf`}}, "http://x/y", "my file.pdf"},
		{"url path", http.Header{}, "http://x/dir/some%20file.iso?token=1", "some file.iso"},
		{"traversal in disposition", http.Header{"Content-Disposition": {`attachment; filename="../../etc/passwd"`}}, "http://x/", "passwd"},
		{"windows traversal", http.Header{"Content-Disposition": {`attachment; filename="..\\..\\evil.exe"`}}, "http://x/", "evil.exe"},
		{"content type fallback", http.Header{"Content-Type": {"image/png"}}, "http://x/", "download.png"},
		{"plain fallback", http.Header{}, "http://x/", "download"},
	}
	for _, c := range cases {
		if got := FileName(c.h, u(c.url)); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		`a<b>c:d"e|f?g*h.txt`: "a_b_c_d_e_f_g_h.txt",
		"trailing. . ":        "trailing",
		"CON":                 "_CON",
		"nul.txt":             "_nul.txt",
		"..":                  "",
		"":                    "",
		"ctl\x01char":         "ctl_char",
	}
	for in, want := range cases {
		if got := SanitizeFileName(in); got != want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}

	long := strings.Repeat("é", 300) + ".mp4"
	got := SanitizeFileName(long)
	if len(got) > maxNameBytes || !strings.HasSuffix(got, ".mp4") {
		t.Errorf("long name not truncated correctly: %d bytes, %q", len(got), got[len(got)-8:])
	}
}

func TestUniqueName(t *testing.T) {
	taken := map[string]bool{"a.txt": true, "a (1).txt": true, "noext": true}
	check := func(n string) bool { return taken[n] }
	if got := UniqueName("a.txt", check); got != "a (2).txt" {
		t.Errorf("got %q", got)
	}
	if got := UniqueName("noext", check); got != "noext (1)" {
		t.Errorf("got %q", got)
	}
	if got := UniqueName("free.txt", check); got != "free.txt" {
		t.Errorf("got %q", got)
	}
}

func TestNewClientProxyValidation(t *testing.T) {
	for _, ok := range []string{"", "http://127.0.0.1:8080", "socks5://127.0.0.1:1080"} {
		if _, err := NewClient(ok); err != nil {
			t.Errorf("NewClient(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"ftp://x", "http://", "://"} {
		if _, err := NewClient(bad); err == nil {
			t.Errorf("NewClient(%q) should fail", bad)
		}
	}
}
