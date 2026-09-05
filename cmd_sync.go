package main

import (
	"bufio"
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
	"github.com/yager/gpget/internal/plan"
	"github.com/yager/gpget/internal/xfer"
)

func cmdSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	ip := fs.String("ip", "", "camera address; default = auto-discover")
	cfgPath := fs.String("config", "", "config file path")
	dest := fs.String("dest", "", "destination root; overrides config")
	label := fs.String("label", "", "value for {label} in folder templates")
	typ := fs.String("type", "", "filter: video,photo,group")
	sidecars := fs.String("sidecars", "", "override config sidecars (none|gpr,lrv|all)")
	since := fs.String("since", "", "only items captured on/after YYYY-MM-DD")
	onDate := fs.String("date", "", "only items captured on YYYY-MM-DD")
	dryRun := fs.Bool("dry-run", false, "show the plan and exit")
	yes := fs.Bool("y", false, "skip the confirmation prompt")
	quiet := fs.Bool("quiet", false, "suppress progress output")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))

	cfg, err := loadCfg(*cfgPath)
	if err != nil {
		return err
	}
	if *sidecars != "" {
		if err := cfg.Set("general.sidecars", *sidecars); err != nil {
			return err
		}
	}
	off, err := plan.ParseClockOffset(cfg.Timezone)
	if err != nil {
		return err
	}
	sinceT, err := parseSinceFlag(*since)
	if err != nil {
		return err
	}
	onDateS, err := parseDateFlag(*onDate)
	if err != nil {
		return err
	}

	cl, info, err := connect(ctx, cfg, *ip)
	if err != nil {
		return err
	}
	pl, ml, err := buildPlan(ctx, cl, info, cfg, planFilters{
		dest: *dest, label: *label, types: parseTypeFilter(*typ),
		since: sinceT, onDate: onDateS,
	})
	if err != nil {
		return err
	}
	_ = ml

	return runTransfer(ctx, cl, cfg, pl, transferOpts{
		dryRun: *dryRun, yes: *yes || !cfg.Confirm, quiet: *quiet, offset: off,
	})
}

type transferOpts struct {
	dryRun bool
	yes    bool
	quiet  bool
	offset time.Duration
	onFile func(done, total int)
}

// work item: either a plain file or one group frame.
type workItem struct {
	dir, src string
	finalAbs string
	expSize  int64
	cre      int64
	label    string // for the plan listing
}

