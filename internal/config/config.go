// Package config loads and saves gpget's single INI config file.
//
// Location: os.UserConfigDir()/gpget/config.ini
//
//	macOS   ~/Library/Application Support/gpget/config.ini
//	Windows %AppData%\gpget\config.ini
//	Linux   ${XDG_CONFIG_HOME:-~/.config}/gpget/config.ini
//
// Resolution order for any value: CLI flag > config.ini > built-in default.
// Only --config overrides the file location; no other config files are read.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Config is the fully-resolved configuration (file values merged over defaults).
type Config struct {
	// [general]
	Dest      string
	Folder    string
	GroupDir  string
	Sidecars  string // "none" | "gpr,lrv" | "all"
	Overwrite string // "skip" | "rename" | "replace"
	Confirm   bool
	Timezone  string // "camera" (no shift) or an offset like "+9" / "-05:30"

	// [chapters]
	Regroup     string // "always" | "multi" | "never"
	ChapterName string

	// [camera]
	IP string

	// [autostart]
	AutostartMode string // "notify" | "auto"

	// Path this config was loaded from (may not exist yet).
	Path string
}

// Defaults returns the built-in configuration.
func Defaults() Config {
	return Config{
		Dest:          DefaultDest(),
		Folder:        "{date:%Y-%m-%d}_GoPro",
		GroupDir:      "{stem}",
		Sidecars:      "gpr,lrv",
		Overwrite:     "skip",
		Confirm:       true,
		Timezone:      "camera",
		Regroup:       "multi",
		ChapterName:   "{prefix}{clip}_{chapter:02d}",
		IP:            "",
		AutostartMode: "notify",
	}
}

// DefaultDest is "<OS videos dir>/GoPro":
//
//	macOS    ~/Movies/GoPro
//	Windows  %USERPROFILE%\Videos\GoPro
//	Linux    $XDG_VIDEOS_DIR/GoPro  (else ~/Videos/GoPro)
//
// Go has no stdlib "user videos dir", so this is constructed. Returns an
// absolute path; falls back to "~/Movies/GoPro" if the home dir is unknown.
func DefaultDest() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("~", "Movies", "GoPro")
	}
	var videos string
	switch runtime.GOOS {
	case "darwin":
		videos = filepath.Join(home, "Movies")
	case "windows":
		videos = filepath.Join(home, "Videos")
	default: // linux, *bsd
		if x := strings.TrimSpace(os.Getenv("XDG_VIDEOS_DIR")); x != "" {
			videos = expandXDG(x, home)
		} else if d := readUserDir("XDG_VIDEOS_DIR", home); d != "" {
			videos = d
		} else {
			videos = filepath.Join(home, "Videos")
		}
	}
	return filepath.Join(videos, "GoPro")
}

// expandXDG handles the "$HOME/..." form that XDG_VIDEOS_DIR sometimes uses.
func expandXDG(v, home string) string {
	v = strings.Trim(v, `"`)
	if v == "$HOME" {
		return home
	}
	if strings.HasPrefix(v, "$HOME/") {
		return filepath.Join(home, v[len("$HOME/"):])
	}
	return v
}

// readUserDir parses ~/.config/user-dirs.dirs for a key like XDG_VIDEOS_DIR.
func readUserDir(key, home string) string {
	f, err := os.Open(filepath.Join(home, ".config", "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, key+"=") {
			continue
		}
		return expandXDG(strings.TrimPrefix(line, key+"="), home)
	}
	return ""
}

// DefaultPath returns the standard config file path.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpget", "config.ini"), nil
}

