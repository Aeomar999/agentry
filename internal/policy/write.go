package policy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// backupTime names backups so they sort by time, without characters some
// file systems refuse.
const backupTime = "20060102T150405.000000000Z"

// WriteFile replaces the policy file at path with text, for a change
// Agentry makes itself. It refuses text that wouldn't load. The file as it
// was is first copied to path + "-backup-" + the time, next to it, and the
// new text is written to a temporary file there and renamed over the old,
// so the file is never half written, and keeps the old file's mode. Both
// names start with the policy's own, which keeps them as off limits to
// agents as the policy. It returns the backup's path, or "" when there was
// no file to back up.
//
// It refuses a policy file that is a symbolic link: renaming over it would
// turn the link into a plain file, and following it would put the backup
// and the temporary file under names the policy doesn't protect.
func WriteFile(path string, text []byte, now time.Time) (backup string, err error) {
	if _, err := Parse(text, "", nil); err != nil {
		return "", fmt.Errorf("Agentry won't write a policy that doesn't load: %w", err)
	}
	mode := os.FileMode(0o600)
	switch info, err := os.Lstat(path); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return "", err
	case info.Mode()&os.ModeSymlink != 0:
		return "", fmt.Errorf("%s is a link; edit it yourself", filepath.Base(path))
	default:
		mode = info.Mode().Perm()
	}
	if backup, err = backUp(path, now); err != nil {
		return "", err
	}
	return backup, replace(path, text, mode)
}

func backUp(path string, now time.Time) (string, error) {
	old, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	backup := path + "-backup-" + now.UTC().Format(backupTime)
	f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(old); err != nil {
		f.Close()
		return "", err
	}
	return backup, f.Close()
}

func replace(path string, text []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-new-*")
	if err != nil {
		return err
	}
	err = temp.Chmod(mode) // CreateTemp makes it 0600
	if err == nil {
		_, err = temp.Write(text)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		os.Remove(temp.Name())
	}
	return err
}
