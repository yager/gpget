//go:build darwin && cgo

package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
int gpgetNotifyNative(const char *cid, const char *ctitle, const char *cbody);
*/
import "C"

import "unsafe"

// native posts through UserNotifications so the banner is attributed to gpget
// itself. Only meaningful inside the .app bundle; returns false otherwise.
// A non-empty id replaces the previous request with that identifier.
func native(id, title, body string) bool {
	var cid *C.char
	if id != "" {
		cid = C.CString(id)
		defer C.free(unsafe.Pointer(cid))
	}
	t := C.CString(title)
	b := C.CString(body)
	defer C.free(unsafe.Pointer(t))
	defer C.free(unsafe.Pointer(b))
	return C.gpgetNotifyNative(cid, t, b) == 1
}
