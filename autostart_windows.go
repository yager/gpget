//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// platformAutostartRun has no Windows-specific behavior: a Scheduled Task runs
// with normal network access, so the generic probe-and-notify path in
// autostartRun handles it.
func platformAutostartRun(_ context.Context, _ string) (bool, error) { return false, nil }

// Windows: a Scheduled Task repeats every minute and runs a tiny .vbs that
// starts `gpget autostart run` with no window. `gpget autostart run` exits in
// microseconds when no GoPro interface is present. A device-arrival event
// trigger is a future improvement.
//
// Why a .vbs and not gpget.exe directly: a Scheduled Task action that runs a
// console program flashes a console window every time it fires -- once a minute
// here. wscript.exe is a GUI-subsystem host, and Shell.Run(cmd, 0, False)
// starts the child hidden, so nothing appears on screen.
//
// Why Register-ScheduledTask -Xml and not New-ScheduledTaskTrigger: the cmdlet
// requires a RepetitionDuration, and the usual "forever" value
// [TimeSpan]::MaxValue serializes to a Duration that the Task Scheduler schema
// rejects at register time ("a value which is incorrectly formatted or out of
// range" -- the days field is four digits max, ~27 years). Omitting <Duration>
// in the XML is the documented way to repeat indefinitely. The previous
// implementation hit exactly this and, because its PowerShell had no
// error-action guard, printed success anyway while nothing was registered.
const taskName = `gpget-autostart`

// hiddenLauncherPath is the .vbs that Task Scheduler runs.
func hiddenLauncherPath() string {
	return filepath.Join(autostartStateDir(), "autostart-run.vbs")
}

func writeHiddenLauncher(exe string) error {
	if err := os.MkdirAll(filepath.Dir(hiddenLauncherPath()), 0o755); err != nil {
		return err
	}
	inner := `"` + exe + `" autostart run`
	// GPGET_AUTOSTART_HIDDEN tells `autostart run` to send its output to the
	// log file: it has a (hidden) console, so it cannot tell on its own that
	// no one is watching.
	vbs := "Set sh = CreateObject(\"WScript.Shell\")\r\n" +
		"sh.Environment(\"PROCESS\")(\"GPGET_AUTOSTART_HIDDEN\") = \"1\"\r\n" +
		"sh.Run \"" + strings.ReplaceAll(inner, `"`, `""`) + "\", 0, False\r\n"
	return os.WriteFile(hiddenLauncherPath(), []byte(vbs), 0o644)
}

// taskXML is a Task Scheduler 1.2 definition: run the hidden launcher once a
// minute, forever, as the current interactive user, no elevation.
//
// No <?xml?> declaration on purpose: this string goes to
// Register-ScheduledTask -Xml, which takes an already-decoded string, and a
// declaration that claims an encoding the string is not is a known way to make
// registration fail.
func taskXML(vbs string) string {
	return fmt.Sprintf(`<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>gpget: offload a connected GoPro over USB</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <StartBoundary>2020-01-01T00:00:00</StartBoundary>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT1M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <ExecutionTimeLimit>PT6H</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>wscript.exe</Command>
      <Arguments>//nologo //b "%s"</Arguments>
    </Exec>
  </Actions>
</Task>
`, escapeXMLText(vbs))
}

func escapeXMLText(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}

func powershell(script string) ([]byte, error) {
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
}

func autostartArtifact(exe string) string {
	vbs := "Set sh = CreateObject(\"WScript.Shell\")\r\n" +
		"sh.Environment(\"PROCESS\")(\"GPGET_AUTOSTART_HIDDEN\") = \"1\"\r\n" +
		"sh.Run \"\"\"" + exe + "\"\" autostart run\", 0, False\r\n"
	return "gpget autostart (Windows) — set up by hand\n\n" +
		"STEP 1. Save the lines below as:\n" +
		"        " + hiddenLauncherPath() + "\n" +
		"----- autostart-run.vbs -----\n" +
		vbs +
		"-----------------------------\n\n" +
		"STEP 2. Run this in PowerShell:\n\n" +
		"$xml = @'\n" +
		taskXML(hiddenLauncherPath()) +
		"'@\n" +
		"Register-ScheduledTask -TaskName '" + taskName + "' -Xml $xml -Force\n\n" +
		"To remove it later:\n" +
		"Unregister-ScheduledTask -TaskName '" + taskName + "' -Confirm:$false\n"
}

func autostartInstall(exe string) (string, error) {
	if err := writeHiddenLauncher(exe); err != nil {
		return "", fmt.Errorf("could not write the launcher script: %w", err)
	}

	script := "$ErrorActionPreference = 'Stop'\n" +
		"$xml = @'\n" +
		taskXML(hiddenLauncherPath()) +
		"'@\n" +
		"try {\n" +
		"  Register-ScheduledTask -TaskName '" + taskName + "' -Xml $xml -Force | Out-Null\n" +
		"  if (-not (Get-ScheduledTask -TaskName '" + taskName + "' -ErrorAction SilentlyContinue)) {\n" +
		"    throw 'the task is not present after Register-ScheduledTask returned'\n" +
		"  }\n" +
		"  'ok'\n" +
		"} catch {\n" +
		"  \"FAILED: $_\"\n" +
		"  exit 1\n" +
		"}\n"

	out, err := powershell(script)
	s := strings.TrimSpace(string(out))
	if err != nil || !strings.Contains(s, "ok") {
		return "", fmt.Errorf("could not register the scheduled task:\n%s\n\nfor manual steps run: gpget autostart install --print", indentBlock(s))
	}

	return "Scheduled task '" + taskName + "' registered (polls every minute)\n" +
		"Log: gpget autostart log  (" + autostartLogPath() + ")", nil
}

func indentBlock(s string) string {
	if s == "" {
		return "  (powershell produced no output)"
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func autostartUninstall() error {
	// Best effort: a missing task is already the desired end state, and the
	// "not found" message is localized, so there is nothing worth parsing.
	_, _ = powershell("Unregister-ScheduledTask -TaskName '" + taskName + "' -Confirm:$false -ErrorAction SilentlyContinue")
	_ = os.Remove(hiddenLauncherPath())
	return nil
}

func autostartInstalled() (bool, string) {
	out, err := powershell("(Get-ScheduledTask -TaskName '" + taskName + "' -ErrorAction SilentlyContinue).State")
	s := strings.TrimSpace(string(out))
	if err != nil || s == "" {
		return false, ""
	}
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

// autostartBundleExe has no meaning off macOS; there is no app bundle.
func autostartBundleExe() string { return "gpget" }

// autostartAppPath has no meaning here: there is no app bundle to keep track
// of, so status has nothing extra to print.
func autostartAppPath() string { return "" }
