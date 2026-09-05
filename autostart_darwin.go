//go:build darwin

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yager/gpget/internal/gopro"
	"github.com/yager/gpget/internal/notify"
	"github.com/yager/gpget/internal/usbwatch"
)

const launchLabel = "com.gpget.autostart"

// bundleID is the .app bundle identifier. macOS keys local-network permission
// on this (plus the code signature), so it must stay stable across releases.
const bundleID = "com.gpget.gpget"

func launchLogPath() string { return autostartLogPath() }

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

// goproUSBVendorID is GoPro's USB vendor ID (0x2672), used to tell "the camera
// is plugged in but its network interface has not appeared yet" from "nothing
// is plugged in".
//
// Do NOT try to turn this into a launchd LaunchEvents / com.apple.iokit.matching
// trigger. That was implemented and measured on 2026-09-05 (macOS 15.7.7): with
// a correct dictionary (IOUSBHostDevice -- IOUSBDevice is the pre-10.11 class
// and matches nothing) launchd registers the trigger and shows it in
// `launchctl print`, but plugging the camera in leaves `runs = 0`. In the user
// domain the IOKit stream only delivers to an already-running job; it cannot
// start a stopped one. Apple's own LaunchAgents that use it (passd,
// apfsuseragent) are resident jobs with MachServices.
const goproUSBVendorID = 9842

// reachRetryFor bounds how long we keep retrying a camera we can see but
// cannot reach. Long enough to cover macOS re-authorising a changed signature.
const reachRetryFor = 40 * time.Second

// ifaceWait is how long to wait for the camera's network interface after its
// USB device appears. The IOKit launch event fires at USB enumeration, several
// seconds before the interface is configured, so returning immediately would
// throw the event away.
const ifaceWait = 30 * time.Second

// waitForInterfaces polls until the camera's network interface shows up.
// launchd never runs two copies of one job at once, so blocking here cannot
// pile up processes.
func waitForInterfaces(d time.Duration) []gopro.Candidate {
	deadline := time.Now().Add(d)
	for {
		if c := gopro.PresentInterfaces(); len(c) > 0 {
			return c
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// autostartBundlePath is the .app bundle that does the actual transfer.
//
// Why a bundle at all — gpget has no GUI and never shows a window (LSUIElement
// is true). macOS decides local-network access by *bundle identity*: a bare
// executable has none, so when launchd starts it the connection to the camera
// is refused with "no route to host" and no prompt is ever shown. Running the
// same binary from inside a bundle is allowed. Measured 2026-09-05 on 15.7.7;
// see docs/design.md "macOS の .app バンドルについて".
func autostartBundlePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Library", "Application Support")
	}
	return filepath.Join(dir, "gpget", "gpget.app")
}

// runningInsideBundle reports whether this process was started from the bundle
// above. That is how the two roles tell themselves apart: the LaunchAgent copy
// only looks at interfaces, the bundle copy does the network work.
// autostartBundleExe is the gpget copy inside the bundle. Only that one has
// bundle identity, so only it can post through UserNotifications or reach the
// camera under Local Network privacy.
func autostartBundleExe() string {
	return filepath.Join(autostartBundlePath(), "Contents", "MacOS", "gpget")
}

func runningInsideBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(exe, ".app/Contents/MacOS/")
}

// binaryVersion asks the binary itself, because the process writing the bundle
// may be an older copy of gpget rebuilding around a newer one (self-heal).
func binaryVersion(exe string) string {
	out, err := exec.Command(exe, "version").Output()
	if err == nil {
		if f := strings.Fields(string(out)); len(f) == 2 {
			return f[1]
		}
	}
	return version
}

