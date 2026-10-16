# Plan: agentry

Updated: 2026-10-16 | Pass: 26

## Goal

The smallest MVP, in the user's words: "a simple page just plain and perhaps one page, I want to be able to add task and also have a couple agents, I will like to see a cordinator, and other agents, like engineer, reviewer, tester, deployer each with its prepared prompt", and "the cordinator to be the one that take every task and decide on who to pass the work to work on next and when each agent is done is should send the work back to the cordinator, and the cordinator and see who the send to next this repets ontil everything is done, also if there is any issue, let it go back to the cordinator". Agents are the user's to shape: "these agent prompts are things that ideally user should be able to add on the UI, and edit and all that, it is just that for starters I want you to provide that". It is built on the backbone the proofs tested: every step is an event in one append-only log, every agent action passes Agentry's gate, and the watchdog catches silence. The code must be easy to read, with each concern in its own package: "prioritize readability easy to read and understand code, separation of concern".

## Definition of done

- [x] `go test ./...` passes at the repo root
- [x] One plain page, served by `agentry serve`, adds a task with a repo path and shows every task's status, current agent and handoff trail, updating live
- [x] A real task runs coordinator, then engineer, reviewer, tester and deployer as the coordinator chooses, each starting from its own prompt, and every agent hands back to the coordinator until it says done
- [x] Agents are data, not code: the five defaults are seeded on first start, and the coordinator routes to whichever agents exist, so adding one later needs no code change
- [x] An issue (denied action, crash, stall, or a blocker the agent reports) goes back to the coordinator; a task with 24 turns (about 12 handoffs there and back) or 3 issues in a row, and no done, goes to the user instead
- [x] An action the policy holds shows on the page with approve and deny; a denied command never runs; the deployer's push to `dev` always waits for approval
- [x] Every step is an event in the append-only log, and the page shows a task's events
- [x] The page shows the working agent's terminal live (proof 04), and can take control and hand it back
- [x] Every finding in "From the proofs" below is used by the task it names

## Now

Nothing in Next. The policy is now a file you edit (T-040 to T-042); the next step is the user's choice from Later, where T-037 (edit agents on the page), T-043 (edit the policy on the page) and T-044 (a new folder as a task's workspace) are first.

## Next

| ID | Task | Why | Size | Blocked by |
|---|---|---|---|---|

## Later

| ID | Task | Why |
|---|---|---|
| T-043 | Edit the policy from the page, keeping each rule's `why` | Follows T-042; the file is the source until then |
| T-044 | A task can name a new folder as its workspace, for tasks that start a new project | The "create a new GitLab repo" task worked outside its worktree, so nearly everything asked |
| T-037 | Add, edit and remove agents and their prompts from the page | "ideally user should be able to add on the UI, and edit and all that". The data model is ready from T-031, so this is the API endpoints and a form. First in line after the MVP |
| T-007 | CLI: `agentry task create / show`, `agentry events <task>` | The page replaced the CLI for the MVP |
| T-012 | Delivery outbox: prompt written to a file, one typed line (or `codex queue`), acknowledged by hash, retried, `delivery.failed` | Needed once a running session gets follow-ups. In the MVP each turn is a new session (T-011). Proof 02: a leading `!` runs as a shell command |
| T-015 | Review from the CLI: show the worktree diff, approve, send back, discard | The reviewer agent and the page cover the MVP |
| T-016 | Milestone with Codex as the engine, changing only the agent's engine setting | The adapter is ready for it (T-008) |
| T-017 | Web board in React: columns computed from events, updating live | "a board like Jira". The plain page comes first |
| T-019 | GitHub extension: open the PR after `review.approved`; notification and email extensions | Open the merge request once a task is reviewed, and tell people |
| T-020 | MCP server for the board (`get_ticket`, `update_status`, `add_comment`, `create_subtask`) | A second channel to the board, so agents can read and update tickets directly |
| T-021 | Supervisor agents: an ordinary agent with permission to propose `ticket.assigned` | "supervisor is just an agent". The coordinator (T-032) is the first one |
| T-022 | Remote workers: join link, machine credential, outbound connection, inventory | "workers url that we can use install on other machines" |
| T-023 | Output gate: hold, redact or block a tool result before the agent sees it | "review outputs before we let them reach the AI" |
| T-024 | Senior-approval extension: route some requests to someone other than the ticket owner | "an extension that takes this to another level" |
| T-025 | Desktop app with Wails | "can users use it like a desktop app? if maybe I wrap it" |
| T-026 | Gemini CLI adapter | Not signed in yet; untested in proof 07 |
| T-027 | Hand a half-finished ticket from one engine to another | Not tested in proof 07 |
| T-028 | Register Codex hook trust properly instead of `--dangerously-bypass-hook-trust` | Proof 07 |
| T-029 | Autonomous policy profile: allow everything on the machine, ask for anything external, keep a short deny list | "someone who wants their agents to have fully autonomy do what every it wants and when it needs external requests certain ones, it raise a request and user approve" |
| T-030 | Network egress gate: agent traffic through an Agentry proxy, allowlisted hosts pass, unknown hosts go to Needs you; OS-level block so the proxy is the only way out | Hooks see commands, not effects: `python deploy.py` looks harmless. Needed for T-029 to be trustworthy |
| T-036 | JSON Schema per event type, generated from or checked against the Go types | "all of these events should be validated by some rules". Go validation covers the MVP; schemas matter once extensions write events |
| T-038 | Several tasks at once, each in its own worktree and port | The MVP runs one task at a time |

