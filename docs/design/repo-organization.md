# Repo organization: the target layout

Status: proposed, for task-250. Design only: no code, package, route or
build file changes with this document. The moves it plans are later work.

## In short

`internal/` becomes eight folders in place of 46 packages side by side:
`platform`, `core`, `domain`, `extensions`, `runtime`, `web`, `mcp` and
`testkit`. Each package sits on one layer, and imports only from the layers
below it. A folder can span two: `core`, `domain` and `extensions` each hold a
low root and higher packages. A test checks that on every `go test ./...`.

The user's proposal is mostly kept. It departs from the proposal in five
places, all because of what the import graph really is:

1. **A domain is a folder, not one package.** Between them, `current`,
   `rules` and `store` import `tasks`, `agents`, `workers`, `connections`
   and `settings` (`store` only `agents`). So a
   domain package that also held its HTTP routes (which need `current` and
   `store`) would make an import cycle. The domain's model stays the folder's
   root package, low in the graph. Its routes, tools and background work go in
   subpackages of the same folder, high in the graph. One change to workers
   still touches one folder: `internal/domain/workers/`.
2. **`core` sits above the domain models, except `events`.** The proposal's
   "core knows no domain" is not true today and can't be made true by moving
   files: the read model is built from the domain models. The rule becomes
   "a domain model knows the log's words (`core/events`) and nothing else of
   core".
3. **The extension framework is the parent of the extensions.** The
   proposal had `core/features` and also `features/ideas`, two things called
   features. The framework, `internal/features` today, becomes
   `internal/extensions`, and each extension nests under it
   (`internal/extensions/ideas`), as `image/png` nests under `image`. The UI
   already has this shape in `ui/src/features/`, which becomes
   `ui/src/extensions/`. It is named extensions, not features, by the user's
   decision 5 (section 10).
4. **Some packages change group.** `machines` goes to `runtime`, not
   `platform`, because it imports `agents`, `workers` and `worktrees`.
   `sessions` and `terminal` go to `platform`, not `domain` or `runtime`,
   because they know no domain. `desk` imports only `sessions` too, but it
   stays in `runtime`: its whole job is an Agentry role, the desk session. Sign-in stays in `web`: it is how the web
   checks who you are.
5. **Moves and refactors are separate steps.** Moving a package is a
   script-made commit that changes only paths. Taking handlers out of `web`
   and tools out of `mcp` changes code, so each domain does that in its own
   reviewed task, after the moves.

The user decided five things (section 10): stop committing the built UI but
keep the history; build the page on the publishing machine and copy it to
the Mac, so the Mac app stays buildable with only Go; move `Agentry/` out of
the checkout and ignore `brag-output/`; land the moves one group per
step, between tasks; and name the plug-in framework extensions, while
`features.toml` and `/api/features` keep their names.

![The layers and what each may import](repo-organization/layers.svg)

## 1. What is there today

Measured on `fc373495` (lines are Go, without tests / tests):

- 46 folders under `internal/`, 48 packages with `sessions/sessionstest`
  and `tasks/taskstest`. The largest are `web` (6,221 / 12,009),
  `policy` (3,687), `cmd/agentry` (3,272), `coordinator` (2,842) and
  `store` (2,707).
- `internal/web` has 89 routes in `server.go` and imports 23 of Agentry's
  packages. Its files are already split by domain (`agents_api.go`,
  `workers_api.go`, `board_api.go`, ...), so the split exists, inside one
  package.
- `internal/mcp` is `agentry mcp`, a separate process that reaches the server
  only through its HTTP API (`client`). Its tools for tasks, files and
  waiting live in `mcp`; the ideas tools already live in `internal/ideas`,
  through `features.Tool` (`extensions.Tool` in the target).
- `ideas` and `schedules` already work the way the proposal wants every
  domain to: one package with its events, state, rules, routes, tools and
  waiting entry, registered through the extension framework,
  `internal/features` today.
- The real graph has layers. From the bottom:

