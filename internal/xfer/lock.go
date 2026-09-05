package xfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Lock is a whole-destination advisory lock (<dest>/.gpget.lock). It stops a
// manual `sync` and an autostart `auto` run from writing the same .part files.
type Lock struct{ path string }

// Acquire takes the lock for dir. A lock held by a dead PID is stolen.
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, ".gpget.lock")

	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "pid=%d\nstarted=%d\nhost=%s\n", os.Getpid(), time.Now().Unix(), hostname())
			f.Close()
			return &Lock{path: p}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		pid, started := readLock(p)
		if pid > 0 && processAlive(pid) {
			age := time.Since(time.Unix(started, 0)).Round(time.Second)
			return nil, fmt.Errorf("another gpget (pid %d, running %s) is using %s", pid, age, dir)
		}
		// stale — remove and retry
		os.Remove(p)
	}
	return nil, fmt.Errorf("could not acquire lock at %s", p)
}

// Release removes the lock file.
func (l *Lock) Release() {
	if l != nil {
		os.Remove(l.path)
	}
}

// InspectLock reports whether a live process holds <dir>/.gpget.lock.
// It never deletes the file: status must not steal a lock.
func InspectLock(dir string) LockInfo {
	p := filepath.Join(dir, ".gpget.lock")
	info := LockInfo{Path: p}
	pid, started := readLock(p)
	if pid == 0 {
		return info
	}
	info.PID = pid
	if started != 0 {
		info.Started = time.Unix(started, 0)
	}
	if processAlive(pid) {
		info.Held = true
		return info
	}
	info.Stale = true
	return info
}

// LockInfo is a snapshot of an advisory dest lock.
type LockInfo struct {
	Held    bool
	Stale   bool
	PID     int
	Started time.Time
	Path    string
}

func readLock(p string) (pid int, started int64) {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "pid":
			pid, _ = strconv.Atoi(v)
		case "started":
			started, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	return pid, started
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if syscallZero == nil {
		// Windows: cannot cheaply probe; assume held (stale locks need manual
		// deletion, which is rare).
		return true
	}
	return p.Signal(syscallZero) == nil
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
