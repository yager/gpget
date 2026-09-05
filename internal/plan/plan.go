package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yager/gpget/internal/config"
	"github.com/yager/gpget/internal/gopro"
)

// Options control how a Plan is built.
type Options struct {
	DestRoot string // absolute, expanded
	Label    string // --label
	Model    string // raw model name
	Cam      string // camera name / ssid
	Offset   time.Duration
	Sidecars sidecarSet

	// filters (zero value = no filter)
	Types  map[string]bool // "video" | "photo" | "group"; nil/empty = all
	Since  time.Time       // inclusive lower bound on capture date
	OnDate string          // "YYYY-MM-DD" exact match, "" = any
}

type sidecarSet struct{ GPR, LRV bool }

// ParseSidecars turns "gpr,lrv" / "none" / "all" into a set.
func ParseSidecars(s string) sidecarSet {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "all" {
		return sidecarSet{true, true}
	}
	var set sidecarSet
	for _, p := range strings.Split(s, ",") {
		switch strings.TrimSpace(p) {
		case "gpr":
			set.GPR = true
		case "lrv":
			set.LRV = true
		}
	}
	return set
}

// ParseClockOffset resolves the `timezone` config value to an offset added to
// the camera wall clock.
//
// GoPro's `cre` is the camera's wall-clock time encoded as if it were UTC
// (verified on ILS: a clip shot at 16:40 JST has cre == 16:40 "UTC"). So the
// correct time is time.Unix(cre,0).UTC() read as naive local components — no
// zone conversion. `timezone` only lets the user nudge a wrong/travelling
// camera clock:
//
//	camera | local | "" | utc   -> no shift (default)
//	+9  +09:00  -5  -05:30       -> shift the wall clock by that many hours
func ParseClockOffset(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "camera", "local", "utc":
		return 0, nil
	}
	sign := time.Duration(1)
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		sign = -1
		s = s[1:]
	}
	h, m := s, "0"
	if i := strings.IndexByte(s, ':'); i >= 0 {
		h, m = s[:i], s[i+1:]
	}
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("timezone: want 'camera' or an offset like +9 / -05:30, got %q", s)
	}
	return sign * (time.Duration(hi)*time.Hour + time.Duration(mi)*time.Minute), nil
}

// WallClock turns a GoPro `cre` epoch into the camera's local wall-clock time,
// expressed in the machine's local zone so it displays with the same digits.
// offset is applied (usually zero).
func WallClock(cre int64, offset time.Duration) time.Time {
	u := time.Unix(cre, 0).UTC().Add(offset)
	return time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), u.Minute(), u.Second(), 0, time.Local)
}

// File is one plain file to (maybe) transfer.
type File struct {
	Dir, Src   string // camera side
	DestRel    string // relative to Options.DestRoot
	ExpSize    int64  // -1 if unknown (GPR / would need HEAD)
	Cre        int64
	Category   string // "video" | "photo" | "sidecar"
	Have       bool
	HaveReason string
}

// DestAbs returns the absolute destination path.
func (f File) DestAbs(root string) string { return filepath.Join(root, filepath.FromSlash(f.DestRel)) }

// Group is one burst/timelapse/interval to (maybe) transfer as a unit.
type Group struct {
	Dir, RepName string
	GroupID      string
	Kind         GroupKind
	Cre          int64
	DestDirRel   string   // folder/<group_dir>, relative to root
	Frames       []string // present camera filenames
	GroupSize    int64    // media/list `s` (sum of present frames)
	Have         bool
	HaveReason   string
	NeedFrames   []string // frames not yet on disk (best-effort, refined at transfer time)
}

// Plan is the full set of work for one sync/list.
type Plan struct {
	DestRoot string
	Files    []File
	Groups   []Group
}

