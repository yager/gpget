package main

import (
	"context"
	"flag"
	"fmt"
	"path"
	"strings"

	"github.com/yager/gpget/internal/plan"
)

func cmdGet(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	ip := fs.String("ip", "", "camera address; default = auto-discover")
	cfgPath := fs.String("config", "", "config file path")
	dest := fs.String("dest", "", "destination root; overrides config")
	label := fs.String("label", "", "value for {label} in folder templates")
	sidecars := fs.String("sidecars", "", "override config sidecars (none|gpr,lrv|all)")
	dryRun := fs.Bool("dry-run", false, "show the plan and exit")
	yes := fs.Bool("y", false, "skip the confirmation prompt")
	quiet := fs.Bool("quiet", false, "suppress progress output")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))

	pats := fs.Args()
	if len(pats) == 0 {
		return fmt.Errorf("usage: gpget get <name|glob>...  (e.g. gpget get 'GX0100*' GP010009.JPG)")
	}

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

	cl, info, err := connect(ctx, cfg, *ip)
	if err != nil {
		return err
	}
	pl, _, err := buildPlan(ctx, cl, info, cfg, planFilters{dest: *dest, label: *label})
	if err != nil {
		return err
	}

	// keep only files/groups whose camera source (or dest basename) matches a pattern
	filtered := &plan.Plan{DestRoot: pl.DestRoot}
	var matched int
	for _, f := range pl.Files {
		if matchAny(pats, f.Src) || matchAny(pats, baseName(f.DestRel)) {
			filtered.Files = append(filtered.Files, f)
			matched++
		}
	}
	for _, g := range pl.Groups {
		if matchAny(pats, g.RepName) || matchAny(pats, plan.Stem(g.RepName)) {
			filtered.Groups = append(filtered.Groups, g)
			matched++
		}
	}
	if matched == 0 {
		return fmt.Errorf("no media matched: %s", strings.Join(pats, " "))
	}

	return runTransfer(ctx, cl, cfg, filtered, transferOpts{
		dryRun: *dryRun, yes: *yes || !cfg.Confirm, quiet: *quiet, offset: off,
	})
}

func matchAny(pats []string, name string) bool {
	for _, p := range pats {
		if p == name {
			return true
		}
		if ok, _ := path.Match(p, name); ok {
			return true
		}
		if ok, _ := path.Match(strings.ToUpper(p), strings.ToUpper(name)); ok {
			return true
		}
	}
	return false
}
