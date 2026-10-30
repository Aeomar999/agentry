//go:build !windows

package machines

import (
	"os"
	"path/filepath"
	"syscall"
)

// diskSpace is the size and free space of the disk dir is on, or of its
// nearest folder that exists.
func diskSpace(dir string) (total, free uint64) {
	for dir != "" && dir != "/" {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	var fs syscall.Statfs_t
	if dir == "" || syscall.Statfs(dir, &fs) != nil {
		return 0, 0
	}
	return uint64(fs.Blocks) * uint64(fs.Bsize), uint64(fs.Bavail) * uint64(fs.Bsize)
}
