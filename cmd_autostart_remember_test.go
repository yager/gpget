package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yager/gpget/internal/xfer"
)

func TestRememberAutostart(t *testing.T) {
	fail := errors.New("camera unreachable")
	cases := []struct {
		mode string
		err  error
		want bool
	}{
		{"auto", nil, true},
		{"auto", fail, false},
		{"notify", nil, true},
		{"notify", fail, true},
	}
	for _, tc := range cases {
		got := rememberAutostart(tc.mode, tc.err)
		if got != tc.want {
			t.Errorf("rememberAutostart(%q, %v) = %v, want %v", tc.mode, tc.err, got, tc.want)
		}
	}
}

func TestTestNotifyReport(t *testing.T) {
	ok := testNotifyReport(true)
	if !strings.Contains(ok, "test notification was sent") {
		t.Fatalf("success blurb: %q", ok)
	}
	if strings.Contains(ok, "could not be sent") {
		t.Fatalf("success blurb must not look like failure: %q", ok)
	}
	fail := testNotifyReport(false)
	if !strings.Contains(fail, "could not be sent") {
		t.Fatalf("failure blurb: %q", fail)
	}
	if !strings.Contains(fail, "autostart test-notify") {
		t.Fatalf("failure blurb must tell the user what to run: %q", fail)
	}
}

func TestAutostartRunSilentWhenLocked(t *testing.T) {
	dest := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.ini")
	body := "[general]\ndest = " + dest + "\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if destBusy(cfgPath) {
		t.Fatal("destBusy with no lock")
	}
	lk, err := xfer.Acquire(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Release()
	if !destBusy(cfgPath) {
		t.Fatal("destBusy with live lock")
	}
	if err := autostartRun(context.Background(), cfgPath); err != nil {
		t.Fatalf("locked run should be silent, got %v", err)
	}
}
