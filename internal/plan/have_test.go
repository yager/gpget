package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yager/gpget/internal/gopro"
)

func TestHaveFileSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "GX010014.MP4")
	if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	have, reason := haveFile(p, 100)
	if have {
		t.Fatalf("short file counted as have: %s", reason)
	}
	have, _ = haveFile(p, 5)
	if !have {
		t.Fatal("matching size should be have")
	}
	have, _ = haveFile(p, -1)
	if !have {
		t.Fatal("unknown size + present should be have")
	}
}

func TestHaveGroupTruncatedFrame(t *testing.T) {
	dir := t.TempDir()
	frames := []string{"GPAA0016.JPG", "GPAA0017.JPG", "GPAA0018.JPG"}
	for i, f := range frames {
		b := []byte("full-frame-contents")
		if i == 1 {
			b = []byte("no")
		}
		if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	full := int64(len("full-frame-contents")) * 3
	have, _, need := haveGroup(dir, frames, full)
	if have {
		t.Fatal("truncated group counted as have")
	}
	if !reflect.DeepEqual(need, frames) {
		t.Fatalf("need = %v, want all frames so transfer can HEAD the short one", need)
	}
}

func TestHaveGroupMissingFrameOnly(t *testing.T) {
	dir := t.TempDir()
	frames := []string{"GPAA0016.JPG", "GPAA0017.JPG", "GPAA0018.JPG"}
	body := []byte("full-frame-contents")
	if err := os.WriteFile(filepath.Join(dir, frames[0]), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, frames[1]), body, 0o644); err != nil {
		t.Fatal(err)
	}
	have, _, need := haveGroup(dir, frames, int64(len(body))*3)
	if have {
		t.Fatal("missing frame counted as have")
	}
	if len(need) != 1 || need[0] != frames[2] {
		t.Fatalf("need = %v, want only the missing name", need)
	}
}

func TestExpandGroupExcludesMissing(t *testing.T) {
	item := gopro.MediaItem{
		Name: "GPAD0099.JPG", Group: "1004", Begin: "99", Last: "102",
		Missing: []string{"100"},
	}
	got, err := ExpandGroup(item)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GPAD0099.JPG", "GPAD0101.JPG", "GPAD0102.JPG"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