## Done

| ID | Task | Landed | Evidence |
|---|---|---|---|
| T-001 | Go module `gitlab.com/dev.chempa/agentry`, `cmd/agentry` with `serve` / `worker` / `hook` stubs | 2026-10-15 | `go vet ./... && go test ./...` ok; `bin/agentry` prints usage; `agentry hook PreToolUse` exits 2 (stubs fail closed) |
| T-002 | Event catalogue: every MVP event type (tasks, agents, handoffs, sessions, actions, approvals, watchdog) as a Go type with validation, `action.asked` included | 2026-10-16 | `go test ./internal/events` ok: 11 shape and who cases, every type in the catalogue |
| T-003 | Event store: SQLite append-only (triggers), single writer, `Propose()` running shape, who, then an order check passed in (package rules). Policy decisions are events from the policy actor, not a validator check | 2026-10-16 | `go test ./internal/store` ok: rejections recorded, UPDATE and DELETE refused by triggers, 20 racing proposals accept exactly 1, log survives reopening |
| T-004 | Task state from events (status, turns, actions, handoff trail, current agent) and package rules: the order and who checks for every task event, run by the store | 2026-10-16 | `go test ./internal/rules` ok: a task through coordinator, engineer and back, 12 refused steps, Needs you only from `action.asked` or a question |
| T-005 | Readers pull from the log by position, each with its own cursor | 2026-10-16 | `go test ./internal/store -run Reader` ok: 1,200 writes in 0.07 s with a reader not reading, then all read back in order; `Next` waits for the next event |
| T-031 | Agents as data: `agent.created` / `agent.changed`, the roster computed from them, five default agents with prompts in `internal/agents/defaults/*.md` seeded only when there are none, agent rules (name, engine, prompt, a coordinator always left), and the coordinator decision format with its parser | 2026-10-16 | `go test ./internal/agents ./internal/coordinator ./internal/rules` ok: 5 defaults seeded once with prompts, changes replace an agent, 9 agent rule cases, 8 decision parsing cases |
| T-008 | Engine adapters for Claude and Codex, ported from proof 07, returning engine-neutral signals; each agent names its engine | 2026-10-16 | `go test ./internal/engines` ok: 12 recorded payloads give the same signals on both engines, 5 gate replies, hooks written fail closed, launch in never-ask mode, adapters match `rules.KnownEngines` |
| T-010 | Worktree per task: branch `agentry/<task>` from the current commit (base recorded), untracked `.env` files copied, node_modules hard-linked, Agentry files in `info/exclude`, a leased port from 7900 to 7999, removal only once merged into dev | 2026-10-16 | `go test ./internal/worktrees` ok: `.env` copied, node_modules hard-linked, Agentry files ignored by git, unmerged branch kept, merged one removed with its branch despite a stray lockfile, busy ports skipped |
| T-011 | Session per agent turn: one tmux server per task (`agentry-<task>`), `remain-on-exit` set before the command starts, stale socket removed, `AGENTRY_*` and `PORT` set, the engine in never-ask mode on a one-line prompt pointing at `.agentry/<turn>.md`, its hash recorded in `turn.started` | 2026-10-16 | `go test ./internal/sessions ./internal/turns` ok: env and dir reach the process, a killed process leaves a dead pane, a new turn replaces the old, a stale socket is removed, the brief is a file and the launch is one line with its hash in `turn.started`. Live check: Claude in a worktree of a trusted repo starts with no dialog and fires `UserPromptSubmit` and `Stop` for the command-line prompt |
| T-009 | `agentry hook <event>` posts to the server (`/hooks`, hook token) with its own deadline and exits 2 on any failure; package hooks turns signals into events, gates actions through the policy, and waits for your decision on an ask | 2026-10-16 | `go test ./cmd/agentry ./internal/hooks` ok: unreachable, slow and broken servers all exit 2 within the 300 ms deadline; reports acknowledge the prompt and end the turn; allow, deny and ask follow the policy; an ask waits for your approval and is denied if the hook gives up; a call from an agent that is not working is denied; the endpoint needs the hook token |
| T-013 | Policy as data: the developer defaults from proof 01 as one ordered rule list (deny, then explicit asks such as every `git push`, then allows, then ask), judged by effect; an ask writes `action.asked` and waits for you (T-009) | 2026-10-16 | `go test ./internal/policy` ok: 40 cases, the 32 from proof 01 plus the push ask, `cd` into the worktree, relative file commands, `2>&1`, an agent trying to approve itself (asks), Agentry's database and hook token (denied) |
| T-032 | Coordinator loop: reads the log by position and does the one move `Next` gives (worktree, turn, decision, issue, or ask you); the coordinator's brief lists the agents, the trail and refused actions; each agent's report and every issue go back to it; safety stops at 24 turns or 3 issues in a row | 2026-10-16 | `go test ./internal/coordinator` ok: on real store, tmux and git a task goes coordinator, engineer, coordinator, done, each brief holding its prompt, instructions or reports; the session ends with the turn; an unmerged worktree is kept. 11 `Next` cases and both safety stops |
| T-014 | Watchdog on a 1 s tick: dead pane, silence, tool open too long, repeated command, each an issue that ends the turn and goes back to the coordinator; an unacknowledged prompt waits for you | 2026-10-16 | `go test ./internal/watchdog` ok: a dead process flagged after the 3 s grace on a 1 s tick (under 5 s) and its session stopped; silence over 10 min is a stall; an hour waiting for your approval is not; a tool over 20 min; the same command 5 times; a prompt never acknowledged after 60 s waits for you and clears when it arrives |
| T-033 | Deployer: its prompt merges `origin/dev` and pushes `HEAD:dev`, the policy asks for every push, and the worktree is removed once its branch is in `origin/dev` | 2026-10-16 | `go test ./internal/agents ./internal/worktrees` ok: every git command in the default prompts passes the policy and the push asks; after `git merge origin/dev` and `git push origin HEAD:dev` to a bare origin the worktree is merged and removed |
| T-006 | `agentry serve`: one process with the store, seeded agents, coordinator loop, watchdog, hook endpoint and the HTTP API for the page (tasks, answer, approve, deny, agents, long-poll changes), behind a UI token the agents never get | 2026-10-16 | `go test ./internal/web` ok: create, list newest first with the default repo, show with trail and events, approve only what was asked (twice is a 409), no UI token or the hook token is a 401, the printed link sets the cookie, `/api/changes` answers as soon as the log moves. Smoke run of `agentry serve`: 5 agents seeded, a task on a non-repo lands in needs you with the reason |
| T-034 | The page: one plain HTML file embedded in the binary, no build step; add a task (repo defaults to `serve --repo`), task list with status, the selected task with Needs you (approve, deny, answer), handoff trail, actions and every event, the agents and their prompts; live by long-polling the log | 2026-10-16 | Rendered in headless Chromium against `agentry serve` driven by a scripted fake Claude over the real hook path: a task went coordinator, engineer, reviewer, tester, deployer in 8 turns, the push showed in Needs you with its rule, Approve through the API pushed to `origin/dev`, turn 9 said done and the worktree was removed. A second task reused port 7900 |
| T-018 | Terminal panel: one `tmux -C` client per watched session fanned out over a websocket to xterm.js 5.5.0, input only from the viewer in control, kept across the page's redraws and reconnected for each turn. Also: hook calls are cancelled on shutdown and asks left open are denied at start | 2026-10-16 | `go test -race ./internal/terminal` ok: two websocket viewers see the same output, keys typed while watching are dropped, only one takes control, both are disconnected when the session ends. Rendered on the page in headless Chromium: the deployer's live terminal under its push approval, with Take control. Restart check: approvals left open are denied at start, so no task shows an approval nothing waits for |
| T-035 | MVP milestone: a real task on a real repo, from the page to done, through every agent and one approval | 2026-10-16 | Real Claude Code 2.1.301 on a fresh Go repo with a local bare origin: you, coordinator, engineer, coordinator, reviewer, coordinator, deployer, coordinator, done in 7 turns; the coordinator skipped the tester and said why; the push was approved and `origin/dev` got `a052bd6` with `Hello("")` fixed, the TODO gone and a test added; worktree removed. 89 events, 0 rejections; 17 actions, 14 allowed by policy, 3 asked. The trust dialog of the new repo showed as Needs you after 60 s and was answered in the session |
| T-039 | Policy judges only shell code: quoted text and heredoc bodies are data, a redirect to a relative path is an edit inside the worktree, and a newline separates commands | 2026-10-16 | `go test ./internal/policy` ok: both commands that needlessly asked in T-035 are now allowed; redirects out, `$(...)` inside double quotes, a heredoc into `bash` and an unterminated heredoc still ask; `ls` then `rm` on the next line now asks (it was allowed before) |
| T-040 | Policy file: `~/.agentry/policy.toml` in TOML, written on first start from the shipped `default.toml`; rules match tools, `runs_any` / `runs_only` globs over the command as parsed by mvdan.cc/sh, `paths` and `where`; four profiles (developer, read-only, autonomous, allow-everything) and `use_for_everyone`; checked on load; Agentry's own files protected in code. The Go rule list is gone | 2026-10-16 | `go test ./internal/policy` ok: every earlier case holds on the shipped `default.toml` (developer), plus loops over known words, variables set in the command, `set -e` and Claude's scratch folder now allowed; read-only, autonomous and allow-everything profiles; `use_for_everyone` wins over every profile; Agentry's own files denied in all four; 9 malformed files refused with the reason |
| T-041 | A policy profile per agent: `profile` on the agent, shown on the page; reviewer and coordinator `read-only`, engineer, tester and deployer `developer`; existing default agents get theirs on the next start; the gate judges with the working agent's profile | 2026-10-16 | `go test ./internal/agents ./internal/hooks` ok: the five defaults carry their profiles; upgrades fill only empty profiles, once, and keep a profile the user set; the gate judges with the working agent's profile (the engineer: developer) |
| T-042 | `policy.toml` reloads when saved: a valid file replaces the policy and writes `policy.loaded`, a broken one is refused (`policy.refused`), the policy before it stays and the page shows why; every approval and denial records the policy hash; the page shows the profiles and rules | 2026-10-16 | `go test ./internal/policy ./internal/web` ok: valid change loaded, broken file refused once and the working policy kept, a fix clears the problem, a broken first file stops the start. Live on the running server: `version = 9` refused with the reason and the working policy kept; restoring the file cleared it; the five agents got their profiles |

