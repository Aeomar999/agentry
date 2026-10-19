package engines

import (
	"os"
	"slices"
	"strings"
)

// A Claude Code or Codex session marks the processes it runs with variables
// that say "you are inside me". Agentry started from such a session (say,
// `agentry open` run by Claude's Bash tool) would hand them on to every
// session it starts, and the engine there believes it is a parent's child:
// Claude Code then saves no transcript, so a turn's tokens can't be read.
//
// Only these markers go. A blanket CLAUDE_* or CODEX_* strip would be wrong:
// CLAUDE_CONFIG_DIR, CLAUDE_CODE_USE_BEDROCK, CLAUDE_CODE_USE_VERTEX,
// CODEX_HOME and the API keys are the user's own configuration, and the
// session needs them to run as the user set it up.
var (
	parentSessionVars = []string{
		// Claude Code (2.1.303)
		"CLAUDECODE",
		"CLAUDE_CODE_CHILD_SESSION",    // turns transcript saving off
		"CLAUDE_CODE_SESSION_ID",       // the parent's session
		"CLAUDE_CODE_SESSION_ATTENDED", // whether someone watches the parent
		"CLAUDE_CODE_ENTRYPOINT",       // how the parent was started
		"CLAUDE_CODE_EXECPATH",         // the parent's claude binary
		"CLAUDE_CODE_SSE_PORT",         // the parent's IDE connection
		"CLAUDE_PID",                   // the parent's process
		"CLAUDE_EFFORT",                // the parent turn's effort level
		// Codex (0.163.0) sets these on the commands it runs.
		"CODEX_SANDBOX",
		"CODEX_SANDBOX_NETWORK_DISABLED",
		"CODEX_THREAD_ID",
		"CODEX_TOOL_CALL_ID",
		"CODEX_NETWORK_PROXY_ACTIVE",
	}
	parentSessionPrefixes = []string{
		"CLAUDE_CODE_MESSAGING_", // the socket and token to talk to the parent
	}
)

// IsParentSessionVar says whether a variable ties a process to a running
// engine session, rather than configuring the engine.
func IsParentSessionVar(name string) bool {
	name = strings.ToUpper(name) // Windows names ignore case
	if slices.Contains(parentSessionVars, name) {
		return true
	}
	return slices.ContainsFunc(parentSessionPrefixes, func(p string) bool { return strings.HasPrefix(name, p) })
}

// DropParentSession removes the parent-session markers from this process's
// environment, so nothing it starts inherits them, and names what it removed.
func DropParentSession() []string {
	var removed []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if IsParentSessionVar(name) {
			os.Unsetenv(name)
			removed = append(removed, name)
		}
	}
	slices.Sort(removed)
	return removed
}
