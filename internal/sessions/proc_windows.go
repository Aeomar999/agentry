//go:build windows

package sessions

import (
	"os"
	"path/filepath"
)

// sessionsDir is in this user's local app data.
func sessionsDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "agentry", "sessions")
}

// killTree ends the program; closing its pseudo-console ends what runs in it.
func killTree(process *os.Process) { process.Kill() }
