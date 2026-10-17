package machines

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Local is this machine: the one Agentry runs on, or, inside `agentry
// worker`, the worker's own. Its sessions are tmux servers here, and its
// worktrees folders here.
type Local struct {
	Label        string // its name: workers.Local, or the worker's
	ServerURL    string // where its sessions' hooks post
	HookCommand  string // runs `agentry hook`
	WorktreeRoot string

	// Hub is set on a worker: where tasks' work comes from and goes back
	// to. On the machine Agentry runs on, the repos themselves are the hub.
	Hub *Hub
}

// Hub is the Agentry server's git side, as a worker reaches it.
type Hub struct {
	URL        func(task string) string // the git address of a task's repo
	Credential string                   // the worker's, which lets it in
	MirrorRoot string                   // where the worker keeps its copy of each repo
}

func (h *Hub) header() string { return "Authorization: Bearer " + h.Credential }

func (l *Local) Name() string { return l.Label }

func (l *Local) Prepare(_ context.Context, r WorktreeRequest) (worktrees.Worktree, error) {
	port, err := worktrees.LeasePort(r.Ports)
	if err != nil {
		return worktrees.Worktree{}, err
	}
	if l.Hub == nil {
		return worktrees.Create(r.Repo, l.WorktreeRoot, r.Task, port)
	}
	mirror := worktrees.Mirror{Dir: filepath.Join(l.Hub.MirrorRoot, r.RepoKey), Hub: l.Hub.URL(r.Task),
		Header: l.Hub.header(), Origin: r.Origin}
	return mirror.Worktree(l.WorktreeRoot, r.Task, r.Base, port)
}

func (l *Local) HandOff(ctx context.Context, w worktrees.Worktree, message string) error {
	w = l.own(w)
	if _, err := w.CommitLeftovers(message); err != nil {
		return fmt.Errorf("committing what was left: %w", err)
	}
	if l.Hub != nil {
		if err := l.upload(ctx, w); err != nil {
			return err
		}
	}
	if err := w.Discard(); err != nil {
		return err
	}
	if l.Hub != nil {
		return worktrees.DeleteBranch(w.Repo, w.Branch) // the hub has it now
	}
	return nil
}

// upload gives the hub the branch's commits, as a git bundle.
func (l *Local) upload(ctx context.Context, w worktrees.Worktree) error {
	file := filepath.Join(os.TempDir(), "agentry-"+filepath.Base(w.Path)+".bundle")
	defer os.Remove(file)
	if made, err := w.Bundle(file); err != nil || !made {
		return err
	}
	body, err := os.Open(file)
	if err != nil {
		return err
	}
	defer body.Close()
	task := filepath.Base(w.Path)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(l.Hub.URL(task), "/")+"/bundle", body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+l.Hub.Credential)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("sending the work to Agentry: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("sending the work to Agentry: HTTP %d", response.StatusCode)
	}
	return nil
}

func (l *Local) Remove(_ context.Context, w worktrees.Worktree) error { return l.own(w).Remove() }

// own is the worktree as this machine has it. On a worker its repo is the
// worker's copy, not the hub the server knows it by.
func (l *Local) own(w worktrees.Worktree) worktrees.Worktree {
	if l.Hub == nil {
		return w
	}
	if common, err := exec.Command("git", "-C", w.Path, "rev-parse", "--path-format=absolute", "--git-common-dir").Output(); err == nil {
		w.Repo = strings.TrimSpace(string(common))
	}
	return w
}

func (l *Local) PrepareTurn(_ context.Context, t Turn) error {
	engine, ok := engines.Get(t.Engine)
	if !ok {
		return fmt.Errorf("engine %q has no adapter", t.Engine)
	}
	if err := writeFile(filepath.Join(t.Worktree, t.BriefFile), t.Brief); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(t.Worktree, KnowledgeFile), t.Knowledge+"\n"); err != nil {
		return err
	}
	if t.Plan != "" {
		if err := writeFile(filepath.Join(t.Worktree, PlanFile(t.ID)), t.Plan+"\n"); err != nil {
			return err
		}
	}
	if err := engine.Prepare(l.session(t)); err != nil {
		return fmt.Errorf("preparing %s's hooks: %w", t.Engine, err)
	}
	return nil
}

func (l *Local) StartTurn(_ context.Context, t Turn) error {
	engine, ok := engines.Get(t.Engine)
	if !ok {
		return fmt.Errorf("engine %q has no adapter", t.Engine)
	}
	env := map[string]string{hookcall.EnvServer: l.ServerURL, hookcall.EnvWorktree: t.Worktree}
	for name, value := range t.Env {
		env[name] = value
	}
	return sessions.Start(sessions.Spec{Socket: sessions.SocketFor(t.Task), Dir: t.Worktree,
		Env: env, Command: engine.Launch(l.session(t), t.Prompt)})
}

func (l *Local) session(t Turn) engines.Session {
	return engines.Session{Worktree: t.Worktree, StateDir: filepath.Join(t.Worktree, StateDir),
		HookCommand: l.HookCommand, Model: t.Model, Theme: t.Theme}
}

func (l *Local) Stop(task string)           { sessions.Stop(sessions.SocketFor(task)) }
func (l *Local) IsRunning(task string) bool { return sessions.IsRunning(sessions.SocketFor(task)) }

func (l *Local) Files(_ context.Context, w workspace.Workspace) ([]workspace.File, error) {
	return w.Files()
}

func (l *Local) Commits(_ context.Context, w workspace.Workspace) ([]workspace.Commit, error) {
	return w.Commits()
}

func (l *Local) Read(_ context.Context, w workspace.Workspace, path string) (workspace.Contents, error) {
	return w.Read(path)
}

func (l *Local) Attach(task string, onOutput func([]byte)) (terminal.Pane, error) {
	return terminal.Attach(task, onOutput)
}

func (l *Local) About(context.Context) (About, error) {
	repos := ""
	if l.Hub != nil {
		repos = l.Hub.MirrorRoot
	}
	return AboutHere(l.WorktreeRoot, repos), nil
}

// StateDir is where Agentry keeps a task's files inside its worktree. Git
// ignores it (package worktrees).
const StateDir = ".agentry"

// KnowledgeFile is where the working agent's knowledge is put at the start
// of its turn, for it to keep up to date; `agentry hook` reads it back when
// the turn ends.
var KnowledgeFile = filepath.Join(StateDir, "knowledge.md")

// PlanFile is where the agent keeps its plan for a turn, a checklist it
// ticks as it goes; `agentry hook` sends it with every call, so the page
// follows it live. One file per turn, so no turn starts on another's plan.
func PlanFile(turn string) string { return filepath.Join(StateDir, turn+"-plan.md") }

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

var _ Machine = (*Local)(nil)

// IsLocal says whether name is the machine Agentry runs on.
func IsLocal(name string) bool { return name == "" || name == workers.Local }
