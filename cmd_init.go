package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/yager/gpget/internal/config"
)

func cmdInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file path")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))

	cur, err := config.Load(*cfgPath) // existing values become the prompt defaults
	if err != nil {
		return err
	}
	_, existed := os.Stat(cur.Path)
	if existed == nil {
		fmt.Printf("editing existing config: %s\n\n", cur.Path)
	}

	r := bufio.NewReader(os.Stdin)
	ask := func(label, def string) string {
		fmt.Printf("%-38s [%s]: ", label, def)
		line, _ := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}
	askValidated := func(label, def string, set func(string) error) {
		for {
			v := ask(label, def)
			if err := set(v); err != nil {
				fmt.Printf("  %v\n", err)
				continue
			}
			return
		}
	}

	next := cur
	next.Path = cur.Path

	next.Dest = ask("Destination base directory", cur.Dest)
	askValidated("Dated folder template", cur.Folder, func(v string) error { return next.Set("general.folder", v) })
	askValidated("Camera clock offset (camera / +9 etc.)", cur.Timezone, func(v string) error { return next.Set("general.timezone", v) })
	askValidated("Subfolder for groups (empty = directly under the date)", cur.GroupDir, func(v string) error {
		next.GroupDir = v
		return nil
	})
	askValidated("Also fetch sidecars (gpr,lrv / none / all)", cur.Sidecars, func(v string) error { return next.Set("general.sidecars", v) })
	askValidated("Existing files (skip/rename/replace)", cur.Overwrite, func(v string) error { return next.Set("general.overwrite", v) })
	askValidated("Chapter renaming (always/multi/never)", cur.Regroup, func(v string) error { return next.Set("chapters.regroup", v) })
	askValidated("Confirm before transferring (y/n)", boolStr(cur.Confirm), func(v string) error { return next.Set("general.confirm", v) })

	next.Folder = strings.TrimSpace(next.Folder)
	if err := next.Save(); err != nil {
		return err
	}
	fmt.Printf("\n→ saved to %s\n", next.Path)
	return nil
}

func boolStr(b bool) string {
	if b {
		return "y"
	}
	return "n"
}
