package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateAutostartLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "autostart.log")
	if err := os.WriteFile(path, []byte("small"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateAutostartLog(path, 100)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("small file should stay: %v", err)
	}
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatal("small file should not rotate")
	}

	big := strings.Repeat("x", 200)
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateAutostartLog(path, 100)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("current log should have been renamed")
	}
	got, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != big {
		t.Fatalf("old log: %q", got)
	}

	if err := os.WriteFile(path, []byte("next"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("y", 200)), 0o644); err != nil {
		t.Fatal(err)
	}
	rotateAutostartLog(path, 100)
	got, err = os.ReadFile(path + ".old")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "y") != 200 {
		t.Fatalf("second rotation should replace .old, got %q", got)
	}
}

func TestLastNLines(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"", 5, ""},
		{"a\nb\nc\n", 2, "b\nc\n"},
		{"a\nb\nc", 2, "b\nc"},
		{"only\n", 10, "only\n"},
		{"a\nb\nc\n", 0, ""},
		{"a\nb\nc\n", 3, "a\nb\nc\n"},
		{"a\nb\nc\n", 99, "a\nb\nc\n"},
	}
	for _, tc := range cases {
		got := lastNLines([]byte(tc.in), tc.n)
		if got != tc.want {
			t.Errorf("lastNLines(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestReadAutostartLogBytesSpansOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "autostart.log")

	_, err := readAutostartLogBytes(path)
	if !os.IsNotExist(err) {
		t.Fatalf("missing both: %v", err)
	}

	if err := os.WriteFile(path+".old", []byte("old1\nold2"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readAutostartLogBytes(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old1\nold2\n" {
		t.Fatalf("old only: %q", got)
	}

	if err := os.WriteFile(path, []byte("new1\nnew2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = readAutostartLogBytes(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old1\nold2\nnew1\nnew2\n" {
		t.Fatalf("old+current: %q", got)
	}
	tail := lastNLines(got, 3)
	if tail != "old2\nnew1\nnew2\n" {
		t.Fatalf("tail across rotate: %q", tail)
	}
}

func TestAutostartLogPathNotEmpty(t *testing.T) {
	p := autostartLogPath()
	if p == "" {
		t.Fatal("empty path")
	}
	if !filepath.IsAbs(p) {
		t.Fatalf("want absolute, got %q", p)
	}
}
