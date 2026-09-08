package xfer

import (
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

// errIdleStall means a streaming body delivered no data for idleReadTimeout.
// The connection is almost certainly wedged in TCP retransmission -- measured
// on MISSION 1 PRO ILS, mid-stream freezes of 7-420s with the transfer resuming
// at full speed afterwards. stream() treats this like any transient read error
// and retries with a fresh Range request, which starts within ~30ms instead of
// waiting the freeze out.
var errIdleStall = errors.New("stalled: no data within idle read timeout")

var (
	// idleReadTimeout: how long a streaming body may deliver nothing before the
	// connection is dropped and resumed. Above the ~7s self-recovering blips
	// seen in the field, below the transport's 20s ResponseHeaderTimeout.
	idleReadTimeout = envSeconds("GPGET_IDLE_TIMEOUT", 15*time.Second)

	// maxResumeRetries: Range-resume attempts per file before giving up. Total
	// patience per file is roughly maxResumeRetries * idleReadTimeout; a file
	// that gives up keeps its .part and resumes on the next run.
	maxResumeRetries = envCount("GPGET_MAX_RESUMES", 6)
)

func envSeconds(key string, def time.Duration) time.Duration {
	if n, ok := envPositive(key); ok {
		return time.Duration(n) * time.Second
	}
	return def
}

func envCount(key string, def int) int {
	if n, ok := envPositive(key); ok {
		return n
	}
	return def
}

func envPositive(key string) (int, bool) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// idleReader wraps a response body. A Read that blocks longer than timeout is
// unblocked by closing the body, and then reported as errIdleStall. The timer
// is rearmed each time bytes arrive, so a healthy stream never trips it.
type idleReader struct {
	rc      io.ReadCloser
	timeout time.Duration
	timer   *time.Timer

	mu      sync.Mutex
	tripped bool
}

func newIdleReader(rc io.ReadCloser, timeout time.Duration) *idleReader {
	ir := &idleReader{rc: rc, timeout: timeout}
	ir.timer = time.AfterFunc(timeout, ir.trip)
	return ir
}

func (ir *idleReader) trip() {
	ir.mu.Lock()
	ir.tripped = true
	ir.mu.Unlock()
	_ = ir.rc.Close() // unblocks a Read stuck on the wedged connection
}

func (ir *idleReader) Read(p []byte) (int, error) {
	n, err := ir.rc.Read(p)
	if n > 0 {
		ir.mu.Lock()
		if !ir.tripped {
			ir.timer.Reset(ir.timeout)
		}
		ir.mu.Unlock()
	}
	// Only report a stall when the read came back empty. If the trip races with
	// the final bytes, let those bytes through; the caller detects completion.
	if err != nil && n == 0 {
		ir.mu.Lock()
		tripped := ir.tripped
		ir.mu.Unlock()
		if tripped {
			return 0, errIdleStall
		}
	}
	return n, err
}

func (ir *idleReader) Close() error {
	ir.timer.Stop()
	return ir.rc.Close()
}
