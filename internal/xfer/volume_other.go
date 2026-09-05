//go:build !darwin && !linux

package xfer

import "errors"

// statVolume is not implemented on this OS yet (Windows: free-space and
// filesystem-type checks are a TODO — the cloud-path denylist and the real
// test write still run).
func statVolume(dir string) (volumeInfo, error) {
	return volumeInfo{}, errors.New("statVolume: unsupported OS")
}
