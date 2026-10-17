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
- **Budget:** `-budget` is the tokens a new task may use before it asks you; 0 for no limit.
- **Trust:** Claude asks whether to trust each new folder. Agentry answers for every task's worktree before Claude starts there, in Claude's own config (`~/.claude.json`), so turns don't stop at the question.
- **Deploying:** the deployer merges `origin/dev` and pushes the task branch to `dev`. As shipped, agents allow everything, so the push goes straight through; with the profiles on, every push waits for your approval on the page.

## Deploy it

`agentry.chempa.dev` runs from the `dev` branch: GitLab tests the commit, builds the image (`Dockerfile`: the server, git, tmux and Claude Code) and asks Berth to deploy `docker-stack.dev.yml`, pinned to the workstation. Everything worth keeping is in the container's home, a volume: Claude's sign-in, the repos under `~/work`, and Agentry's log and policy in `~/.agentry`.

- **The link:** the server prints it with its token: `ssh vps berth logs agentry dev server`.
- **Signing Claude in:** once, after the first deploy: open the dock's Claude tab and follow Claude's sign-in. The agents use the same sign-in.
- **Repos:** clone them into `~/work` (ask the desk, or from an agent's terminal) and name them on a task.

## Add workers

Agents can run on other machines that have Claude Code, git and tmux. On the Workers page, Add worker shows one command with a one-time code in it; run it on the other machine. It installs the `agentry` command (in `~/.local/bin`, added to your PATH), joins the server, and starts the worker in the background: a systemd user service on Linux, a launchd agent on macOS. Run on a machine that has joined, it updates it instead.

On the worker, `agentry worker` does the rest:

| Command | Does |
| --- | --- |
| `agentry worker status` | the server, the service, the link, the engines and tools found, the tasks running here |
| `agentry worker logs [-f]` | its log, `~/.agentry-worker/worker.log` |
| `agentry worker stop` / `start` / `restart` | stop or start the background service |
| `agentry worker update` | get the server's agentry now and restart; the worker also does this whenever it connects |
| `agentry worker leave [-purge]` | take the machine off the server and stop; `-purge` deletes its repos and worktrees too |
| `agentry worker run` | run in the foreground (what the service runs) |

Give an agent workers under Runs on in its settings; it uses the first one online, in that order. A task's work moves to the machine of the agent working next, through git: the repo on the server is the hub, and what was left uncommitted goes too. The worker dials out to the server, so it needs no open port; its sessions post their hooks to the server's address. The server hands out agentry for macOS and Linux, on Intel and ARM (`make workers` builds them next to `bin/agentry`).

## Use it

The page is an admin console: tasks, agents, workers and the policy in the sidebar, and everything updates live.

- **Add a task:** New task (or press `n`). Describe it in your own words; a title, labels and a token budget are optional. The coordinator reads it first and gives it a title that says what it is.
- **Tasks:** a list, or a board (keys `1` and `2`) with Backlog, To do, In progress, Review and Done. Drag a card onto any column to tell the coordinator what you want: park it, start it, have it reviewed, send it back, wrap it up, or reopen a finished one. Drag within a column, or a row in the list, to set your priority order. The Backlog shows when you ask for it. Labels filter with a click.
- **A task:** its plan down the left, and the path its work took (`engineer › reviewer › tester`) above the tabs. Each agent writes its plan before it starts, as a checklist in `.agentry/<turn>-plan.md`, and ticks it off and changes it as it works; the page follows it live, with earlier agents' plans greyed and closed under it. The tabs: Coordinator, a chat with the task's coordinator (what you send stops the turn in progress so it reads it at once, and it replies with what it changed; its questions show here and are answered in Questions), Activity (everything that happened, newest first, with times and what is still running down the right), the agent's live Terminal (you watch; Take control lets you type into it), Handoffs, Actions, Events and Details. A handoff, an action or an event opens the activity of its turn, with the row you clicked picked out.
- **A task's Workspace** (its button on the task page): the worktree as an editor's git view shows it. Source control lists what isn't committed, then the branch's commits, newest first, each opening to the files it changed; the explorer shows every file with its git status (M, A, D, U). A file opens next to its last committed version, or in a commit next to the version before it, so each round of work is the commits at the top. `Ctrl+J` opens its dock: the Coordinator chat, Claude, the agent's Terminal and the Agents who worked here.
- **Claude in the Workspace:** a plain Claude Code session of your own in the task's worktree, on whichever machine has it, for your own questions. It changes no files and has none of Agentry's hooks. When you want something changed it runs `agentry draft "..."`, which only drafts a message: it waits in the chat and in its Claude tab for you to send, change or discard, because sending starts the agents' work.
- **Agents:** add, edit and remove them: prompt, engine, model, policy profile, colour, which column their work counts as, and the machines they may run on (none: the one Agentry runs on). Each agent's colour marks its work everywhere.
- **The dock:** `Ctrl+J`, or Terminal in the status bar, opens a panel under the page; drag its top edge to make it taller, up to the whole page. If you open the page inside VS Code, VS Code keeps `Ctrl+J` for itself, so use the button. Whenever something is waiting for you, it opens on Questions. Its tabs:
  - **Questions:** everything waiting for you, from every task: approvals, the coordinator's questions (sometimes with options, or a small page it made to show you something), a task stopped at its budget, an agent stuck on a prompt. The status bar counts them.
  - **Claude:** a Claude Code session Agentry runs for you in the default repo. Tell it what you want done; it reads the code, drafts the task, and hands it to the coordinator. It can also answer a coordinator's question for you. It can't approve an agent's actions or edit files.
- **Budgets:** every task has a token budget (`-budget`, 5M by default). When a task reaches it, it stops before the next turn and asks you to raise it or wrap up.
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
