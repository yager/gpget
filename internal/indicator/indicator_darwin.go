//go:build darwin && cgo

package indicator

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit
#include <stdlib.h>

void gpgetIndicatorStart(void);
void gpgetIndicatorSetIdle(const char *last);
void gpgetIndicatorSetProgress(int done, int total, const char *detail);
void gpgetIndicatorSetMessage(const char *msg);
// Write idle + progress PNGs under dir for visual review (no menu bar needed).
// Returns 1 on success, 0 on failure.
int gpgetIndicatorWritePreview(const char *dir);
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"unsafe"
)

func start() {
	C.gpgetIndicatorStart()
}

func setIdle(last string) {
	c := C.CString(last)
	defer C.free(unsafe.Pointer(c))
	C.gpgetIndicatorSetIdle(c)
}

func setProgress(done, total int, detail string) {
	c := C.CString(detail)
	defer C.free(unsafe.Pointer(c))
	C.gpgetIndicatorSetProgress(C.int(done), C.int(total), c)
}

func setMessage(msg string) {
	c := C.CString(msg)
	defer C.free(unsafe.Pointer(c))
	C.gpgetIndicatorSetMessage(c)
}

// WritePreview renders the current idle/progress artwork to PNG files under
// dir (created if needed). Used to iterate on icon design without reinstalling
// the menu-bar agent. Darwin+cgo only.
func WritePreview(dir string) error {
	if dir == "" {
		return fmt.Errorf("preview dir is empty")
	}
	c := C.CString(dir)
	defer C.free(unsafe.Pointer(c))
	if C.gpgetIndicatorWritePreview(c) != 1 {
		return fmt.Errorf("could not write indicator preview PNGs to %s", dir)
	}
	return nil
}

//export gpgetIndicatorOnSyncNow
func gpgetIndicatorOnSyncNow() {
	h := currentHooks()
	if h.SyncNow != nil {
		h.SyncNow()
	}
}

//export gpgetIndicatorOnOpenDest
func gpgetIndicatorOnOpenDest() {
	h := currentHooks()
	dir := ""
	if h.DestDir != nil {
		dir = h.DestDir()
	}
	if dir == "" {
		return
	}
	_ = exec.Command("open", dir).Start()
}

//export gpgetIndicatorOnOpenLog
func gpgetIndicatorOnOpenLog() {
	h := currentHooks()
	if h.LogPath == "" {
		return
	}
	if _, err := os.Stat(h.LogPath); err != nil {
		// Reveal the parent folder if the log has not been written yet.
		_ = exec.Command("open", "-R", h.LogPath).Start()
		return
	}
	_ = exec.Command("open", "-R", h.LogPath).Start()
}

//export gpgetIndicatorOnQuit
func gpgetIndicatorOnQuit() {
	h := currentHooks()
	if h.Quit != nil {
		h.Quit()
	}
}
