// Command gpget offloads GoPro media over the local HTTP API without removing
// the microSD card. See docs/design.md for the full spec.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

const usage = `gpget — offload GoPro media over USB

usage: gpget <command> [flags]

commands:
  sync        offload new media into dated folders (default when no command)
  list        list media on the camera (ls -l style)
  status      card summary + how much is not yet offloaded + orphan .part
  get         transfer specific files (name / glob)
  probe       connection diagnostics
  autostart   run gpget automatically when a camera is connected
  init        interactive configuration
  config      get / set / path / edit configuration

run "gpget <command> -h" for command flags.
`

// version is stamped at build time:
//
//	go build -ldflags "-X main.version=v0.1.0"
//
// It also becomes the bundle's CFBundleShortVersionString, so `gpget version`
// and the .app always agree.
var version = "dev"

func main() {
	// The macOS agent owns an NSStatusItem. AppKit requires the process main
	// thread; LockOSThread here (before any other Go work can migrate us) so
	// the indicator and CFRunLoop share that thread for the process lifetime.
	if len(os.Args) >= 3 && os.Args[1] == "autostart" && os.Args[2] == "agent" {
		runtime.LockOSThread()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	cmd := "sync"
	if len(args) > 0 && !isFlag(args[0]) {
		cmd, args = args[0], args[1:]
	}

	// The macOS autostart bundle is launched by `open`, which passes no
	// arguments (only, on some releases, a -psn_* process serial). Launching
	// it means exactly one thing, so don't fall through to interactive sync.
	if autostartTriggered() && (len(os.Args) == 1 ||
		(len(os.Args) == 2 && strings.HasPrefix(os.Args[1], "-psn"))) {
		cmd, args = "autostart", []string{"run"}
	}

	var err error
	switch cmd {
	case "sync":
		err = cmdSync(ctx, args)
	case "list", "ls":
		err = cmdList(ctx, args)
	case "status":
		err = cmdStatus(ctx, args)
	case "get":
		err = cmdGet(ctx, args)
	case "probe":
		err = cmdProbe(ctx, args)
	case "autostart":
		err = cmdAutostart(ctx, args)
	case "init":
		err = cmdInit(ctx, args)
	case "config":
		err = cmdConfig(ctx, args)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	case "version", "--version":
		fmt.Printf("gpget %s\n", version)
		return
	default:
		fmt.Fprintf(os.Stderr, "gpget: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "gpget: interrupted")
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "gpget: "+err.Error())
		os.Exit(1)
	}
}

func isFlag(s string) bool { return len(s) > 0 && s[0] == '-' }
