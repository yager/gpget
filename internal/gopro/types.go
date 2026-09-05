// Package gopro is a small client for the GoPro Open GoPro local HTTP API.
//
// Verified against a MISSION 1 PRO ILS (FW H26.03.03.00.00). See docs/gopro-api.md
// for the field semantics and the ILS-specific quirks this package relies on.
package gopro

import (
	"strconv"
	"strings"
)

// CameraInfo is the response of GET /gopro/camera/info.
type CameraInfo struct {
	ModelNumber     string `json:"model_number"`
	ModelName       string `json:"model_name"`
	FirmwareVersion string `json:"firmware_version"`
	SerialNumber    string `json:"serial_number"`
	APMACAddr       string `json:"ap_mac_addr"`
	APSSID          string `json:"ap_ssid"`
}

// MediaList is the response of GET /gopro/media/list.
type MediaList struct {
	ID    string         `json:"id"`
	Media []MediaListDir `json:"media"`
}

// MediaListDir is one DCIM directory ("100GOPRO") within a media list.
type MediaListDir struct {
	Dir string      `json:"d"`
	FS  []MediaItem `json:"fs"`
}

// MediaItem is one entry in a media list directory. Fields are stored as
// strings; UnmarshalJSON accepts JSON strings, numbers, bools, or null so a
// firmware that stops quoting numbers does not break media/list entirely.
//
// Sidecars (.GPR for photos with Raw=="1", .LRV for videos with GLRV!="") are
// NOT separate entries; they are fetched by transforming the name.
type MediaItem struct {
	Name string `json:"n"`
	Size string `json:"s"`
	Cre  string `json:"cre"`
	Mod  string `json:"mod"`

	Raw  string `json:"raw"`  // "1" => a .GPR sidecar exists
	GLRV string `json:"glrv"` // size of the .LRV proxy, if any
	LS   string `json:"ls"`   // MP4 attribute, "-1" == normal

	// Group fields (burst / timelapse / interval). When Group != "" this entry
	// represents a whole group; Begin..Last are frame numbers (== filename tail),
	// Missing lists deleted/absent frame numbers (NOT zero-padded), and Size is
	// the summed byte count of the *present* frames (not a file count).
	Group   string   `json:"g"`
	Begin   string   `json:"b"`
	Last    string   `json:"l"`
	TypeTag string   `json:"t"` // always "b" on ILS; do not use for classification
	Missing []string `json:"m"`
}

// MediaInfo is the (partial) response of GET /gopro/media/info?path=d/n.
// Only the fields gpget uses. All values arrive as JSON strings.
// NOTE: for videos, Cre here is UTC (differs from media/list) and is sometimes a
// broken value ("3"); never use it for dates.
type MediaInfo struct {
	Cre string `json:"cre"`
	Siz string `json:"s"`
	CT  string `json:"ct"` // capture-type hint; undocumented, labels only
	W   string `json:"w"`
	H   string `json:"h"`
}

// CTInt returns CT parsed as an int, or -1.
func (mi MediaInfo) CTInt() int {
	n, err := strconv.Atoi(strings.TrimSpace(mi.CT))
	if err != nil {
		return -1
	}
	return n
}

// Version is GET /gopro/version.
type Version struct {
	Version string `json:"version"`
}

// SizeBytes returns Size parsed as int64, or -1 if unparseable.
func (m MediaItem) SizeBytes() int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(m.Size), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// CreUnix returns Cre parsed as a unix timestamp, or 0 if unparseable.
func (m MediaItem) CreUnix() int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(m.Cre), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// IsGroup reports whether this entry represents a burst/timelapse/interval group.
func (m MediaItem) IsGroup() bool { return m.Group != "" }

// IsVideo reports whether the filename looks like a GoPro video (GX/GH prefix, .MP4).
func (m MediaItem) IsVideo() bool {
	u := strings.ToUpper(m.Name)
	return strings.HasSuffix(u, ".MP4") && (strings.HasPrefix(u, "GX") || strings.HasPrefix(u, "GH"))
}

// IsPhoto reports whether the filename looks like a single GoPro photo.
func (m MediaItem) IsPhoto() bool {
	u := strings.ToUpper(m.Name)
	return strings.HasSuffix(u, ".JPG") && !m.IsGroup()
}