// Build turns a media list + config into a Plan (final paths + incremental
// status). It stats the destination but issues no camera requests.
func Build(ml *gopro.MediaList, cfg config.Config, opt Options) (*Plan, error) {
	pl := &Plan{DestRoot: opt.DestRoot}

	// Pass 1: count MP4 chapters per (dir, prefix, clip) so regroup=multi knows.
	chapterCount := map[string]int{}
	for _, d := range ml.Media {
		for _, m := range d.FS {
			if m.IsGroup() || !m.IsVideo() {
				continue
			}
			if g, ok := ParseGoProName(m.Name); ok {
				chapterCount[d.Dir+"|"+g.Prefix+"|"+g.Clip]++
			}
		}
	}

	for _, d := range ml.Media {
		for _, m := range d.FS {
			cre := m.CreUnix()
			date := WallClock(cre, opt.Offset)

			if !opt.passFilter(m, date) {
				continue
			}

			folder, err := Render(cfg.Folder, opt.vars(date, m))
			if err != nil {
				return nil, err
			}

			switch {
			case m.IsGroup():
				g, err := buildGroup(d.Dir, m, folder, cfg, opt, date)
				if err != nil {
					return nil, err
				}
				pl.Groups = append(pl.Groups, *g)

			case m.IsVideo():
				name := m.Name
				if wantRegroup(cfg.Regroup, chapterCount, d.Dir, m.Name) {
					if gn, ok := ParseGoProName(m.Name); ok {
						v := opt.vars(date, m)
						v.Prefix, v.Clip, v.Chapter = gn.Prefix, gn.Clip, gn.Chapter
						rn, err := Render(cfg.ChapterName, v)
						if err != nil {
							return nil, err
						}
						name = rn + "." + strings.ToLower(gn.Ext)
					}
				}
				destRel := path3(folder, name)
				pl.addFile(File{
					Dir: d.Dir, Src: m.Name, DestRel: destRel,
					ExpSize: m.SizeBytes(), Cre: cre, Category: "video",
				}, opt.DestRoot)

				if opt.Sidecars.LRV && m.GLRV != "" {
					if lrvSrc, ok := LRVSourceName(m.Name); ok {
						lrvDest := path3(folder, Stem(name)+".LRV")
						sz := int64(-1)
						if n := parseInt(m.GLRV); n > 0 {
							sz = n
						}
						pl.addFile(File{
							Dir: d.Dir, Src: lrvSrc, DestRel: lrvDest,
							ExpSize: sz, Cre: cre, Category: "sidecar",
						}, opt.DestRoot)
					}
				}

			default: // single photo (or other)
				destRel := path3(folder, m.Name)
				pl.addFile(File{
					Dir: d.Dir, Src: m.Name, DestRel: destRel,
					ExpSize: m.SizeBytes(), Cre: cre, Category: "photo",
				}, opt.DestRoot)

				if opt.Sidecars.GPR && m.Raw == "1" {
					if gprSrc, ok := GPRSourceName(m.Name); ok {
						gprDest := path3(folder, Stem(m.Name)+".GPR")
						pl.addFile(File{
							Dir: d.Dir, Src: gprSrc, DestRel: gprDest,
							ExpSize: -1, Cre: cre, Category: "sidecar",
						}, opt.DestRoot)
					}
				}
			}
		}
	}

	sort.Slice(pl.Files, func(i, j int) bool {
		if pl.Files[i].Cre != pl.Files[j].Cre {
			return pl.Files[i].Cre < pl.Files[j].Cre
		}
		return pl.Files[i].DestRel < pl.Files[j].DestRel
	})
	sort.Slice(pl.Groups, func(i, j int) bool { return pl.Groups[i].Cre < pl.Groups[j].Cre })
	return pl, nil
}

func (pl *Plan) addFile(f File, root string) {
	f.Have, f.HaveReason = haveFile(f.DestAbs(root), f.ExpSize)
	pl.Files = append(pl.Files, f)
}

