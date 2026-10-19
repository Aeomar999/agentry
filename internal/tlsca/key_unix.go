//go:build !windows

package tlsca

import (
	"errors"
	"os"
	"syscall"
)

// openKey opens the CA key without following a link, so the file checked is
// the file read. O_NONBLOCK keeps a FIFO put in its place from blocking.
func openKey(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		return nil, linkedKeyError(path)
	}
	return f, err
}
