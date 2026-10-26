//go:build !windows

package procs

import (
	"os"
	"os/exec"
	"syscall"
)

// Detach makes the command its own session, so it outlives the program
// that started it and that program's terminal.
func Detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// NoConsole does nothing: only Windows opens a console window for a command.
func NoConsole(cmd *exec.Cmd) {}

// KillGroupOnCancel starts a command made with exec.CommandContext in a
// process group of its own, and kills the whole group when the context ends,
// so the programs it started don't outlive it.
func KillGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

// Alive says whether a process is running.
func Alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// Terminate asks a process to stop.
func Terminate(process *os.Process) { process.Signal(syscall.SIGTERM) }

// Restart replaces this process with the program at self, with the same
// arguments and environment: the same process id, the new program.
func Restart(self string) (pid int, err error) {
	return 0, syscall.Exec(self, os.Args, os.Environ())
}
