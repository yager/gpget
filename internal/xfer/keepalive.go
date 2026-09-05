package xfer

import (
	"context"
	"time"

	"github.com/yager/gpget/internal/gopro"
)

// KeepAlive pings the camera every interval until ctx is cancelled. GoPro drops
// the wired link after some idle time; this holds it open during a long
// transfer. Errors are ignored (the transfer itself will surface a real drop).
func KeepAlive(ctx context.Context, cl *gopro.Client, interval time.Duration) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = cl.KeepAlive(ctx)
		}
	}
}
