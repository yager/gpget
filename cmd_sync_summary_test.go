package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yager/gpget/internal/xfer"
)

func TestTransferSummaryUnreachable(t *testing.T) {
	got := transferSummary(xfer.QueueResult{
		OK: 1, Fail: 1, Left: 2,
		Unreachable: errors.New("connection refused"),
	})
	if !strings.Contains(got, "camera unreachable") {
		t.Fatalf("got %q, want camera unreachable", got)
	}
	if !strings.Contains(got, "2 remaining") {
		t.Fatalf("got %q, want remaining count", got)
	}
}

func TestTransferSummaryTimedOut(t *testing.T) {
	got := transferSummary(xfer.QueueResult{
		OK: 1, Fail: 1, Left: 2,
		TimedOut: true,
		FirstErr: context.DeadlineExceeded,
	})
	if strings.Contains(got, "camera unreachable") {
		t.Fatalf("deadline must not look like camera loss: %q", got)
	}
	if strings.Contains(got, "interrupted") {
		t.Fatalf("deadline must not look like Ctrl-C: %q", got)
	}
	if !strings.Contains(got, "timed out") {
		t.Fatalf("got %q, want timed out", got)
	}
}

func TestTransferSummaryCanceled(t *testing.T) {
	got := transferSummary(xfer.QueueResult{
		OK: 1, Fail: 1, Left: 2,
		Canceled: true,
		FirstErr: context.Canceled,
	})
	if strings.Contains(got, "camera unreachable") {
		t.Fatalf("Ctrl-C must not look like camera loss: %q", got)
	}
	if !strings.Contains(got, "interrupted") {
		t.Fatalf("got %q, want interrupted", got)
	}
}

func TestTransferSummaryLeftWithoutReason(t *testing.T) {
	got := transferSummary(xfer.QueueResult{OK: 1, Left: 2})
	if strings.Contains(got, "camera unreachable") {
		t.Fatalf("unknown stop must not look like camera loss: %q", got)
	}
	if !strings.Contains(got, "2 remaining") {
		t.Fatalf("got %q, want remaining count", got)
	}
}

func TestTransferSummaryNormal(t *testing.T) {
	got := transferSummary(xfer.QueueResult{OK: 3, Skip: 1})
	want := "transferred 3, skipped 1, failed 0"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
