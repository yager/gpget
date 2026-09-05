//go:build darwin

package xfer

import "syscall"

func statVolume(dir string) (volumeInfo, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return volumeInfo{}, err
	}
	const mntRDONLY = 0x1 // sys/mount.h MNT_RDONLY
	vi := volumeInfo{
		fstype:    int8ToString(st.Fstypename[:]),
		freeBytes: int64(st.Bavail) * int64(st.Bsize),
		readOnly:  st.Flags&mntRDONLY != 0,
	}
	switch vi.fstype {
	case "smbfs", "nfs", "webdav", "afpfs", "ftp":
		vi.network = true
	case "msdos", "vfat", "fat", "fat32", "exfatfs":
		if vi.fstype != "exfatfs" {
			vi.maxFile = 4<<30 - 1 // FAT32 single-file limit
		}
	}
	return vi, nil
}

func int8ToString(b []int8) string {
	buf := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}
