//go:build windows

package main

import "os"

func autostartRedirectLog() {
	// The scheduled task starts gpget through a hidden wscript.exe, which still
	// gives the child a (hidden) console -- so stdoutIsTTY() is true even though
	// nothing is watching. GPGET_AUTOSTART_HIDDEN, set by that launcher, is the
	// reliable signal to send output to the log instead.
	if os.Getenv("GPGET_AUTOSTART_HIDDEN") == "" && stdoutIsTTY() {
		return
	}
	f := openAutostartLog()
	if f == nil {
		return
	}
	prev := autostartLogFile
	os.Stdout = f
	os.Stderr = f
	autostartLogFile = f
	if prev != nil {
		prev.Close()
	}
}
