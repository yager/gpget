package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	autostartLogMaxBytes  = 1 << 20 // 1 MiB, then keep one .old
	autostartLogTailLines = 80
)

// autostartLogPath is the file `gpget autostart log` reads. On macOS it is
// also LaunchAgent StandardOutPath, so agent + worker share one trail.
func autostartLogPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", "gpget-autostart.log")
	case "windows":
		dir, err := os.UserCacheDir() // %LOCALAPPDATA%
		if err != nil || dir == "" {
			dir = os.TempDir()
		}
		return filepath.Join(dir, "gpget", "autostart.log")
	default:
		state := os.Getenv("XDG_STATE_HOME")
		if state == "" {
			home, _ := os.UserHomeDir()
			state = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(state, "gpget", "autostart.log")
	}
}

var autostartLogFile *os.File

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func rotateAutostartLog(path string, maxBytes int64) {
	if maxBytes <= 0 {
		return
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() < maxBytes {
		return
	}
	old := path + ".old"
	_ = os.Remove(old)
	_ = os.Rename(path, old)
}

func openAutostartLog() *os.File {
	path := autostartLogPath()
	rotateAutostartLog(path, autostartLogMaxBytes)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
}

func lastNLines(b []byte, n int) string {
	if n <= 0 || len(b) == 0 {
		return ""
	}
	end := len(b)
	count := 0
	for i := end - 1; i >= 0; i-- {
		if b[i] != '\n' {
			continue
		}
		if i == end-1 {
			continue
		}
		count++
		if count == n {
			return string(b[i+1:])
		}
	}
	return string(b)
}

// readAutostartLogBytes concatenates path+".old" then path, in write order,
// so a rotation is invisible to `gpget autostart log`.
func readAutostartLogBytes(path string) ([]byte, error) {
	var buf []byte
	old, err := os.ReadFile(path + ".old")
	switch {
	case err == nil:
		buf = old
		if len(buf) > 0 && buf[len(buf)-1] != '\n' {
			buf = append(buf, '\n')
		}
	case !os.IsNotExist(err):
		return nil, err
	}
	cur, err := os.ReadFile(path)
	switch {
	case err == nil:
		return append(buf, cur...), nil
	case os.IsNotExist(err):
		if len(buf) == 0 {
			return nil, err
		}
		return buf, nil
	default:
		return nil, err
	}
}

func autostartShowLog(ctx context.Context, follow bool) error {
	path := autostartLogPath()
	b, err := readAutostartLogBytes(path)
	switch {
	case err == nil:
		out := lastNLines(b, autostartLogTailLines)
		if out == "" {
			fmt.Println("(empty)")
		} else {
			fmt.Print(out)
			if !strings.HasSuffix(out, "\n") {
				fmt.Println()
			}
		}
	case os.IsNotExist(err):
		fmt.Println("no log yet")
	default:
		return err
	}
	if !follow {
		return nil
	}
	return followLog(ctx, path, fileSize(path))
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

func followLog(ctx context.Context, path string, offset int64) error {
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			st, err := f.Stat()
			if err != nil {
				f.Close()
				continue
			}
			size := st.Size()
			if size < offset {
				offset = 0 // rotated
			}
			if size > offset {
				if _, err := f.Seek(offset, io.SeekStart); err == nil {
					n, _ := io.Copy(os.Stdout, f)
					offset += n
				}
			}
			f.Close()
		}
	}
}
