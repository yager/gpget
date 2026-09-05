package plan

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/yager/gpget/internal/gopro"
)

// GroupKind is a human label for a group's capture mode, derived from
// media/info's undocumented `ct`. Transfer logic never depends on this.
type GroupKind string

const (
	KindBurst     GroupKind = "burst"
	KindTimelapse GroupKind = "timelapse"
	KindInterval  GroupKind = "interval"
	KindGroup     GroupKind = "group" // unknown ct
)

// KindFromCT maps a media/info `ct` value to a label (ILS FW H26.03.03.00.00).
func KindFromCT(ct int) GroupKind {
	switch ct {
	case 5:
		return KindBurst
	case 6:
		return KindTimelapse
	case 10:
		return KindInterval
	default:
		return KindGroup
	}
}

var reTrailingDigits = regexp.MustCompile(`(\d+)$`)

// ExpandGroup returns the present frame filenames of a group entry:
// prefix + %0Nd over [b..l], excluding every number in `m`.
//
// The prefix is fixed within a group (it only advances between groups), and the
// tail digit width is taken from the representative name, so this reproduces the
// exact on-camera filenames (verified 80/80 on ILS).
func ExpandGroup(item gopro.MediaItem) ([]string, error) {
	if !item.IsGroup() {
		return nil, fmt.Errorf("%s: not a group entry", item.Name)
	}
	b, err := strconv.Atoi(item.Begin)
	if err != nil {
		return nil, fmt.Errorf("%s: bad b=%q", item.Name, item.Begin)
	}
	l, err := strconv.Atoi(item.Last)
	if err != nil {
		return nil, fmt.Errorf("%s: bad l=%q", item.Name, item.Last)
	}
	if l < b {
		return nil, fmt.Errorf("%s: l(%d) < b(%d)", item.Name, l, b)
	}

	stem, ext := Stem(item.Name), Ext(item.Name)
	loc := reTrailingDigits.FindStringIndex(stem)
	if loc == nil {
		return nil, fmt.Errorf("%s: no trailing digit run", item.Name)
	}
	head := stem[:loc[0]]
	width := loc[1] - loc[0]

	missing := make(map[int]bool, len(item.Missing))
	for _, s := range item.Missing {
		if n, err := strconv.Atoi(s); err == nil {
			missing[n] = true
		}
	}

	out := make([]string, 0, l-b+1-len(missing))
	for i := b; i <= l; i++ {
		if missing[i] {
			continue
		}
		out = append(out, fmt.Sprintf("%s%0*d.%s", head, width, i, ext))
	}
	return out, nil
}

// GroupFrameCount is len(ExpandGroup) without allocating the slice.
func GroupFrameCount(item gopro.MediaItem) int {
	b, _ := strconv.Atoi(item.Begin)
	l, _ := strconv.Atoi(item.Last)
	if l < b {
		return 0
	}
	miss := 0
	for _, s := range item.Missing {
		if n, err := strconv.Atoi(s); err == nil && n >= b && n <= l {
			miss++
		}
	}
	return l - b + 1 - miss
}
