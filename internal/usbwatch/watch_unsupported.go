//go:build !darwin || !cgo

package usbwatch

import "errors"

// Supported reports whether this build can watch for devices.
func Supported() bool { return false }

// Run is unavailable on this platform.
func Run(_ int, _, _ func()) error {
	return errors.New("USB watching is not supported on this platform")
}