| Layer | Packages today | Imports of Agentry's own |
|---|---|---|
| Know no Agentry concept | `exactgob`, `procs`, `hostaddrs`, `schema`, `engines`, `hookcall`, `sessions`, `terminal`, `worktrees`, `workspace`, `tlsca`, `pinned`, `client`, `launch` | only each other |
| What the log holds | `events` | none |
| Models: state from events | `agents`, `tasks`, `workers`, `connections`, `settings`, `policy`, `sharedfiles` | `events`, each other |
| The extension framework, named `features` today | `features` | the models |
| The log and the read model | `store`, `current`, `rules`, `httpapi` | the models, `features` |
| What runs | `coordinator`, `turns`, `watchdog`, `archive`, `machines`, `desk` | all of the above |
| A domain's or an extension's surface | `hooks`, `workerlink`, `workerservice`, `waitlist`, `settingsfile`, `webpush`, `ideas`, `schedules` | all of the above |
| The surfaces | `web`, `mcp` | everything |

  Today exactly one line of Go code breaks these layers:
  `coordinator/followups.go` calls `ideas.Visionaries`, a runtime package
  naming an extension. The hook that replaces it already exists,
  `features.Coordinator` (`internal/features/features.go:160`;
  `extensions.Coordinator` after step 1a); only this call is left to move
  onto it, as `docs/design/extensions.md` plans. (Tests import upward
  freely, as Go's external test packages may.)
- `tasks`, `current` and `coordinator` are tightly bound, but as a chain, not
  a cycle: `coordinator` imports `current`, `current` imports `tasks`. Each
  sits on its own layer, so they can live in different folders.
- `spec/functional/*.feature` names 16 areas. Only 4 of the 16 files have a
  `# Sources:` comment naming their code (workers, claude-code-tools,
  worktrees-and-branches, shared-files), and nothing checks those lines.
  `schedules` has no feature file at all.
- `spec/constraints.md` section 1 lists every package by concern, and its
  line 77 has the reviewer check it against `ls internal`. `internal/features/imports_test.go` and
  `internal/httpapi/imports_test.go` check that those two stay leaves.
- About 1,600 paths in `spec/` (`@test` citations and `# Sources:` lines) name code, `internal/web`
  (247), `internal/policy` (169) and `cmd/agentry` (165) most. Every move
  must rewrite them, or `spec/check` fails.
- At the root: `bin/` and `release/` are build output and already
  git-ignored. `Agentry/` (an unpacked release, 138 MB) and `brag-output/`
  (launch videos, 15 MB) are untracked leftovers in the main checkout.
  `internal/web/dist` is committed: 95 files, changed in 107 of 1,488
  commits, and 53 MB of the history's 65 MB of blobs (81%).

## 2. The target tree

```
cmd/
  agentry/            the one binary: flags, wiring, and each subcommand's own steps
  agentry-desktop/    the desktop app (its own go.mod, Wails)
internal/
  platform/           the outside world: OS, git, engines, TLS, encodings; no Agentry concept
  core/               what the log holds (events), the log (store), the read model (current),
                      check 3 (rules), and how every handler answers (httpapi)
  domain/             one folder per spec area; its root package is the model,
                      its subpackages are its routes, tools and background work
  extensions/         the extension framework (the package), and each extension (a subfolder)
  runtime/            what runs: the coordinator loop, turns, the watchdog, machines, the desk
  web/                agentry serve's HTTP side: sign-in, the page, mounting every route
  mcp/                agentry mcp: the protocol, notices, mounting every tool
  testkit/            what tests share across packages; never imported by code
tools/                programs that build or check the repo, never shipped
ui/src/
  components/         shared atoms, molecules, organisms, templates: no domain
  shell/              the frame every page sits in: sidebar, top bar, Ctrl K, toasts
  domains/<area>/     each domain's pages, panels, hooks and types
  extensions/<name>/  each extension's UI, listed by extensions/index.ts
  api/ live/ lib/     the HTTP client, the live event stream, helpers with no domain
spec/  docs/  deploy/  packaging files go to tools/; the rest of the root is as today
```

Inside `internal/`, in full:

```
platform/   client  engines  exactgob  hookcall  hostaddrs  launch  pinned  procs
            schema  sessions (+sessionstest)  terminal  tlsca  workspace  worktrees
core/       events  store  current  rules  httpapi
domain/
  agents/          the model (agents, teams, templates); later agentsapi/
  tasks/           the model (+taskstest); waitlist/; later tasksapi/
  workers/         the model; workerlink/  workerservice/; later workersapi/
  connections/     the model; later connectionsapi/
  policy/          the model (deciding one call); hooks/; later egress/ (proposal)
  settings/        the model; settingsfile/
  sharedfiles/     the model (Disk, Hub)
  notifications/   webpush/
extensions/ (the framework)  ideas/  schedules/; later githost/  email/ (proposals)
runtime/    coordinator  turns  watchdog  archive  machines  desk
web/        (one package; later only sign-in, the page, mounting)
mcp/        (one package; later only the protocol, notices, mounting)
testkit/    storetest  deeptest
```

Depth is at most four below the module: `internal/domain/workers/workerlink`.
Package names don't change, so `workers.Roster` and `workerlink.Server` read
as today; only import paths do. The one exception is the framework, whose
package `features` becomes `extensions` in its own step before any move
(section 7, step 1a).

### Where routes and tools go

A domain's HTTP routes and MCP tools go in one subpackage named after it:
`domain/tasks/tasksapi`, `domain/workers/workersapi`. It returns the same
types an extension does, `[]extensions.Route` and `[]extensions.Tool`, so
`web` and `mcp` mount a domain the way they mount an extension: one way, not
two. A domain's api package takes the dependencies it needs (the store, the
fleet, the loop) in its own struct; an extension gets only `extensions.Core`,
because an extension never holds the store.

`domain/settings/settingsapi` was the first to move (step 12), and the others
copy it:

- **The struct** holds only what its handlers use, as the narrowest types
  that work: `settingsapi.API{Log, State}`, an `extensions.Proposer` (what
  `httpapi.ProposeOrRefuse` takes; web passes the store) and a small `State`
  interface of the reads it makes (web passes the read model). It has
  `Routes() []extensions.Route`, each route with its `Auth`, as an
  extension's has. It has `Tools() []extensions.Tool` only when the domain
  has tools; `mcp` appends those as it appends an extension's.
- **Web mounts it** in `Handler`, through the one `mount(owner, routes, mux)`
  that mounts every extension's routes too, so each sits behind the wrapper
  its `Auth` names. Domains are mounted before the extensions, so an
  extension's route that clashes with one is still the one that fails, by
  name. The `api(...)` lines and the handler file leave `web`; a helper
  `web` uses elsewhere stays there.
- **Tests move with their handlers**, names unchanged, into the api
  package's own `_test` package. They call the routes through
  `web.NewServer`, as the extensions' route tests do, so the token check
  is tested with them.
- **The spec follows**: `@test` citations' paths and any line numbers in
  them, and the `# Sources:` lines that named the old file. The api package
  goes in the Sources of the domain's own feature file (`settings.feature`
  here). Other feature files whose Sources named one of its files keep
  naming that file at its new path, as `ideas.feature` and
  `tasks-and-board.feature` name `settingsapi.go`.

