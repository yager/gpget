package plan

import (
	"regexp"
	"strconv"
	"strings"
)

// GoProName is the decomposition of an 8-char GoPro media filename PPNNNNNN.EXT:
//
//	PP       two-letter prefix  (GX=HEVC, GH=AVC, GL=LRV proxy, GS=360)
//	first 2  chapter number     (01, 02, ...)
//	last 4   clip number        (shared across chapters of one recording)
type GoProName struct {
	Prefix  string
	Chapter int
	Clip    string // 4 digits, as-is
	Ext     string // upper, no dot (e.g. "MP4")
}

var reGoPro8 = regexp.MustCompile(`^([A-Za-z]{2})(\d{2})(\d{4})$`)

// ParseGoProName decomposes "GX012495.MP4". ok is false for names that don't
// fit the 8-digit scheme (single photos GP0100xx.JPG parse fine too: chapter 01,
// clip 00xx — callers decide whether chapter semantics apply).
func ParseGoProName(name string) (g GoProName, ok bool) {
	stem := name
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		stem, g.Ext = name[:i], strings.ToUpper(name[i+1:])
	}
	m := reGoPro8.FindStringSubmatch(stem)
	if m == nil {
		return GoProName{}, false
	}
	g.Prefix = strings.ToUpper(m[1])
	g.Chapter, _ = strconv.Atoi(m[2])
	g.Clip = m[3]
	return g, true
}

// LRVSourceName returns the camera-side filename of the .LRV proxy that pairs
// with an MP4 (GX012495.MP4 -> GL012495.LRV). ok is false if name isn't a
// GX/GH video.
func LRVSourceName(mp4 string) (string, bool) {
	g, ok := ParseGoProName(mp4)
	if !ok || (g.Prefix != "GX" && g.Prefix != "GH") {
		return "", false
	}
	return "GL" + pad2(g.Chapter) + g.Clip + ".LRV", true
}

// GPRSourceName returns the camera-side filename of the .GPR raw that pairs with
// a JPG (GP010009.JPG -> GP010009.GPR).
func GPRSourceName(jpg string) (string, bool) {
	u := strings.ToUpper(jpg)
	if !strings.HasSuffix(u, ".JPG") && !strings.HasSuffix(u, ".JPEG") {
		return "", false
	}
	i := strings.LastIndexByte(jpg, '.')
	return jpg[:i] + ".GPR", true
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// Stem returns a filename without its extension.
func Stem(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[:i]
	}
	return name
}

// Ext returns the upper-case extension without the dot ("MP4"), or "".
func Ext(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return strings.ToUpper(name[i+1:])
	}
	return ""
}
