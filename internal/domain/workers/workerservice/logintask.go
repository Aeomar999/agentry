package workerservice

import (
	"os/exec"
	"path/filepath"
)

// loginTask is the worker on Windows: a background process, started again
// when you log in by a Task Scheduler task that runs `agentry worker start`.
type loginTask struct{ background }

const taskName = "Agentry worker"

func (loginTask) Kind() string {
	return `background process, started at login by the Task Scheduler task "` + taskName + `"`
}

func (l loginTask) Start(spec Spec) error {
	if err := l.background.Start(spec); err != nil {
		return err
	}
	// Running already; at login the task starts it again. If the task
	// can't be made, it still runs now.
	exec.Command("schtasks", "/Create", "/F", "/SC", "ONLOGON", "/RL", "LIMITED", "/TN", taskName,
		"/TR", `"`+spec.Binary+`" worker start -dir "`+spec.Dir+`"`).Run()
	return nil
}

func (l loginTask) Remove() error {
	exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).Run()
	return l.background.Remove()
}

func pidFile(dir string) string { return filepath.Join(dir, "worker.pid") }
