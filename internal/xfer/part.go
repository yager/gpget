// Package xfer implements the safe transfer engine: .part staging, exact-size
// verification, Range resume, orphan detection, destination preflight, locking
// and progress.
package xfer

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PartMeta is written next to a <name>.part as <name>.part.meta. It lets a
// later run decide whether a leftover .part may be resumed (same file) or is
// stale (card reformatted, numbers wrapped, template changed).
type PartMeta struct {
	Src       string // camera path "100GOPRO/GX010014.MP4"
	ExpSize   int64  // expected final size, -1 if unknown
	Cre       int64  // media/list cre of the source
	StartedAt int64  // unix seconds
}

func partPath(final string) string      { return final + ".part" }
func metaPath(final string) string      { return final + ".part.meta" }
func metaPathOfPart(part string) string { return part + ".meta" }

// WriteMeta writes the sidecar for the .part of final.
func WriteMeta(final string, m PartMeta) error {
	if m.StartedAt == 0 {
		m.StartedAt = time.Now().Unix()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "src=%s\n", m.Src)
	fmt.Fprintf(&b, "size=%d\n", m.ExpSize)
	fmt.Fprintf(&b, "cre=%d\n", m.Cre)
	fmt.Fprintf(&b, "started=%d\n", m.StartedAt)
	tmp := metaPath(final) + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, metaPath(final))
}

// ReadMeta reads the sidecar for a .part file (pass the .part path).
func ReadMeta(partFile string) (PartMeta, bool) {
	f, err := os.Open(metaPathOfPart(partFile))
	if err != nil {
		return PartMeta{}, false
	}
	defer f.Close()
	m := PartMeta{ExpSize: -1}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch k {
		case "src":
			m.Src = v
		case "size":
			m.ExpSize, _ = strconv.ParseInt(v, 10, 64)
		case "cre":
			m.Cre, _ = strconv.ParseInt(v, 10, 64)
		case "started":
			m.StartedAt, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	return m, true
}

// RemoveMeta deletes the sidecar for final's .part (best effort).
func RemoveMeta(final string) { os.Remove(metaPath(final)) }
