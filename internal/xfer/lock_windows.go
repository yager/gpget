//go:build windows

package xfer

import "os"

// On Windows we can't cheaply send signal 0; processAlive falls back to
// "assume held" when syscallZero is nil.
var syscallZero os.Signal
