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
}

// Reply is what the hook prints and exits with.
type Reply struct {
	Stdout   string `json:"stdout"`
	ExitCode int    `json:"exitCode"`
}
