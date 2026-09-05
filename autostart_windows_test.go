//go:build windows

package main

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The bug this file guards against: the old install used
// New-ScheduledTaskTrigger with -RepetitionDuration ([TimeSpan]::MaxValue),
// which serializes to a Duration the Task Scheduler schema rejects at register
// time. Register-ScheduledTask failed, but the install PowerShell had no
// error-action guard, so it printed success while nothing was registered and
// `autostart status` then reported "disabled".

func TestTaskXMLIsWellFormedAndRepeatsForever(t *testing.T) {
	x := taskXML(`C:\Users\Someone\AppData\Local\gpget\autostart-run.vbs`)

	if err := xml.Unmarshal([]byte(x), new(struct{})); err != nil {
		t.Fatalf("taskXML is not well-formed XML: %v\n%s", err, x)
	}

	// The whole point of the rewrite: repeat every minute, with NO <Duration>.
	if !strings.Contains(x, "<Interval>PT1M</Interval>") {
		t.Error("missing <Interval>PT1M</Interval>")
	}
	if strings.Contains(x, "<Duration>") {
		t.Error("<Duration> is present again — that is exactly what broke registration")
	}
	if strings.Contains(strings.ToLower(x), "timespan") {
		t.Error("XML mentions TimeSpan; it should be a plain schema value")
	}

	// The action must be the hidden wscript launcher, not gpget.exe directly
	// (which would flash a console window every minute).
	if !strings.Contains(x, "<Command>wscript.exe</Command>") {
		t.Error("action Command is not wscript.exe")
	}
	if !strings.Contains(x, `autostart-run.vbs`) {
		t.Error("action Arguments do not reference the .vbs launcher")
	}
}

func TestTaskXMLEscapesTheLauncherPath(t *testing.T) {
	x := taskXML(`C:\weird & <path>\autostart-run.vbs`)
	if err := xml.Unmarshal([]byte(x), new(struct{})); err != nil {
		t.Fatalf("taskXML with a nasty path is not well-formed: %v", err)
	}
	if !strings.Contains(x, "weird &amp; &lt;path&gt;") {
		t.Errorf("launcher path was not XML-escaped:\n%s", x)
	}
}

func TestEscapeXMLText(t *testing.T) {
	got := escapeXMLText(`a & b < c > d " e ' f`)
	want := `a &amp; b &lt; c &gt; d &quot; e &apos; f`
	if got != want {
		t.Errorf("escapeXMLText = %q, want %q", got, want)
	}
}

func TestWriteHiddenLauncher(t *testing.T) {
	// Point the launcher path at a temp dir so a real install is not touched.
	t.Setenv("LOCALAPPDATA", t.TempDir())

	exe := `C:\Program Files\gpget\gpget.exe`
	if err := writeHiddenLauncher(exe); err != nil {
		t.Fatalf("writeHiddenLauncher: %v", err)
	}
	b, err := os.ReadFile(hiddenLauncherPath())
	if err != nil {
		t.Fatal(err)
	}
	vbs := string(b)

	if !strings.Contains(vbs, `GPGET_AUTOSTART_HIDDEN") = "1"`) {
		t.Error("launcher does not set GPGET_AUTOSTART_HIDDEN")
	}
	// Shell.Run(..., 0, False): 0 = hidden window.
	if !strings.Contains(vbs, `, 0, False`) {
		t.Error("launcher does not start the child with a hidden window")
	}
	// The exe path must survive as a quoted token inside the VBS string literal
	// (embedded quotes doubled).
	if !strings.Contains(vbs, `"""`+exe+`"" autostart run"`) {
		t.Errorf("launcher command line is not quoted correctly:\n%s", vbs)
	}
}

// End-to-end: prove Register-ScheduledTask -Xml accepts the exact XML shape the
// installer builds, on the real Task Scheduler. Uses a throwaway task name and
// a temp launcher path, and always cleans up.
func TestRegisterScheduledTaskAcceptsOurXML(t *testing.T) {
	if _, err := exec.LookPath("powershell"); err != nil {
		t.Skip("powershell not available")
	}

	const testTask = "gpget-autostart-selftest"
	vbs := filepath.Join(t.TempDir(), "autostart-run.vbs")
	script := "$ErrorActionPreference = 'Stop'\n" +
		"$xml = @'\n" + taskXML(vbs) + "'@\n" +
		"try {\n" +
		"  Register-ScheduledTask -TaskName '" + testTask + "' -Xml $xml -Force | Out-Null\n" +
		"  $present = [bool](Get-ScheduledTask -TaskName '" + testTask + "' -ErrorAction SilentlyContinue)\n" +
		"  Unregister-ScheduledTask -TaskName '" + testTask + "' -Confirm:$false -ErrorAction SilentlyContinue\n" +
		"  if ($present) { 'ok' } else { 'FAILED: not registered' }\n" +
		"} catch {\n" +
		"  Unregister-ScheduledTask -TaskName '" + testTask + "' -Confirm:$false -ErrorAction SilentlyContinue\n" +
		"  \"FAILED: $_\"\n" +
		"}\n"

	out, err := powershell(script)
	got := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		_, _ = powershell("Unregister-ScheduledTask -TaskName '" + testTask + "' -Confirm:$false -ErrorAction SilentlyContinue")
	})

	if err != nil && got == "" {
		t.Skipf("could not run powershell: %v", err)
	}
	if !strings.Contains(got, "ok") {
		t.Fatalf("Register-ScheduledTask rejected the installer XML:\n%s", got)
	}
}
