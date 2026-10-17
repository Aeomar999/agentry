//go:build windows

package machines

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// diskSpace is the size and free space of the disk dir is on, or of its
// nearest folder that exists.
func diskSpace(dir string) (total, free uint64) {
	for dir != "" && dir != filepath.Dir(dir) {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0
	}
	var available uint64
	if windows.GetDiskFreeSpaceEx(path, &available, &total, nil) != nil {
		return 0, 0
	}
	return total, available
}
