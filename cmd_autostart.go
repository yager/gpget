package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yager/gpget/internal/config"
	"github.com/yager/gpget/internal/gopro"
	"github.com/yager/gpget/internal/notify"
	"github.com/yager/gpget/internal/plan"
	"github.com/yager/gpget/internal/xfer"
)

func cmdAutostart(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("autostart", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file path")
	printOnly := fs.Bool("print", false, "print the OS artifact instead of installing")
	follow := fs.Bool("follow", false, "with log: print new lines as they arrive")
	fs.Parse(reorderArgs(args, "print", "follow"))
	rest := fs.Args()

	sub := "status"
	if len(rest) > 0 {
		sub = rest[0]
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)

	switch sub {
	case "run":
		return autostartRun(ctx, *cfgPath)

	case "install":
		if *printOnly {
			fmt.Print(autostartArtifact(exe))
			return nil
		}
		summary, err := autostartInstall(exe)
		if err != nil {
			return err
		}
		fmt.Println(summary)
		fmt.Println("\ngpget now starts when you plug the camera in.")
		fmt.Printf("What it does is set by [autostart] mode in the config (currently: %s).\n", mustMode(*cfgPath))
		return nil

	case "uninstall":
		if err := autostartUninstall(); err != nil {
			return err
		}
		fmt.Println("Autostart removed.")
		return nil

	case "agent":
		return autostartAgent(ctx, *cfgPath)

	case "test-notify":
		autostartRedirectLog()
		via := notify.Via("gpget: test notification", "If you can see this, transfer results will reach you too.")
		if via == "" {
			return fmt.Errorf("could not send the test notification")
		}
		return nil

	case "print":
		fmt.Print(autostartArtifact(exe))
		return nil

	case "status":
		on, desc := autostartInstalled()
		if on {
			fmt.Printf("enabled: %s\n", desc)
		} else {
			fmt.Println("disabled (enable it with: gpget autostart install)")
		}
		fmt.Printf("mode: %s\n", mustMode(*cfgPath))
		fmt.Printf("log: %s\n", autostartLogPath())
		printTransferStatus(*cfgPath)
		return nil

	case "log":
		return autostartShowLog(ctx, *follow)

	default:
		return fmt.Errorf("unknown autostart subcommand %q (install|uninstall|status|log|print|run|agent|test-notify)", sub)
	}
}

// errCameraUnreachable means a camera is attached but we could not talk to it --
// on macOS, almost always local-network access being denied.
var errCameraUnreachable = errors.New("could not reach the camera")

func mustMode(cfgPath string) string {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "notify"
	}
	return cfg.AutostartMode
}

func printTransferStatus(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return
	}
	dest, err := cfg.ExpandDest()
	if err != nil {
		return
	}
	fmt.Printf("destination: %s\n", dest)
	info := xfer.InspectLock(dest)
	switch {
	case info.Held:
		age := ""
		if !info.Started.IsZero() {
			age = fmt.Sprintf(" (%s)", time.Since(info.Started).Round(time.Second))
		}
		fmt.Printf("transferring: pid %d%s\n", info.PID, age)
	case info.Stale:
		fmt.Printf("not transferring (stale lock, pid %d)\n", info.PID)
	default:
		fmt.Println("not transferring")
	}
}

