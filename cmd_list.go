package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/yager/gpget/internal/plan"
)

func cmdList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	ip := fs.String("ip", "", "camera address; default = auto-discover")
	cfgPath := fs.String("config", "", "config file path")
	dest := fs.String("dest", "", "destination root (for the DEST column); overrides config")
	typ := fs.String("type", "", "filter: video,photo,group (comma-separated)")
	video := fs.Bool("video", false, "shorthand for --type video")
	photo := fs.Bool("photo", false, "shorthand for --type photo")
	since := fs.String("since", "", "only items captured on/after YYYY-MM-DD")
	onDate := fs.String("date", "", "only items captured on YYYY-MM-DD")
	expand := fs.Bool("expand", false, "list every group frame instead of one row per group")
	onlyNew := fs.Bool("new", false, "only rows not yet in the destination")
	asJSON := fs.Bool("json", false, "JSON output")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))

	cfg, err := loadCfg(*cfgPath)
	if err != nil {
		return err
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
	tset := parseTypeFilter(*typ)
	if *video {
		if tset == nil {
			tset = map[string]bool{}
		}
		tset["video"] = true
	}
	if *photo {
		if tset == nil {
			tset = map[string]bool{}
		}
		tset["photo"] = true
	}

	cl, info, err := connect(ctx, cfg, *ip)
	if err != nil {
		return err
	}
	pl, _, err := buildPlan(ctx, cl, info, cfg, planFilters{
		dest: *dest, types: tset, since: sinceT, onDate: onDateS, kinds: true,
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return listJSON(pl, *expand, *onlyNew, off)
	}
	return listTable(pl, *expand, *onlyNew, off)
}

type listRow struct {
	typ  string
	date string
	size string
	name string
	dest string
}

func listTable(pl *plan.Plan, expand, onlyNew bool, off time.Duration) error {
	var rows []listRow

	// map "<stem>" -> sidecar exts, so we can fold "(+GPR)" onto the parent row
	sidecars := map[string][]string{}
	if !expand {
		for _, f := range pl.Files {
			if f.Category == "sidecar" {
				sidecars[plan.Stem(baseName(f.DestRel))] = append(
					sidecars[plan.Stem(baseName(f.DestRel))], plan.Ext(f.DestRel))
			}
		}
	}

	for _, f := range pl.Files {
		if onlyNew && f.Have {
			continue
		}
		if f.Category == "sidecar" && !expand {
			continue // folded into the parent below
		}
		t := "V"
		switch f.Category {
		case "photo":
			t = "P"
		case "sidecar":
			t = "+"
		}
		name := baseName(f.DestRel)
		if sc := sidecars[plan.Stem(name)]; len(sc) > 0 {
			name += "(+" + joinStr(sc, ",") + ")"
		}
		rows = append(rows, listRow{
			typ: t, date: tsDate(f.Cre, off), size: humanBytes(f.ExpSize),
			name: name, dest: destMark(f.Have),
		})
	}

	for _, g := range pl.Groups {
		if onlyNew && g.Have {
			continue
		}
		if expand {
			for _, fr := range g.Frames {
				rows = append(rows, listRow{
					typ: "P", date: tsDate(g.Cre, off), size: "-",
					name: g.DestDirRel + "/" + fr, dest: destMark(g.Have),
				})
			}
			continue
		}
		label := fmt.Sprintf("[%s ×%d]", g.Kind, len(g.Frames))
		rows = append(rows, listRow{
			typ: "G", date: tsDate(g.Cre, off), size: humanBytes(g.GroupSize),
			name: label + " " + g.RepName, dest: destMark(g.Have),
		})
	}

	if len(rows) == 0 {
		fmt.Println("(no media)")
		return nil
	}

	wName := len("NAME")
	for _, r := range rows {
		if len(r.name) > wName {
			wName = len(r.name)
		}
	}
	fmt.Printf("%-4s %-16s %8s  %-*s  %s\n", "TYPE", "DATE", "SIZE", wName, "NAME", "DEST")
	for _, r := range rows {
		fmt.Printf("%-4s %-16s %8s  %-*s  %s\n", r.typ, r.date, r.size, wName, r.name, r.dest)
	}
	return nil
}

func listJSON(pl *plan.Plan, expand, onlyNew bool, off time.Duration) error {
	type row struct {
		Type string `json:"type"`
		Cre  int64  `json:"cre"`
		Size int64  `json:"size"`
		Name string `json:"name"`
		Have bool   `json:"have"`
	}
	var rows []row
	for _, f := range pl.Files {
		if onlyNew && f.Have {
			continue
		}
		rows = append(rows, row{f.Category, f.Cre, f.ExpSize, f.DestRel, f.Have})
	}
	for _, g := range pl.Groups {
		if onlyNew && g.Have {
			continue
		}
		if expand {
			need := map[string]bool{}
			for _, n := range g.NeedFrames {
				need[n] = true
			}
			for _, fr := range g.Frames {
				have := g.Have || !need[fr]
				if onlyNew && have {
					continue
				}
				rows = append(rows, row{"photo", g.Cre, -1, g.DestDirRel + "/" + fr, have})
			}
			continue
		}
		rows = append(rows, row{string(g.Kind), g.Cre, g.GroupSize, g.DestDirRel, g.Have})
	}
	return printJSON(rows)
}

func tsDate(unix int64, off time.Duration) string {
	if unix <= 0 {
		return "?"
	}
	return plan.WallClock(unix, off).Format("2006-01-02 15:04")
}

func destMark(have bool) string {
	if have {
		return "✓"
	}
	return "-"
}

func baseName(rel string) string {
	for i := len(rel) - 1; i >= 0; i-- {
		if rel[i] == '/' {
			return rel[i+1:]
		}
	}
	return rel
}

func joinStr(ss []string, sep string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}
