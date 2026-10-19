// Package hookcall is the contract between `agentry hook`, which an engine
// runs, and the server: the environment a session gives the hook, and the
// request and reply it sends. Both sides import it, so they can't disagree.
package hookcall

import "encoding/json"

// The environment every agent session has, read by `agentry hook`.
const (
	EnvServer   = "AGENTRY_SERVER"     // the server's base URL
	EnvToken    = "AGENTRY_HOOK_TOKEN" // lets the hook post hook calls, and nothing else
	EnvTask     = "AGENTRY_TASK"
	EnvTurn     = "AGENTRY_TURN"
	EnvAgent    = "AGENTRY_AGENT"
	EnvEngine   = "AGENTRY_ENGINE"
	EnvDeadline = "AGENTRY_HOOK_DEADLINE" // shorter than the engine's hook timeout (proof 01)
	EnvWorktree = "AGENTRY_WORKTREE"      // the task's worktree, on the machine the session runs on
)

// Path is where the server takes hook calls, and TokenHeader carries EnvToken.
const (
	Path        = "/hooks"
	TokenHeader = "X-Agentry-Hook-Token"
)

// Call is one hook call, as the hook posts it.
type Call struct {
	Task    string          `json:"task"`
	Turn    string          `json:"turn"`
	Agent   string          `json:"agent"`
	Engine  string          `json:"engine"`
	Event   string          `json:"event"`   // the engine's hook event name, e.g. PreToolUse
	Payload json.RawMessage `json:"payload"` // the engine's stdin, unchanged

	// When the call ends the turn, what only the machine running the
	// session can read: the tokens its transcript says it used, and the
	// agent's knowledge file. The session may be on a worker, where the
	// server can't read either.
	Ending *Ending `json:"ending,omitempty"`

	// The turn's plan file as it is at this call, read on the machine
	// running the session; nil when the agent hasn't written one.
	Plan *string `json:"plan,omitempty"`
}

type Ending struct {
	Tokens int `json:"tokens"`
	// Why Tokens is 0, when the transcript couldn't tell; a worker older
	// than this field sends none.
	TokensUnknown string  `json:"tokensUnknown,omitempty"`
	Report        string  `json:"report"`              // the agent's final message, when the engine read it from the transcript
	Knowledge     *string `json:"knowledge,omitempty"` // nil when the file couldn't be read
}

// Reply is what the hook prints and exits with.
type Reply struct {
	Stdout   string `json:"stdout"`
	ExitCode int    `json:"exitCode"`
}
