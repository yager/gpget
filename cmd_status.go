package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/yager/gpget/internal/plan"
	"github.com/yager/gpget/internal/xfer"
)

func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	ip := fs.String("ip", "", "camera address; default = auto-discover")
	cfgPath := fs.String("config", "", "config file path")
	dest := fs.String("dest", "", "destination root; overrides config")
	clean := fs.Bool("clean", false, "delete orphan .part files that cannot be matched to a current media item")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))

	cfg, err := loadCfg(*cfgPath)
	if err != nil {
		return err
	}

	off, err := plan.ParseClockOffset(cfg.Timezone)
	if err != nil {
		return err
	}

	cl, info, err := connect(ctx, cfg, *ip)
	if err != nil {
		return err
	}
	pl, ml, err := buildPlan(ctx, cl, info, cfg, planFilters{dest: *dest})
	if err != nil {
		return err
	}

	// per-date rollup over the whole card (unfiltered plan already is the whole card)
	type bucket struct {
		vids, photos, groups int
		bytes                int64
		newBytes             int64
		newCount             int
	}
	buckets := map[string]*bucket{}
	get := func(k string) *bucket {
		if buckets[k] == nil {
			buckets[k] = &bucket{}
		}
		return buckets[k]
	}

	for _, f := range pl.Files {
		k := tsDay(f.Cre, off)
		b := get(k)
		if f.Category == "photo" {
			b.photos++
		} else if f.Category == "video" {
			b.vids++
		}
		if f.ExpSize > 0 {
			b.bytes += f.ExpSize
		}
		if !f.Have {
			b.newCount++
			if f.ExpSize > 0 {
				b.newBytes += f.ExpSize
			}
		}
	}
	for _, g := range pl.Groups {
		k := tsDay(g.Cre, off)
		b := get(k)
		b.groups++
		if g.GroupSize > 0 {
			b.bytes += g.GroupSize
		}
		if !g.Have {
			b.newCount += len(g.NeedFrames)
			// bytes unknown without HEAD; approximate with proportion of frames left
			if g.GroupSize > 0 && len(g.Frames) > 0 {
				b.newBytes += g.GroupSize * int64(len(g.NeedFrames)) / int64(len(g.Frames))
			}
		}
	}

	days := make([]string, 0, len(buckets))
	for k := range buckets {
		days = append(days, k)
	}
	sort.Strings(days)

	destStr := cfg.Dest
	if *dest != "" {
		destStr = *dest
	}
	var totFiles, totGroups int
	var totBytes int64
	for _, d := range ml.Media {
		for _, m := range d.FS {
			totFiles++
			if m.IsGroup() {
				totGroups++
			}
			if b := m.SizeBytes(); b > 0 {
				totBytes += b
			}
		}
	}

	fmt.Printf("Card         %d entries (%d groups)   %s\n", totFiles, totGroups, humanBytes(totBytes))
	fmt.Printf("Destination  %s\n\n", pl.DestRoot)

	for _, k := range days {
		b := buckets[k]
		parts := ""
		if b.vids > 0 {
			parts += fmt.Sprintf("%d video ", b.vids)
		}
		if b.photos > 0 {
			parts += fmt.Sprintf("%d photo ", b.photos)
		}
		if b.groups > 0 {
			parts += fmt.Sprintf("%d group ", b.groups)
		}
		state := "✓ offloaded"
		if b.newCount > 0 {
			state = fmt.Sprintf("⚠ %d not offloaded (~%s)", b.newCount, humanBytes(b.newBytes))
		}
		fmt.Printf("  %-12s %-26s %10s   %s\n", k, trim(parts), humanBytes(b.bytes), state)
	}
	_ = destStr
	_ = trim

	// orphan .part files under the destination
	orphans, expandFailed, err := xfer.FindOrphans(pl.DestRoot, ml)
	if err == nil && len(orphans) > 0 {
		fmt.Printf("\nUnfinished (.part) under %s:\n", pl.DestRoot)
		for _, o := range orphans {
			rel, _ := filepath.Rel(pl.DestRoot, o.Path)
			status := "resumable"
			if !o.Matches {
				status = "STALE — does not match any current media item"
			}
			fmt.Printf("  %-40s %10s  %s\n", rel, humanBytes(o.Size), status)
		}
		if *clean {
			_, msg := xfer.CleanStale(orphans, expandFailed)
			fmt.Printf("\n%s\n", msg)
		} else {
			fmt.Println("\n  gpget sync           resume the resumable ones")
			fmt.Println("  gpget status --clean remove the stale ones")
		}
	}
	return nil
}

func tsDay(unix int64, off time.Duration) string {
	if unix <= 0 {
		return "unknown"
	}
	return plan.WallClock(unix, off).Format("2006-01-02")
}

func trim(s string) string {
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
