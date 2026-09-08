package xfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// timingSink, when non-nil, gets one tab-separated line per finished transfer:
//
//	TIMING\t<src>\t<bytes>\t<seconds>\t<status>
//
// status is one of ok | skip | resumed | err. It is enabled by setting
// GPGET_TIMING to any non-empty value and exists to measure per-file transfer
// time (stall analysis). It changes no behaviour.
var timingSink io.Writer

func init() {
	if os.Getenv("GPGET_TIMING") != "" {
		timingSink = os.Stderr
	}
}

// timedDownload runs Download, recording how long it took when timing is
// enabled. The result and error pass through unchanged.
func timedDownload(ctx context.Context, it Request) (Result, error) {
	if timingSink == nil {
		return Download(ctx, it)
	}
	t0 := time.Now()
	res, err := Download(ctx, it)
	status := "ok"
	switch {
	case err != nil:
		status = "err"
	case res.Skipped:
		status = "skip"
	case res.Resumed:
		status = "resumed"
	}
	fmt.Fprintf(timingSink, "TIMING\t%s\t%d\t%.3f\t%s\n",
		it.Src, res.Bytes, time.Since(t0).Seconds(), status)
	return res, err
}
