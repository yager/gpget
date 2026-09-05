//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// platformAutostartRun has no Windows-specific behavior: a Scheduled Task runs
// with normal network access, so the generic probe-and-notify path in
// autostartRun handles it.
func platformAutostartRun(_ context.Context, _ string) (bool, error) { return false, nil }

// Windows: a Scheduled Task that repeats every minute and runs `gpget autostart
// run`, which exits immediately when no GoPro interface is present. A device-
// arrival event trigger is a future improvement.

const taskName = `gpget-autostart`

func autostartArtifact(exe string) string {
	// The PowerShell that `install` runs (also shown by --print).
	return fmt.Sprintf("# gpget autostart — run by \"gpget autostart install\":\n\n"+
		"$act = New-ScheduledTaskAction -Execute %q -Argument 'autostart run'\n"+
		"$trg = New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes 1) -RepetitionDuration ([TimeSpan]::MaxValue)\n"+
		"$set = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Hours 6)\n"+
		"Register-ScheduledTask -TaskName '%s' -Action $act -Trigger $trg -Settings $set -Force\n\n"+
		"# uninstall: Unregister-ScheduledTask -TaskName '%s' -Confirm:$false\n",
		exe, taskName, taskName)
}

func powershell(script string) ([]byte, error) {
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return c.CombinedOutput()
}

func autostartInstall(exe string) (string, error) {
	script := fmt.Sprintf(`
$act = New-ScheduledTaskAction -Execute %q -Argument 'autostart run'
$trg = New-ScheduledTaskTrigger -Once -At (Get-Date)
$trg.Repetition = (New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Minutes 1) -RepetitionDuration ([TimeSpan]::MaxValue)).Repetition
$set = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Hours 6)
Register-ScheduledTask -TaskName '%s' -Action $act -Trigger $trg -Settings $set -Force | Out-Null
Write-Output 'ok'`, exe, taskName)
	out, err := powershell(script)
	if err != nil || !strings.Contains(string(out), "ok") {
		return "", fmt.Errorf("Register-ScheduledTask failed: %v: %s\nFor manual steps run `gpget autostart install --print`", err, strings.TrimSpace(string(out)))
	}
	return "Scheduled task '" + taskName + "' registered (polls every minute)\nLog: gpget autostart log  (" + autostartLogPath() + ")", nil
}

func autostartUninstall() error {
	_, _ = powershell(fmt.Sprintf(`Unregister-ScheduledTask -TaskName '%s' -Confirm:$false -ErrorAction SilentlyContinue`, taskName))
	return nil
}

func autostartInstalled() (bool, string) {
	out, err := powershell(fmt.Sprintf(`(Get-ScheduledTask -TaskName '%s' -ErrorAction SilentlyContinue) | Select-Object -ExpandProperty State`, taskName))
	s := strings.TrimSpace(string(out))
	if err != nil || s == "" {
		return false, ""
	}
	_ = os.Stdout
	return true, "Scheduled Task '" + taskName + "' (" + s + ")"
}

// autostartTriggered: these platforms poll from a long-lived trigger, so an
// absent camera is the normal case and must stay silent.
func autostartTriggered() bool { return false }

// autostartAgent is macOS-only: the resident IOKit watcher has no counterpart
// here yet. These platforms use the timer-driven `autostart run` instead.
func autostartAgent(_ context.Context, _ string) error {
	return errors.New("autostart agent is not supported on this platform")
}