Earlier work: seven proofs, run before the main build. Their findings are in "From the proofs" below; the proof code isn't in this repository.

## From the proofs

Every finding the MVP depends on, and the task that must use it. A task is not done until its rows here hold. The proof's README has the measurements.

| Proof | Finding to use | Task |
|---|---|---|
| 01 claude-hooks | The gate is `PreToolUse`; the hook has its own deadline (`AGENTRY_HOOK_DEADLINE`) shorter than the engine's timeout, and ends `\|\| exit 2`, so a killed or crashed hook blocks | T-009 |
| 01 claude-hooks | Reporting hooks end `\|\| true` and never exit 2 (exit 2 on `UserPromptSubmit` blocks the prompt) | T-009 |
| 01 claude-hooks | Rules judge the effect, not the tool: refused Edit, Claude used `sed -i`. Developer default: inside the worktree allowed, reaching outside asks, dangerous denied | T-013 |
| 02 prompt-delivery | `UserPromptSubmit` carries the exact text, so its hash proves delivery | T-011 |
| 02 prompt-delivery | Never paste: one line pointing at a file. Pastes get wrapped, tabs change, `send-keys` stops under 16 KB; a leading `/` or `!` runs as a command, `!` past the gate | T-011 |
| 03 watchdog-signals | `Stop` ends a turn (under 1 s) and carries `last_assistant_message`, the agent's report | T-032 |
| 03 watchdog-signals | `Notification` with `permission_prompt` (about 6 s) or `idle_prompt` (about 60 s) | T-014 |
| 03 watchdog-signals | A crash fires no hook: only `#{pane_dead}` with `remain-on-exit` shows it | T-011, T-014 |
| 03 watchdog-signals | Network stalls and stuck tools need timeouts; the screen keeps animating, so "screen unchanged" detects nothing. Loop detection compares the command, not its description | T-014 |
| 04 terminal-view | One `tmux -C` client per session fanned out over a websocket to xterm.js 5.5.0; input dropped from watchers; one controller at a time; survives a 1.4 MB burst | T-018 |
| 04 terminal-view | A stale tmux socket after a kill makes the next server die: remove it first | T-011 |
| T-011 check, 2026-10-16 | Claude in a new folder stops at "do you trust this folder". A git worktree inherits its main repo's trust, so a repo opened with Claude once needs nothing. Once trusted, the prompt on the command line fires `UserPromptSubmit` and `Stop` | T-011, T-014 |
| 05 event-store | SQLite append-only by triggers, one writer, every write through `propose()` (shape, who, order, policy); a race on one task accepts exactly one | T-003 |
| 05 event-store | `action.asked` is the only route to Needs you, or the board flickers | T-002, T-004 |
| 05 event-store | Readers pull by position with their own cursor; a pushed slow reader stalls writes | T-005 |
| 06 worktree-lifecycle | A leased port per task (shared port: 2 of 3 tests hit another agent's server); copy `.env`; hard-link deps (about 10 ms against 0.4 s for an install) | T-010 |
| 06 worktree-lifecycle | Git refuses one branch in two worktrees. Removal: confirm merged, `remove --force` (setup leaves untracked files), delete the branch, `git worktree prune` | T-010, T-033 |
| 07 engine-adapters | One adapter interface (Prepare, Launch, ParseHook, GateReply, plus IsGate) and its recorded-payload tests. Deliver waits for follow-ups (T-012); Support and InstructionsFile wait until something reads them | T-008 |
| 07 engine-adapters | Codex: same hook events and fields in `.codex/hooks.json`; `PreToolUse` can only deny and needs a reason; allow goes through `PermissionRequest`; no `Notification`, so `PermissionRequest` is the waiting signal | T-008, T-014 |

## Decisions

- 2026-10-15 Proofs first, then the main build. Proof code is never imported; findings are rewritten properly. Rules out copying proof packages wholesale.
- 2026-10-15 Go for the server, worker and hook, as one `agentry` binary; TypeScript and React for the web board, embedded in the binary. Rules out a separate runtime on worker machines.
- 2026-10-15 SQLite through the pure-Go driver for v1, append-only by triggers, one writer. Postgres or NATS only when there are many workers.
- 2026-10-15 Engines run in never-ask mode; Agentry's hook is the only gate. Requires the hook's own deadline, because a killed hook fails open in that mode.
- 2026-10-15 Prompts are delivered as one typed line pointing at a file in the worktree, never pasted. Proof 02: pastes get wrapped, `send-keys` stops at 16 KB, and a leading `!` runs as a shell command past the gate.
- 2026-10-15 Developers approve their own agents' requests in v1; every request is routed to an approver so senior approval can be added later without changing the gate.
- 2026-10-15 One repository, `agentry`: the binary in `cmd/agentry`, Go packages in `internal/`, the board in `web/`, proofs kept outside the repository. Confirmed by the user. Replaces the `agentry-api` / `agentry-web` / `agentry-cli` split in `IDEA.md`, because v1 is one binary.
- 2026-10-15 Git on branch `dev` (the house rule names branches after environments). One commit per task, including this file. Confirmed by the user.
- 2026-10-15 Remote: private GitLab project `gitlab.com/dev.chempa/agentry`, `dev` pushed after each task. The user asked for the GitLab repo; private because nothing says it should be public yet. The module path matches it.
- 2026-10-15 `.gitignore` already covered `bin/`; T-001 needed no change to it.
- 2026-10-16 The goal moved from the full core loop to the coordinator MVP, at the user's request. The old Next list is kept: what the MVP needs stays in Next, the rest is in Later with its ID unchanged.
- 2026-10-16 A task, not a ticket, in the MVP's code and page: it is the user's word. Event names follow (`task.created`); the board can rename later if it needs tickets.
- 2026-10-16 Agents are data in the event log (`agent.created`, `agent.changed`), not files or code. The five defaults ship in the binary only as the seed for first start. Rules out a hard-coded list of roles: the user wants to add and edit agents from the page (T-037), and the coordinator must route to whatever agents exist.
- 2026-10-16 The coordinator is an ordinary agent with its own prompt, like the others ("supervisor is just an agent"). It answers in a fixed format that the loop parses; it never starts agents itself. The loop adds the current agent list to its prompt each turn.
- 2026-10-16 Each agent turn is a fresh Claude session in the task's worktree, started with its prompt plus the coordinator's instructions on the command line. Its final message (the `Stop` hook's `last_assistant_message`, proof 03) is its report. Rules out typing follow-ups into a running session for the MVP.
- 2026-10-16 Safety stops: 24 turns since the task started or the user last answered (about 12 handoffs there and back), or 3 issues in a row, and the task goes to the user. Without them a coordinator that keeps sending work back, or an agent that keeps failing to start, loops forever. Counted in turns rather than handoffs because coordinator turns that only raise issues cost the same.
- 2026-10-16 The deployer merges the task branch into `dev` and pushes, and the push always asks. Chosen by the user over running a deploy command or preparing only.
- 2026-10-16 Each task names a local repo path on the page; `serve --repo` sets the default. Chosen by the user over a sample repo or this one.
- 2026-10-16 The MVP page is one plain HTML file with a little script, embedded in the binary. React waits for the full board (T-017). Rules out a build step for the MVP.
- 2026-10-16 Every proof finding the MVP needs is listed in "From the proofs" with the task that uses it, at the user's request ("the feaures that we have built in the proofs, make sure to use them please to make things workk well"). This brought the terminal panel (T-018), the Codex adapter (T-008) and the prompt hash check (T-011) back into the MVP. Proof code is still ported, not imported.
- 2026-10-16 One concern per package, at the user's request: `internal/events`, `store`, `tasks`, `agents`, `coordinator`, `engines`, `hook`, `worktrees`, `sessions`, `policy`, `watchdog`, `web`. Packages depend downward only (`events` depends on nothing). Each file does one job, and names say what it does without a comment.
- 2026-10-16 Adapters return engine-neutral signals, not catalogue events, so `engines` depends on nothing; package hooks turns signals into proposals. The proof's adapter returned events, which tied every adapter to the catalogue.
- 2026-10-16 Agentry never writes Claude's trust into `~/.claude.json`: it is the user's file, and Claude rewrites it while running. An untrusted repo shows the dialog in the session instead; the watchdog flags a prompt that never arrives as waiting for you, and you answer it from the terminal panel.
- 2026-10-16 A turn waiting on something only a person can answer (a prompt never acknowledged, or the engine's own permission prompt) is Needs you, besides `action.asked`. Proof 05's flicker came from the transient permission notification; this lasts until the agent acts or the prompt arrives.
- 2026-10-16 The policy is a TOML file, not code, at the user's request ("I don't want rules in code please"; "let use toml please"). TOML because a hand-edited safety file must not guess types (YAML), must allow comments (JSON) and must keep patterns readable (single-quoted literal strings). Rules match with words and globs over a parsed shell command (mvdan.cc/sh), not regexes; every rule carries a `why` field, which survives the page rewriting the file where comments wouldn't. One protection stays in code: agents can never touch the policy file or Agentry's data, or an agent could delete that rule and allow itself anything.

## Blocked
