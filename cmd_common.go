package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yager/gpget/internal/config"
	"github.com/yager/gpget/internal/gopro"
	"github.com/yager/gpget/internal/plan"
)

// loadCfg loads the config file (default location unless configPath given).
func loadCfg(configPath string) (config.Config, error) {
	return config.Load(configPath)
}

// connect resolves the camera address, opens a client, verifies it, and turns
// on wired control. ipFlag overrides both config and auto-discovery.
func connect(ctx context.Context, cfg config.Config, ipFlag string) (*gopro.Client, *gopro.CameraInfo, error) {
	explicit := ipFlag
	if explicit == "" {
		explicit = cfg.IP
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	base, found, err := gopro.ResolveBaseURL(dctx, explicit)
	if err != nil {
		return nil, nil, err
	}
	cl := gopro.NewClient(base)

	info := (*gopro.CameraInfo)(nil)
	if found != nil {
		info = found.Info
	}
	if info == nil {
		info, err = cl.Info(dctx)
		if err != nil {
			return nil, nil, fmt.Errorf("camera at %s did not respond to /gopro/camera/info: %w", cl.Host(), err)
		}
	}
	// non-fatal
	_ = cl.EnableWiredControl(dctx)
	return cl, info, nil
}

// buildPlan fetches the media list and builds a transfer plan for the given
// filters. destOverride (from --dest) wins over config.
func buildPlan(ctx context.Context, cl *gopro.Client, info *gopro.CameraInfo, cfg config.Config, f planFilters) (*plan.Plan, *gopro.MediaList, error) {
	mctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ml, err := cl.MediaList(mctx)
	if err != nil {
		return nil, nil, err
	}

	destStr := cfg.Dest
	if f.dest != "" {
		destStr = f.dest
	}
	destRoot, err := config.ExpandUser(destStr)
	if err != nil {
		return nil, nil, err
	}

	off, err := plan.ParseClockOffset(cfg.Timezone)
	if err != nil {
		return nil, nil, err
	}

	opt := plan.Options{
		DestRoot: destRoot,
		Label:    f.label,
		Model:    info.ModelName,
		Cam:      info.APSSID,
		Offset:   off,
		Sidecars: plan.ParseSidecars(cfg.Sidecars),
		Types:    f.types,
		Since:    f.since,
		OnDate:   f.onDate,
	}
	pl, err := plan.Build(ml, cfg, opt)
	if err != nil {
		return nil, nil, err
	}
	if f.kinds {
		enrichGroupKinds(ctx, cl, pl)
	}
	return pl, ml, nil
}

// enrichGroupKinds fills each group's Kind from media/info's `ct` (labels only;
// transfer logic never depends on it). Best-effort: errors leave Kind = "group".
func enrichGroupKinds(ctx context.Context, cl *gopro.Client, pl *plan.Plan) {
	for i := range pl.Groups {
		g := &pl.Groups[i]
		ictx, cancel := context.WithTimeout(ctx, 8*time.Second)
		mi, err := cl.MediaInfo(ictx, g.Dir, g.RepName)
		cancel()
		if err == nil && mi != nil {
			g.Kind = plan.KindFromCT(mi.CTInt())
		}
	}
}

type planFilters struct {
	dest   string
	label  string
	types  map[string]bool
	since  time.Time
	onDate string
	kinds  bool // fetch media/info for group labels (list only)
}

// parseTypeFilter turns "video,photo" into a set; "" means all.
func parseTypeFilter(s string) map[string]bool {
	if s == "" {
		return nil
	}
	set := map[string]bool{}
	for _, p := range splitComma(s) {
		switch p {
		case "video", "photo", "group":
			set[p] = true
		case "mp4":
			set["video"] = true
		case "jpg", "jpeg":
			set["photo"] = true
		}
	}
	return set
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' || r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// reorderArgs moves flag tokens ahead of positional tokens so Go's flag package
// (which stops at the first non-flag) accepts flags placed after positionals.
// boolFlags names the flags that take no value.
func reorderArgs(args []string, boolFlags ...string) []string {
	isBool := map[string]bool{}
	for _, b := range boolFlags {
		isBool[b] = true
	}
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name = name[:eq]
			flags = append(flags, a)
			continue
		}
		flags = append(flags, a)
		if !isBool[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, pos...)
}

// parseDateFlag accepts "YYYY-MM-DD" and returns it verbatim after validation.
func parseDateFlag(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", fmt.Errorf("date must be YYYY-MM-DD: %q", s)
	}
	return s, nil
}

// parseSinceFlag accepts "YYYY-MM-DD" and returns midnight-local of that day,
// to compare against a plan.WallClock time.
func parseSinceFlag(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since must be YYYY-MM-DD: %q", s)
	}
	return t, nil
}
