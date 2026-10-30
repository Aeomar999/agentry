# What is left in web and mcp, and where it goes

Status: proposed, for task-297 (a story of epic-251). Design only: no code,
route or spec changes with this note. It follows
[repo-organization.md](repo-organization.md), section 2 ("Where routes and
tools go") and section 7, step 12.

## In short

Step 12 moved every domain's page routes and tools out of `internal/web`
and `internal/mcp` (tasks 267 to 274). The target is that `web` and `mcp`
only mount routes and tools. Measured on `49144b69`:

- `internal/mcp` is close to the target already. Its 11 files are the
  protocol, the notices, the waiting list `mcp` words, and the list of tools.
  Two small helpers are left that belong elsewhere.
- `internal/web` has 42 files and `static/`. Most of them serve the page,
  sign-in and the few routes that span every domain, as section 2 says.
  What doesn't fit is the routes a machine calls without the page's token:
  the install scripts, joining and connecting with a one-time code, and
  every route a worker calls with its credential. Those routes are
  domain work (workers, connections, shared files), but `extensions.Auth`
  has only `UI`, `Desk` and `Signed`, so no domain's api package can take
  them.
- Some files hold two or three jobs (`api.go`, `workers_api.go`,
  `signin.go`, `extensions_api.go`). Splitting them changes no package and
  no rule.

So the plan has two parts. Stories 1 to 3 fit today's rules and can start
now. Stories 4 to 7 need the user's answer to question 1: whether
`extensions.Auth` gains a `Worker` and an `Open` kind.

## How each home was checked

A home is one of: stays in `web` or `mcp`, an existing domain api package,
or `internal/platform/client`. Every home is an existing package, so no
story adds a package. The layers below are section 3's, as
`tools/importrules` reads them off the path (`layerOf`, `allowed`):

| Home | Layer (section 3) | Checked with `allowed` |
|---|---|---|
| `internal/web`, `internal/mcp` | 8 | `mcp` → `platform/client`: yes; `mcp` → `domain/tasks/tasksapi`: yes |
| `internal/domain/workers/workersapi` | 7, folder `domain/workers` | → `domain/workers/workerlink`: yes (same folder); → `core/store`: yes; → `platform/worktrees`: yes |
| `internal/domain/connections/connectionsapi` | 7, folder `domain/connections` | → `domain/workers` (the root, for `workers.Hash`): yes; → `workersapi`: **no** |
| `internal/domain/sharedfiles/sharedfilesapi` | 7, folder `domain/sharedfiles` | → `domain/sharedfiles`: yes |
| `internal/domain/tasks/tasksapi` | 7, folder `domain/tasks` | → `domain/workers/workerlink`: **no** |
| `internal/platform/client` | 1 | imports nothing of Agentry's for `Raw` |
| `internal/extensions` (the framework) | 4 | → `domain/workers` (the root, for `extensions.WorkerOf`): yes |
| `internal/web/install` (a web subfolder) | **none**: `layerOf` gives it no layer, so `TestEveryPackageHasALayer` fails, and `web` → `web/install` is refused | – |

Two results change what earlier stories found:

- `worker_git.go` serves a task's repo, but its home is `workersapi`, not
  `tasksapi`. Its routes are built from `workerlink.GitPath`, and `tasksapi`
  may not import `workerlink`, another domain's subpackage.
- A web subfolder such as `internal/web/install` has no layer today. It
  would need a new `layerOf` case, a row in section 3 and a line in
  `internal/README.md` (question 3).

## Every file

Every path `git ls-tree --name-only origin/dev internal/web/ internal/mcp/`
lists on `49144b69`: 11 in `mcp`, 42 files and `static/` in `web`, 54 rows.
"Story" is the story below that moves or splits the file; "–" means the
file stays as it is. Rows marked Q1 move only if the answer to question 1
is yes; with a no, those files stay in `web`.

### internal/mcp

| File | Its one job | Home | Story |
|---|---|---|---|
| `archived_test.go` | `show_task` reads an archived or erased task from a real web server | stays: a test over the protocol | – |
| `conn.go` | the stdio JSON-RPC transport, both ways | stays: the protocol | – |
| `extensions.go` | which extensions' tools are shown, read again as the log moves | stays: mounting the extensions' tools | – |
| `extensions_test.go` | a shown extension brings its tools; replies end with what waits | stays | – |
| `main_test.go` | fails a run that leaves a session running | stays | – |
| `notify.go` | desktop notices of what newly waits for you | stays: notices | – |
| `server.go` | initialize, the tool calls, asking you (elicitation), the instructions | stays: the protocol | – |
| `server_test.go` | the tasks tools over the protocol, against a fake server | stays: `tasksapi/tools_test.go` has the tools' own tests | – |
| `tools.go` | the list of every tool (tasks, shared files, extensions), and `raw`, a call whose body isn't JSON | the list stays (mounting); `raw` goes to `platform/client` as `Client.Raw` | 2 |
| `waiting.go` | what waits and the news, for replies, notices, the status line and `agentry wait` | stays: the waiting list `mcp` words; its mirrors of `tasksapi`'s `task`, `shownIDs` and `idsAs` go | 3 |
| `waiting_test.go` | the news is only what is new | stays | – |

### internal/web

| File | Its one job | Home | Story |
|---|---|---|---|
| `addresses.go` | the addresses other machines reach the server at, the install commands for each, and the trust page for the server's own CA | stays: what `web` hands out about itself; gains `publicURL` | 1 |
| `addresses_test.go` | the commands come for every address, behind a proxy too | stays | – |
| `api.go` | `/api/config`, `/api/theme` and `/api/changes`; each domain's `API` as `web` builds it | stays, split: `config_api.go` (the three routes) and `domains.go` (the builders and the reads they take: `task`, `roster`, `workers`, `version`) | 1 |
| `api_test.go` | web's test server and helpers; the token checks, the position header, the page, the desk token, the config | stays | – |
| `archived_bench_test.go` | Target 7: an archived task opens in time | stays: it clears web's archives cache, and shares helpers with the other benchmarks | – |
| `archives_test.go` | an archived task reads as before, with a counting blob store swapped into web's cache | stays: it sets `Server.archives`, which only package `web` reaches | – |
| `claude-code.ps1` | the PowerShell script that connects Claude Code on a machine | Q1: `connectionsapi`, embedded there | 7 |
| `claude-code.sh` | the same, for sh | Q1: `connectionsapi`, embedded there | 7 |
| `claude_api.go` | the message Claude in a task's Workspace sends, and `requireClaude`, its token | stays: a token check and one mount line; the handler is `tasksapi`'s `Say` (question 2) | – |
| `claude_api_test.go` | Claude's token sends only that message | stays | – |
| `erase_test.go` | erasing a task drops what web's archives cache keeps of it | stays: `web` holds the one cache that `tasksapi` and `settingsapi` share | – |
| `events_bench_test.go` | measures a task's events route on a copy of a real log | stays, with the other benchmarks | – |
| `extensions_api.go` | `GET /api/features`; mounting every route behind the wrapper its `Auth` names | stays, split: `mount`, `wrapperOf`, `mountRoute` and `coreOf` go to `mount.go` | 1 |
| `extensions_api_test.go` | an extension's routes: their `Auth`, their proposer, a clash, a signed route | stays | – |
| `held_approval_test.go` | a held approval end to end (the hook, the questions, approving); the hook token doesn't open the API | stays: only `web` mounts the hook handler and `tasksapi`'s routes together | – |
| `install.ps1` | the PowerShell script that joins a machine as a worker | Q1: `workersapi`, embedded there | 6 |
| `install.sh` | the same, for sh | Q1: `workersapi`, embedded there | 6 |
| `install_https_test.go` | the install commands pin the server's own CA and run in sh and PowerShell, for workers and Claude Code alike | stays: it tests `addresses.go` and `scripts.go` | – |
| `integrations_api.go` | the connect flow with a one-time code (scripts, binary, connect), and `carriesConnection` | Q1: the connect flow to `connectionsapi`; `carriesConnection` and `connections` to `server.go`, beside `requireUI`, which calls them | 7 |
| `loggrowth_bench_test.go` | measures replay, snapshots and archive sizes on a copy of a real log | stays: it uses the read-model benchmark's helpers (see "Seen on the way") | – |
| `main_test.go` | keeps the tests off your Claude Code config; fails a run that leaves a session running | stays | – |
| `mirrors_test.go` | the task detail and events types web's tests decode | stays | – |
| `page.go` | the embedded page, `notbuilt.html` when it isn't built, and `?token=` | stays: the page | – |
| `page_test.go` | the not-built page | stays | – |
| `policy_test.go` | a policy version's read carries the position header | stays: it tests web's `atPosition` | – |
| `race_off_test.go` | `raceSlowdown` without the race detector | stays, with `archived_bench_test.go` | – |
| `race_on_test.go` | `raceSlowdown` with the race detector | stays, with `archived_bench_test.go` | – |
| `readmodel_bench_test.go` | measures the read model and the page's reads on a copy of a real log | stays, with the other benchmarks | – |
| `redact_api_test.go` | redacting an archived task leaves no trace on disk | stays: it tests how `web` wires `settingsapi` to `tasksapi` | – |
| `scripts.go` | fills an install script with the server, the code and the CA; the commands that run it, pinned or not | stays; gains `shellQuoted`. With Q1, `web` hands it to `workersapi` and `connectionsapi` as a func field, as it hands `InstallCommands` today | 1 |
| `server.go` | `Config`, `Server`, `Handler` (every mount), the token wrappers, the position header | stays: mounting | – |
| `shared_api.go` | the shared files a worker fetches and sends, by hash | Q1: `sharedfilesapi`, with `Auth` `Worker` | 5 |
| `signin.go` | one-time sign-in codes and the limit on failed tries; issuing a code, signing in and out | stays, split: the codes and the limit go to `signin_codes.go` | 1 |
| `signin_test.go` | codes sign in once, expire and are limited; the token pasted in; signing out | stays | – |
| `static/` | `notbuilt.html` in git; `dist/` when built (ignored) | stays: the page | – |
| `trust_test.go` | `/api/config` tells the page where to trust the server's own CA | stays | – |
| `unarchive_test.go` | reopening an archived task unarchives it first | stays: it sets `Server.archives`, as `archives_test.go` does | – |
| `waiting_api.go` | `GET /api/waiting`, and `Waiting` for the pushes | stays: spans every domain | – |
| `waiting_api_test.go` | the core's entries first, then each extension's | stays | – |
| `worker_git.go` | a task's repo as workers reach it: git fetch, and the bundle sent back | Q1: `workersapi` (not `tasksapi`: it needs `workerlink.GitPath`) | 4 |
| `workers_api.go` | `workersAPI`; joining; `requireWorker`; a worker's connect, stream and leave; the install scripts; the binary download and update; small helpers | split. Story 1: `workersAPI` and `workers` to `domains.go`, `publicURL` to `addresses.go`, `serveBinary` and `binaryFor` to a new `binary.go`, `shellQuoted` to `scripts.go`. Q1: `requireWorker` to `mount.go` as the `Worker` wrapper (story 4); the worker's routes and the update to `workersapi` (story 4); joining and installing to `workersapi` (story 6). The file goes. | 1, 4, 6 |
| `workers_https_test.go` | a worker pinned to the server's own CA joins and works, end to end | stays: it needs web's whole mux with the fleet and the hub, which only `web` puts together | – |
| `workers_test.go` | a machine joins with a code and runs a turn, end to end | stays, for the same reason | – |

## The tree after the stories

With question 1 answered yes, and stories 1 to 7 done:

```
internal/web/
  server.go          Config, Server, Handler: every mount; the token wrappers; the position header
  mount.go           mount, wrapperOf (UI, Desk, Signed, Worker, Open), mountRoute, coreOf
  domains.go         each domain's API as web builds it, and the reads it hands them
  extensions_api.go  GET /api/features
  config_api.go      GET /api/config, POST /api/theme, GET /api/changes
  waiting_api.go     GET /api/waiting
  claude_api.go      the Workspace Claude's message and its token
  signin.go          issuing a code, signing in and out
  signin_codes.go    one-time codes and the limit on failed tries
  page.go            the page
  static/            notbuilt.html; dist/ when built
  addresses.go       the server's addresses, install commands and trust page
  scripts.go         filling an install script; the commands that run it
  binary.go          agentry for a machine's OS and architecture
  *_test.go          as in the table: web's own, the wiring, end to end, benchmarks
internal/mcp/
  conn.go            the stdio transport
  server.go          the protocol, asking you, the instructions
  tools.go           the list of every tool
  extensions.go      which extensions are shown
  waiting.go         what waits, and the news
  notify.go          desktop notices
  *_test.go          as today
```

With a no, `web` also keeps `integrations_api.go`, `worker_git.go`,
`shared_api.go`, the four scripts, and a `workers_api.go` that holds
joining, `requireWorker` and the worker's routes.

## The stories, in order

Each story changes no behaviour, passes `make test` and `spec/check`, and
moves the spec's citations and `# Sources:` lines with the code.

**Part one: fits today's rules.** Stories 1, 2 and 3 touch different files
and can run at once.

1. **Split web's files by job.** `api.go` into `config_api.go` and
   `domains.go`; `mount`, `wrapperOf`, `mountRoute` and `coreOf` from
   `extensions_api.go` into `mount.go`; the codes and the limit from
   `signin.go` into `signin_codes.go`; from `workers_api.go`, the helpers
   that stay (`workersAPI`, `workers`, `publicURL`, `serveBinary`,
   `binaryFor`, `shellQuoted`) into the files the table names. A pure
   split, proved as task-291 proved `redact.go`'s. Touches `api.go`,
   `extensions_api.go`, `signin.go`, `workers_api.go`, `addresses.go`,
   `scripts.go`.
2. **`client.Client.Raw`, and mcp's tool list only lists.** `raw` moves
   from `mcp/tools.go` into `platform/client`; `sharedfilesapi.Client`
   takes the `client.Client`, as `tasksapi.Client` does, in place of its
   `Raw` func. Touches `mcp/tools.go`, `platform/client/client.go`,
   `sharedfilesapi/tools.go`.
3. **mcp's news without mirrors.** `tasksapi` exports the board's task
   type and `shownIDs` and `idsAs`; `mcp/waiting.go` uses them and its
   copies go. Touches `mcp/waiting.go`, `tasksapi/tools.go`.

**Part two: if question 1 is yes.** Every story here touches `server.go`'s
mount lines, so they chain: 4, 5, 6, 7, each after the one before.
Story 4 needs story 1 (`mount.go`). Each new kind lands with the first
routes that use it, so no kind is ever unused.

4. **`extensions.Auth` gains `Worker`, and the worker's own routes move to
   `workersapi`.** `Worker` is behind `requireWorker`, which puts the
   worker in the request's context for `extensions.WorkerOf(r)`. `mount`
   refuses it from an extension, with a panic naming the extension, as a
   clash does. Then connect, stream, leave, the binary update
   (`ServeBinary` as a func field) and `worker_git.go` go to `workersapi`.
   Touches `extensions/extensions.go`, `mount.go`, `server.go`,
   `workers_api.go`, `worker_git.go`, `domains.go`, `workersapi`.
5. **The workers' shared-file routes move to `sharedfilesapi`.**
   `blobFor` and `blobFrom`, with `Auth` `Worker`. Touches `server.go`,
   `shared_api.go`, `sharedfilesapi`.
6. **`extensions.Auth` gains `Open`, and joining moves to
   `workersapi`.** An `Open` route has no wrapper: its handler checks its
   own code, as these handlers already do. `mount` refuses it from an
   extension, as it refuses `Worker`. Then `join`, the names it picks,
   `codeWorks`, the install scripts with `install.sh` and `install.ps1`
   embedded there, and the binary download move, all `Open`, with
   `ServeScript` as a func field. `workers_api.go` goes. Touches
   `extensions/extensions.go`, `mount.go`, `server.go`, `workers_api.go`,
   `domains.go`, `workersapi`.
7. **Connecting Claude Code moves to `connectionsapi`.** The two scripts,
   the binary and `connect`, all `Open`. `carriesConnection` and
   `connections` go to `server.go`. `integrations_api.go` goes. Touches
   `server.go`, `integrations_api.go`, `domains.go`, `connectionsapi`.

When the stories are done, section 2's "What stays in `web`" paragraph is
brought in line with the tree above.

## Questions for the user

1. **Should `extensions.Auth` gain a `Worker` and an `Open` kind, so the
   routes a machine calls without the page's token leave `web`?**
   - (a) No: they stay in `web`. `web` then keeps 18 routes of workers',
     connections' and shared files' work, and story 1 only regroups them
     by file.
   - (b) No, but put them in a web subfolder, `internal/web/install` (see
     question 3).
   - (c) Yes, both, for domains only: `mount` refuses them from an
     extension, which keeps `Signed` for a webhook.
   - **Recommended: (c).** It is the only option that reaches the target,
     web and mcp only mounting, and it adds no second way of mounting: the
     routes go through the one `mount` and the domain api pattern. An
     `Open` route checks its own code, as these handlers already do. The
     cost: a domain can now mount a route with no token, so a reviewer
     must check that each `Open` handler refuses a bad code.
2. **Should the Workspace Claude's message get its own kind and move to
   `tasksapi`?** It is one route behind `requireClaude`, whose handler
   already calls `tasksapi`'s `Say`.
   - (a) No: it stays in `web` as a token check and one mount line.
   - (b) Yes: a third kind, `extensions.Workspace`.
   - **Recommended: (a).** A third kind for one route is more to read than
     the route itself. With (b), one more story follows story 5: the
     message moves to `tasksapi` behind `extensions.Workspace`.
3. **Should a web or mcp subfolder be a place for packages?** Today
   `layerOf` gives `internal/web/<x>` and `internal/mcp/<x>` no layer.
   - (a) No: `web` and `mcp` stay one package each, as section 2 says.
   - (b) Yes: a subfolder is a surface in `web`'s folder (layer 7), which
     `web` may import. That needs a `layerOf` case, a row in section 3 and
     a line in `internal/README.md`.
   - **Recommended: (a).** With question 1 at (c), nothing is left that a
     subfolder would hold. With question 1 at (a), files named by their
     job say the same as a folder, with no new rule.

## Seen on the way

Not in this note's scope; for the epic to decide whether to file them.

- `noTask` has three copies, in `tasksapi`, `sharedfilesapi` and
  `settingsapi` (none is in `web` any more). `core/httpapi` could hold
  one, taking the erasure lookup as a func: `httpapi` may import domain
  roots, and every api package already imports `httpapi`.
- `loggrowth_bench_test.go` measures the store and the read model and
  uses nothing of `web`'s. It could move to `core/store` with the helpers
  it borrows from `readmodel_bench_test.go`, but
  `docs/design/log-growth.md` names its path, and a benchmark gains little
  from moving.
