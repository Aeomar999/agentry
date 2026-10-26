//go:build !windows

package procs

import (
	"context"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

// A command started with KillGroupOnCancel leads a session of its own, so
// it has no terminal of the caller's to stop for, and a group of its own.
func TestKillGroupOnCancelStartsANewSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sleep", "10")
	KillGroupOnCancel(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); cmd.Wait() }()
	pid := cmd.Process.Pid
	sid, err := unix.Getsid(pid)
	if err != nil {
		t.Fatal(err)
	}
	ours, _ := unix.Getsid(0)
	if sid != pid || sid == ours {
		t.Errorf("session %d, want %d, its own, not the caller's %d", sid, pid, ours)
	}
	if group, err := unix.Getpgid(pid); err != nil || group != pid {
		t.Errorf("process group %d (%v), want %d, its own", group, err, pid)
	}
}
