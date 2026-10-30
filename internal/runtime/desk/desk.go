// Package desk runs Claude, the session you talk to in the page's terminal
// dock. It turns what you say into tasks for the coordinator, and answers
// the coordinator's questions for you, through `agentry task`. It runs as
// you, in the default repo, so it can read the code to write a good task;
// it never gets the UI token, so it can't approve an agent's actions.
package desk

import (
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/platform/sessions"
)

// Name is the desk's session name; its session is agentry-desk. Tasks
// are named task-N, so it never collides with one.
const Name = "desk"

// Env names: how `agentry task` finds the server and proves it is the desk.
const (
	EnvServer = "AGENTRY_SERVER"
	EnvCA     = "AGENTRY_SERVER_CA" // the CA its https is pinned to, as for hooks
	EnvToken  = "AGENTRY_DESK_TOKEN"
)

//go:embed prompt.md
var prompt string

type Config struct {
	Dir       string // where Claude starts: the default repo
	ServerURL string
	ServerCA  string // the CA ServerURL's https is pinned to, PEM; "" for plain HTTP
	Token     string // opens only what the desk may do, see internal/web
	Binary    string // this agentry binary, put first on the desk's PATH
	Theme     string // "light" or "dark": the page's, so Claude's colours suit the terminal
}

// socket is the desk's session. Tests use their own, so they never
// touch a desk you have open.
var socket = sessions.SocketFor(Name)

// Socket is the desk's session.
func Socket() string { return socket }

var (
	starting     sync.Mutex // two viewers opening the desk at once start it once
	runningTheme string     // the theme the running desk was started in
	startedHere  bool       // this server started the running desk, with this build's prompt
	conversation string     // the desk's own Claude session id, so a restart resumes it and nothing else
)

// Start starts the desk unless it is running. A desk whose Claude exited
// (you typed /exit, or it crashed) is started again. A desk running in
// another theme is restarted in this one: Claude reads its theme only when
// it starts. A desk left running by an earlier server is started again
// too, so it has this build's prompt and commands. Each start resumes the
// desk's own conversation by id once Claude has saved it, and otherwise
// starts fresh with that id. (Not --continue, which resumes whichever
// conversation in the folder is newest, and that may not be the desk's.)
func Start(c Config) error {
	starting.Lock()
	defer starting.Unlock()
	running := sessions.IsRunning(Socket())
	if running && startedHere && c.Theme == runningTheme {
		return nil
	}
	settings, _ := json.Marshal(map[string]string{"theme": c.Theme})
	if c.Theme == "" {
		settings = []byte("{}")
	}
	command := []string{"claude", "--append-system-prompt", prompt, "--settings", string(settings),
		// It talks and hands over work; it never changes the code. Only
		// `agentry task` runs without asking; anything else asks you.
		"--permission-mode", "default",
		"--allowedTools", "Bash(agentry task:*)",
		"--disallowedTools", "Edit", "Write", "MultiEdit", "NotebookEdit"}
	if conversation == "" {
		conversation = newSessionID()
	}
	if hasTranscript(conversation) {
		command = append(command, "--resume", conversation)
	} else {
		command = append(command, "--session-id", conversation)
	}
	runningTheme, startedHere = c.Theme, true
	return sessions.Start(sessions.Spec{
		Socket: Socket(),
		Dir:    c.Dir,
		// This binary comes first on the PATH, so `agentry task` is it.
		Env: map[string]string{EnvServer: c.ServerURL, EnvCA: c.ServerCA, EnvToken: c.Token,
			"PATH": filepath.Dir(c.Binary) + string(os.PathListSeparator) + os.Getenv("PATH")},
		Command: command,
	})
}

// Stop ends the desk, as when the server stops.
func Stop() { sessions.Stop(Socket()) }

// hasTranscript is true once Claude has saved the conversation, which it does
// at the first message: a session nobody spoke in can't be resumed.
func hasTranscript(id string) bool {
	home, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", id+".jsonl"))
	return len(matches) > 0
}

// newSessionID is a random version 4 UUID, the form Claude's --session-id takes.
func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