func buildGroup(dir string, m gopro.MediaItem, folder string, cfg config.Config, opt Options, date time.Time) (*Group, error) {
	frames, err := ExpandGroup(m)
	if err != nil {
		return nil, err
	}
	v := opt.vars(date, m)
	v.Stem = Stem(m.Name)
	groupDir := ""
	if strings.TrimSpace(cfg.GroupDir) != "" {
		groupDir, err = Render(cfg.GroupDir, v)
		if err != nil {
			return nil, err
		}
	}
	destDirRel := folder
	if groupDir != "" {
		destDirRel = folder + "/" + groupDir
	}

	g := &Group{
		Dir: dir, RepName: m.Name, GroupID: m.Group, Kind: KindGroup,
		Cre: m.CreUnix(), DestDirRel: destDirRel, Frames: frames,
		GroupSize: m.SizeBytes(),
	}
	g.Have, g.HaveReason, g.NeedFrames = haveGroup(filepath.Join(opt.DestRoot, filepath.FromSlash(destDirRel)), frames, g.GroupSize)
	return g, nil
}

// ---- filters & vars ----

func (o Options) passFilter(m gopro.MediaItem, date time.Time) bool {
	if len(o.Types) > 0 {
		cat := "video"
		switch {
		case m.IsGroup():
			cat = "group"
		case m.IsPhoto():
			cat = "photo"
		case m.IsVideo():
			cat = "video"
		}
		if !o.Types[cat] {
			return false
		}
	}
	if !o.Since.IsZero() && date.Before(o.Since) {
		return false
	}
	if o.OnDate != "" && date.Format("2006-01-02") != o.OnDate {
		return false
	}
	return true
}

func (o Options) vars(date time.Time, m gopro.MediaItem) Vars {
	return Vars{
		Date:  date,
		Label: o.Label,
		Model: SanitizeModel(o.Model),
		Cam:   o.Cam,
		Stem:  Stem(m.Name),
	}
}

func wantRegroup(mode string, counts map[string]int, dir, name string) bool {
	switch mode {
	case "never":
		return false
	case "always":
		_, ok := ParseGoProName(name)
		return ok
	default: // "multi"
		g, ok := ParseGoProName(name)
		if !ok {
			return false
		}
		return counts[dir+"|"+g.Prefix+"|"+g.Clip] >= 2
	}
}

// path3 joins folder + name with forward slashes (DestRel is always slash-form).
func path3(folder, name string) string { return folder + "/" + name }

func parseInt(s string) int64 {
	var n int64
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

// ---- incremental checks (filesystem) ----

func haveFile(abs string, expSize int64) (bool, string) {
	fi, err := os.Stat(abs)
	if err != nil {
		return false, ""
	}
	if fi.IsDir() {
		return false, "destination is a directory"
	}
	if expSize < 0 {
		return true, "present (size not verifiable)"
	}
	if fi.Size() == expSize {
		return true, "present, size matches"
	}
	return false, "size differs (have " + humanish(fi.Size()) + ", want " + humanish(expSize) + ")"
}

// haveGroup implements F-12b: compare the destination dir's file count and total
// bytes against the group's frame count and `s`, without any HEAD request.
func haveGroup(dirAbs string, frames []string, groupSize int64) (have bool, reason string, need []string) {
	ent, err := os.ReadDir(dirAbs)
	if err != nil {
		return false, "", frames
	}
	onDisk := map[string]int64{}
	var total int64
	for _, e := range ent {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		onDisk[e.Name()] = info.Size()
		total += info.Size()
	}
	if len(onDisk) == len(frames) && groupSize > 0 && total == groupSize {
		return true, "all frames present, total bytes match", nil
	}
	need = nil
	for _, f := range frames {
		if _, ok := onDisk[f]; !ok {
			need = append(need, f)
		}
	}
	// All names can be present while a file is truncated: then need would be
	// empty and status would show "offloaded". Re-check every frame (HEAD at
	// transfer time skips the ones whose size matches).
	if len(need) == 0 {
		need = append([]string{}, frames...)
	}
	reason = "partial: " + itoa(len(frames)-len(need)) + "/" + itoa(len(frames)) + " frames on disk"
	if len(need) == len(frames) && len(onDisk) == len(frames) {
		reason = "partial: all frames present, total bytes differ"
	}
	return false, reason, need
}

func humanish(n int64) string {
	if n >= 1<<20 {
		return itoa(int(n>>20)) + "M"
	}
	if n >= 1<<10 {
		return itoa(int(n>>10)) + "K"
	}
	return itoa(int(n)) + "B"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
