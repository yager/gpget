package plan

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Vars holds the values a template can reference.
type Vars struct {
	Date    time.Time // capture time, already in the target location
	Label   string    // --label
	Model   string    // camera model, sanitized (e.g. "MISSION1PROILS")
	Cam     string    // camera name / ap_ssid (e.g. "GoPro 12345678")
	Stem    string    // filename stem (no extension), e.g. "GPAA0015"
	Prefix  string    // GoPro filename prefix, e.g. "GX"
	Clip    string    // 4-digit clip number
	Chapter int       // chapter number (1..)
}

// Render expands {token} / {token:spec} in tmpl.
//
//	{date:%Y-%m-%d}   strftime of Date
//	{chapter:02d}     printf of Chapter
//	{label} {model} {cam} {stem} {prefix} {clip}   plain substitution
//
// An unknown token is left verbatim (so a typo is visible, not silently empty).
// The result is cleaned of path-hostile characters.
func Render(tmpl string, v Vars) (string, error) {
	var b strings.Builder
	for i := 0; i < len(tmpl); {
		c := tmpl[i]
		if c != '{' {
			b.WriteByte(c)
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated { in template %q", tmpl)
		}
		token := tmpl[i+1 : i+end]
		i += end + 1

		name, spec, hasSpec := strings.Cut(token, ":")
		out, ok, err := renderToken(name, spec, hasSpec, v)
		if err != nil {
			return "", err
		}
		if !ok {
			b.WriteByte('{')
			b.WriteString(token)
			b.WriteByte('}')
			continue
		}
		b.WriteString(out)
	}
	return sanitizePathSegment(b.String()), nil
}

func renderToken(name, spec string, hasSpec bool, v Vars) (string, bool, error) {
	switch name {
	case "date":
		if !hasSpec {
			spec = "%Y-%m-%d"
		}
		return strftime(v.Date, spec), true, nil
	case "chapter":
		if !hasSpec {
			return strconv.Itoa(v.Chapter), true, nil
		}
		// spec like "02d" -> "%02d"
		return fmt.Sprintf("%"+spec, v.Chapter), true, nil
	case "label":
		return v.Label, true, nil
	case "model":
		return v.Model, true, nil
	case "cam":
		return v.Cam, true, nil
	case "stem":
		return v.Stem, true, nil
	case "prefix":
		return v.Prefix, true, nil
	case "clip":
		return v.Clip, true, nil
	default:
		return "", false, nil
	}
}

// strftime implements the subset of format verbs gpget templates use.
func strftime(t time.Time, f string) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+1 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		i++
		switch f[i] {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(f[i])
		}
	}
	return b.String()
}

// sanitizePathSegment removes characters that are illegal or troublesome in a
// path segment on macOS/Windows/Linux. It keeps unicode letters (Japanese
// labels etc.) and collapses whitespace runs.
func sanitizePathSegment(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			b.WriteByte('_')
			prevSpace = false
		case ' ', '\t', '\n':
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	out := strings.TrimSpace(b.String())
	// no path segment may be "." or ".." or end in "." (Windows)
	out = strings.TrimRight(out, ".")
	if out == "" {
		out = "_"
	}
	return out
}

// SanitizeModel turns "MISSION 1 PRO ILS" into "MISSION1PROILS".
func SanitizeModel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '-' || r == '_' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