// autostartRun is what the OS trigger invokes. It is deliberately cheap and
// silent unless there is something new. Multi-fire on one connection is
// suppressed via a state file keyed on the camera "signature".
func autostartRun(parent context.Context, cfgPath string) error {
	// A full card takes far longer than a poll interval. 6h matches the macOS
	// agent budget; Win/Linux used to be 90s and would abort mid-offload.
	ctx, cancel := context.WithTimeout(parent, 6*time.Hour)
	defer cancel()

	if destBusy(cfgPath) {
		return nil
	}

	autostartRedirectLog()

	// Platforms where the trigger process can't safely talk to the camera
	// (macOS: a background LaunchAgent is blocked by Local Network privacy)
	// handle the connection their own way and return handled=true.
	if handled, err := platformAutostartRun(ctx, cfgPath); handled {
		return err
	}

	statePath := autostartStatePath()

	// 1. Is a camera-shaped interface present at all? (microseconds)
	found, _ := gopro.Discover(ctx) // Discover already filters to 172.16-31 + probes
	if len(found) == 0 {
		os.Remove(statePath) // connection gone -> allow next connect to fire
		if autostartTriggered() {
			// We were started *because* a camera appeared, so this is a real
			// failure. The caller decides whether to retry or report.
			return errCameraUnreachable
		}
		return nil
	}
	cam := found[0]
	sig := cam.CameraIP.String() + "|" + cam.Info.SerialNumber
	if autostartTriggered() {
		fmt.Printf("%s  gpget: connected to %s (%s)\n",
			time.Now().Format("2006-01-02 15:04:05"), cam.CameraIP, cam.Info.ModelName)
	}

	if readState(statePath) == sig {
		return nil // already handled this connection
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		if autostartTriggered() {
			notify.Send("gpget: cannot read the config", err.Error())
		}
		return err
	}
	cl := gopro.NewClient(cam.CameraIP.String())
	_ = cl.EnableWiredControl(ctx)

	pl, _, err := buildPlan(ctx, cl, cam.Info, cfg, planFilters{})
	if err != nil {
		// Transient: try again on the next trigger without recording state.
		// But when we were launched for this connection, silence would look
		// exactly like success, so report it.
		if autostartTriggered() {
			notify.Send("gpget: nothing was offloaded",
				err.Error()+" — try `gpget sync` in a terminal")
		}
		return nil
	}

	newFiles, newBytes := planNewTotals(pl)
	if newFiles == 0 {
		fmt.Printf("%s  gpget: nothing new to offload\n", time.Now().Format("2006-01-02 15:04:05"))
		writeState(statePath, sig)
		return nil
	}

	body := fmt.Sprintf("%d new file(s) (~%s)", newFiles, humanBytes(newBytes))
	fmt.Printf("%s  gpget autostart: %s  [%s]\n",
		time.Now().Format("2006-01-02 15:04:05"), body, cfg.AutostartMode)

	var xferErr error
	switch cfg.AutostartMode {
	case "auto":
		off, _ := plan.ParseClockOffset(cfg.Timezone)
		notify.Pulse(notify.TransferID, "gpget: transferring",
			fmt.Sprintf("0/%d files (~%s)", newFiles, humanBytes(newBytes)))
		pulse := notify.NewPulser(notify.TransferID, newFiles)
		xferErr = runTransfer(ctx, cl, cfg, pl, transferOpts{
			yes: true, quiet: false, offset: off,
			onFile: func(done, total int) {
				pulse.File(done, "gpget: transferring",
					fmt.Sprintf("%d/%d files (~%s)", done, total, humanBytes(newBytes)))
			},
		})
		if xferErr != nil {
			notify.Replace(notify.TransferID, "gpget: transfer failed", xferErr.Error())
		} else {
			notify.Replace(notify.TransferID, "gpget: transfer complete", body)
		}
	default: // notify
		notify.Send("GoPro connected", body+" — run `gpget` to offload")
	}

	if rememberAutostart(cfg.AutostartMode, xferErr) {
		writeState(statePath, sig)
	}
	return nil
}

// rememberAutostart reports whether this connection should be treated as
// handled. A failed auto transfer must not be: the cable is still in, and
// the next tick should retry.
func rememberAutostart(mode string, transferErr error) bool {
	return mode != "auto" || transferErr == nil
}

// destBusy is true when another gpget already holds the destination lock.
// Win/Linux poll every minute; a long auto transfer would otherwise error
// every tick. Held lock → this run has nothing to do.
func destBusy(cfgPath string) bool {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return false
	}
	dest, err := cfg.ExpandDest()
	if err != nil {
		return false
	}
	return xfer.InspectLock(dest).Held
}

func testNotifyReport(ok bool) string {
	if ok {
		return "A test notification was sent. If no banner appeared, turn gpget on in\nSystem Settings > Notifications (without it you cannot see the result)."
	}
	return "The test notification could not be sent.\nRun `gpget autostart test-notify` in a terminal to check."
}

func logf(format string, a ...interface{}) {
	fmt.Printf("%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
}

func planNewTotals(pl *plan.Plan) (files int, bytes int64) {
	for _, f := range pl.Files {
		if !f.Have {
			files++
			if f.ExpSize > 0 {
				bytes += f.ExpSize
			}
		}
	}
	for _, g := range pl.Groups {
		if !g.Have {
			n := len(g.NeedFrames)
			if n == 0 {
				n = len(g.Frames)
			}
			files += n
			if g.GroupSize > 0 && len(g.Frames) > 0 {
				bytes += g.GroupSize * int64(n) / int64(len(g.Frames))
			}
		}
	}
	return files, bytes
}

func autostartStateDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "gpget")
}

// autostartStatePath records which connection has already been handled, so a
// restart of the agent does not re-offload the same camera.
func autostartStatePath() string {
	return filepath.Join(autostartStateDir(), "autostart.state")
}

func readState(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeState(p, sig string) {
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(sig+"\n"), 0o644)
}
