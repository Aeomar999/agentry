package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive takes file's lock without waiting, or returns errLocked.
// It locks the first byte, which is enough: every serve locks the same one.
func lockExclusive(file *os.File) error {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLocked
	}
	return err
}
