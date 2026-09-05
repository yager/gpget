// Package notify shows a desktop notification, best-effort, with no external
// dependency: it shells out to the platform's standard tool and falls back to
// stderr.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Send posts a one-shot notification. It never returns an error; failure just
// prints to stderr.
//
// It always leaves a line on stderr saying which channel it used. A banner that
// never appears is otherwise indistinguishable from one the user missed, and in
// autostart mode stderr is the log file -- the only place to look.
func Send(title, body string) {
	_ = Via(title, body)
}

// Via is Send, but it returns the channel that accepted the notification
// (empty if none did). install uses this so it can tell the user the truth.
func Via(title, body string) string {
	return deliver("", title, body, true)
}

// Replace posts (or updates) the notification identified by id so a live
// transfer stays one banner instead of a stack. Empty id uses TransferID.
func Replace(id, title, body string) {
	if id == "" {
		id = TransferID
	}
	deliver(id, title, body, true)
}

// Pulse is Replace without a log line — progress ticks would drown the file
// log that already records each transferred file.
func Pulse(id, title, body string) {
	if id == "" {
		id = TransferID
	}
	deliver(id, title, body, false)
}

func deliver(id, title, body string, log bool) string {
	via := try(id, title, body)
	if !log {
		return via
	}
	if via != "" {
		fmt.Fprintf(os.Stderr, "[notify:%s] %s — %s\n", via, title, body)
		return via
	}
	fmt.Fprintf(os.Stderr, "[notify:FAILED] %s — %s\n", title, body)
	return ""
}

// try returns the name of the channel that accepted the notification, or "".
// Note that a zero exit only proves the tool ran: macOS can still suppress the
// banner if notifications are off for the app it was attributed to.
//
// A non-empty id asks the platform to replace the previous banner with that
// key. Support varies; a failure falls back to a fresh notification.
func try(id, title, body string) string {
	switch runtime.GOOS {
	case "darwin":
		// A macOS banner is attributed to some *app* and is dropped silently
		// unless that app is registered and allowed. So post as ourselves --
		// which requires running inside the .app bundle. Then the user sees
		// "gpget" in Settings > Notifications and is asked once.
		if native(id, title, body) {
			return "UserNotifications"
		}
		// Fallbacks below are unreliable and are only here so a bare-binary run
		// is not completely mute. Measured on macOS 15.7.7 (2026-09-05):
		//   - System Events is background-only and is NOT in the notification
		//     registry at all, so its banners are always dropped.
		//   - Bare osascript is attributed to Script Editor, whose own
		//     notification style may be "none" -- it was on the test machine.
		// Both still exit 0 either way, which is what made this invisible.
		if run("osascript", "-e", fmt.Sprintf(`display notification %q with title %q`, body, title)) {
			return "osascript (unreliable)"
		}
		// Optional: if the user happens to have terminal-notifier, it is a real
		// bundle and shows up as itself. Never required -- gpget must not need
		// a Homebrew dependency to tell the user what happened.
		if _, err := exec.LookPath("terminal-notifier"); err == nil {
			group := "gpget"
			if id != "" {
				group = id
			}
			if run("terminal-notifier", "-title", title, "-message", body, "-group", group) {
				return "terminal-notifier"
			}
		}
		return ""
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return ""
		}
		if id != "" {
			// -r is libnotify; the hint is the older Ubuntu replace-key.
			// Either may be unknown, so fall through to a plain send.
			if run("notify-send", "-a", "gpget", "-r", "74676574",
				"-h", "string:x-canonical-private-synchronous:"+id, title, body) {
				return "notify-send"
			}
			if run("notify-send", "-a", "gpget",
				"-h", "string:x-canonical-private-synchronous:"+id, title, body) {
				return "notify-send"
			}
		}
		if run("notify-send", "-a", "gpget", title, body) {
			return "notify-send"
		}
		return ""
	case "windows":
		// PowerShell toast via the WinRT API. Verbose but dependency-free.
		// Tag+Group replaces the previous toast with the same key (no progress
		// XML; that would be a bigger surface than this tool needs).
		ps := `
$ErrorActionPreference='Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] | Out-Null
$t=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$n=$t.GetElementsByTagName('text')
$n.Item(0).AppendChild($t.CreateTextNode($env:GPGET_NT_TITLE)) | Out-Null
$n.Item(1).AppendChild($t.CreateTextNode($env:GPGET_NT_BODY)) | Out-Null
$toast=[Windows.UI.Notifications.ToastNotification]::new($t)
if ($env:GPGET_NT_ID) { $toast.Tag = $env:GPGET_NT_ID; $toast.Group = 'gpget' }
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('gpget').Show($toast)`
		c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
		c.Env = append(os.Environ(),
			"GPGET_NT_TITLE="+title,
			"GPGET_NT_BODY="+body,
			"GPGET_NT_ID="+id,
		)
		if c.Run() == nil {
			return "toast"
		}
		return ""
	}
	return ""
}

func run(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}
