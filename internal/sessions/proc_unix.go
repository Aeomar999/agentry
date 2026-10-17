//go:build !windows

package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// sessionsDir is short, as tmux's is: a socket's path may be only about 100
// bytes long, and a home folder can be deep. The folder is this user's alone.
func sessionsDir() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("agentry-%d", os.Getuid()))
}

// killTree ends the program and everything it started: the pty made it the
// leader of its own process group. Asked first, then made to.
func killTree(process *os.Process) {
	group := -process.Pid
	syscall.Kill(group, syscall.SIGHUP)
	syscall.Kill(group, syscall.SIGTERM)
	time.AfterFunc(2*time.Second, func() { syscall.Kill(group, syscall.SIGKILL) })
}
