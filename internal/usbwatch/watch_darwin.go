//go:build darwin && cgo

// Package usbwatch reports USB attach/detach for a vendor, using IOKit
// notifications rather than polling. The process must stay resident.
package usbwatch

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
int gpgetWatchUSB(int vendorID);
*/
import "C"

import (
	"errors"
	"runtime"
	"sync"
)

var (
	mu       sync.Mutex
	onAttach func()
	onDetach func()
)

//export gpgetUSBAttached
func gpgetUSBAttached() {
	mu.Lock()
	f := onAttach
	mu.Unlock()
	if f != nil {
		f()
	}
}

//export gpgetUSBDetached
func gpgetUSBDetached() {
	mu.Lock()
	f := onDetach
	mu.Unlock()
	if f != nil {
		f()
	}
}

// Supported reports whether this build can watch for devices.
func Supported() bool { return true }

// Run blocks forever, calling attach/detach as devices come and go. attach also
// fires once at startup for a device that is already connected.
//
// The callbacks run on the run loop's thread; keep them from blocking for long,
// or dispatch the work elsewhere.
func Run(vendorID int, attach, detach func()) error {
	mu.Lock()
	onAttach, onDetach = attach, detach
	mu.Unlock()

	runtime.LockOSThread() // CFRunLoopRun is tied to the calling thread
	defer runtime.UnlockOSThread()

	if C.gpgetWatchUSB(C.int(vendorID)) == 0 {
		return errors.New("could not start the IOKit USB watcher")
	}
	return nil // CFRunLoopRun returned, which normally does not happen
}
