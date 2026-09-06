//go:build darwin && cgo

package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
int gpgetNotifySettings(char *buf, int cap);
*/
import "C"

import "unsafe"

// Settings returns what macOS itself reports about this app's notification
// settings, or "" when it cannot be read (not running inside the bundle).
// Diagnostic only: nothing branches on it.
func Settings() string {
	const cap = 4096
	buf := (*C.char)(C.malloc(cap))
	defer C.free(unsafe.Pointer(buf))
	n := C.gpgetNotifySettings(buf, C.int(cap))
	if n <= 0 {
		return ""
	}
	return C.GoStringN(buf, n)
}
