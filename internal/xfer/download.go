package xfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yager/gpget/internal/gopro"
)

// ErrStalePart means a leftover .part could not be matched to the current media
// item (card reformatted / numbers wrapped / template changed). The .part is
// left in place for the user to review.
var ErrStalePart = errors.New("leftover .part does not match the current media item")

const (
	maxResumeRetries = 3
	copyChunk        = 512 << 10
)

// Request describes one file transfer.
type Request struct {
	Client    *gopro.Client
	Dir, Src  string
	FinalPath string // absolute
	ExpSize   int64  // -1 if unknown (GPR / group frame)
	Cre       int64
	Overwrite string    // "skip" | "rename" | "replace"
	ModTime   time.Time // set on the finalized file (capture wall-clock time)

	// Progress, if set, is called with (bytesDone, bytesTotal). Total is -1
	// while unknown.
	Progress func(done, total int64)
}

// Result reports the outcome of a transfer.
type Result struct {
	Path     string // final path actually written
	Bytes    int64
	Skipped  bool // already present, nothing transferred
	Resumed  bool // continued an existing .part
	Verified bool // final size checked against an expected value
}

// Download performs one safe transfer: .part staging, exact-size verification,
// Range resume, atomic finalize.
func Download(ctx context.Context, r Request) (Result, error) {
	final := r.FinalPath
	part := partPath(final)

	// 1. incremental skip / overwrite policy (F-5)
	if fi, err := os.Stat(final); err == nil && !fi.IsDir() {
		if r.ExpSize >= 0 && fi.Size() == r.ExpSize {
			return Result{Path: final, Skipped: true, Verified: true}, nil
		}
		switch r.Overwrite {
		case "skip", "":
			if r.ExpSize < 0 {
				// Size unknown (GPR / some frames): presence is enough.
				return Result{Path: final, Skipped: true}, nil
			}
			// Known size but it does not match: the file is not complete.
			// Fall through and replace it. "skip" means skip-if-complete.
		case "rename":
			final = uniqueName(final)
			part = partPath(final)
		case "replace":
			// proceed; finalize will replace
		}
	}

	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return Result{}, err
	}

	// 2. resume decision for an existing .part
	var startAt int64
	var complete bool // .part already holds every expected byte
	if pi, err := os.Stat(part); err == nil {
		meta, ok := ReadMeta(part)
		matches := ok &&
			strings.EqualFold(normalizeSrc(meta.Src), normalizeSrc(r.Dir+"/"+r.Src)) &&
			(meta.Cre == 0 || r.Cre == 0 || meta.Cre == r.Cre)
		switch {
		case !matches:
			return Result{}, fmt.Errorf("%s: %w", filepath.Base(part), ErrStalePart)
		case r.ExpSize >= 0 && pi.Size() > r.ExpSize:
			// Longer than the camera says the file is: it cannot be a prefix of
			// the real file, so appending to it would only make it worse.
			return Result{}, fmt.Errorf("%s: larger than expected (%d of %d bytes) — delete it and transfer again",
				filepath.Base(part), pi.Size(), r.ExpSize)
		case r.ExpSize >= 0 && pi.Size() == r.ExpSize:
			// Every byte is already on disk. Do NOT ask the camera to resume:
			// a Range starting at EOF is out of range, and this camera answers
			// it with 206 plus zero bytes (and a nonsensical Content-Range)
			// rather than 416 -- measured on MISSION 1 PRO ILS, 2026-09-05.
			// Streaming that would leave the byte count short and fail forever.
			complete = true
			startAt = pi.Size()
		default:
			startAt = pi.Size()
		}
	}

	if err := WriteMeta(final, PartMeta{Src: r.Dir + "/" + r.Src, ExpSize: r.ExpSize, Cre: r.Cre}); err != nil {
		return Result{}, err
	}

	written, total, resumed := startAt, int64(-1), startAt > 0
	if !complete {
		var err error
		written, total, resumed, err = stream(ctx, r, part, startAt)
		if err != nil {
			// Nothing on disk worth resuming: drop the empty staging files so a
			// pulled cable does not leave one 0-byte pair per remaining item.
			if startAt == 0 && written == 0 {
				os.Remove(part)
				RemoveMeta(final)
			}
			return Result{}, err
		}
	}

	// 3. verification
	expected := r.ExpSize
	if expected < 0 {
		expected = total // Content-Length fallback
	}
	verified := expected >= 0
	if verified && written != expected {
		return Result{}, fmt.Errorf("%s: incomplete (%d of %d bytes); kept %s",
			r.Src, written, expected, filepath.Base(part))
	}

	// 4. atomic finalize
	if err := finalize(part, final); err != nil {
		return Result{}, err
	}
	RemoveMeta(final)

	// 5. restore the capture time (best effort)
	if !r.ModTime.IsZero() {
		_ = os.Chtimes(final, r.ModTime, r.ModTime)
	}

	return Result{Path: final, Bytes: written, Resumed: resumed, Verified: verified}, nil
}

