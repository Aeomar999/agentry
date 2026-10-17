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

- **Add a task:** New task (or press `n`). Describe it in your own words; a title, labels and a token budget are optional. Its work goes in the default repo, another repo, or a new repo Agentry makes at the path you give (`--new-repo` from the desk). Every task works in a worktree of its repo, so a new project should be a new repo, not a task in the default one. A task made in the wrong repo moves with Change repo in its Details: when the turn in progress ends, its worktree is made again in the new repo, the old one is kept aside, and the coordinator is told. The coordinator reads it first and gives it a title that says what it is.
- **Tasks:** a list, or a board (keys `1` and `2`) with Backlog, To do, In progress, Review and Done. Drag a card onto any column to tell the coordinator what you want: park it, start it, have it reviewed, send it back, wrap it up, or reopen a finished one. Drag within a column, or a row in the list, to set your priority order. The Backlog shows when you ask for it. Labels filter with a click.
- **A task:** its plan down the left, and the path its work took (`engineer › reviewer › tester`) above the tabs. Each agent writes its plan before it starts, as a checklist in `.agentry/<turn>-plan.md`, and ticks it off and changes it as it works; the page follows it live, with earlier agents' plans greyed and closed under it. The tabs: Coordinator, a chat with the task's coordinator (what you send stops the turn in progress so it reads it at once, and it replies with what it changed; its questions show here and are answered in Questions), Activity (everything that happened, newest first, with times and what is still running down the right), the agent's live Terminal (you watch; Take control lets you type into it), Handoffs, Actions, Events and Details. A handoff, an action or an event opens the activity of its turn, with the row you clicked picked out.
- **A task's Workspace** (its button on the task page): the worktree as an editor's git view shows it. Source control lists what isn't committed, then the branch's commits, newest first, each opening to the files it changed; the explorer shows every file with its git status (M, A, D, U). A file opens next to its last committed version, or in a commit next to the version before it, so each round of work is the commits at the top. `Ctrl+J` opens its dock: the Coordinator chat, Claude, the agent's Terminal and the Agents who worked here.
- **Claude in the Workspace:** a plain Claude Code session of your own in the task's worktree, on whichever machine has it, for your own questions. It changes no files and has none of Agentry's hooks. When you ask it to send the coordinator a change, it runs `agentry send "..."`: the message goes at once, as yours, the same as typing it in the chat, and the chat marks it as sent from Claude. Claude Code asks you before it runs that command, so Claude never sends one on its own.
- **Showing work:** agents write what they want you to see as files in the task's worktree (a design in Markdown, its SVG diagrams) and link them in the chat, as `[the design](docs/design.md)`. A link opens the file in the task's Workspace, in a new tab, already in its preview.
- **Claude Code in any repo:** `agentry mcp install` adds Agentry to Claude Code for every repo, and its status line (`agentry · 1 needs you · 3 running`). Then plain `claude` in a repo can create tasks and epics there, file and order stories, start them, tell a coordinator something, answer its questions, and show your board (`/mcp__agentry__board`). Every reply ends with what waits for you. Approving an agent's action or closing an epic is asked of you by Claude Code itself (MCP elicitation), so the model can't approve for you; where that isn't supported, Claude gives you the page's link. Claude also hears from Agentry between your messages, through Claude Code's hooks: what waits for you is added when a session starts and with each message, and after each reply a waiter runs in the background that wakes Claude with news the moment there is some, so it tells you without being asked: something new waiting for you (a question, an approval, a budget stop, an agent stuck on a prompt, an epic to close), a task that finished with its summary, a coordinator's reply to what you told it, an epic split into stories that wait for you to start them, and a machine gone offline with work on it. While Claude Code runs, the same news shows as a notification on your computer. It all goes into the same log, so the page shows it at once. To connect any machine, open **Integrations** in the sidebar and make an install command: `curl -fsSL '<server>/integrations/claude-code.sh?code=…' | sh`. It installs agentry, connects with a code that works once for 15 minutes, and sets up Claude Code there. The machine gets its own credential (kept in `~/.config/agentry/connection.json`; the log holds only its hash), the page lists every machine connected, and Revoke stops one at once. On the machine Agentry runs on, `agentry mcp install` does the same with nothing to connect, since `serve` writes where it is to `~/.agentry/server.json`. Machines of another kind download the builds `make workers` makes.
- **Plan:** a page for shaping work before it runs. Each epic is a column of its stories, and one more column holds the tasks of no epic. Drag a card up or down its column to set its priority, or to another column to file it there, at the place you drop it; the epic's Stories tab reorders the same way, and its coordinator reads its stories in that order. A story that hasn't started has Start, and each epic has Start the ready ones, for its stories that wait for no other. Started work is on the Tasks board, which shows the Backlog only when you ask. Finished epics fold away at the end.
- **New task** is a modal over any page (`N`, the sidebar, or + Story and + Task on Plan); its address (`?new=task`, `?new=epic`, `?new=story&newIn=task-E`) can be linked.
- **Epics and stories:** a task is a story: one goal, on one branch, checked before it is done. A goal too big for that is an epic. Its coordinator splits it into stories with a `split` decision, and each story is a task of its own, run by its team's coordinator (another team's, for a story given to them). Stories run side by side, except that one that builds on others (`after`) starts once they are done, from dev with their work in it. Each finished story's summary goes back to the epic, whose coordinator adds stories, waits for the rest, or asks you. Agentry refuses the epic's "done" while any story isn't, and once they all are, the epic asks you: Close the epic, or say what else it needs and its coordinator plans it. Stories close on their own; only you close an epic, on the page. A story can't be split, so there are only two levels. New task can make a task a Story or an Epic; by default the coordinator splits a task that is too big. The epic's page has a Stories tab with their progress. In the list, stories sit under their epic and fold away. New stories wait in your Backlog, grouped by epic, until you drag them on. Tasks you have already can be filed under an epic: drag the card to the epic's column on Plan, pick the epic in a task's Details, or use Add tasks on the epic's Stories tab, which also has New story and Take out. A task is in one epic at most, so filing it again moves it. The desk does the same, with `agentry task create --kind epic --story task-N`, `--epic task-E` on create, and `agentry task epic task-E --add task-N --remove task-M`. Done keeps only the last 7 days; the list keeps everything.
- **Machines:** a coordinator can choose where an agent works, with `"machine"` on an assign: any machine that agent may run on (its Runs on). Its brief lists the machines, whether each is online, what engines it has, which agents may run there, and where the task's work is now; Agentry moves the work there, with its branch, before the turn. A machine the agent may not use, one that isn't there, or one that is offline goes back to the coordinator with why. Unchosen, a turn runs where the work is if the agent may run there, or else on the first of its machines that is online.
- **Checks:** a coordinator can't finish a task until its work is checked. Each agent's work counts as building, review or shipping. The reviewer, tester, fact-checker and critics review, and end their reports with `Verdict: approved` or `Verdict: changes needed`. After the last turn that changed the work, every reviewer that has checked it must approve it again, so a fix made after a review always gets another round. Agentry refuses "done" until then, and tells the coordinator what is missing. The deployer ships, so deploying doesn't call for another review. To finish a task unchecked, drag it to Done on the board.
- **Teams:** agents work in teams, one for each kind of work. Each team has a coordinator, which hands a task's steps only to its own agents. Agentry starts with the Software team; Add a team on the Agents page adds Writing (an editor, researcher, writer and fact-checker), Research (a lead, investigator, analyst and critic) or Design (a product lead, product designer, UX writer and design critic), each agent with a prompt written for that work. New task can pick the team (`--team` from the desk); by default the coordinators decide: the first team's coordinator reads the task and passes it to the team whose work it is. A coordinator that needs something from another team asks it: Agentry makes that team a task, linked to the one that asked, and the asking task waits until it is done, then carries on with its summary. Requests can go three deep, so teams can't ask each other in circles. The Agents page shows the teams as cards, as org charts, or as one list. Remove team takes a team and all its agents, and an agent card's bin removes one agent; Agentry refuses while an unfinished task needs them, and names the tasks. The last team stays, since every task needs a coordinator.
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
