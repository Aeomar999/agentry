package sessionstest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// startedHere are the processes still running with a sessions folder in
// base: this run's hosts, the programs in them, and anything else its tests
// started. A process can't be from this run if it started before it.
func startedHere(base string) []string {
	var found []string
	for _, pid := range fromThisRun(base) {
		command, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		found = append(found, fmt.Sprintf("pid %d: %s", pid, strings.TrimSpace(string(bytes.ReplaceAll(command, []byte{0}, []byte{' '})))))
	}
	return found
}

func killStartedHere(base string) {
	for _, pid := range fromThisRun(base) {
		syscall.Kill(pid, syscall.SIGKILL)
	}
}

func fromThisRun(base string) []int {
	self := os.Getpid()
	started := startTime(self)
	entries, _ := os.ReadDir("/proc")
	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == self {
			continue
		}
		if at := startTime(pid); at >= started && inBase(pid, base) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// inBase is true when the process's sessions folder is base or in it.
func inBase(pid int, base string) bool {
	environ, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return false // gone, or not this user's
	}
	for _, kv := range bytes.Split(environ, []byte{0}) {
		if dir, ok := strings.CutPrefix(string(kv), sessions.DirEnv+"="); ok {
			return dir == base || strings.HasPrefix(dir, base+string(filepath.Separator))
		}
	}
	return false
}

// startTime is when the process started, in clock ticks since boot; -1 if
// it can't be read.
func startTime(pid int) int64 {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return -1
	}
	// The fields after the command's name, which is in brackets and may hold
	// spaces: starttime is the 22nd field, the 20th of these.
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	if len(fields) < 20 {
		return -1
	}
	at, _ := strconv.ParseInt(fields[19], 10, 64)
	return at
}