// stream copies the file body to part, resuming from startAt, with bounded
// Range-based retries on transient read errors.
func stream(ctx context.Context, r Request, part string, startAt int64) (written, total int64, resumed bool, err error) {
	// Not O_APPEND: a 200 answer to a Range request has to be recoverable by
	// rewinding to zero, and O_APPEND would force every write to the end.
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, -1, false, err
	}
	defer f.Close()
	if startAt > 0 {
		if _, err := f.Seek(startAt, io.SeekStart); err != nil {
			return 0, -1, false, err
		}
	} else if err := f.Truncate(0); err != nil {
		return 0, -1, false, err
	}

	written = startAt
	total = -1
	retries := 0

	for {
		body, t, res, oerr := r.Client.OpenFile(ctx, r.Dir, r.Src, written)
		if oerr != nil {
			return written, total, resumed, oerr
		}
		if t >= 0 {
			total = t
		}
		// We asked to continue but got the whole file back (200). Appending it
		// would corrupt the .part, so start over from zero instead. Not seen on
		// this camera, but a plain 200 is a legal answer to a Range request.
		if written > 0 && !res {
			if err := f.Truncate(0); err != nil {
				body.Close()
				return written, total, resumed, err
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				body.Close()
				return written, total, resumed, err
			}
			written = 0
		}
		resumed = resumed || res || startAt > 0

		cerr := copyBody(ctx, f, body, &written, total, r.Progress)
		body.Close()

		if cerr == nil {
			if err := f.Sync(); err != nil {
				return written, total, resumed, err
			}
			return written, total, resumed, nil
		}

		// The connection can break *after* the last byte arrived. Every
		// expected byte is on disk, so the transfer objectively succeeded --
		// reporting it as a failure would strand a complete .part.
		if want := expectedBytes(r.ExpSize, total); want >= 0 && written >= want {
			if err := f.Sync(); err != nil {
				return written, total, resumed, err
			}
			return written, total, resumed, nil
		}

		if ctx.Err() != nil {
			return written, total, resumed, ctx.Err()
		}
		// transient? retry with Range from where we are.
		if retries >= maxResumeRetries {
			return written, total, resumed, cerr
		}
		retries++
		time.Sleep(time.Duration(retries) * 500 * time.Millisecond)
	}
}

func copyBody(ctx context.Context, dst *os.File, src io.Reader, written *int64, total int64, prog func(done, total int64)) error {
	// *written advances only after dst.Write succeeds. The "every expected
	// byte is on disk, so an interrupted connection still counts as success"
	// check therefore cannot fire on a write failure. If this accounting
	// changes, that check has to be revisited too.
	buf := make([]byte, copyChunk)
	last := time.Now()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			*written += int64(n)
			if prog != nil && time.Since(last) > 100*time.Millisecond {
				prog(*written, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			if prog != nil {
				prog(*written, total)
			}
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// finalize moves the staged .part into place.
//
// Rename first, always: on POSIX it replaces the destination atomically, so
// there is never a moment where neither file exists. Removing the destination
// beforehand would open exactly that window and lose the original if the
// process died inside it. Windows refuses to rename onto an existing file, so
// there the remove is unavoidable -- and only there.
func finalize(part, final string) error {
	err := os.Rename(part, final)
	if err == nil {
		return nil
	}
	// Only Windows's refusal to rename onto an existing file justifies removing
	// the destination -- and only while the staged file is still there to put
	// in its place. Removing it for any other failure would destroy the user's
	// existing file and leave nothing behind.
	if _, serr := os.Stat(part); serr != nil {
		return fmt.Errorf("finalize %s: %w", final, err)
	}
	if _, serr := os.Stat(final); serr != nil {
		return fmt.Errorf("finalize %s: %w", final, err)
	}
	if rerr := os.Remove(final); rerr != nil {
		return fmt.Errorf("cannot replace existing %s: %w", final, rerr)
	}
	if rerr := os.Rename(part, final); rerr != nil {
		return fmt.Errorf("finalize %s: %w", final, rerr)
	}
	return nil
}

// expectedBytes picks the authoritative size: what the media list said, or the
// Content-Length if the list did not know. -1 when neither is known.
func expectedBytes(listed, contentLength int64) int64 {
	if listed >= 0 {
		return listed
	}
	return contentLength
}

func uniqueName(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
			return cand
		}
	}
}
