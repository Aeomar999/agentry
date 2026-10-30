//go:build windows

package tlsca

import (
	"io/fs"
	"os"
)

// openKey opens the CA key, refusing a link. Windows has no O_NOFOLLOW, so
// the link is looked for before the file is opened.
func openKey(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, linkedKeyError(path)
	}
	return os.Open(path)
}
