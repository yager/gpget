package xfer

import (
	"context"
	"errors"
)

// QueueResult is the outcome of ProcessItems.
type QueueResult struct {
	OK, Skip, Fail int
	// Left is the number of items not attempted because the queue stopped.
	Left int
	// Unreachable is the camera-loss error that stopped the queue, if any.
	Unreachable error
	// Canceled is set when the parent context ended (Ctrl-C).
	Canceled bool
	// TimedOut is set when the transfer budget elapsed.
	TimedOut bool
	FirstErr error
}

// ProcessItems transfers each item in order. Per-file errors (404, short
// write, stale .part) are counted and the queue continues; a camera that
// can no longer be reached stops the rest of the queue so a pulled cable
// does not produce one error line per remaining file.
func ProcessItems(ctx context.Context, items []Request, start func(Request), done func(Request, Result, error)) QueueResult {
	var qr QueueResult
	for i, it := range items {
		if err := ctx.Err(); err != nil {
			qr.Left = len(items) - i
			noteContextStop(&qr, err)
			if qr.FirstErr == nil {
				qr.FirstErr = err
			}
			return qr
		}
		if start != nil {
			start(it)
		}
		res, err := timedDownload(ctx, it)
		if done != nil {
			done(it, res, err)
		}
		switch {
		case err != nil && errors.Is(err, ErrStalePart):
			qr.Fail++
			if qr.FirstErr == nil {
				qr.FirstErr = err
			}
		case err != nil && IsUnreachable(err):
			qr.Fail++
			qr.Unreachable = err
			if qr.FirstErr == nil {
				qr.FirstErr = err
			}
			qr.Left = len(items) - i - 1
			return qr
		case err != nil && errors.Is(err, context.DeadlineExceeded):
			qr.Fail++
			qr.TimedOut = true
			if qr.FirstErr == nil {
				qr.FirstErr = err
			}
			qr.Left = len(items) - i - 1
			return qr
		case err != nil && errors.Is(err, context.Canceled):
			qr.Fail++
			qr.Canceled = true
			if qr.FirstErr == nil {
				qr.FirstErr = err
			}
			qr.Left = len(items) - i - 1
			return qr
		case err != nil:
			qr.Fail++
			if qr.FirstErr == nil {
				qr.FirstErr = err
			}
		case res.Skipped:
			qr.Skip++
		default:
			qr.OK++
		}
	}
	return qr
}

func noteContextStop(qr *QueueResult, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		qr.TimedOut = true
		return
	}
	qr.Canceled = true
}
