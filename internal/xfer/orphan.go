package xfer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yager/gpget/internal/gopro"
	"github.com/yager/gpget/internal/plan"
)

// Orphan is a leftover *.part file found under the destination root.
type Orphan struct {
	Path    string // the .part file
	Size    int64  // current bytes written
	Meta    PartMeta
	HasMeta bool
	Matches bool // meta.Src + meta.Cre correspond to a current media item
}

// FindOrphans walks root for "*.part" files and classifies each against the
// current media list. A .part whose meta matches a current item is resumable;
// anything else is stale.
func FindOrphans(root string, ml *gopro.MediaList) ([]Orphan, int, error) {
	present, expandFailed := currentSources(ml)

	var out []Orphan
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".part") {
			return nil
		}
		// Only .part files gpget created carry a .part.meta sidecar. Anything
		// else (yt-dlp, browser, other tools) is not ours — ignore it.
		meta, ok := ReadMeta(p)
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		o := Orphan{Path: p, Size: info.Size(), Meta: meta, HasMeta: true}
		if o.Meta.Src != "" {
			key := normalizeSrc(o.Meta.Src)
			if cre, ok := present[key]; ok {
				o.Matches = (o.Meta.Cre == 0 || cre == 0 || o.Meta.Cre == cre)
			}
		}
		out = append(out, o)
		return nil
	})
	if err != nil {
		return nil, expandFailed, err
	}
	return out, expandFailed, nil
}

// RemovePart deletes a .part file and its .meta sidecar.
func RemovePart(o Orphan) error {
	os.Remove(metaPathOfPart(o.Path))
	return os.Remove(o.Path)
}

// CleanStale removes unmatched .part files. If any group failed to expand,
// it deletes nothing: we cannot tell stale from "present but unreadable".
func CleanStale(orphans []Orphan, expandFailed int) (int, string) {
	if expandFailed > 0 {
		return 0, "some groups could not be expanded, so nothing was deleted"
	}
	n := 0
	for _, o := range orphans {
		if o.Matches {
			continue
		}
		if err := RemovePart(o); err == nil {
			n++
		}
	}
	return n, fmt.Sprintf("removed %d stale .part file(s)", n)
}

// currentSources maps "dir/name" (and every group frame) to its cre.
func currentSources(ml *gopro.MediaList) (map[string]int64, int) {
	m := map[string]int64{}
	if ml == nil {
		return m, 0
	}
	failed := 0
	for _, d := range ml.Media {
		for _, it := range d.FS {
			cre := it.CreUnix()
			m[normalizeSrc(d.Dir+"/"+it.Name)] = cre
			if it.IsGroup() {
				frames, err := plan.ExpandGroup(it)
				if err != nil {
					failed++
					continue
				}
				for _, fr := range frames {
					m[normalizeSrc(d.Dir+"/"+fr)] = cre
				}
			}
		}
	}
	return m, failed
}

func normalizeSrc(s string) string {
	return strings.ToUpper(strings.TrimPrefix(strings.ReplaceAll(s, "\\", "/"), "/"))
}
