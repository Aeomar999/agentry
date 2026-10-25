// Package engines is everything Agentry needs from an agent CLI, so the rest
// of Agentry never knows which engine it is running. Each adapter turns its
// engine's hook calls into Signals, and Agentry's gate decisions back into
// what that engine's hooks expect (proof 07).
package engines

import (
	"sort"
	"time"
)

type Engine interface {
	Name() string

	// Prepare writes the engine's hook configuration for a session.
	Prepare(s Session) error

	// Launch is the command to run in the session's terminal, in never-ask
	// mode so Agentry's gate is the only one, starting on the given prompt.
	Launch(s Session, prompt string) []string

	// IsGate says whether a hook event waits for Agentry's decision.
	IsGate(event string) bool

	// ParseHook turns one hook call (event name and stdin JSON) into signals.
	// started is when the turn's session started, by the clock of the
	// machine running it, so only this turn's use is counted; zero when
	// unknown, and then the whole transcript is the turn's.
	ParseHook(event string, payload []byte, started time.Time) ([]Signal, error)

	// GateReply is what the gate hook prints and exits with for a decision.
	GateReply(event string, decision Decision, reason string) Reply
}

// Session is what an adapter needs to know about one agent turn.
type Session struct {
	Worktree    string // where the agent works
	StateDir    string // where Agentry keeps this turn's files, inside the worktree
	HookCommand string // runs Agentry's hook: HookCommand + " " + event name
	Model       string // "" for the engine's default
	Theme       string // "light" or "dark", the page's; "" leaves the engine's own
}

type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
)

type Reply struct {
	Stdout   string
	ExitCode int
}

// GateTimeoutSeconds is how long an engine lets the gate hook run. The hook's
// own deadline must be shorter, or the engine kills it and fails open (proof 01).
const GateTimeoutSeconds = 1800

var all = map[string]Engine{}

func register(e Engine) { all[e.Name()] = e }

func Get(name string) (Engine, bool) {
	e, ok := all[name]
	return e, ok
}

// Names lists the engines there are adapters for.
func Names() []string {
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
