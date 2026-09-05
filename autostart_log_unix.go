//go:build unix

package main

import (
	"os"
)

// autostartRedirectLog points stdout/stderr at the autostart log when we are
// not attached to a terminal (LaunchAgent, systemd timer, piped debug).
// File descriptors are Dup2'd, not just the Go variables: cgo on macOS writes
// to C's stderr (fd 2), which reassigning os.Stderr would leave untouched.
func autostartRedirectLog() {
	if stdoutIsTTY() {
		return
	}
	f := openAutostartLog()
	if f == nil {
		return
	}
	_ = dupTo(int(f.Fd()), 1)
	_ = dupTo(int(f.Fd()), 2)
	prev := autostartLogFile
	os.Stdout = f
	os.Stderr = f
	autostartLogFile = f
	if prev != nil {
		prev.Close()
	}
}
