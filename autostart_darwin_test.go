//go:build darwin

package main

import "strings"

import "testing"

// A LaunchAgent left over from a version that kept the bundle somewhere else
// keeps working for transfers but is invisible to notifications, so `status`
// has to say so instead of just printing a path.
func TestDescribeAutostartApp(t *testing.T) {
	const want = "/Users/x/Applications/gpget.app"
	plist := func(p string) string {
		return "<array>\n\t\t<string>" + p + "/Contents/MacOS/gpget</string>\n\t</array>"
	}

	if got := describeAutostartApp(plist(want), want); got != want {
		t.Errorf("current path: got %q, want %q", got, want)
	}

	old := "/Users/x/Library/Application Support/gpget/gpget.app"
	got := describeAutostartApp(plist(old), want)
	if !strings.HasPrefix(got, old) {
		t.Errorf("stale path: got %q, want it to start with %q", got, old)
	}
	if !strings.Contains(got, "autostart install") {
		t.Errorf("stale path must tell the user what to run: %q", got)
	}

	if got := describeAutostartApp("<plist>no app here</plist>", want); got != "" {
		t.Errorf("no bundle in plist: got %q, want empty", got)
	}
}
