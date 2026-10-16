// Package desk runs Claude, the session you talk to in the page's terminal
// dock. It turns what you say into tasks for the coordinator, and answers
// the coordinator's questions for you, through `agentry task`. It runs as
// you, in the default repo, so it can read the code to write a good task;
// it never gets the UI token, so it can't approve an agent's actions.
package desk

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// Name is the desk's session name; its tmux server is agentry-desk. Tasks
// are named task-N, so it never collides with one.
const Name = "desk"

// Env names: how `agentry task` finds the server and proves it is the desk.
const (
	EnvServer = "AGENTRY_SERVER"
	EnvToken  = "AGENTRY_DESK_TOKEN"
)

//go:embed prompt.md
var prompt string

type Config struct {
	Dir       string // where Claude starts: the default repo
	ServerURL string
	Token     string // opens only what the desk may do, see internal/web
	Binary    string // this agentry binary, put first on the desk's PATH
}

// socket is the desk's tmux server. Tests use their own, so they never
// touch a desk you have open.
var socket = sessions.SocketFor(Name)

// Socket is the desk's tmux server.
func Socket() string { return socket }

// starting makes two viewers opening the desk at once start it once.
var starting sync.Mutex

// Start starts the desk unless it is running. A desk whose Claude exited
// (you typed /exit) is started again.
func Start(c Config) error {
	starting.Lock()
	defer starting.Unlock()
	if sessions.IsRunning(Socket()) {
		return nil
	}
	return sessions.Start(sessions.Spec{
		Socket: Socket(),
		Dir:    c.Dir,
		Env:    map[string]string{EnvServer: c.ServerURL, EnvToken: c.Token},
		// PATH goes through env: tmux gives a new session the client's PATH,
		// whatever -e says. This binary comes first, so `agentry task` is it.
		Command: []string{"env", "PATH=" + filepath.Dir(c.Binary) + string(os.PathListSeparator) + os.Getenv("PATH"),
			"claude", "--append-system-prompt", prompt,
			// It talks and hands over work; it never changes the code. Only
			// `agentry task` runs without asking; anything else asks you.
			"--permission-mode", "default",
			"--allowedTools", "Bash(agentry task:*)",
			"--disallowedTools", "Edit", "Write", "MultiEdit", "NotebookEdit"},
	})
}

// Stop ends the desk, as when the server stops.
func Stop() { sessions.Stop(Socket()) }
