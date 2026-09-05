package xfer

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Progress renders per-file and overall transfer progress. On a TTY it updates
// one line in place; otherwise it prints a line per file. Safe for nil.
type Progress struct {
	w          io.Writer
	tty        bool
	quiet      bool
	totalFiles int
	totalBytes int64

	doneFiles int
	doneBytes int64 // bytes of finished files

	curName  string
	curDone  int64
	curTotal int64
	curStart time.Time
	lastLine int
}

// NewProgress builds a progress reporter. quiet suppresses all output.
func NewProgress(totalFiles int, totalBytes int64, quiet bool) *Progress {
	fi, _ := os.Stdout.Stat()
	tty := fi != nil && fi.Mode()&os.ModeCharDevice != 0
	return &Progress{
		w: os.Stdout, tty: tty, quiet: quiet,
		totalFiles: totalFiles, totalBytes: totalBytes,
	}
}

// StartFile marks the beginning of a file transfer.
func (p *Progress) StartFile(name string, size int64) {
	if p == nil {
		return
	}
	p.curName, p.curDone, p.curTotal, p.curStart = name, 0, size, time.Now()
	p.render()
}

// Update reports current-file progress (from xfer.Download's callback).
func (p *Progress) Update(done, total int64) {
	if p == nil {
		return
	}
	p.curDone = done
	if total >= 0 {
		p.curTotal = total
	}
	p.render()
}

// FinishFile marks a file done (or skipped, with a note like "skipped").
func (p *Progress) FinishFile(bytes int64, note string) {
	if p == nil {
		return
	}
	p.doneFiles++
	p.doneBytes += bytes
	if p.quiet {
		return
	}
	p.clearLine()
	tag := "ok"
	if note != "" {
		tag = note
	}
	fmt.Fprintf(p.w, "  [%d/%d] %-40s %10s  %s\n", p.doneFiles, p.totalFiles,
		truncate(p.curName, 40), humanish(bytes), tag)
	if p.tty {
		p.render()
	}
}

// Done finishes the display.
func (p *Progress) Done() {
	if p == nil || p.quiet {
		return
	}
	p.clearLine()
	fmt.Fprintf(p.w, "%d file(s), %s\n", p.doneFiles, humanish(p.doneBytes))
}

func (p *Progress) render() {
	if p == nil || p.quiet || !p.tty {
		return
	}
	var rate string
	if el := time.Since(p.curStart).Seconds(); el > 0.5 && p.curDone > 0 {
		rate = fmt.Sprintf("  %s/s", humanish(int64(float64(p.curDone)/el)))
	}
	pct := ""
	if p.curTotal > 0 {
		pct = fmt.Sprintf(" %3d%%", p.curDone*100/p.curTotal)
	}
	line := fmt.Sprintf("[%d/%d] %s  %s/%s%s%s",
		p.doneFiles+1, p.totalFiles, truncate(p.curName, 28),
		humanish(p.curDone), humanish(p.curTotal), pct, rate)
	p.clearLine()
	fmt.Fprint(p.w, line)
	p.lastLine = len(line)
}

func (p *Progress) clearLine() {
	if p.tty && p.lastLine > 0 {
		fmt.Fprint(p.w, "\r"+strings.Repeat(" ", p.lastLine)+"\r")
		p.lastLine = 0
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
