//go:build windows

package main

import "os"

func autostartRedirectLog() {
	if stdoutIsTTY() {
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
