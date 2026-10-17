package workerservice

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// background is a plain background process, for machines with neither
// systemd nor launchd. It isn't started again after a reboot.
type background struct {
	pidFile string
}

func (background) Kind() string { return "background process (not started again after a reboot)" }

func (b background) Start(spec Spec) error {
	b.Stop()
	log, err := os.OpenFile(spec.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	command := spec.command()
	process := exec.Command(command[0], command[1:]...)
	process.Env = append(os.Environ(), "PATH="+spec.Path)
	process.Stdout, process.Stderr = log, log
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // out of this shell's session, so it outlives it
	if err := process.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(b.pidFile, []byte(strconv.Itoa(process.Process.Pid)), 0o644); err != nil {
		return err
	}
	return process.Process.Release()
}

func (b background) Stop() error {
	process, ok := b.process()
	if !ok {
		return nil
	}
	process.Signal(syscall.SIGTERM)
	for i := 0; i < 50 && b.Running(); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	os.Remove(b.pidFile)
	if b.Running() {
		return errors.New("it didn't stop within 5 s")
	}
	return nil
}

func (b background) Running() bool {
	process, ok := b.process()
	return ok && process.Signal(syscall.Signal(0)) == nil
}

func (b background) Remove() error { return b.Stop() }

func (b background) process() (*os.Process, bool) {
	content, err := os.ReadFile(b.pidFile)
	if err != nil {
		return nil, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return nil, false
	}
	process, err := os.FindProcess(pid)
	return process, err == nil
}
