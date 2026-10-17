package machines

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// Helper is a plain Claude session you open beside a task, in its worktree
// on the machine the work is on: for your own questions about the code. It
// changes no files, so it never gets in the way of an agent's work, and it
// can't reach the coordinator: `agentry draft` only writes a message for
// you to send or not. It has no Agentry hooks, so nothing it does is gated
// or counted as the task's work: it is yours.
type Helper struct {
	Task     string            `json:"task"`
	Worktree string            `json:"worktree"`
	Prompt   string            `json:"prompt"` // added to Claude's system prompt
	Theme    string            `json:"theme"`
	Env      map[string]string `json:"env"` // the token and task `agentry draft` needs; the machine adds the server
}

// HelperSession names the task's helper session, apart from its agents'.
func HelperSession(task string) string { return task + "-claude" }

// Env names the helper's `agentry draft` reads.
const EnvHelperToken = "AGENTRY_HELPER_TOKEN"

// StartHelper starts the task's helper unless it is running: one per task,
// kept between visits, so you come back to the same conversation.
func (l *Local) StartHelper(_ context.Context, h Helper) error {
	socket := sessions.SocketFor(HelperSession(h.Task))
	if sessions.IsRunning(socket) {
		return nil
	}
	if err := engines.TrustFolder(h.Worktree); err != nil {
		return err
	}
	settings, _ := json.Marshal(map[string]string{"theme": h.Theme})
	if h.Theme == "" {
		settings = []byte("{}")
	}
	path := os.Getenv("PATH")
	if self, err := os.Executable(); err == nil {
		path = filepath.Dir(self) + string(os.PathListSeparator) + path // `agentry draft` is this agentry
	}
	env := map[string]string{hookcall.EnvServer: l.ServerURL}
	for name, value := range h.Env {
		env[name] = value
	}
	return sessions.Start(sessions.Spec{Socket: socket, Dir: h.Worktree, Env: env,
		// PATH goes through env in the command: tmux gives a new session the
		// client's PATH, whatever -e says.
		Command: []string{"env", "PATH=" + path, "claude", "--append-system-prompt", h.Prompt, "--settings", string(settings),
			"--permission-mode", "default", "--allowedTools", "Bash(agentry draft:*)",
			"--disallowedTools", "Edit", "Write", "MultiEdit", "NotebookEdit"}})
}

// HelperPrompt is what the helper is told about its place, added to Claude's
// own system prompt.
func HelperPrompt(task, title, branch string) string {
	return fmt.Sprintf(`You are helping the user look at Agentry task %s, %q, in its git worktree, on branch %s.
Agents work in this same worktree, so you never change files: read, search, run read-only commands, and explain.
The commits on the branch are the agents' work; git log and git show tell you what each round did.
When the user wants something changed, or more work done, don't do it yourself. Write the request for the task's
coordinator with: agentry draft "<what to do, and what done looks like>". It only drafts: the user reviews it in the
page and sends it, or changes or discards it, because sending it starts the agents' work. After drafting, tell the
user it is waiting for them there.`, task, title, branch)
}
