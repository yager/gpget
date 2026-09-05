package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/yager/gpget/internal/gopro"
)

func cmdProbe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	ip := fs.String("ip", "", "camera address (host or host:port); default = auto-discover")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))

	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if *ip != "" {
		fmt.Printf("target      %s (from --ip)\n", *ip)
		return probeOne(tctx, gopro.NewClient(*ip))
	}

	found, err := gopro.Discover(tctx)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		fmt.Println("no GoPro found on any 172.16-31.x.x/24 interface.")
		fmt.Println()
		fmt.Println("  - is the camera connected by USB and powered on?")
		fmt.Println("  - is the cable a data cable (not charge-only)?")
		fmt.Println("  - try: gpget probe --ip <addr>")
		return nil
	}
	for i, f := range found {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("interface   %s  (this host: %s)\n", f.Iface, f.HostIP)
		fmt.Printf("camera IP   %s\n", f.CameraIP)
		match := "no — camera answered but IP does not match the serial formula"
		if f.SerialMatch {
			match = "yes (172.2X.1YZ.51)"
		}
		fmt.Printf("serial↔IP   %s\n", match)
		if err := probeOne(tctx, gopro.NewClient(f.CameraIP.String())); err != nil {
			fmt.Printf("            probe error: %v\n", err)
		}
	}
	if len(found) > 1 {
		fmt.Println("\nmultiple candidates — pass --ip <addr> to pick one.")
	}
	return nil
}

func probeOne(ctx context.Context, cl *gopro.Client) error {
	info, err := cl.Info(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("model       %s (model_number %s)\n", info.ModelName, info.ModelNumber)
	fmt.Printf("firmware    %s\n", info.FirmwareVersion)
	fmt.Printf("serial      %s\n", info.SerialNumber)
	fmt.Printf("wifi ssid   %s\n", info.APSSID)

	if v, err := cl.APIVersion(ctx); err == nil {
		fmt.Printf("api version %s\n", v)
	} else {
		fmt.Printf("api version unavailable: %v\n", err)
	}

	if dt, err := cl.GetDateTime(ctx); err == nil {
		off := dt.TZone + dt.DST*60
		fmt.Printf("camera clock %s %s  (tzone %+d:%02d, dst %d)\n",
			dt.Date, dt.Time, off/60, abs(off%60), dt.DST)
		fmt.Printf("            note: media/list cre is the camera wall clock; gpget uses it as-is\n")
	}

	if err := cl.EnableWiredControl(ctx); err != nil {
		fmt.Printf("wired_usb   not accepted (%v) — usually still fine\n", err)
	} else {
		fmt.Printf("wired_usb   ok\n")
	}

	ml, err := cl.MediaList(ctx)
	if err != nil {
		fmt.Printf("media/list  FAILED: %v\n", err)
		return nil
	}
	var files, groups int
	var bytes int64
	for _, d := range ml.Media {
		for _, m := range d.FS {
			files++
			if m.IsGroup() {
				groups++
			}
			if b := m.SizeBytes(); b > 0 {
				bytes += b
			}
		}
	}
	fmt.Printf("media/list  ok — %d entries (%d groups), ~%s, session %s\n",
		files, groups, humanBytes(bytes), ml.ID)
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
