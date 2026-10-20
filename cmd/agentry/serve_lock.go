package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// lockFile is the file in the data folder that one serve at a time holds
// locked. The OS drops the lock when the process ends, however it ends, so
// a crash leaves nothing to clean up.
const lockFile = "serve.lock"

// errLocked is what lockExclusive returns when another process holds the lock.
var errLocked = errors.New("locked by another process")

// lockDataDir locks dataDir for this process alone, and returns the release.
// While another serve holds it, it fails without changing anything.
func lockDataDir(dataDir string) (release func(), err error) {
	path := filepath.Join(dataDir, lockFile)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // no O_TRUNC: the holder's pid stays
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(file); err != nil {
		file.Close()
		if errors.Is(err, errLocked) {
			return nil, fmt.Errorf("another agentry serve is using %s%s", dataDir, holder(path))
		}
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	// The pid is only for the error above; the lock is what keeps a second serve out.
	if err := file.Truncate(0); err == nil {
		file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return func() { file.Close() }, nil // closing the file releases the lock
}

// holder names the pid in the lock file at path, when it can be read.
func holder(path string) string {
	saved, err := os.ReadFile(path)
	pid := strings.TrimSpace(string(saved))
	if err != nil || pid == "" {
		return ""
	}
	return " (pid " + pid + ")"
}
