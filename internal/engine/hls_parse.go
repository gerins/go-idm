package engine

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// HLSVariant is one quality level of a multi-bitrate stream.
type HLSVariant struct {
	URL       string `json:"url"`
	Bandwidth int    `json:"bandwidth"` // bits per second
	Width     int    `json:"width"`
	Height    int    `json:"height"`

	audio string // group of separate audio renditions it plays with, "" if none
}

// hlsRendition is an alternate track (audio) a variant refers to by group.
type hlsRendition struct {
	typ, group, uri string
	isDefault       bool
	autoselect      bool
}

type masterPlaylist struct {
	variants   []HLSVariant // best first
	renditions []hlsRendition
}

// hlsKey describes how the segments that follow it are encrypted.
type hlsKey struct {
	method string // "NONE" or "AES-128"
	uri    string
	iv     []byte // nil: derived from the segment's sequence number
}

type hlsSegment struct {
	uri      string
	duration float64
	key      *hlsKey
	seq      int64
	length   int64 // EXT-X-BYTERANGE length, -1 for the whole resource
	offset   int64 // and where it starts
}

type mediaPlaylist struct {
	segments []hlsSegment
	init     *hlsSegment // EXT-X-MAP: header to put before the segments
	endList  bool
	live     bool // an EVENT playlist that is still being written
}

// parseAttrs splits `KEY=value,KEY2="quoted, value"` into a map.
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for len(s) > 0 {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := strings.TrimSpace(s[:eq])
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				val, s = s[1:], ""
			} else {
				val, s = s[1:1+end], s[end+2:]
			}
		} else {
			end := strings.IndexByte(s, ',')
			if end < 0 {
				val, s = s, ""
			} else {
				val, s = s[:end], s[end:]
			}
		}
		s = strings.TrimPrefix(strings.TrimSpace(s), ",")
		s = strings.TrimSpace(s)
		out[key] = val
	}
	return out
}

func resolve(base *url.URL, ref string) (string, error) {
	u, err := base.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func parseByteRange(s string) (length, offset int64, err error) {
	l, o, hasOff := strings.Cut(s, "@")
	length, err = strconv.ParseInt(l, 10, 64)
	if err != nil || length <= 0 {
		return 0, 0, fmt.Errorf("bad byte range %q", s)
	}
	offset = -1
	if hasOff {
		if offset, err = strconv.ParseInt(o, 10, 64); err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("bad byte range %q", s)
		}
	}
	return length, offset, nil
}

func parseIV(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s)%2 == 1 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) == 0 || len(b) > 16 {
		return nil, fmt.Errorf("bad IV %q", s)
	}
	iv := make([]byte, 16)
	copy(iv[16-len(b):], b)
	return iv, nil
}

