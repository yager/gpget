//go:build linux

package xfer

import "syscall"

// filesystem magic numbers (linux/magic.h)
const (
	magicMSDOS = 0x4d44
	magicEXFAT = 0x2011bab0
	magicNFS   = 0x6969
	magicSMB   = 0x517b
	magicSMB2  = 0xfe534d42
	magicCIFS  = 0xff534d42
	magicFUSE  = 0x65735546
	stRdonly   = 0x1 // linux/statfs.h ST_RDONLY
)

func statVolume(dir string) (volumeInfo, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return volumeInfo{}, err
	}
	vi := volumeInfo{
		freeBytes: int64(st.Bavail) * st.Bsize,
		readOnly:  st.Flags&stRdonly != 0,
	}
	switch int64(st.Type) {
	case magicNFS, magicSMB, magicSMB2, magicCIFS:
		vi.network = true
		vi.fstype = "network"
	case magicMSDOS:
		vi.fstype = "vfat"
		vi.maxFile = 4<<30 - 1
	case magicEXFAT:
		vi.fstype = "exfat"
	case magicFUSE:
		// FUSE mounts (sshfs, rclone mount, gvfs, ...) cannot be confirmed local.
		vi.fstype = "fuse"
		vi.network = true
	}
	return vi, nil
}
