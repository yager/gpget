//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// platformAutostartRun has no Linux-specific behavior: a systemd user service
// runs in the user session with normal network access, so the generic
// probe-and-notify path in autostartRun handles it.
func platformAutostartRun(_ context.Context, _ string) (bool, error) { return false, nil }

// Linux: a systemd *user* timer (no root) polls every minute. `autostart run`
// exits in microseconds when no GoPro interface is present, so this is cheap.
//
// A udev-driven approach (fire exactly on connect) needs a root-owned rule and
// a bridge into the user session; deferred. `--print` shows both units.

const (
	sysdService = "gpget-autostart.service"
	sysdTimer   = "gpget-autostart.timer"
)

func userUnitDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "systemd", "user")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func autostartArtifact(exe string) string {
	return fmt.Sprintf(`# %s
[Unit]
Description=gpget: offload GoPro media when connected

[Service]
Type=oneshot
TimeoutStartSec=6h
ExecStart=%s autostart run

# ---- %s
[Unit]
Description=gpget autostart poll

[Timer]
OnBootSec=30s
OnUnitActiveSec=1min
AccuracySec=15s

[Install]
WantedBy=timers.target

# install:
#   mkdir -p ~/.config/systemd/user
#   (write the two units above, split at the "----" line)
#   systemctl --user daemon-reload
#   systemctl --user enable --now %s
`, sysdService, exe, sysdTimer, sysdTimer)
}

func writeUnits(exe string) error {
	dir := userUnitDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	svc := fmt.Sprintf("[Unit]\nDescription=gpget: offload GoPro media when connected\n\n[Service]\nType=oneshot\nTimeoutStartSec=6h\nExecStart=%s autostart run\n", exe)
	tmr := "[Unit]\nDescription=gpget autostart poll\n\n[Timer]\nOnBootSec=30s\nOnUnitActiveSec=1min\nAccuracySec=15s\n\n[Install]\nWantedBy=timers.target\n"
	if err := os.WriteFile(filepath.Join(dir, sysdService), []byte(svc), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sysdTimer), []byte(tmr), 0o644)
}

func autostartInstall(exe string) (string, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "", fmt.Errorf("systemctl not found; set it up by hand using `gpget autostart install --print`")
	}
	if err := writeUnits(exe); err != nil {
		return "", err
	}
	for _, a := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "--now", sysdTimer},
	} {
		if out, err := exec.Command("systemctl", a...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("systemctl %v: %v: %s", a, err, string(out))
		}
	}
	return "systemd user timer enabled (" + filepath.Join(userUnitDir(), sysdTimer) + ", every minute)\nLog: gpget autostart log  (" + autostartLogPath() + ")", nil
}

func autostartUninstall() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "--user", "disable", "--now", sysdTimer).Run()
	}
	dir := userUnitDir()
	os.Remove(filepath.Join(dir, sysdTimer))
	os.Remove(filepath.Join(dir, sysdService))
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	return nil
}

func autostartInstalled() (bool, string) {
	p := filepath.Join(userUnitDir(), sysdTimer)
	if _, err := os.Stat(p); err != nil {
		return false, ""
	}
	active := exec.Command("systemctl", "--user", "is-active", "--quiet", sysdTimer).Run() == nil
	if active {
		return true, "systemd user timer running (" + p + ")"
	}
	return true, "unit present but stopped (" + p + ")"
}

// autostartTriggered: these platforms poll from a long-lived trigger, so an
// absent camera is the normal case and must stay silent.
func autostartTriggered() bool { return false }

// autostartAgent is macOS-only: the resident IOKit watcher has no counterpart
// here yet. These platforms use the timer-driven `autostart run` instead.
func autostartAgent(_ context.Context, _ string) error {
	return errors.New("autostart agent is not supported on this platform")
}

// autostartBundleExe has no meaning off macOS; there is no app bundle.
func autostartBundleExe() string { return "gpget" }
