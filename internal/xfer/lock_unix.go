//go:build !windows

package xfer

import (
	"os"
	"syscall"
)

var syscallZero os.Signal = syscall.Signal(0)