What stays in `web` after that, as the tree in
[web-and-mcp.md](web-and-mcp.md) has it: the server and its mounts, with
the token wrappers (`server.go`, `mount.go`), each domain's API as web
builds it (`domains.go`), sign-in and its one-time codes (`signin.go`,
`signin_codes.go`), the embedded page (`page.go`), the server's addresses
and install commands (`addresses.go`), filling an install script and the
commands that run it (`scripts.go`; the scripts themselves are embedded in
their domains' api packages), agentry for a machine (`binary.go`), the
Workspace Claude's message and its token (`claude_api.go`), and the few
routes that span every domain: `/api/features` (`extensions_api.go`; its
name stays: decision 5), `/api/config`, `/api/theme` and `/api/changes`
(`config_api.go`), and `/api/waiting` (`waiting_api.go`). What stays in
`mcp`: the stdio transport, the protocol and the server instructions, the
list of every tool, which extensions are shown, the waiting list and news
it words, and desktop notices. `web` and `mcp` stay one package each.

[web-and-mcp.md](web-and-mcp.md) lists every file left in `web` and `mcp`
after step 12, where each goes, and the stories that take them there.

## 3. The dependency rules

From the bottom up. A package may import its own layer's packages where the
table says so, and anything in a lower layer.

| Layer | Folder | May import | Never |
|---|---|---|---|
| 1 | `platform/*` | other `platform` packages | anything else of Agentry's |
| 2 | `core/events` | `platform` | anything else of Agentry's |
| 3 | `domain/<area>` (the root: the model) | `core/events`, `platform`, other domains' roots | the rest of `core`, any subpackage, `extensions` |
| 4 | `extensions` (the framework) | domain roots, `core/events`, `platform` | the rest of `core`, any extension |
| 5 | `core/httpapi` | `extensions`, domain roots, `core/events`, `platform` | `store`, `current`, `rules`, and everything above |
| 5 | `core/{store,current,rules}` | each other, `extensions`, domain roots | `httpapi`, runtime, subpackages, extensions' folders |
| 6 | `runtime/*` | each other, `core`, `extensions`, domain roots | subpackages, extensions' folders |
| 7 | `domain/<area>/<sub>` and `extensions/<name>` | everything below, and their own folder's other subpackages | another domain's subpackages, another extension |
| 8 | `web`, `mcp` | everything below | each other |
| 9 | `cmd/*`, `tools/*` | everything | |
| – | `testkit/*` | anything | being imported by code that isn't a test |

In words:

- A model knows only what its events mean. It never reads the log or the
  read model.
- The log and the read model know every model, so that they can build state,
  but no extension by name and nothing that runs.
- `httpapi`, how every handler answers, stays the leaf it is today: it never
  imports the store or the read model, so `web` and every extension's routes
  can use it without a cycle.
- What runs reads the state and writes through the store. It never reaches
  into a domain's routes or an extension.
- Surfaces sit on top. A domain's surface may use the loop, the fleet and the
  store; another domain's surface it may not, so two domains talk through
  the state, not each other's handlers.
- Tests are exempt, as Go's external test packages already are: a test may
  import whatever proves its claim. Only `testkit` is test-only.

Checked against today's graph with the mapping in section 4, these rules
hold for every package but one edge: `coordinator` → `ideas` (section 1).
The import test starts with that edge on a list of known exceptions, which
may only shrink.

## 4. Every package, old path to new path

All Go packages of the module, both binaries and the tools. "Step" is the
migration step in section 7 that moves or renames it.

| Old path | New path | Its job | Step |
|---|---|---|---|
| `internal/client` | `internal/platform/client` | calls a running server's API, for the commands beside it | 5 |
| `internal/engines` | `internal/platform/engines` | everything Agentry needs from an agent CLI | 5 |
| `internal/exactgob` | `internal/platform/exactgob` | gob that reads back nil and empty apart | 5 |
| `internal/hookcall` | `internal/platform/hookcall` | the contract between `agentry hook` and the server | 5 |
| `internal/hostaddrs` | `internal/platform/hostaddrs` | the addresses other machines reach this one on | 5 |
| `internal/launch` | `internal/platform/launch` | finds or starts the server, for `agentry open` and the desktop app | 5 |
| `internal/pinned` | `internal/platform/pinned` | clients that trust only Agentry's CA | 5 |
| `internal/procs` | `internal/platform/procs` | processes, Unix and Windows | 5 |
| `internal/schema` | `internal/platform/schema` | JSON Schemas of tools' arguments | 5 |
| `internal/sessions` | `internal/platform/sessions` | each agent in a session kept by a host process | 5 |
| `internal/sessions/sessionstest` | `internal/platform/sessions/sessionstest` | keeps a test run's sessions to that run | 5 |
| `internal/terminal` | `internal/platform/terminal` | a session live in the browser | 5 |
| `internal/tlsca` | `internal/platform/tlsca` | Agentry's own certificate authority | 5 |
| `internal/workspace` | `internal/platform/workspace` | reads a worktree for the Workspace page | 5 |
| `internal/worktrees` | `internal/platform/worktrees` | each task's worktree, branch and port | 5 |
| `internal/events` | `internal/core/events` | what the log holds | 6 |
| `internal/store` | `internal/core/store` | the append-only log, in SQLite | 6 |
| `internal/current` | `internal/core/current` | the state of every stream in memory | 6 |
| `internal/rules` | `internal/core/rules` | check 3 of every proposal | 6 |
| `internal/httpapi` | `internal/core/httpapi` | how every handler answers | 6 |
| `internal/agents` | `internal/domain/agents` | who can be handed work; teams and templates | 7 |
| `internal/tasks` | `internal/domain/tasks` | a task's state from its events | 7 |
| `internal/tasks/taskstest` | `internal/domain/tasks/taskstest` | logs for tests of tasks | 7 |
| `internal/waitlist` | `internal/domain/tasks/waitlist` | the core's part of what waits for you | 7 |
| `internal/workers` | `internal/domain/workers` | the machines that joined | 7 |
| `internal/workerlink` | `internal/domain/workers/workerlink` | a worker's one websocket to the server | 7 |
| `internal/workerservice` | `internal/domain/workers/workerservice` | keeps `agentry worker run` running | 7 |
| `internal/connections` | `internal/domain/connections` | the Claude Codes that have Agentry's tools | 7 |
| `internal/policy` | `internal/domain/policy` | decides one tool call | 7 |
| `internal/hooks` | `internal/domain/policy/hooks` | the server side of `agentry hook`, the gate | 7 |
| `internal/settings` | `internal/domain/settings` | your settings from the settings stream | 7 |
| `internal/settingsfile` | `internal/domain/settings/settingsfile` | features.toml: extensions' settings and secrets | 7 |
| `internal/sharedfiles` | `internal/domain/sharedfiles` | a task's shared files | 7 |
| `internal/webpush` | `internal/domain/notifications/webpush` | Web Push keys and subscriptions | 7 |
| `internal/features` | `internal/extensions` | the extension framework, renamed in place | 1a |
| `internal/ideas` | `internal/extensions/ideas` | the Ideas extension | 8 |
| `internal/schedules` | `internal/extensions/schedules` | the Schedules extension | 8 |
| `internal/coordinator` | `internal/runtime/coordinator` | the loop | 9 |
| `internal/turns` | `internal/runtime/turns` | starts and stops turns | 9 |
| `internal/watchdog` | `internal/runtime/watchdog` | catches agents that go quiet | 9 |
| `internal/archive` | `internal/runtime/archive` | moves done tasks out of the live log | 9 |
| `internal/machines` | `internal/runtime/machines` | where agents work, local or a worker | 9 |
| `internal/desk` | `internal/runtime/desk` | the Claude session in the dock | 9 |
| `internal/web` | `internal/web` (stays) | `agentry serve`'s HTTP side | – |
| `internal/mcp` | `internal/mcp` (stays) | `agentry mcp` | – |
| `internal/storetest` | `internal/testkit/storetest` | opens stores for tests as serve does | 4 |
| `internal/deeptest` | `internal/testkit/deeptest` | checks copies of state by reflection | 4 |
| `internal/speccheck` | `tools/speccheck` | `spec/check` | 4 |
| `packaging` | `tools/packaging` | `make package`, `make desktop`, `make release-check` | 4 |
| `tools/measure` (shell) | `tools/measure` (stays) | the measuring scripts | – |
| `cmd/agentry` | `cmd/agentry` (stays) | the binary | – |
| `cmd/agentry-desktop` | `cmd/agentry-desktop` (stays) | the desktop app; its imports of `client`, `engines`, `launch` and `procs` change in step 5 | – |

Packages the proposals in `spec/proposals/` will add: `githost` and
`email` are extensions (each cites a `feature_test.go` to write), so
`internal/extensions/githost` and `internal/extensions/email`. `egress`, the
network gate, is part of policy: `internal/domain/policy/egress`.

**Names users see stay.** The framework's rename is inside the code only.
The settings file `features.toml`, the `GET /api/features` endpoint and its
JSON keep their names on purpose: users and existing installs see them, and
renaming them would be a behaviour change (decision 5). So do the words users
read: the Settings page's "Features that are off", "Every feature is on." and
"Couldn't read the features" (`ui/src/pages/SettingsPage.tsx` lines 33-38),
and the scenarios that say the same (`spec/functional/settings.feature` lines
7 and 13, `ideas.feature` line 66). Only names in the code change.

`cmd/agentry` stays one package. Its 21 files are subcommands, and each
holds steps its tests call directly, so it is not just wiring. The rule for
it is the one already in force: when a step grows logic another command or
the server would share, it moves to the package it is about.

### The spec areas and their code

Step 3 writes this table into the feature files as `# Sources:` lines, in
today's paths; the moves then rewrite them like any other path.

| Spec file | Where its code is |
|---|---|
| `tasks-and-board` | `domain/tasks`, `domain/settings` |
| `epics-and-stories` | `domain/tasks` |
| `teams-and-templates` | `domain/agents` |
| `agents` (the agents' turns) | `runtime/turns`, `domain/agents` |
| `coordinator-and-handoffs` | `runtime/coordinator`, `runtime/watchdog` |
| `approvals-and-questions` | `domain/policy/hooks`, `domain/tasks/waitlist` |
| `policy` | `domain/policy`, `domain/policy/hooks` |
| `workers` | `domain/workers`, `runtime/machines` |
| `shared-files` | `domain/sharedfiles` |
| `settings` | `extensions`, `web` (redact), `core/store` and `core/rules` (redact), `ui/src/pages/SettingsPage.tsx` |
| `notifications` | `domain/notifications` |
| `sessions` | `platform/sessions`, `platform/terminal`, `runtime/desk` |
| `worktrees-and-branches` | `platform/worktrees`, `platform/workspace` |
| `sign-in` | `web` |
| `claude-code-tools` | `mcp`, `domain/connections` |
| `ideas` | `extensions/ideas`, `domain/settings` |

Step 3 checked this table against the code; from then on the `# Sources:`
lines are what is kept true, and this table is not.

A spec area doesn't have to be a domain: sessions and worktrees know no
domain, so their code is platform. The map goes from spec areas to code,
not the other way. `extensions/schedules` has no spec area yet; section 6
says how the check treats it.

### The UI

`ui/src` is 31k lines in atomic tiers (`components/atoms` to `templates`),
`pages/`, `lib/`, `api/`, `live/` and `features/ideas` (the Ideas
extension). The UI mirrors the
Go domains in folder names only; it does not copy Go's layers. A
`ui/src/domains/<area>` folder exists only where Go has `domain/<area>`;
a page about something that isn't a domain goes where its Go code goes.

| Today | Target |
|---|---|
| `components/atoms`, `molecules` | stay: shared, no domain |
| `components/organisms` used by one domain (`TaskBoard`, `HandoffTimeline`, `TeamPanel`, `AgentForm`, `AddWorkerDialog`, `SharedFiles`, `PolicyHistory`, `RedactText`, ...) | `domains/<area>/` |
| `components/organisms` shared across domains (`Toasts`, `CommandPalette`, `Sidebar`, `TopBar`, `StatusBar`, `ShortcutOverlay`, `AccountMenu`) and `components/templates` | `shell/` |
| `pages/TasksPage`, `TaskPage`, `PlanPage` | `domains/tasks/` |
| `pages/AgentsPage`, `AgentPage`, `AgentEditPage` | `domains/agents/` |
| `pages/WorkersPage` | `domains/workers/` |
| `pages/PolicyPage` | `domains/policy/` |
| `pages/SettingsPage` | `domains/settings/` |
| `pages/IntegrationsPage` | `domains/connections/` |
| `pages/WorkspacePage` with `WorkspaceDock`, `SourceControl` | `domains/tasks/workspace/`: a task's Workspace, served from `/api/tasks/{task}/files`, which moves to `tasksapi` |
| `pages/SignInPage` with `SignInDeviceDialog`, `lib/signin.ts`, `lib/trust.ts` | `shell/signin/`: how you get into the shell, as sign-in is part of `web` in Go |
| `pages/ErrorPage`, `NotFoundPage` | `shell/` |
| `lib/*.ts` about one domain (`teams.ts`, `templates.ts`, `epics.ts`, `plan.ts`, `push.ts`, `shared.ts`, `profiles.ts`, ...) | that domain's folder |
| `lib/*.ts` with no domain (`cn`, `time`, `rows`, `stored`, `drag`, `theme`, `monaco`, ...) | stay |
| `api/`, `live/` | stay |
| `features/` | `extensions/`: the registry and each extension, renamed in place in step 1a |

The UI's spec areas go in `domains/`, and its extensions in `extensions/`,
as Go has `internal/domain` and `internal/extensions`. `ui/src/extensions`
means what `internal/extensions` means: something a team or a setting turns
on. One rule is checked, by ESLint's `no-restricted-imports`: `components/`
imports neither `domains/` nor `extensions/`.

## 5. The repo root

| What | Today | Target |
|---|---|---|
| `internal/web/dist` | built by `make ui`, committed so `go build`, CI and the Dockerfile need no Node | `internal/web/static/dist`, git-ignored, beside a committed `internal/web/static/notbuilt.html` (below) |
| history | 94 MB packed; `dist` is 81% of blob bytes | kept as it is, by the user's decision: no rewrite, no force-push. It stops growing from step 10 on. |
| `bin/`, `release/`, `/agentry` | build output, already ignored | unchanged |
| `Agentry/` | an unpacked release, untracked, 138 MB | moved out of the main checkout, by the user's decision |
| `brag-output/` | the brag skill's videos, untracked, 15 MB | added to `.gitignore`, since the skill writes there by design (the user's decision) |
| `packaging/` | a root folder | `tools/packaging` |
| `docker-stack.dev.yml` | at the root | `deploy/`, beside `entrypoint.sh` (the `Dockerfile` stays at the root, where Docker looks) |
| `ui/go.mod` | none, so `go ... ./...` reached the Go package that npm's `flatted` ships in `ui/node_modules` | an empty module that fences `ui/` off: Go doesn't descend into a nested module |

### The built page, no longer committed (step 10)

A placeholder inside `dist` wouldn't survive: Vite builds with
`emptyOutDir: true`, so `make ui` would delete it and leave the tree dirty.
And `//go:embed all:dist` doesn't compile when `dist` is missing. So:

- Vite writes to `internal/web/static/dist` (its `outDir` changes), which
  `.gitignore` lists. Vite empties only that folder.
- `internal/web/static/notbuilt.html` is committed. It says the page isn't
  built, and to run `make ui`.
- `page.go` embeds `all:static`, which always matches, because
  `notbuilt.html` is there with or without `dist`. It serves `static/dist`
  when `static/dist/index.html` exists, and `notbuilt.html` for every page
  path when it doesn't. The API is not affected. This replaces `page.go`'s
  answer today when `index.html` is missing, a 500 with "the UI isn't
  built: run `make ui`": the same advice, as a page with status 200.
- CI's check job and the Dockerfile build the UI in a Node stage first, so
  every image and every CI run serves the real page.

This is the one step that changes behaviour: a binary built from a clone
without `make ui` serves the "not built" page, where today it serves the
committed UI. So the step adds a scenario to
`spec/non-functional/operations.feature` (a server built without the UI says
how to build it), with a test in `internal/web` that serves from an embedded
tree with no `dist` and checks both the page and that `/api/` still answers.

**What relies on the committed page today.** Step 10 changes each of these:

| Where | What it relies on | After step 10 |
|---|---|---|
| `make desktop` (Makefile, its comment, README lines 376 and 398) | it has no `ui` prerequisite, by design: the Mac app is built on a Mac with only Go, from the committed page (the user's choice in task-249) | see "The Mac build" below |
| `make package` | runs `make ui` first | unchanged |
| `internal/web/api_test.go:285-300` | a real `index.html` that names bundles, so `go test ./...` fails on a clone without `make ui` | `make test` depends on the file `internal/web/static/dist/index.html`, whose rule runs `make ui`. So the page is built once per worktree, when it is missing, not on every test run (`make test` already needs Node, for `npm run lint`). A page left from an older commit isn't rebuilt by `make test`; the test checks only that the page names bundles it can serve, so that is fine, and `make ui` refreshes it. The test skips, saying "the UI isn't built: run `make ui`", when `static/dist/index.html` is missing, so a bare `go test ./...` on a fresh clone passes. CI builds the UI first, so CI never skips it. |
| `packaging/main.go:97` (`checkRoot`) | checks `internal/web/dist/index.html` | checks `internal/web/static/dist/index.html`, and its `built-from` (below) |
| `.gitlab-ci.yml:3` and `:29` | the header says the pipeline needs no Node; the check job fails when `dist` isn't committed | the check job builds the UI in a Node stage; both lines go |
| Makefile lines 2-3 | "the built UI is committed, so `go build` and `go install` work without it" | says `go build` without `make ui` serves the "not built" page |
| README lines 108 and 380 | "built into the binary, so this needs no Node"; "`internal/web/dist`, which is committed" | 108 still holds for a release binary; 380 names `static/dist` and says it is built, not committed |
| `page.go` | answers 500 when `index.html` is missing | serves `notbuilt.html` (above) |

**The Mac build: copy the page in.** Step 10 would end Node-free Mac builds
unless it keeps them on purpose, and the user chose to keep them (section
10). The release steps already copy the Mac zip back to the publishing
machine; step 10 adds a copy the other way first, of
`internal/web/static/dist`, built on the publishing machine from the tagged
commit. So the Mac still needs only Go, and `make desktop` keeps no `ui`
prerequisite.

A stale copy can't ship:

- `make ui` writes the commit it built from (`git rev-parse HEAD`) into
  `static/dist/built-from`.
- `checkRoot`, which `make desktop` runs (`desktop.go:63`), reads that file
  and compares it with `git rev-parse HEAD`. When they differ, or the file is
  missing, it refuses with: "the page was built from <commit>, not from HEAD
  (<commit>): run `make ui`, or copy in a page built from this commit".
- So a developer's `make desktop` after a new commit is refused with that
  hint, not silently; with Node, `make ui` fixes it.

README line 398's Mac step gains the copy, and loses "it uses the committed
page". A fresh clone needs Node 24 to build a working page, which the user
accepted.

## 6. How the rules are enforced

**An import test,** `tools/importrules/rules_test.go`, run by
`go test ./...`. It replaces the framework's `imports_test.go`
(`internal/features/imports_test.go` today, `internal/extensions/` after
step 1a) and `internal/httpapi/imports_test.go`, so there is one check, not
three. It
keeps the point of their limits, that the extension framework and `httpapi`
never import
`store`, `current` or `rules`, though not their exact lists: today each may
import only agents, tasks, workers, connections, settings, client and
events, and the new rows allow every domain root (so also `policy` and
`sharedfiles`), `core/events` and `platform` (and `httpapi` also `extensions`).
Nothing they may reach imports the store, so no cycle can come back.

```go
// Each of Agentry's packages, with the layer its path puts it on.
func layerOf(path string) layer // "internal/domain/workers" -> domainRoot

// Allowed is section 3's table, row by row.
func allowed(from, to string) bool

// known are the edges that break the rules today; one may be removed, never added.
// The paths are those of step 2; steps 8 and 9 rewrite them like any other path,
// to internal/extensions/ideas and internal/runtime/coordinator.
var known = []edge{{"internal/coordinator", "internal/ideas"}}

func TestEveryImportPointsDown(t *testing.T) {
	// go list -json ./... ; for each package, its Imports (not TestImports):
	// fail on an edge allowed refuses unless it is in known,
	// and on an edge in known that no longer exists (so the list shrinks).
}

func TestEveryPackageHasALayer(t *testing.T) {
	// a new folder under internal/ outside the eight groups fails, with
	// "put it under platform, core, domain, extensions, runtime or testkit".
}

func TestTestkitIsOnlyForTests(t *testing.T) {
	// no package's Imports (not its TestImports) names internal/testkit.
}
```

Until the moves are done, `layerOf` reads a table of the flat names at that
point (`internal/extensions` after step 1a, `internal/ideas`,
`internal/schedules`), each with its layer from section 4's "New path"
column; step 11 replaces the table with the path
prefixes. The test lands first (step 2), so it is worth having
even if the moves wait.

**A spec/check rule, in two parts.** Step 3 writes the missing `# Sources:`
lines, from the table in section 4, so all 16 feature files have one. In the
same step `spec/check` starts reading them and fails on a path that doesn't
exist, so a move that forgets the spec is caught. Step 11 adds the second
part: it fails when a package under `domain/` or `extensions/` is named by no
feature file's Sources, since a package nobody specified is either dead or
missing its spec. A package with no feature file yet goes on a list in
`spec/README.md`, "Code with no spec yet", which may only shrink, like the
import test's known edges. It starts with one entry, `extensions/schedules`,
and empties when someone writes `schedules.feature`.

**`spec/constraints.md` section 1** stays right through every move:

- Step 2 rewrites line 77. It names the two `imports_test.go` files that
  step deletes, so it names the new import test instead. Its "the list above
  matching `ls internal`" becomes "matching the last element of each path
  `go list ./internal/...` prints, leaving out a `<name>test` package beside
  its `<name>` package". That leaves out `sessionstest` and `taskstest`, which
  `ls internal` never showed. The same step adds `launch` to the list, which
  is missing today, so the check passes on its first run. Package names
  don't change in a move, so the check still holds after the folders change.
- Each move step's script rewrites the paths in `spec/constraints.md`, such
  as `go test ./internal/store`, as it does in the rest of `spec/`.
- Step 11 regroups the list by folder, to match the tree.

## 7. The migration, for later

Every step but step 10 changes no behaviour, builds, passes `make test` and
`spec/check`, and is one task through review.

| Step | What | Kind |
|---|---|---|
| 1 | This document | doc |
| 1a | Rename the framework in place: `internal/features` → `internal/extensions`, `ui/src/features` → `ui/src/extensions`, the Go and UI names, and `@/features` imports; the package name in `spec/constraints.md` section 1, an **extension** entry in `spec/glossary.md`, and `internal/features` and `ui/src/features` paths in `spec/`; `features.toml`, `/api/features`, its JSON and the words users read keep their names | rename, no behaviour change |
| 2 | The import test, with the flat names after step 1a in its table; drop the two old `imports_test.go`; rewrite `spec/constraints.md` line 77 | test only |
| 3 | `# Sources:` lines in the 12 feature files without one; `spec/check` fails on a Sources path that doesn't exist | spec and check |
| 4 | `testkit` and `tools`: `storetest`, `deeptest`, `speccheck`, `packaging` | move |
| 5 | `platform` (15 packages, and the desktop app's four imports) | move |
| 6 | `core` | move |
| 7 | `domain`, one area per commit: agents, tasks, workers, connections, policy, settings, sharedfiles, notifications | move |
| 8 | `extensions/ideas`, `extensions/schedules` (from `internal/ideas`, `internal/schedules`) | move |
| 9 | `runtime` | move |
| 10 | The root: stop committing the built page (history kept), with a "not built" page in its place; CI, Docker and `make test` build the UI; the Mac build copies the page in, with the built-from check (section 5); `brag-output/` ignored; `docker-stack.dev.yml` to `deploy/` | **behaviour change**, agreed by the user; needs a scenario |
| 11 | The import test reads layers from paths; `spec/check` fails on a `domain/` or `extensions/` package no Sources names; `spec/constraints.md` section 1 grouped by folder | test and spec |
| 12 | One task per domain: its routes out of `web` and its tools out of `mcp` into `domain/<area>/<area>api`, mounted as extensions' are | refactor |
| 13 | `coordinator` stops naming `ideas` (`extensions.Coordinator`), and the import test's known list is empty | refactor |
| 14 | The UI: `shell/`, then one `domains/<area>/` per task | move |

Steps 4 to 9 are made by one script, `tools/relayout`, from the table in
section 4. For each package in the step it:

1. `git mv`s the folder;
2. rewrites the old path wherever it appears: import paths and any other
   string in `.go` files (such as the hint in `internal/speccheck/cite.go`
   that names its own path), `spec/` (`@test` citations and `# Sources:`),
   `spec/constraints.md`, `Makefile`, `.gitlab-ci.yml`, `spec/check`,
   `ui/vite.config.ts` and `README.md`; then `gofmt -w` re-sorts the import
   blocks. The package name stays, so no other line changes;
3. matches whole path segments only. Several old paths are the start of
   others: `internal/store` of `internal/storetest`, `internal/web` of
   `internal/webpush`, `internal/workers` of `internal/workerservice`,
   `internal/settings` of `internal/settingsfile`. So the script replaces an
   old path only where the next character is not a letter, digit, `_` or `-`
   (a `/`, a quote, a space, punctuation or the end of the line), never by a
   plain text replace. In a Go import that is the full path followed by `"`
   or `/`;
4. fixes the relative paths that tests and builds use from a moved
   package's folder. Found today:

   | File | Path | Step |
   |---|---|---|
   | `internal/web/policy_api_test.go:168` | `"../policy/shipped/..."` | 7 (policy moves, web stays) |
   | `cmd/agentry/tokens_test.go:14` | `"../../internal/engines/testdata/..."` | 5 |
   | `internal/events/fixtures_test.go:17` | `const module = "../.."` | 6 |
   | `internal/speccheck/check_test.go:358` | `filepath.Join("..", "..")`, and the `"../..."` fixture paths near lines 37, 269 and 368 | 4 |
   | `packaging/desktop_test.go:80` | `filepath.Join("..", path)` | 4 |
   | `packaging/desktop_install_test.go:33` | `t.Chdir("..")` | 4 |

   In each moved package the script also runs
   `grep -nE '"\.\./|"\.\."|Chdir\(' --include=*.go`, which finds a path
   string, a `filepath.Join("..", ...)` and a `Chdir`, and stops on any hit
   that is in neither list, so a person decides it. `make test` would catch
   a miss too, but the plan doesn't rely on that. The second list, the
   script's allow-list, holds the files whose hits are not paths to fix:
   comments about `..` and tests that a path can't escape its folder.

   | File | Why it is not a path to fix |
   |---|---|
   | `internal/policy/decide.go`, `reach.go`, `shell.go` | comments on how `..` is cleaned, and splitting a shell range on `..` |
   | `internal/sharedfiles/folder.go`, `blobs_test.go`, `sharedfiles_test.go` | refusing names that escape the folder, and their tests |
   | `internal/worktrees/repo_test.go` | a test that a file can't be made outside the worktree |
   | `internal/workspace/workspace_test.go` | a test that a path can't leave the workspace |
   | `internal/coordinator/handoff_brief_test.go` | a brief's text that names a commit range |

   The allow-list names files, not lines, so an edit to one of them doesn't
   stop the script; a new file with a hit does.

Step 1a renames the framework's package in place, before the import test
lands, so no move has to change a package name. `gopls rename` turns
`features.Feature` into `extensions.Extension`, `installedFeatures()` in
`cmd/agentry` into `installedExtensions()`, and the other names that say
feature for the framework; `features.Route`, `features.Tool`,
`features.Core` and `features.Coordinator` need only the new qualifier,
`extensions.`. The folders `internal/features` and `ui/src/features` become
`internal/extensions` and `ui/src/extensions`, and `internal/ideas` and
`internal/schedules` stay where they are until step 8.

`tools/relayout` doesn't exist yet at step 1a, so the step names everything
else it rewrites:

- `spec/`: the bare package name `features` becomes `extensions` in
  `spec/constraints.md` section 1 (line 7, "the leaves, `events`, `features`
  and `httpapi`"; line 31, the package's own entry; line 64, `httpapi`
  "imports only events and features"; and `go test ./internal/features` on
  line 77). Line 77 has the reviewer compare section 1 with the packages,
  so a miss there is caught by no test. In `spec/glossary.md`, **feature**
  (line 56) stays, as the user's word, though its `internal/features`
  citation is rewritten like any other path; the step adds an **extension** entry,
  or a line in the feature entry, naming extension as the code's name for
  it, `internal/extensions` and `ui/src/extensions`. Every `internal/features`
  and `ui/src/features` path in `spec/` (such as `spec/constraints.md` line
  97, the 15 `ui/src/features` paths in `spec/functional/ideas.feature`, and
  `spec/proposals/extensions-github-and-email.feature`) is rewritten by
  whole path segments, as item 3 of the script above says. `spec/check`
  doesn't look up proposals' citations, so grep `spec/proposals/` for both
  paths rather than rely on it.
- The UI: `@/features` imports become `@/extensions` (in `App.tsx`,
  `Sidebar.tsx`, `AdminShell.tsx`, `SettingsPage.tsx` and its test,
  `TaskPage.tsx` and `TasksPage.tsx` today). A rewrite of `ui/src/features`
  doesn't match the alias, but `tsc` catches a miss.
- Other names: those that mean the framework or its plug-ins are renamed,
  for example Go's `features.Reader`, `features.StreamsOf`,
  `storetest.WithFeatures()` and the coordinator's `Loop.Features` field,
  files such as `internal/mcp/features.go` and
  `internal/web/features_api_test.go`, and the UI's `FEATURES`,
  `useFeatures`, `visibleFeatures`, `hiddenFeatures` and `FeatureUI`. Names
  that mirror the JSON of `GET /api/features` or `features.toml`, such as
  `FeatureLine` and `FeaturesReply` in `ui/src/api/types.ts`, keep theirs.
  A template's `features` is the JSON of the templates API and of saved
  templates (`internal/agents/teams.go` line 21,
  `internal/web/agents_api.go` line 458, `ui/src/api/types.ts` line 375):
  the Go field `Template.Features` may be renamed, but its JSON tag
  `features` stays.

It leaves `features.toml`, `/api/features`, its JSON and the words users
read in the UI as they are (section 4), so the step changes no behaviour,
and section 6's "Package names don't change in a move" stays true for every
move after it.

Old design docs under `docs/design/` keep the paths they were written with;
they record what was true then.

A move is one commit holding the `git mv` and its rewrites. A commit with
the `git mv` alone wouldn't build, and every commit should build so `git
bisect` works. Git still sees each file as renamed, because only its import
lines change.

### Not getting in the way of open tasks

There are 12 task worktrees open as this is written. A move conflicts with a
task that edits a moved package or adds an import of one.

- Move one group per step, landed between tasks, as the user decided, so
  each step touches the fewest open tasks.
- Before a step, list what each open task's branch touches
  (`git diff --stat dev...agentry/task-N`), and run the step when no open
  task touches its packages, or right after those tasks land.
- Git carries a branch's edits to a renamed file on rebase, since the file
  is more than 50% the same. What it can't fix is a new file added in an old
  folder, or a new import of an old path. So the script is safe to run
  again: on a rebased branch, `go run ./tools/relayout` moves stray files
  and rewrites old imports, and changes nothing else.
- Don't mix a move with any other change in the same task, so a reviewer
  checks it by running the script again and diffing.
- Spec IDs don't change in a move; only citations' paths do, so the spec
  ID collisions seen between parallel branches don't get worse.

## 8. Alternatives considered

| Alternative | Why not |
|---|---|
| The proposal as written: each domain one package with its model, routes and tools, and a `core` that knows no domain | `current`, `rules` and `store` import the models, and routes need `current` and `store`, so one package per domain would be an import cycle for tasks, agents, workers, connections and settings. Kept its intent: one folder per domain, with the model as its root. |
| Make `core` truly domain-free: every domain registers its state slice through `extensions`, as ideas does | The right direction for some domains later (`docs/design/extensions.md`), but it rewrites how the store, the rules and the read model hold tasks and agents. That is a behaviour risk, not a move. This layout doesn't block it. |
| By layer: `models/`, `services/`, `handlers/` | That is today's problem with folder names on it: one change to workers still touches four folders. |
| Leave `internal/` flat and only add the import test | Cheapest, and the test is step 2 of the plan anyway. But 46 names in one list don't tell a reader where to look, and the rules would be a table nobody sees in the tree. |
| Handlers stay in `web`, one file per domain | Already true, and `web` still imports 23 packages and decides how each domain is served. It's where the repo is until step 12, not the target. |
| `core/features` beside `features/ideas` | Two things named features. The framework as the parent folder of its extensions says what it is, and matches the UI. |
| Keep the name `features` for the framework | "Feature" already means a spec `.feature` file, and any capability in everyday words. The user chose extensions (decision 5). |
| One Go module per domain | Agentry is one binary. More modules mean `replace` lines, like the desktop app's, and versioning between them for no gain. |
| Deeper nesting, such as `domain/workers/link/server` | Package names that only make sense with their path (`link`, `server`, `api`) clash when imported together. `workerlink` and `workersapi` read alone. |
| `machines` in `platform` | It imports `agents`, `workers`, `sessions` and `worktrees`: it is what runs agents on a machine, so `runtime`. |
| `desk` in `platform` | It imports only `sessions`, like `terminal`. But `platform` means "no Agentry concept", which is about what a package does, not only what it imports: the desk is an Agentry role, with its own token and its own way of making tasks. Imports set the lowest folder a package can go in; its job picks among the ones left. |
| `sessions` as a domain | It knows no Agentry concept, only engines and processes: `platform`. Its spec area points there. |
| A `signin` domain | Sign-in is how `web` checks who calls it, with no events of its own. It stays in `web`. |
| `workerservice` in `platform` | It imports only `procs`, so it could. It is only about running a worker, so it sits with workers, where a worker change looks. |
| Move the built page to `ui/dist` behind a Go package in `ui/` | `go list ./...` would walk `ui/node_modules`, and an npm package that ships `.go` files would break `go vet ./...`. `internal/web/static/dist`, ignored, avoids that. |
| Rewrite history to drop `internal/web/dist` (`git filter-repo --path internal/web/dist --invert-paths`) | It would take the blobs from about 65 MB to about 12 MB, but needs a force-push of every branch and every clone and worktree cloned again, with no task open. The user chose to keep the history. |
| Commit `dist` only on release tags, so a tag builds anywhere without Node | The tag's commit is on no branch, or `dev` carries `dist` again; history still grows by the page once per release; and there are two kinds of commit to explain. Copying the page to the Mac, checked against the commit, keeps the Mac Node-free without them. |
| Install Node on the Mac, with `ui` a prerequisite of `desktop` | Simplest, but undoes the user's choice in task-249 that the Mac app builds with only Go. |
| Delete `Agentry/` and `brag-output/`, or ignore both | `Agentry/` is a whole release that has no place in the checkout, and ignoring it would hide 138 MB there; `brag-output/` is where a skill writes by design. The user chose to move the first and ignore the second. |
| All the moves in one quiet hour, with no task open | One review instead of six, but it needs the board empty, and any task started after it rebases over every move at once. The user chose one group per step, between tasks. |
| `git mv` alone in one commit, the rewrites in the next | The first commit wouldn't build. One commit per move, made by a script, is as easy to review. |

## 9. Disagreements and how they were settled

The proposal is the user's; the first table lists where the design departs
from it and why. The reviewer agreed with every row (review 1). The second
table holds the reviewer's challenges and how each was settled.

| # | The proposal | This design | Why |
|---|---|---|---|
| 1 | `domain/` packages each own model, routes and tools | a domain is a folder: model at its root, routes and tools in subpackages | the cycle in section 8, row 1 |
| 2 | `core/` holds `events, store, current, rules, features, httpapi, schema`, and knows no domain | `core` holds `events, store, current, rules, httpapi`; it sits above the models, and only `events` sits below them | `current`, `rules` and `store` import the models; the extension framework imports them too |
| 3 | `features` (the framework) in `core/`, and `features/ideas, schedules` | `internal/extensions` is the framework, with each extension in a subfolder | one name, one meaning; the UI matches; named extensions by decision 5 |
| 4 | `schema` in `core` | `platform/schema` | it imports nothing of Agentry's |
| 5 | `machines` in `platform` | `runtime/machines` | it imports domain models |
| 6 | `sessions` in `domain`, `terminal` in `runtime` | both in `platform` | neither knows a domain |
| 7 | `signin` a domain, and `web` doing auth | sign-in stays in `web` | the proposal names it twice; it is web's auth |
| 8 | `policy (+gate/hooks)` | `domain/policy` with `hooks/` under it | agreed; `hookcall`, the hook contract, goes to `platform` because `machines` and `turns` import it |
| 9 | `workers (+workerlink, workerservice)` | agreed, as subpackages | |
| 10 | `notifications (webpush)` | `domain/notifications/webpush` | agreed; a folder with no root package, until it has a model |
| 11 | `settings` | `domain/settings` with `settingsfile/` under it | `settingsfile` was not placed in the proposal |
| 12 | `testkit: storetest, deeptest` | agreed, and test-only by rule | `sessionstest` and `taskstest` stay beside their packages, as Go does |
| 13 | not placed: `waitlist`, `client`, `pinned`, `launch`, `workspace`, `hookcall`, `speccheck`, `packaging`, `mcp` | see section 4 | |
| 14 | `ui/src/features/<domain>` | `ui/src/domains/<area>`, and `ui/src/extensions` (today's `ui/src/features`) for extensions | `features` already means something in the UI |
| 15 | `cmd/` thin: flags and wiring only | `cmd/agentry` keeps each subcommand's steps | its tests call those steps directly; moving them is a refactor per command, when one grows |

The reviewer's challenges:

| # | The reviewer | The engineer | Settled |
|---|---|---|---|
| R1 | Putting `httpapi` in one row with `store`, `current` and `rules` lets it import the store, which `internal/httpapi/imports_test.go` forbids today, so the new test would be weaker than the one it replaces. | Agreed: a slip of grouping, not a choice. | `core/httpapi` has its own row in section 3, with today's limit, and the import test keeps it. The diagram shows it as its own row. |
| R2 | `ui/src/domains/workspace` and `domains/signin` look like domains, but Go has neither, and section 8 rejects a signin domain. | Agreed: the UI should follow where the Go code is, not add domains. | A UI domain folder exists only where Go has one. The Workspace page goes to `domains/tasks/workspace` (its routes are a task's and move to `tasksapi`); sign-in goes to `shell/signin` (part of `web` in Go). |
| R3 | `desk` imports only `sessions`, so by the reasoning that puts `sessions` and `terminal` in `platform` it would be `platform` too. Agrees with `runtime`, but wants the reason stated. | `runtime`: it is an Agentry role, not an OS capability. | Kept in `runtime`, with the rule stated: imports set the lowest folder a package can go in, and its job picks among those left (In short, item 4; section 8). |
| R4 | Step 10 said `make desktop` already runs `make ui`. It doesn't, by design: the Mac app is built with only Go from the committed page. Dropping the committed page quietly ends that, and breaks `api_test.go`, `checkRoot` and CI's check. | Not committing `dist` is still right for history, but the Mac build has to be kept on purpose, not by accident. | Section 5 lists everything that relies on the committed page and what each becomes. The user chose to keep the Mac build Node-free by copying the page built on the publishing machine, checked against HEAD by `checkRoot` (section 10, decision 2). |

## 10. Decisions by the user (2026-10-30)

1. **The committed built page:** stop committing `internal/web/dist` from
   step 10 on. The history stays as it is: no rewrite, no force-push.
2. **The Mac build:** the page is built on the publishing machine from the
   tagged commit and copied to the Mac. `checkRoot` compares the page's
   `built-from` with HEAD, so a stale page can't ship, and `make desktop`
   stays Node-free on the Mac (section 5). Committing `dist` on release tags
   and installing Node on the Mac are under alternatives (section 8).
3. **`Agentry/` and `brag-output/`:** move `Agentry/` out of the main
   checkout, and git-ignore `brag-output/`.
4. **When to move:** one group per step, each landed between tasks
   (section 7, "Not getting in the way of open tasks").
5. **The framework's name:** the plug-in framework is named extensions, not
   features: `internal/extensions`, `ui/src/extensions` and
   `extensions.Extension`. "Feature" collides with the spec's `.feature`
   files and with the everyday word for any capability, and extensions
   matches `docs/design/extensions.md`. The settings file `features.toml`,
   the `GET /api/features` endpoint and its JSON are deliberately not
   renamed: users and existing installs see them, and renaming them would be
   a behaviour change. The words users read in the UI, such as "Features
   that are off" on the Settings page, stay "feature" too, and so does the
   glossary's **feature**: only names in the code change.
