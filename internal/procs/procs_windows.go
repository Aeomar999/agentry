//go:build windows

package procs

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// Detach starts the command with no console and in its own process group,
// so it outlives the program that started it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}

// Alive says whether a process is running.
func Alive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) == nil && code == stillActive
}

const stillActive = 259

// Terminate stops a process. Windows has no polite signal for a process
// without a console, so it is ended.
func Terminate(process *os.Process) { process.Kill() }

// Restart starts the program at self again, with the same arguments,
// output and environment, as a new process this one hands over to:
// Windows can't replace a running process's program. The caller exits.
func Restart(self string) (pid int, err error) {
	next := exec.Command(self, os.Args[1:]...)
	next.Stdout, next.Stderr, next.Env = os.Stdout, os.Stderr, os.Environ()
	Detach(next)
	if err := next.Start(); err != nil {
		return 0, err
	}
	return next.Process.Pid, next.Process.Release()
}
