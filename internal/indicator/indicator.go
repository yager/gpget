// Package indicator shows a compact status surface while the autostart agent
// is resident. On macOS that is a menu-bar item; elsewhere it is a no-op.
//
// Progress used to ride on desktop notifications. Those banners are reserved
// for start/complete/fail outcomes; live counts belong here instead.
package indicator

import "sync"

// Hooks are optional actions the menu (when present) can trigger.
type Hooks struct {
	// SyncNow starts an offload if one is not already running.
	SyncNow func()
	// DestDir returns the current offload destination (may be empty).
	DestDir func() string
	// LogPath is the autostart log file, opened via the platform's file reveal.
	LogPath string
	// Quit stops the resident agent for this login session (e.g. via
	// launchctl bootout on macOS) without uninstalling autostart -- it
	// comes back automatically next login, same as Dropbox/Google Drive's
	// own "Quit" leaves their login item alone.
	Quit func()
}

var (
	mu    sync.Mutex
	hooks Hooks
)

// SetHooks stores the menu actions. Safe to call before or after Start.
func SetHooks(h Hooks) {
	mu.Lock()
	hooks = h
	mu.Unlock()
}

func currentHooks() Hooks {
	mu.Lock()
	defer mu.Unlock()
	return hooks
}

// Start shows the indicator if this platform has one. Idempotent; later calls
// refresh the idle title. No-op when unsupported.
func Start() { start() }

// SetIdle shows the quiet resting state. last is a short "last sync" note for
// the tooltip (empty is fine).
func SetIdle(last string) { setIdle(last) }

// SetProgress updates the live transfer readout (done/total files).
func SetProgress(done, total int, detail string) { setProgress(done, total, detail) }

// SetMessage shows a short non-progress status (e.g. waiting for the camera).
func SetMessage(msg string) { setMessage(msg) }
