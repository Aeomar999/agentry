# Agentry

A task board where the workers are AI agents. You add a task on the page, or tell Claude in the page's terminal and it writes the task for you. A coordinator agent decides who works on it next: the engineer, reviewer, tester or deployer. Every agent hands its report back to the coordinator, and any problem goes back to it too, until the coordinator says the task is done.

Every action an agent takes passes Agentry's gate first, and your policy file decides what runs, what is refused and what waits for you. A watchdog catches agents that crash or go silent. You can watch any agent's terminal live and take control of it. Every step is an event in one append-only log.

`PLAN.md` has what's built, what's next, and the decisions behind it.

## How it works

### How it fits together

Everything is one program, `agentry serve`. Its pieces never call each other to say what happened: they write events to the log, and the others read the log. Each agent runs in its own tmux session, in its own git worktree of your repo, and every action it takes comes back through the gate.

<p align="center">
  <img src="docs/diagrams/how-it-fits.svg" alt="How Agentry fits together: the page, the log, the coordinator loop, the gate and the watchdog in one program, and each task in its own tmux session and worktree" width="100%">
  <br><sub>The page, the coordinator loop, the gate and the watchdog only talk through the log.</sub>
</p>

### How a task moves

The coordinator is an ordinary agent. It never starts anyone itself: it ends each turn with a decision, and the loop carries it out. Every agent's report, and every issue the watchdog finds, goes back to the coordinator, until it says the task is done.

<p align="center">
  <img src="docs/diagrams/how-a-task-moves.svg" alt="How a task moves: the coordinator at the centre sends a brief to the engineer, reviewer, tester or deployer and gets a report back" width="100%">
  <br><sub>The coordinator sends each agent a brief and gets its report back; a real run took 7 turns.</sub>
</p>

### What happens to one action

Before an agent runs a command or edits a file, its engine calls `agentry hook`. Anything short of a clear yes blocks the action, including the server being down or slow: the gate fails closed.

<p align="center">
  <img src="docs/diagrams/one-action.svg" alt="What happens to one action: the hook, the log, the built-in protection, the profile rule, then allow, ask or deny" width="100%">
  <br><sub>Five checks before anything runs, and every way out that is not a clear yes blocks it.</sub>
</p>

## Run it

You need Go 1.27, tmux and git. For the agents, install Claude Code (signed in) and, optionally, Codex.

```sh
go build -o bin/agentry ./cmd/agentry
./bin/agentry serve -repo /path/to/your/repo
```

The UI is built into the binary, so this needs no Node. To change the UI, see [Work on it](#work-on-it).

Open the link it prints. The token in the link signs the page in, and stays the same across restarts. The default address is `127.0.0.1:7800`. If you're on a remote machine, forward that port.

- **Repo:** each task works in its own git worktree of the repo (`-repo` sets the default; the page can name another), on branch `agentry/<task>`, with its own port in `PORT`.
- **Data:** `~/.agentry` holds the log (`agentry.db`), the worktrees and the policy. Change it with `-data`.
- **Trust:** open the repo with Claude Code once beforehand. Claude asks whether to trust a new folder, and a worktree inherits its repo's answer. If you skip this, the first task shows "needs you". Open its terminal on the page and answer the question there.
- **Deploying:** the deployer merges `origin/dev` and pushes the task branch to `dev`. As shipped, agents allow everything, so the push goes straight through; with the profiles on, every push waits for your approval on the page.

## Use it

The page is an admin console: tasks, agents and the policy in the sidebar, and everything updates live.

- **Add a task:** New task (or press `n`), or ask Claude (below).
- **The terminal:** `Ctrl+J`, or Terminal in the status bar, opens a terminal under the page. It has a tab for each agent that is working and one for Claude. Drag its top edge to make it taller, up to the whole page. You watch an agent's session; Take control lets you type into it. If you open the page inside VS Code, VS Code keeps `Ctrl+J` for itself, so use the button.
- **Claude:** the Claude tab is a Claude Code session that Agentry runs for you in the default repo. Tell it what you want done. It reads the code, drafts the task (title, what to change, what done looks like, which repo), shows it to you, and hands it to the coordinator. It can also answer a coordinator's question for you. It can't approve an agent's actions or edit files: approving stays on the page.
- **Needs you:** a task waiting for you shows its question or the action to approve at the top of its page, and the bell counts them.
- **Keys:** `Ctrl+K` searches everything, and `?` lists every shortcut.

## The policy

What agents may do is in `~/.agentry/policy.toml`, written on first start from [`internal/policy/default.toml`](internal/policy/default.toml), which documents every field. Save the file and it takes effect within two seconds. A file with a mistake is refused, the policy before it stays in force, and the page says what's wrong.

- **Profiles:** each agent has one. The reviewer and coordinator are `read-only`, and the engineer, tester and deployer are `developer`. There are also `autonomous` (anything on this machine, asks before acting outside it) and `allow-everything`.
- **Rules:** in a profile, rules are read top to bottom and the first match decides `allow`, `ask` or `deny`. A rule can match `tools`, commands (`runs_any`, `runs_only`), `paths`, and `where` (the workspace or outside). Every rule has a `why`, which the page shows.
- **Allow everything, as shipped:** the default sets `use_for_everyone = "allow-everything"`, so every agent runs anything. Comment that line out to give each agent its own profile instead.
- **Built in:** no agent may touch the policy, the log or the sign-in token, whatever the file says.

## Work on it

```sh
make test    # go vet, go test, and the UI's type and format checks
make         # rebuild the UI, then the binary
```

The UI lives in `ui/`: React, TypeScript, Vite and Tailwind, built into `internal/web/dist`, which is committed. `cd ui && npm run dev` serves it with live reload against a running `agentry serve`. Its components go `atoms` (spend the design tokens in `src/styles/globals.css`), `molecules`, `organisms` (own state) and one template, `AdminShell`, that every page renders in.

Each folder in `internal/` does one job, and folders only depend on the ones below them:
- `events` lists what can happen.
- `store` keeps the log.
- `tasks` and `agents` compute state from the log.
- `rules` checks each event before it's written.
- `policy`, `hooks`, `engines`, `worktrees`, `sessions` and `turns` run the agents.
- `coordinator` and `watchdog` move tasks along.
- `terminal` and `web` serve the UI and its live terminals.
- `desk` runs the Claude you talk to; `agentry task` is how it hands work over.
