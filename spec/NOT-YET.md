# Not built yet

This is the behaviour left out of the baseline because it is not built on dev. The feature files describe only what Agentry does today, so anything PLAN.md or the design notes name that the code does not do is listed here instead, one line each, with its PLAN ID where it has one. A line here is not a plan to build it.

- Editing the policy's rules from the Policy page, keeping each rule's why, is not built; rules are edited in policy.toml, and the page manages profiles, use_for_everyone and the history (T-043).
- A command that prints the log, agentry events, is not built; agentry task creates, lists, shows and answers tasks (T-007).
- A delivery outbox for follow-up prompts to a running session, written to a file, acknowledged by hash, retried and recorded as delivery.failed, is not built; a message to the coordinator stops the turn in progress instead (T-012).
- Reviewing from the command line, seeing a worktree's diff and approving, sending back or discarding it, is not built (T-015).
- No run of a real task through every agent with Codex as the engine is recorded; the Codex adapter is tested on its own (T-016).
- Counting a Codex turn's tokens is not built; each Codex turn says its tokens are not read.
- Extensions are not built: neither a GitHub extension that opens a pull request once a review approves, nor an email extension (T-019); their design is in docs/design/extensions.md (T-070).
- An MCP server for an outside board, with get_ticket, update_status, add_comment and create_subtask, is not built; agentry mcp serves Agentry's own tools only (T-020).
- Supervisor agents, ordinary agents allowed to assign a task as a coordinator does, are not built; only a team's coordinator assigns (T-021).
- An output gate that holds, redacts or blocks a tool's result before the agent sees it is not built; the gate judges each action before it runs (T-023).
- A senior-approval extension that sends some asks to someone other than the task's owner is not built (T-024).
- A desktop app is not built; the page installs as an app from the browser (T-025).
- A Gemini CLI adapter is not built; the engines are Claude Code and Codex (T-026).
- Handing a half-finished task from one engine to the other with its session is not built; a turn on another engine starts a fresh session (T-027).
- Registering Codex's hook trust is not built; Codex is started with --dangerously-bypass-hook-trust (T-028).
- A network egress gate is not built: agent traffic does not pass through an Agentry proxy, no host is allowlisted or asked about, and nothing blocks traffic at the system level (T-030). The autonomous profile ships without it, so its asks for acting outside the machine rest on the shell and tool rules alone (T-029).