// parsePlaylist parses an HLS playlist fetched from base. Exactly one of the
// results is non-nil.
func parsePlaylist(base *url.URL, text string) (*masterPlaylist, *mediaPlaylist, error) {
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "#EXTM3U" {
		return nil, nil, fmt.Errorf("not an HLS playlist")
	}

	master := &masterPlaylist{}
	media := &mediaPlaylist{}
	var (
		pendingVariant *HLSVariant
		duration       float64
		key            *hlsKey
		seq            int64
		length, offset = int64(-1), int64(-1)
		nextOffset     = map[string]int64{} // where the next unranged-offset byte range starts, per resource
		isMaster       bool
		playlistType   string
	)

	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			uri, err := resolve(base, line)
			if err != nil {
				return nil, nil, fmt.Errorf("bad URI %q: %w", line, err)
			}
			if pendingVariant != nil {
				pendingVariant.URL = uri
				master.variants = append(master.variants, *pendingVariant)
				pendingVariant = nil
				continue
			}
			seg := hlsSegment{uri: uri, duration: duration, key: key, seq: seq, length: length, offset: offset}
			if length > 0 && offset < 0 {
				seg.offset = nextOffset[uri]
			}
			if length > 0 {
				nextOffset[uri] = seg.offset + length
			}
			media.segments = append(media.segments, seg)
			seq++
			duration, length, offset = 0, -1, -1
			continue
		}

		tag, val, _ := strings.Cut(line, ":")
		switch tag {
		case "#EXT-X-STREAM-INF":
			isMaster = true
			a := parseAttrs(val)
			v := HLSVariant{audio: a["AUDIO"]}
			v.Bandwidth, _ = strconv.Atoi(a["BANDWIDTH"])
			if w, h, ok := strings.Cut(a["RESOLUTION"], "x"); ok {
				v.Width, _ = strconv.Atoi(w)
				v.Height, _ = strconv.Atoi(h)
			}
			pendingVariant = &v
		case "#EXT-X-MEDIA":
			a := parseAttrs(val)
			if a["URI"] == "" {
				continue // muxed into the variant itself
			}
			uri, err := resolve(base, a["URI"])
			if err != nil {
				continue
			}
			master.renditions = append(master.renditions, hlsRendition{
				typ: a["TYPE"], group: a["GROUP-ID"], uri: uri,
				isDefault: a["DEFAULT"] == "YES", autoselect: a["AUTOSELECT"] == "YES",
			})
		case "#EXTINF":
			d, _, _ := strings.Cut(val, ",")
			duration, _ = strconv.ParseFloat(strings.TrimSpace(d), 64)
		case "#EXT-X-MEDIA-SEQUENCE":
			seq, _ = strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		case "#EXT-X-PLAYLIST-TYPE":
			playlistType = strings.TrimSpace(val)
		case "#EXT-X-ENDLIST":
			media.endList = true
		case "#EXT-X-BYTERANGE":
			var err error
			if length, offset, err = parseByteRange(strings.TrimSpace(val)); err != nil {
				return nil, nil, err
			}
		case "#EXT-X-KEY":
			a := parseAttrs(val)
			k := &hlsKey{method: a["METHOD"]}
			if k.method == "" {
				return nil, nil, fmt.Errorf("EXT-X-KEY without METHOD")
			}
			if k.method == "NONE" {
				key = nil
				continue
			}
			if a["URI"] != "" {
				uri, err := resolve(base, a["URI"])
				if err != nil {
					return nil, nil, fmt.Errorf("bad key URI: %w", err)
				}
				k.uri = uri
			}
			if a["IV"] != "" {
				iv, err := parseIV(a["IV"])
				if err != nil {
					return nil, nil, err
				}
				k.iv = iv
			}
			key = k
		case "#EXT-X-MAP":
			a := parseAttrs(val)
			if a["URI"] == "" {
				continue
			}
			uri, err := resolve(base, a["URI"])
			if err != nil {
				return nil, nil, fmt.Errorf("bad map URI: %w", err)
			}
			m := &hlsSegment{uri: uri, key: key, length: -1, offset: -1}
			if br := a["BYTERANGE"]; br != "" {
				l, o, err := parseByteRange(br)
				if err != nil {
					return nil, nil, err
				}
				m.length, m.offset = l, max(o, 0)
			}
			if media.init != nil && (media.init.uri != m.uri || media.init.offset != m.offset) {
				return nil, nil, fmt.Errorf("streams whose initialization segment changes are not supported")
			}
			media.init = m
		}
	}

	if isMaster {
		if len(master.variants) == 0 {
			return nil, nil, fmt.Errorf("playlist lists no streams")
		}
		sort.SliceStable(master.variants, func(i, j int) bool { return master.variants[i].Bandwidth > master.variants[j].Bandwidth })
		return master, nil, nil
	}
	media.live = !media.endList && playlistType != "VOD"
	return nil, media, nil
}

// pick returns the variant at url, or the best one when url is empty or unknown.
func (m *masterPlaylist) pick(url string) HLSVariant {
	for _, v := range m.variants {
		if url != "" && v.URL == url {
			return v
		}
	}
	return m.variants[0]
}

// audioFor returns the URI of the separate audio track a variant plays with,
// or "" when its audio is inside the variant itself.
func (m *masterPlaylist) audioFor(v HLSVariant) string {
	if v.audio == "" {
		return ""
	}
	best := ""
	for _, r := range m.renditions {
		if r.typ != "AUDIO" || r.group != v.audio {
			continue
		}
		if r.isDefault {
			return r.uri
		}
		if best == "" || r.autoselect {
			best = r.uri
		}
	}
	return best
}
