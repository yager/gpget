package xfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yager/gpget/internal/gopro"
)

func TestFindOrphansMatchesGroupFrame(t *testing.T) {
	dir := t.TempDir()
	frame := filepath.Join(dir, "GPAA0017.JPG")
	writeFile(t, partPath(frame), []byte("partial"))
	if err := WriteMeta(frame, PartMeta{
		Src: "100GOPRO/GPAA0017.JPG",
		Cre: 1788540010,
	}); err != nil {
		t.Fatal(err)
	}

	ml := &gopro.MediaList{Media: []gopro.MediaListDir{{
		Dir: "100GOPRO",
		FS: []gopro.MediaItem{{
			Name: "GPAA0016.JPG", Group: "1", Begin: "16", Last: "18",
			Cre: "1788540010",
		}},
	}}}
	orphans, expandFailed, err := FindOrphans(dir, ml)
	if err != nil {
		t.Fatal(err)
	}
	if expandFailed != 0 {
		t.Fatalf("expandFailed = %d, want 0", expandFailed)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}
	if !orphans[0].Matches {
		t.Fatal("group-frame .part was STALE; currentSources must include expanded frames")
	}
}

func TestFindOrphansStaleWhenNotOnCard(t *testing.T) {
	dir := t.TempDir()
	frame := filepath.Join(dir, "GPAA0099.JPG")
	writeFile(t, partPath(frame), []byte("partial"))
	if err := WriteMeta(frame, PartMeta{Src: "100GOPRO/GPAA0099.JPG", Cre: 1}); err != nil {
		t.Fatal(err)
	}
	ml := &gopro.MediaList{Media: []gopro.MediaListDir{{
		Dir: "100GOPRO",
		FS:  []gopro.MediaItem{{Name: "GX010014.MP4", Size: "100", Cre: "1"}},
	}}}
	orphans, _, err := FindOrphans(dir, ml)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans[0].Matches {
		t.Fatalf("want unmatched stale, got %+v", orphans)
	}
	n, msg := CleanStale(orphans, 0)
	if n != 1 {
		t.Fatalf("removed %d, want 1; msg=%q", n, msg)
	}
}

func TestFindOrphansExpandFailedDoesNotClean(t *testing.T) {
	dir := t.TempDir()
	frame := filepath.Join(dir, "GPAA0017.JPG")
	writeFile(t, partPath(frame), []byte("partial"))
	if err := WriteMeta(frame, PartMeta{
		Src: "100GOPRO/GPAA0017.JPG",
		Cre: 1788540010,
	}); err != nil {
		t.Fatal(err)
	}

	// l < b → ExpandGroup fails. The frame .part must not be treated as
	// safe-to-delete stale just because it is missing from present.
	ml := &gopro.MediaList{Media: []gopro.MediaListDir{{
		Dir: "100GOPRO",
		FS: []gopro.MediaItem{{
			Name: "GPAA0016.JPG", Group: "1", Begin: "16", Last: "10",
			Cre: "1788540010",
		}},
	}}}
	orphans, expandFailed, err := FindOrphans(dir, ml)
	if err != nil {
		t.Fatal(err)
	}
	if expandFailed == 0 {
		t.Fatal("ExpandGroup failed but expandFailed is 0")
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %d, want 1", len(orphans))
	}
	if orphans[0].Matches {
		t.Fatal("Matches must stay false; we do not pretend the frame is resumable")
	}

	n, msg := CleanStale(orphans, expandFailed)
	if n != 0 {
		t.Fatalf("removed %d, want 0 (expand failed → do not delete)", n)
	}
	if !strings.Contains(msg, "could not be expanded") {
		t.Fatalf("message = %q, want a reason that deletion was skipped", msg)
	}
	if _, err := os.Stat(partPath(frame)); err != nil {
		t.Fatalf(".part was deleted: %v", err)
	}
}
