package engine

import (
	"net/url"
	"strings"
	"testing"
)

func mustParse(t *testing.T, text string) (*masterPlaylist, *mediaPlaylist) {
	t.Helper()
	base, _ := url.Parse("https://cdn.example.com/a/b/index.m3u8")
	m, md, err := parsePlaylist(base, text)
	if err != nil {
		t.Fatal(err)
	}
	return m, md
}

func TestParseAttrs(t *testing.T) {
	got := parseAttrs(`BANDWIDTH=800000,CODECS="avc1.4d401f,mp4a.40.2",RESOLUTION=640x360,URI="a,b.key"`)
	for k, want := range map[string]string{
		"BANDWIDTH": "800000", "CODECS": "avc1.4d401f,mp4a.40.2", "RESOLUTION": "640x360", "URI": "a,b.key",
	} {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}

func TestParseMasterPlaylist(t *testing.T) {
	m, md := mustParse(t, strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="en",DEFAULT=YES,AUTOSELECT=YES,URI="audio/en.m3u8"`,
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="fr",AUTOSELECT=YES,URI="audio/fr.m3u8"`,
		`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="sub",NAME="en",URI="sub/en.m3u8"`,
		`#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360,AUDIO="aud"`,
		"low/index.m3u8",
		`#EXT-X-STREAM-INF:BANDWIDTH=2400000,RESOLUTION=1280x720,AUDIO="aud"`,
		"https://other.example.com/high.m3u8",
		`#EXT-X-STREAM-INF:BANDWIDTH=300000`,
		"muxed.m3u8",
	}, "\n"))
	if md != nil || m == nil {
		t.Fatal("expected a master playlist")
	}
	if len(m.variants) != 3 || m.variants[0].Bandwidth != 2400000 || m.variants[2].Bandwidth != 300000 {
		t.Fatalf("variants not sorted best first: %+v", m.variants)
	}
	if v := m.variants[0]; v.URL != "https://other.example.com/high.m3u8" || v.Width != 1280 || v.Height != 720 {
		t.Errorf("best = %+v", v)
	}
	if v := m.variants[1]; v.URL != "https://cdn.example.com/a/b/low/index.m3u8" {
		t.Errorf("relative URL resolved to %q", v.URL)
	}
	if got := m.audioFor(m.variants[0]); got != "https://cdn.example.com/a/b/audio/en.m3u8" {
		t.Errorf("audio = %q (the DEFAULT rendition should win)", got)
	}
	if got := m.audioFor(m.variants[2]); got != "" {
		t.Errorf("a muxed variant has no separate audio, got %q", got)
	}
	if m.pick("").Bandwidth != 2400000 || m.pick("https://cdn.example.com/a/b/low/index.m3u8").Bandwidth != 800000 || m.pick("nope").Bandwidth != 2400000 {
		t.Error("pick")
	}
}

func TestParseMediaPlaylist(t *testing.T) {
	_, md := mustParse(t, strings.Join([]string{
		"#EXTM3U",
		"#EXT-X-MEDIA-SEQUENCE:7",
		`#EXT-X-MAP:URI="init.mp4"`,
		`#EXT-X-KEY:METHOD=AES-128,URI="k.bin",IV=0x000102030405060708090a0b0c0d0e0f`,
		"#EXTINF:4.5,title",
		"s0.m4s",
		"#EXT-X-KEY:METHOD=NONE",
		"#EXTINF:3,",
		"s1.m4s",
		"#EXT-X-ENDLIST",
	}, "\n"))
	if !md.endList || md.live || len(md.segments) != 2 {
		t.Fatalf("%+v", md)
	}
	if md.init == nil || md.init.uri != "https://cdn.example.com/a/b/init.mp4" {
		t.Errorf("init = %+v", md.init)
	}
	s0, s1 := md.segments[0], md.segments[1]
	if s0.seq != 7 || s1.seq != 8 || s0.duration != 4.5 {
		t.Errorf("seq/duration: %+v %+v", s0, s1)
	}
	if s0.key == nil || s0.key.method != "AES-128" || s0.key.uri != "https://cdn.example.com/a/b/k.bin" || s0.key.iv[15] != 0x0f {
		t.Errorf("key = %+v", s0.key)
	}
	if s1.key != nil {
		t.Error("METHOD=NONE ends encryption")
	}
}

func TestParseByteRanges(t *testing.T) {
	_, md := mustParse(t, strings.Join([]string{
		"#EXTM3U", "#EXT-X-ENDLIST",
		"#EXTINF:2,", "#EXT-X-BYTERANGE:1000@0", "big.ts",
		"#EXTINF:2,", "#EXT-X-BYTERANGE:500", "big.ts", // continues after the previous range
		"#EXTINF:2,", "#EXT-X-BYTERANGE:200@5000", "big.ts",
		"#EXTINF:2,", "plain.ts",
	}, "\n"))
	want := [][2]int64{{0, 1000}, {1000, 500}, {5000, 200}, {-1, -1}}
	for i, w := range want {
		if s := md.segments[i]; s.offset != w[0] || s.length != w[1] {
			t.Errorf("segment %d: offset=%d length=%d, want %v", i, s.offset, s.length, w)
		}
	}
}

func TestParseLiveAndInvalid(t *testing.T) {
	_, live := mustParse(t, "#EXTM3U\n#EXTINF:4,\ns.ts\n")
	if !live.live {
		t.Error("a playlist without ENDLIST is live")
	}
	_, event := mustParse(t, "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXTINF:4,\ns.ts\n")
	if event.live {
		t.Error("VOD is not live")
	}
	base, _ := url.Parse("https://x.example/p.m3u8")
	for _, bad := range []string{"", "hello", "<html></html>", "#EXTM3U\n#EXT-X-KEY:URI=\"k\"\n", "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n"} {
		if _, _, err := parsePlaylist(base, bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestDecryptSegment(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := make([]byte, 16)
	plain := []byte("hello transport stream, longer than one block")
	enc := encryptCBC(t, key, iv, plain)
	got, err := decryptSegment(enc, key, iv)
	if err != nil || string(got) != string(plain) {
		t.Fatalf("got %q, %v", got, err)
	}
	enc2 := encryptCBC(t, key, iv, plain)
	if _, err := decryptSegment(enc2, []byte("wrong key 123456"), iv); err == nil {
		t.Error("a wrong key should be noticed through the padding")
	}
	if _, err := decryptSegment([]byte("short"), key, iv); err == nil {
		t.Error("a partial block should be rejected")
	}
}