func bundleInfoPlist(ver string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleIdentifier</key>         <string>%s</string>
    <key>CFBundleName</key>               <string>gpget</string>
    <key>CFBundleExecutable</key>         <string>gpget</string>
    <key>CFBundlePackageType</key>        <string>APPL</string>
    <key>CFBundleShortVersionString</key> <string>%s</string>
    <key>LSUIElement</key>                <true/>
    <key>NSLocalNetworkUsageDescription</key>
    <string>gpget uses the local network to offload photos and videos from a connected GoPro.</string>
</dict>
</plist>
`, bundleID, ver)
}

// writeAutostartBundle assembles <config>/gpget/gpget.app around a copy of exe
// and ad-hoc signs it. The signature identifier must be set explicitly: the Go
// linker's default is "a.out", which is not a usable identity.
// autostartStampPath records the SHA-256 of the binary the bundle was built
// FROM. The bundle's own copy cannot be compared directly: codesign rewrites it
// in place, so its hash never matches the source. (Measured 2026-09-05: 9560338
// vs 9578480 bytes for the same build.)
func autostartStampPath() string {
	return filepath.Join(filepath.Dir(autostartBundlePath()), "bundle.src.sha256")
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// autostartBundleStale reports whether exe differs from what the bundle was
// built from -- i.e. gpget was updated in place and the bundle still holds the
// old code. Unknown state counts as not stale, so a missing stamp never causes
// a rebuild loop.
func autostartBundleStale(exe string) bool {
	want, _ := autostartStamp()
	if want == "" {
		return false
	}
	got, err := fileSHA256(exe)
	if err != nil {
		return false
	}
	return got != want
}

// autostartStamp returns the recorded hash and the path of the binary the
// bundle was built from.
func autostartStamp() (sum, src string) {
	f := strings.Fields(readState(autostartStampPath()))
	if len(f) == 0 {
		return "", ""
	}
	if len(f) == 1 {
		return f[0], ""
	}
	return f[0], f[1]
}

// autostartSelfHeal rebuilds the bundle when the installed gpget binary has
// changed underneath it (an in-place update). Returns true if it rebuilt, in
// which case the caller should exit so launchd restarts the new code.
func autostartSelfHeal() bool {
	_, src := autostartStamp()
	if src == "" {
		return false
	}
	if !autostartBundleStale(src) {
		return false
	}
	logf("gpget agent: update detected (%s) — rebuilding gpget.app", src)
	if err := writeAutostartBundle(src); err != nil {
		logf("gpget agent: rebuild failed: %v", err)
		return false
	}
	return true
}

func writeAutostartBundle(exe string) error {
	app := autostartBundlePath()
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		return err
	}
	src, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer src.Close()

	dstPath := filepath.Join(macos, "gpget")
	_ = os.Remove(dstPath) // replacing a running/signed binary in place fails
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(bundleInfoPlist(binaryVersion(exe))), 0o644); err != nil {
		return err
	}
	out, err := exec.Command("codesign", "--force", "--deep", "-s", "-", "--identifier", bundleID, app).CombinedOutput()
	if err != nil {
		return fmt.Errorf("codesign: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if sum, err := fileSHA256(exe); err == nil {
		writeState(autostartStampPath(), sum+" "+exe)
	}
	return nil
}

func autostartArtifact(exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>autostart</string>
        <string>agent</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>AssociatedBundleIdentifiers</key>
    <array>
        <string>%s</string>
    </array>
    <key>ProcessType</key>
    <string>Background</string>
    <key>LowPriorityIO</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, launchLabel, exe, bundleID, launchLogPath(), launchLogPath())
}

// platformAutostartRun does nothing special any more. macOS used to run a
// separate launcher process that `open`ed the bundle; the resident agent
// replaced it, so `autostart run` is now just a manual one-shot and takes the
// same path as every other platform.
func platformAutostartRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// autostartAgent is the resident form: it blocks on IOKit device notifications
// instead of being re-launched on a timer. Attach fires on connect and once at
// startup for a camera that is already plugged in.
//
// It must run from inside the .app bundle -- that is what makes the camera
// reachable at all (see docs/design.md).
func autostartAgent(parent context.Context, cfgPath string) error {
	autostartInAgent = true
	autostartRedirectLog()
	logf("gpget agent: watching USB (vendor 0x%04x)", goproUSBVendorID)

	var busy sync.Mutex
	attach := func() {
		logf("gpget agent: GoPro detected")
		if autostartSelfHeal() {
			logf("gpget agent: restarting")
			os.Exit(0) // KeepAlive relaunches us from the rebuilt bundle
		}
		go func() {
			// Serialise: a replug during a transfer must not start a second one.
			busy.Lock()
			defer busy.Unlock()

			if cands := waitForInterfaces(ifaceWait); len(cands) == 0 {
				logf("gpget agent: the network interface never came up")
				notify.Send("gpget: cannot see the camera",
					"The USB device appeared, but no network connection was established.")
				return
			}
			// Keep trying until a deadline rather than a fixed count. After the
			// bundle's signature changes, macOS takes several seconds to allow
			// local-network access again; measured recoveries have landed
			// anywhere from 3 to 10 seconds in, so a short count gives false
			// alarms (it did, on 2026-09-05).
			deadline := time.Now().Add(reachRetryFor)
			for attempt := 1; ; attempt++ {
				err := autostartRun(parent, cfgPath)
				if err == nil {
					return
				}
				if !errors.Is(err, errCameraUnreachable) {
					logf("gpget agent: %v", err)
					return
				}
				if time.Now().After(deadline) {
					logf("gpget agent: still could not reach the camera after %s", reachRetryFor)
					break
				}
				logf("gpget agent: could not reach the camera (attempt %d) — retrying", attempt)
				time.Sleep(3 * time.Second)
			}
			notify.Send("gpget: cannot connect to the camera",
				"Check that local network access is allowed in System Settings > Privacy & Security > Local Network.")
		}()
	}
	detach := func() {
		logf("gpget agent: GoPro disconnected")
		os.Remove(autostartStatePath())
	}
	return usbwatch.Run(goproUSBVendorID, attach, detach)
}

func autostartInstall(exe string) (string, error) {
	if err := writeAutostartBundle(exe); err != nil {
		return "", err
	}
	// The agent is resident, so launchd runs the bundle's own executable rather
	// than `open`ing the app. AssociatedBundleIdentifiers in the plist ties that
	// process to the bundle, which is what grants local-network access on the
	// first try (without it, the first connection after install is refused).
	bundleExe := autostartBundleExe()
	p := launchAgentPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(autostartArtifact(bundleExe)), 0o644); err != nil {
		return "", err
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	launchctlBootout(domain)
	if out, err := exec.Command("launchctl", "bootstrap", domain, p).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %v: %s", err, string(out))
	}
	_ = exec.Command("launchctl", "kickstart", domain+"/"+launchLabel).Run()

	ok := sendInstallTestNotify()
	return fmt.Sprintf(`LaunchAgent installed: %s
Offloading starts the moment you plug the camera in (resident, no window).
The first time, macOS asks to allow notifications and local network access.
Allow both.

%s
While a transfer runs: gpget autostart status
Log:                   gpget autostart log   (--follow to tail it)
App:                   %s
Log file:              %s`, p, testNotifyReport(ok), autostartBundlePath(), launchLogPath()), nil
}

// sendInstallTestNotify runs the bundle binary directly. `open -a --args` is
// discarded when Launch Services already has the agent process, so the test
// banner would silently not fire. Stdio is the log file, not the install TTY:
// otherwise test-notify would skip fd redirect and the log would miss [notify:].
func sendInstallTestNotify() bool {
	exe := autostartBundleExe()
	cmd := exec.Command(exe, "autostart", "test-notify")
	if f := openAutostartLog(); f != nil {
		defer f.Close()
		cmd.Stdout = f
		cmd.Stderr = f
		cmd.Stdin = nil
	}
	return cmd.Run() == nil
}

// launchctlBootout unloads the job and waits for it to actually go away.
// bootout returns before the process has exited, and bootstrapping a label that
// is still tearing down fails with a bare "Input/output error".
func launchctlBootout(domain string) {
	_ = exec.Command("launchctl", "bootout", domain+"/"+launchLabel).Run()
	for i := 0; i < 50; i++ { // up to ~5s
		if exec.Command("launchctl", "print", domain+"/"+launchLabel).Run() != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func autostartUninstall() error {
	p := launchAgentPath()
	domain := "gui/" + strconv.Itoa(os.Getuid())
	launchctlBootout(domain)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	os.RemoveAll(autostartBundlePath())
	// Remove every state file, not just this process's: autostartStatePath()
	// resolves to the launcher's copy here, and the worker's would be orphaned.
	os.Remove(autostartStatePath())
	os.Remove(autostartStampPath())
	os.Remove(launchLogPath())
	return nil
}

func autostartInstalled() (bool, string) {
	p := launchAgentPath()
	if _, err := os.Stat(p); err != nil {
		return false, ""
	}
	loaded := exec.Command("launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())+"/"+launchLabel).Run() == nil
	state := "plist present (not loaded)"
	if loaded {
		state = "resident, reacting to USB connect events"
	}
	if _, err := os.Stat(autostartBundlePath()); err != nil {
		state += " — app missing; run `gpget autostart install` to rebuild it"
	} else if exe, err := os.Executable(); err == nil {
		if exe, err2 := filepath.EvalSymlinks(exe); err2 == nil && autostartBundleStale(exe) {
			state += " — app is out of date; it rebuilds itself on the next connect"
		}
	}
	return true, state + " (" + p + ")"
}

// autostartTriggered reports that this process exists *because* a camera was
// detected, so finishing quietly would be indistinguishable from success.
// Only the bundle copy is launched that way.
func autostartTriggered() bool { return runningInsideBundle() || autostartInAgent }

// autostartInAgent marks the resident agent, which does the camera work itself
// rather than handing off to the bundle. Set once, before any autostart work.
var autostartInAgent bool
