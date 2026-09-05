package xfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Preflight validates a destination directory before any transfer:
//   - not a cloud-synced folder (iCloud / Google Drive / Dropbox / OneDrive)
//   - not a network volume (SMB / NFS / WebDAV)
//   - writable, not read-only
//   - enough free space for needBytes
//   - if FAT32/FAT16, largestFile fits the volume's single-file limit
//   - a real test write succeeds
//
// It creates the directory (and parents) if missing.
func Preflight(dir string, needBytes, largestFile int64) error {
	if err := checkCloudPath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	vi, err := statVolume(dir)
	if err == nil {
		if vi.network {
			return fmt.Errorf("%s is on a network volume (%s) — choose a local disk or a directly connected drive", dir, vi.fstype)
		}
		if vi.readOnly {
			return fmt.Errorf("%s is read-only", dir)
		}
		if vi.maxFile > 0 && largestFile > vi.maxFile {
			return fmt.Errorf("%s (%s) cannot hold a file this large: largest is %s, volume limit is %s",
				dir, vi.fstype, humanish(largestFile), humanish(vi.maxFile))
		}
		if !spaceOK(vi.freeBytes, needBytes) {
			return fmt.Errorf("not enough free space in %s: need %s, free %s",
				dir, humanish(needBytes), humanish(vi.freeBytes))
		}
	}

	return testWrite(dir)
}

// spaceOK reports whether need bytes fit in free.
//
// free < 0 means unknown (statVolume failed). That stays fail-open so Windows,
// which has no statVolume yet, is not blocked. free == 0 is a full disk.
func spaceOK(free, need int64) bool {
	if free < 0 {
		return true
	}
	return need <= free
}

// FreeBytes returns the free space at dir, or -1 if it cannot be determined.
func FreeBytes(dir string) int64 {
	if vi, err := statVolume(dir); err == nil {
		return vi.freeBytes
	}
	return -1
}

type volumeInfo struct {
	fstype    string
	network   bool
	readOnly  bool
	freeBytes int64
	maxFile   int64 // 0 == effectively unlimited / unknown
}

func checkCloudPath(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	abs = filepath.Clean(abs)
	lower := strings.ToLower(filepath.ToSlash(abs))

	home, _ := os.UserHomeDir()
	if home != "" {
		h := strings.ToLower(filepath.ToSlash(home))
		for _, root := range []string{h + "/library/cloudstorage", h + "/library/mobile documents"} {
			if lower == root || strings.HasPrefix(lower, root+"/") {
				return fmt.Errorf("%s is a cloud-synced folder — GoPro clips are large; choose a local disk or an external drive", dir)
			}
		}
	}
	for _, frag := range []string{
		"/onedrive", "/dropbox", "/google drive", "/googledrive",
		"/library/cloudstorage", "/icloud", "/pcloud", "/box sync",
	} {
		if strings.Contains(lower, frag) {
			return fmt.Errorf("%s looks like a cloud-synced folder (%q) — choose a local disk or an external drive", dir, strings.TrimPrefix(frag, "/"))
		}
	}
	return nil
}

func testWrite(dir string) error {
	f, err := os.CreateTemp(dir, ".gpget-write-test-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}
