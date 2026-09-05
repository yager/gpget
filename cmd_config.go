package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/yager/gpget/internal/config"
)

func cmdConfig(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config file path")
	fs.Parse(reorderArgs(args, "y", "dry-run", "quiet", "json", "expand", "new", "video", "photo", "clean"))
	rest := fs.Args()

	sub := "list"
	if len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}

	switch sub {
	case "path":
		p := *cfgPath
		if p == "" {
			var err error
			if p, err = config.DefaultPath(); err != nil {
				return err
			}
		}
		fmt.Println(p)
		return nil

	case "list":
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		exists := "(not created yet — run: gpget init)"
		if _, err := os.Stat(cfg.Path); err == nil {
			exists = ""
		}
		fmt.Printf("# %s %s\n", cfg.Path, exists)
		for _, k := range config.KnownKeys() {
			v, _ := cfg.Get(k)
			fmt.Printf("%-22s %s\n", k, v)
		}
		return nil

	case "get":
		if len(rest) != 1 {
			return fmt.Errorf("usage: gpget config get <section.key>")
		}
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		v, err := cfg.Get(rest[0])
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil

	case "set":
		if len(rest) != 2 {
			return fmt.Errorf("usage: gpget config set <section.key> <value>")
		}
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			return err
		}
		if err := cfg.Set(rest[0], rest[1]); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Printf("%s = %s\n(%s)\n", rest[0], rest[1], cfg.Path)
		return nil

	case "edit":
		p := *cfgPath
		if p == "" {
			var err error
			if p, err = config.DefaultPath(); err != nil {
				return err
			}
		}
		if _, err := os.Stat(p); err != nil {
			// create from defaults first so there's something to edit
			c := config.Defaults()
			c.Path = p
			if err := c.Save(); err != nil {
				return err
			}
		}
		return openEditor(p)

	default:
		return fmt.Errorf("unknown config subcommand %q (path|list|get|set|edit)", sub)
	}
}

func openEditor(path string) error {
	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = os.Getenv("VISUAL")
	}
	if ed == "" {
		switch runtime.GOOS {
		case "windows":
			ed = "notepad"
		default:
			ed = "vi"
		}
	}
	parts := strings.Fields(ed)
	c := exec.Command(parts[0], append(parts[1:], path)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
