// Package policy decides one tool call: allow it, deny it, or ask the user.
// The rules are not here: they are in policy.toml (see default.toml).
package policy

type Decision string

const (
	Allow Decision = "allow"
	Ask   Decision = "ask" // the user decides, from the page
	Deny  Decision = "deny"
)

// Call is one tool call to judge.
type Call struct {
	Tool      string // Bash, Edit, Read, ...
	Input     string // the shell command, or the file path for file tools
	Workspace string // the task's worktree: the agent's own space
}

// Verdict is a decision and the rule that made it, which the agent and the
// page are shown.
type Verdict struct {
	Decision Decision
	Profile  string
	Rule     string
	Why      string
	Policy   string // the hash of the policy that decided
	Order    int    // the matching rule's place in its profile, from 1; 0 when no profile rule decided
}

// Reason is the verdict in one line, for the agent and the log.
func (v Verdict) Reason() string {
	if v.Profile == "" {
		return v.Rule
	}
	return v.Profile + ": " + v.Rule
}