func runTransfer(ctx context.Context, cl *gopro.Client, cfg config.Config, pl *plan.Plan, o transferOpts) error {
	var items []workItem
	var totalBytes, largest int64
	folders := map[string]bool{}

	for _, f := range pl.Files {
		if f.Have {
			continue
		}
		abs := f.DestAbs(pl.DestRoot)
		items = append(items, workItem{
			dir: f.Dir, src: f.Src, finalAbs: abs, expSize: f.ExpSize, cre: f.Cre,
			label: f.Category,
		})
		folders[filepath.Dir(abs)] = true
		if f.ExpSize > 0 {
			totalBytes += f.ExpSize
			if f.ExpSize > largest {
				largest = f.ExpSize
			}
		}
	}

	// group frames: HEAD each needed frame for its exact size
	for gi := range pl.Groups {
		g := &pl.Groups[gi]
		if g.Have {
			continue
		}
		dirAbs := filepath.Join(pl.DestRoot, filepath.FromSlash(g.DestDirRel))
		need := g.NeedFrames
		if need == nil {
			need = g.Frames
		}
		for _, fr := range need {
			sz := int64(-1)
			if !o.dryRun {
				hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				n, code, _ := cl.Head(hctx, g.Dir, fr)
				cancel()
				if code == 200 && n > 0 {
					sz = n
				}
			}
			items = append(items, workItem{
				dir: g.Dir, src: fr, finalAbs: filepath.Join(dirAbs, fr),
				expSize: sz, cre: g.Cre, label: string(g.Kind) + " frame",
			})
			folders[dirAbs] = true
			if sz > 0 {
				if sz > largest {
					largest = sz
				}
			}
		}
		// group byte total: use `s` when transferring the whole group, else a
		// proportional estimate (per-frame HEADs happen above for non-dry runs).
		if g.GroupSize > 0 && len(g.Frames) > 0 {
			totalBytes += g.GroupSize * int64(len(need)) / int64(len(g.Frames))
		}
	}

	if len(items) == 0 {
		fmt.Println("nothing to transfer — everything is already offloaded.")
		return nil
	}

	// plan summary
	fmt.Printf("%d file(s), ~%s → %s\n", len(items), humanBytes(totalBytes), pl.DestRoot)
	if o.dryRun {
		for _, it := range items {
			rel, _ := filepath.Rel(pl.DestRoot, it.finalAbs)
			fmt.Printf("  %-10s %-44s %s\n", it.label, rel, humanBytes(it.expSize))
		}
		return nil
	}

	// preflight
	if err := xfer.Preflight(pl.DestRoot, totalBytes, largest); err != nil {
		return err
	}

	if !o.yes {
		fmt.Print("proceed? [y/N] ")
		rd := bufio.NewReader(os.Stdin)
		ans, _ := rd.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
			fmt.Println("aborted.")
			return nil
		}
	}

	// lock + keep-alive
	lk, err := xfer.Acquire(pl.DestRoot)
	if err != nil {
		return err
	}
	defer lk.Release()

	kaCtx, kaCancel := context.WithCancel(ctx)
	go xfer.KeepAlive(kaCtx, cl, 3*time.Second)
	defer kaCancel()

	prog := xfer.NewProgress(len(items), totalBytes, o.quiet)
	reqs := make([]xfer.Request, 0, len(items))
	for _, it := range items {
		reqs = append(reqs, xfer.Request{
			Client: cl, Dir: it.dir, Src: it.src, FinalPath: it.finalAbs,
			ExpSize: it.expSize, Cre: it.cre, Overwrite: cfg.Overwrite,
			ModTime:  plan.WallClock(it.cre, o.offset),
			Progress: prog.Update,
		})
	}
	finished := 0
	qr := xfer.ProcessItems(ctx, reqs,
		func(r xfer.Request) {
			prog.StartFile(baseName(filepath.ToSlash(r.FinalPath)), r.ExpSize)
		},
		func(_ xfer.Request, res xfer.Result, err error) {
			switch {
			case err != nil && errors.Is(err, xfer.ErrStalePart):
				prog.FinishFile(0, "STALE .part — skipped")
			case err != nil:
				prog.FinishFile(0, "ERROR: "+err.Error())
			case res.Skipped:
				prog.FinishFile(0, "skipped")
			default:
				note := ""
				if !res.Verified {
					note = "ok (size unverified)"
				}
				prog.FinishFile(res.Bytes, note)
			}
			finished++
			if o.onFile != nil {
				o.onFile(finished, len(items))
			}
		},
	)
	prog.Done()
	fmt.Println(transferSummary(qr))
	if qr.Unreachable != nil {
		return fmt.Errorf("camera unreachable: %w", qr.Unreachable)
	}
	if qr.FirstErr != nil {
		return fmt.Errorf("some transfers failed (first: %w)", qr.FirstErr)
	}
	return nil
}

// transferSummary is the one-line tally printed after a sync/get run.
func transferSummary(qr xfer.QueueResult) string {
	base := fmt.Sprintf("transferred %d, skipped %d, failed %d", qr.OK, qr.Skip, qr.Fail)
	if qr.Left <= 0 {
		return base
	}
	reason := "stopped"
	switch {
	case qr.Unreachable != nil:
		reason = "stopped: camera unreachable"
	case qr.TimedOut:
		reason = "stopped: timed out"
	case qr.Canceled:
		reason = "stopped: interrupted"
	}
	return fmt.Sprintf("%s, %s, %d remaining not attempted", base, reason, qr.Left)
}
