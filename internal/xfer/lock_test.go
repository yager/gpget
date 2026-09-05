package xfer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectLockEmpty(t *testing.T) {
	info := InspectLock(t.TempDir())
	if info.Held || info.Stale || info.PID != 0 {
		t.Fatalf("empty dest: %+v", info)
	}
}

func TestInspectLockHeldByUs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".gpget.lock")
	started := time.Now().Add(-90 * time.Second).Unix()
	body := fmt.Sprintf("pid=%d\nstarted=%d\nhost=test\n", os.Getpid(), started)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	info := InspectLock(dir)
	if !info.Held || info.Stale || info.PID != os.Getpid() {
		t.Fatalf("want held by us, got %+v", info)
	}
	if info.Started.Unix() != started {
		t.Fatalf("started: %v", info.Started)
	}
}

func TestInspectLockDeadPID(t *testing.T) {
	if syscallZero == nil {
		t.Skip("Windows cannot probe a dead PID")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, ".gpget.lock")
	// PID 1 is alive on Unix; a huge number is almost certainly not.
	const dead = 99999999
	body := fmt.Sprintf("pid=%d\nstarted=%d\n", dead, time.Now().Unix())
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	info := InspectLock(dir)
	if info.Held {
		t.Fatalf("dead pid should not be held: %+v", info)
	}
	if !info.Stale || info.PID != dead {
		t.Fatalf("want stale pid %d, got %+v", dead, info)
	}
}
