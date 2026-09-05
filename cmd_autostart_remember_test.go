package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	ok := testNotifyReport("UserNotifications")
	if !strings.Contains(ok, "test notification was sent") {
		t.Fatalf("success blurb: %q", ok)
	}
	if strings.Contains(ok, "could not be sent") {
		t.Fatalf("success blurb must not look like failure: %q", ok)
	}
	none := testNotifyReport("")
	if !strings.Contains(none, "could not be sent") {
		t.Fatalf("failure blurb: %q", none)
	}
	if !strings.Contains(none, "autostart test-notify") {
		t.Fatalf("failure blurb must tell the user what to run: %q", none)
	}
	// The regression this guards: install used to call an osascript fallback a
	// success, so a user with no notification permission was told everything
	// was fine and then never heard from gpget again.
	if runtime.GOOS == "darwin" {
		fell := testNotifyReport("osascript (unreliable)")
		if strings.Contains(fell, "should have appeared") {
			t.Fatalf("fallback must not be reported as success: %q", fell)
		}
		if !strings.Contains(fell, "System Settings > Notifications") {
			t.Fatalf("fallback blurb must say how to fix it: %q", fell)
		}
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
