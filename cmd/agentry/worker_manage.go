package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/workerservice"
)

// The `agentry worker` commands that manage the worker on this machine:
// its background service, its state, its log, its updates, and leaving.

// linkState is what the running worker last said about its link, for
// `agentry worker status`: kept in the worker's folder.
type linkState struct {
	Up    bool      `json:"up"`
	Since time.Time `json:"since"`
	Error string    `json:"error,omitempty"`
}

func saveLinkState(dir string, up bool, err error) {
	state := linkState{Up: up, Since: time.Now()}
	if err != nil {
		state.Error = err.Error()
	}
	saved, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(dir, "link.json"), saved, 0o644)
}

func loadLinkState(dir string) (linkState, bool) {
	var state linkState
	saved, err := os.ReadFile(filepath.Join(dir, "link.json"))
	return state, err == nil && json.Unmarshal(saved, &state) == nil
}

// serviceSpec is how the background service runs this agentry: with the
// PATH of the shell starting it, so it finds what that shell finds.
func serviceSpec(dir string) (workerservice.Spec, error) {
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	return workerservice.Spec{Binary: self, Dir: dir, Log: filepath.Join(dir, "worker.log"), Path: os.Getenv("PATH")}, err
}

// runStart starts the background service, or restarts it, then waits a few
// seconds to say whether it got through to the server.
func runStart(w *workerSetup, _ []string, stdout io.Writer) error {
	config, err := loadWorker(w.dir)
	if err != nil {
		return err
	}
	spec, err := serviceSpec(w.dir)
	if err != nil {
		return err
	}
	service := workerservice.ForThisMachine(w.dir)
	started := time.Now()
	if err := service.Start(spec); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Started %s as a %s.\n", config.Name, service.Kind())
	if runtime.GOOS == "darwin" && strings.HasPrefix(service.Kind(), "background") {
		fmt.Fprintln(stdout, "macOS only sets up login services from a session on the Mac itself, not over SSH: run `agentry worker start` in Terminal there to have it start at login.")
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if state, ok := loadLinkState(w.dir); ok && state.Since.After(started) {
			if state.Up {
				fmt.Fprintf(stdout, "Connected to %s. Agents can run here now.\n", config.Server)
				return nil
			}
			return fmt.Errorf("it can't reach %s yet: %s (it keeps trying; see `agentry worker logs`)", config.Server, state.Error)
		}
	}
	return errors.New("it hasn't said anything after 15 s; see `agentry worker logs`")
}

func runStop(w *workerSetup, _ []string, stdout io.Writer) error {
	service := workerservice.ForThisMachine(w.dir)
	if !service.Running() {
		fmt.Fprintln(stdout, "It isn't running.")
		return nil
	}
	if err := service.Stop(); err != nil {
		return err
	}
	saveLinkState(w.dir, false, errors.New("stopped"))
	fmt.Fprintln(stdout, "Stopped. Agents running here keep going until their turn ends; `agentry worker start` starts it again.")
	return nil
}

// runStatus says everything there is to know about the worker here.
func runStatus(w *workerSetup, _ []string, stdout io.Writer) error {
	config, err := loadWorker(w.dir)
	if err != nil {
		return err
	}
	service := workerservice.ForThisMachine(w.dir)
	row := func(label, value string) { fmt.Fprintf(stdout, "  %-10s %s\n", label, value) }

	fmt.Fprintf(stdout, "%s, a worker for %s\n", config.Name, config.Server)
	running := "stopped (`agentry worker start`)"
	if service.Running() {
		running = "running"
	}
	row("service", service.Kind()+": "+running)
	if state, ok := loadLinkState(w.dir); ok && service.Running() {
		if state.Up {
			row("link", "connected since "+state.Since.Format("Jan 2 15:04"))
		} else {
			row("link", "not connected: "+state.Error)
		}
	}
	about := machines.Describe()
	row("agentry", about.Version+" "+updateNote(config))
	row("engines", orNone(strings.Join(about.Engines, ", ")))
	if len(about.Missing) > 0 {
		row("missing", strings.Join(about.Missing, ", ")+" (agents need it)")
	}
	row("running", orNone(strings.Join(sessions.Running(), ", ")))
	row("folder", w.dir)
	return nil
}

// updateNote says whether the server has a newer agentry for this machine.
func updateNote(config workerConfig) string {
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch newer, err := config.link().Newer(ctx, self); {
	case err != nil:
		return "(couldn't ask the server for updates: " + err.Error() + ")"
	case newer:
		return "(the server has a newer one: `agentry worker update`)"
	}
	return "(up to date)"
}

func orNone(text string) string {
	if text == "" {
		return "none"
	}
	return text
}

func runLogs(w *workerSetup, _ []string, stdout io.Writer) error {
	args := []string{"-n", "100"}
	if w.follow {
		args = append(args, "-f")
	}
	tail := exec.Command("tail", append(args, filepath.Join(w.dir, "worker.log"))...)
	tail.Stdout, tail.Stderr = stdout, stdout
	return tail.Run()
}

// runUpdate gets the server's agentry for this machine, if it differs, and
// restarts the service into it.
func runUpdate(w *workerSetup, _ []string, stdout io.Writer) error {
	config, err := loadWorker(w.dir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	updated, err := config.link().Update(ctx)
	if err != nil {
		return err
	}
	if !updated {
		fmt.Fprintln(stdout, "Already the server's agentry.")
		return nil
	}
	fmt.Fprintln(stdout, "Updated to the server's agentry.")
	if service := workerservice.ForThisMachine(w.dir); service.Running() {
		return runStart(w, nil, stdout)
	}
	return nil
}

// runLeave takes this machine off the server, stops and removes the
// service, and forgets the credential.
func runLeave(w *workerSetup, _ []string, stdout io.Writer) error {
	config, err := loadWorker(w.dir)
	if err != nil {
		return err
	}
	request, _ := http.NewRequest(http.MethodPost, config.Server+workerlink.LeavePath, nil)
	request.Header.Set("Authorization", "Bearer "+config.Credential)
	response, err := config.client().Do(request)
	if err != nil {
		return fmt.Errorf("couldn't reach %s to leave it: %w", config.Server, err)
	}
	reason, _ := io.ReadAll(response.Body)
	response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusUnauthorized: // left, or the server had removed it already
	default:
		return fmt.Errorf("the server said: %s", strings.TrimSpace(string(reason)))
	}

	if err := workerservice.ForThisMachine(w.dir).Remove(); err != nil {
		return err
	}
	for _, name := range []string{"worker.json", "link.json"} {
		os.Remove(filepath.Join(w.dir, name))
	}
	if w.purge {
		for _, name := range []string{"repos", "worktrees"} {
			if err := os.RemoveAll(filepath.Join(w.dir, name)); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(stdout, "%s left %s. To join again, use a new command from its Workers page.\n", config.Name, config.Server)
	return nil
}
