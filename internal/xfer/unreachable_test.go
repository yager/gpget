package xfer

import (
	"context"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
)

func TestIsUnreachable(t *testing.T) {
	dialRefused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	readReset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, false},
		{"canceled", context.Canceled, false},
		{"dial refused", dialRefused, true},
		{"wrapped dial", fmt.Errorf("/videos: %w", dialRefused), true},
		{"host unreachable", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}, true},
		{"net unreachable", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH}, true},
		{"read reset", readReset, true},
		{"unexpected EOF", io.ErrUnexpectedEOF, false},
		{"plain error", fmt.Errorf("short write"), false},
	}
	for _, c := range cases {
		if got := IsUnreachable(c.err); got != c.want {
			t.Errorf("%s: IsUnreachable(%v) = %v, want %v", c.name, c.err, got, c.want)
		}
	}
}
