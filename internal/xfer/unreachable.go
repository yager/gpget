package xfer

import (
	"errors"
	"net"
	"syscall"
)

// IsUnreachable reports whether err means the camera can no longer be reached
// (cable unplugged, host gone, dial failed). A single such error should abort
// the rest of the transfer queue.
//
// This is not `net.Error.Timeout()`: a long download of a large file can
// time out without the camera having vanished. Dial failures and the
// connection-lost errnos below are the signals that further files will fail
// the same way.
func IsUnreachable(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && unreachableErrno(errno) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) {
		if op.Op == "dial" {
			return true
		}
		if errors.As(op.Err, &errno) && unreachableErrno(errno) {
			return true
		}
	}
	return false
}

func unreachableErrno(e syscall.Errno) bool {
	switch e {
	case syscall.ECONNREFUSED, syscall.EHOSTUNREACH, syscall.ENETUNREACH,
		syscall.ECONNRESET, syscall.EPIPE:
		return true
	}
	return false
}