// Load reads the config file at path (or the default path if path == ""),
// merging file values over Defaults(). A missing file is not an error.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return cfg, err
		}
		path = p
	}
	cfg.Path = path

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	sec := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, ";") || strings.HasPrefix(raw, "#") {
			continue
		}
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			sec = strings.ToLower(strings.TrimSpace(raw[1 : len(raw)-1]))
			continue
		}
		k, v, ok := strings.Cut(raw, "=")
		if !ok {
			return cfg, fmt.Errorf("%s:%d: not key=value: %q", path, line, raw)
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(stripInlineComment(v))
		if err := cfg.set(sec, k, v); err != nil {
			return cfg, fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
	return cfg, sc.Err()
}

// stripInlineComment removes a trailing " ; comment" or " # comment" that is
// not inside quotes. Templates don't use quotes, so a simple scan suffices.
func stripInlineComment(v string) string {
	for i := 0; i < len(v); i++ {
		if (v[i] == ';' || v[i] == '#') && i > 0 && (v[i-1] == ' ' || v[i-1] == '\t') {
			return strings.TrimSpace(v[:i])
		}
	}
	return strings.TrimSpace(v)
}

// Key is a "section.key" identifier used by `gpget config get/set`.
type Key struct{ Section, Name string }

// knownKeys lists every settable key with its section.
var knownKeys = []Key{
	{"general", "dest"}, {"general", "folder"}, {"general", "group_dir"},
	{"general", "sidecars"}, {"general", "overwrite"}, {"general", "confirm"},
	{"general", "timezone"},
	{"chapters", "regroup"}, {"chapters", "chapter_name"},
	{"camera", "ip"},
	{"autostart", "mode"},
}

func (c *Config) set(section, key, val string) error {
	switch section + "." + key {
	case "general.dest":
		c.Dest = val
	case "general.folder":
		c.Folder = val
	case "general.group_dir":
		c.GroupDir = val
	case "general.sidecars":
		v, err := normalizeSidecars(val)
		if err != nil {
			return err
		}
		c.Sidecars = v
	case "general.overwrite":
		if !oneOf(val, "skip", "rename", "replace") {
			return fmt.Errorf("overwrite must be skip|rename|replace, got %q", val)
		}
		c.Overwrite = val
	case "general.confirm":
		b, err := parseBool(val)
		if err != nil {
			return fmt.Errorf("confirm: %w", err)
		}
		c.Confirm = b
	case "general.timezone":
		if _, err := parseClockOffset(val); err != nil {
			return err
		}
		c.Timezone = val
	case "chapters.regroup":
		if !oneOf(val, "always", "multi", "never") {
			return fmt.Errorf("regroup must be always|multi|never, got %q", val)
		}
		c.Regroup = val
	case "chapters.chapter_name":
		c.ChapterName = val
	case "camera.ip":
		c.IP = val
	case "autostart.mode":
		if !oneOf(val, "notify", "auto") {
			return fmt.Errorf("autostart mode must be notify|auto, got %q", val)
		}
		c.AutostartMode = val
	default:
		return fmt.Errorf("unknown key [%s] %s", section, key)
	}
	return nil
}

// Get returns the string form of a "section.key" value.
func (c *Config) Get(dotted string) (string, error) {
	section, name, ok := strings.Cut(dotted, ".")
	if !ok {
		return "", fmt.Errorf("key must be section.name, e.g. general.dest")
	}
	switch section + "." + name {
	case "general.dest":
		return c.Dest, nil
	case "general.folder":
		return c.Folder, nil
	case "general.group_dir":
		return c.GroupDir, nil
	case "general.sidecars":
		return c.Sidecars, nil
	case "general.overwrite":
		return c.Overwrite, nil
	case "general.confirm":
		return strconv.FormatBool(c.Confirm), nil
	case "general.timezone":
		return c.Timezone, nil
	case "chapters.regroup":
		return c.Regroup, nil
	case "chapters.chapter_name":
		return c.ChapterName, nil
	case "camera.ip":
		return c.IP, nil
	case "autostart.mode":
		return c.AutostartMode, nil
	default:
		return "", fmt.Errorf("unknown key %q", dotted)
	}
}

// Set validates and applies a "section.key" value in memory.
func (c *Config) Set(dotted, val string) error {
	section, name, ok := strings.Cut(dotted, ".")
	if !ok {
		return fmt.Errorf("key must be section.name, e.g. general.dest")
	}
	return c.set(section, name, val)
}

// Save writes the config to c.Path in a stable, commented layout.
func (c *Config) Save() error {
	if c.Path == "" {
		p, err := DefaultPath()
		if err != nil {
			return err
		}
		c.Path = p
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# gpget configuration — see docs/design.md\n\n")
	b.WriteString("[general]\n")
	fmt.Fprintf(&b, "dest       = %s\n", c.Dest)
	fmt.Fprintf(&b, "folder     = %s\n", c.Folder)
	fmt.Fprintf(&b, "group_dir  = %s\n", c.GroupDir)
	fmt.Fprintf(&b, "sidecars   = %s\n", c.Sidecars)
	fmt.Fprintf(&b, "overwrite  = %s\n", c.Overwrite)
	fmt.Fprintf(&b, "confirm    = %s\n", strconv.FormatBool(c.Confirm))
	fmt.Fprintf(&b, "timezone   = %s\n", c.Timezone)
	b.WriteString("\n[chapters]\n")
	fmt.Fprintf(&b, "regroup      = %s\n", c.Regroup)
	fmt.Fprintf(&b, "chapter_name = %s\n", c.ChapterName)
	b.WriteString("\n[camera]\n")
	fmt.Fprintf(&b, "ip = %s\n", c.IP)
	b.WriteString("\n[autostart]\n")
	fmt.Fprintf(&b, "mode = %s\n", c.AutostartMode)

	tmp := c.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.Path)
}

// KnownKeys returns all settable "section.name" keys.
func KnownKeys() []string {
	out := make([]string, len(knownKeys))
	for i, k := range knownKeys {
		out[i] = k.Section + "." + k.Name
	}
	return out
}

// ExpandDest resolves ~ and returns an absolute, cleaned destination path.
func (c *Config) ExpandDest() (string, error) {
	return ExpandUser(c.Dest)
}

// ExpandUser expands a leading ~ or ~/ to the user's home directory.
func ExpandUser(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		p = filepath.Join(home, p[2:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func normalizeSidecars(val string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(val))
	switch v {
	case "none", "all":
		return v, nil
	}
	var parts []string
	seen := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p != "gpr" && p != "lrv" {
			return "", fmt.Errorf("sidecars must be none|all|gpr,lrv (comma-separated), got %q", val)
		}
		if !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "none", nil
	}
	return strings.Join(parts, ","), nil
}

func oneOf(v string, opts ...string) bool {
	for _, o := range opts {
		if v == o {
			return true
		}
	}
	return false
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "y", "1", "on":
		return true, nil
	case "false", "no", "n", "0", "off":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean: %q", v)
}

// parseClockOffset validates a `timezone` value: "camera"/"local"/"utc"/"" or
// an offset like "+9" / "-05:30". Kept in sync with plan.ParseClockOffset.
func parseClockOffset(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "camera", "local", "utc":
		return 0, nil
	}
	body := s
	switch body[0] {
	case '+', '-':
		body = body[1:]
	}
	h, m := body, "0"
	if i := strings.IndexByte(body, ':'); i >= 0 {
		h, m = body[:i], body[i+1:]
	}
	if _, e1 := strconv.Atoi(h); e1 != nil {
		return 0, fmt.Errorf("timezone: use 'camera' or an offset like +9 / -05:30, got %q", s)
	}
	if _, e2 := strconv.Atoi(m); e2 != nil {
		return 0, fmt.Errorf("timezone: use 'camera' or an offset like +9 / -05:30, got %q", s)
	}
	return 0, nil
}
