# Extensions: how teams bring features and UI

Status: proposed, for task-46 ("Research how teams contribute features and
UI: the shape of Agentry's extensions").
Research only: nothing in the server or the page changes with this document.
This is part 1: the inventory, what an extension is, how the page learns what
is on, the options and the core API. Part 2 adds the rest (see the end).

## In short

Adding the Visionaries team brought a whole feature with it: ideas. Ideas
have their own event types, rules, state, a hook in the coordinator's done
decision, an API, MCP tools, a waiting entry and a page. Each of those was
written straight into a core package: `current.State` has an `ideas` map,
`rules.State` has an `Idea` method, `coordinator.Decision` has an `Ideas`
field, `web.Server` mounts `/api/ideas`, and the sidebar and Ctrl K list
Ideas whether or not you have the team. Section 2 counts 27 shared files the
feature touched, besides its own 16.

The recommendation is option (a): a feature is a Go package (and a folder in
`ui/src/features`) that registers with a small, explicit core API. Core asks
each registered feature for its event types, its rules, its slice of the
in-memory state, its coordinator hooks, its routes and MCP tools, and a
manifest entry that says whether the page should show it. The page reads
one list, `GET /api/features`, and the sidebar, Ctrl K, routes and board
columns follow it. Ideas is the pilot: it shows while a Visionaries team
exists or any idea does.

Out-of-process extensions (option b) can't add rules: rules run inside
`Store.Propose` under the store's one write lock, so a network call there
would stall every proposal. They stay possible later as clients of the API
that read events and write proposals, which the MCP server already is.

## 1. Where this comes from

The trigger is small: Ideas is hard-coded in
`ui/src/components/organisms/Sidebar.tsx` (`SECTIONS`) and in the Ctrl K
list in `ui/src/components/templates/AdminShell.tsx` (`paletteItems`), so
it shows on an install with no Visionaries team and no ideas. The pattern
behind it is larger. A team can bring a capability with it, and more teams
will. The user's principle for extensions, from the decisions behind this
project, is that "extensions read events, write proposals and add rules.
They are not read-only." PLAN.md names three that fit: T-019 (GitHub
extension: open the PR after `review.approved`; notifications and email),
T-024 (senior approval: route some requests to someone other than the
ticket owner) and T-036 (a JSON Schema per event type, which "matters once
extensions write events").

Two designs constrain any answer here. `docs/design/read-model.md`: the log
is the only thing stored, `current.State` is rebuilt from it at every start
and changed only by the store, one event at a time, after it commits, and
reads of the log never pass the applied position. `docs/design/log-growth.md`:
old logs must keep replaying, as events are never rewritten.

## 2. Inventory: what Visionaries and Ideas touched

The feature landed in tasks 24 to 28 (commits from 191b4e2 to dfb8e40).
The table lists every file it changed or added, outside tests, by concern.
"Own" means the file or function is only about ideas and would move into
the extension whole. "Hook" means a core file that gained an ideas-specific
branch, field or line, which the extension API has to replace with a
generic point. "Core" means a generic change that stays.

| Concern | File(s) | What it does | Extension owns it? |
|---|---|---|---|
| Team template | `internal/agents/teams.go` (`Templates`, the `Visionaries` entry) | Five agents, all `Profile: "read-mostly"`, `visionary-lead` coordinates, `skeptic` has `Column: Reviews` | Own: the template entry |
| Prompts | `internal/agents/defaults/visionaries/*.md` (visionary-lead, product-strategist, user-advocate, technologist, skeptic) | What each viewpoint does; the lead hands ideas over on its done decision | Own |
| Policy profile | `internal/policy/default.toml` (`[profiles.read-mostly]`, "It suggests ideas; the engineers commit.") | Read-only plus reading the task board, no commit or push (also `git -C`) | Own the need, core keeps profiles (task-35) |
| Profile check on add | `internal/web/agents_api.go` (`unknownProfile`, used by `addTeam` and by `agentFrom` for `createAgent` and `changeAgent`) | Refuses a team whose agent needs a profile the policy file in force lacks, and says to copy it | Core (generic, came with this feature) |
| Event types | `internal/events/types.go` (`IdeaSuggested`, `IdeaAccepted`, `IdeaDismissed`, `IdeaPrefix = "idea-"`) | Three types in `idea-N` streams | Own |
| Catalogue | `internal/events/catalogue.go` (`catalogue` entries, `ideaTypes`, `IsIdeaStream`, `IsTaskStream` excludes ideas, `belongsIn`) | Fields and writers per type; idea types only in idea streams | Hook: registration of a stream prefix and its types |
| Task field | `internal/tasks/task.go` (`Task.Idea`), `internal/tasks/apply.go` (`task.created` reads `"idea"`) | A task remembers the idea it was made from | Hook: a feature-neutral "made from" stream |
| Idea state | `internal/ideas/idea.go` (`Idea`, `State`, `FromEvents`, `Next`, `apply`) | An idea's state from its events, same shape as `tasks` | Own |
| Accept | `internal/ideas/accept.go` (`Accept`, `madeFrom`, `ideaOf`, `makeTask`, `kindOf`, `description`, `Board`) | Makes one Backlog task per idea, however often accepted, then marks it accepted | Own |
| Lists | `internal/ideas/list.go` (`Load`, `List`, `Dismissals`) | Reads ideas from the log (`store.IdeaIDs`, `Store.Stream`), not from `current.State` | Own (and should read the state, see "Surprises") |
| Read model | `internal/current/current.go` (`State.ideas`, `Load` and `Apply` branches on `IsIdeaStream`, `State.Idea`) | Keeps each idea in memory | Hook: a state slice per stream prefix |
| Read model check | `internal/current/equal.go` (`IdeaIDs` loop) | The equivalence test covers ideas | Hook: loop over every slice |
| Log reads | `internal/store/read.go` (`IdeaIDs`), `internal/store/store.go` (`Create`, `made`) | `idea-N` named under the store lock by counting `idea.suggested` | Core: `Create(prefix, p)` is generic already; `IdeaIDs` becomes `StartedBy(type)` |
| Rules | `internal/rules/idea_rules.go` (`checkIdea`, `aGoodSuggestion`, `madeFromIt`), `internal/rules/rules.go` (`State.Idea`, the `IsIdeaStream` branch in `Check`) | Suggested once from a real task, accepted or dismissed once while open | Own the rules; hook in `Check` |
| Coordinator hook | `internal/coordinator/ideas.go` (`VisionariesTeam`, `isVisionaries`, `Idea`, `IdeasFormat`, `suggest`, `dismissedFor`, `dismissedText`) | On done, writes each idea with `Store.Create(events.IdeaPrefix, ...)` as the loop | Own |
| Decision | `internal/coordinator/decision.go` (`Decision.Ideas`) | The done decision's `ideas` field | Hook: a field a feature claims |
| Loop | `internal/coordinator/loop.go` (`apply`, `case Finish`; `startOn` passes `l.dismissedFor`) | Only a Visionaries coordinator's ideas are kept | Hook: on-done and brief hooks |
| Brief | `internal/coordinator/brief.go` (`coordinatorBrief(..., dismissed)`, the "Ideas the user dismissed" section, `IdeasFormat` appended to "Your decision") | Tells the lead what was dismissed and how to hand ideas over | Hook: brief sections from features |
| HTTP API | `internal/web/ideas_api.go` (`listIdeas`, `acceptIdea`, `dismissIdea`, `ideaExists`), `internal/web/server.go` (three `api(...)` routes), `internal/web/views.go` (`ideaView`, `viewOf`, `acceptance`) | `GET /api/ideas`, `POST /api/ideas/{idea}/accept`, `.../dismiss` | Own the handlers; hook to mount them |
| MCP tools | `internal/mcp/ideas.go` (`listIdeas`, `acceptIdea`, `dismissIdea`, `openIdeas`, ...), `internal/mcp/tools.go` (three entries in `tools()`, the ideas note in the status line) | `list_ideas`, `accept_idea`, `dismiss_idea` over the HTTP API | Own the tools; hook to list them |
| Waiting and notices | `internal/mcp/waiting.go` (`WaitingFor` adds `openIdeas`, `Kind "ideas"`), `internal/mcp/notify.go` (title "Agentry: a new idea") | Open ideas count as waiting for you | Hook: a waiting source per feature |
| MCP instructions | `internal/mcp/server.go` (the "Ideas:" paragraph), `cmd/agentry/claudecode.go` (`sayWhatWaits` names `list_ideas`) | Tells Claude Code how to relay ideas | Hook: instruction lines per feature |
| UI types | `ui/src/api/types.ts` (`Idea`, `IdeaAcceptance`) | The API's shapes | Own |
| UI page | `ui/src/pages/IdeasPage.tsx`, `ui/src/components/organisms/IdeaCard.tsx`, `ui/src/components/organisms/IdeaDialogs.tsx` | Open, accepted and dismissed ideas; accept and dismiss | Own |
| UI route | `ui/src/App.tsx` (`{ path: "/ideas", element: <IdeasPage /> }`) | The page's address | Hook: routes from features |
| Sidebar | `ui/src/components/organisms/Sidebar.tsx` (`SECTIONS`, "Work") | Ideas, always | Hook: nav items from features, shown by the manifest |
| Ctrl K | `ui/src/components/templates/AdminShell.tsx` (`paletteItems`, `go-ideas`) | Go to Ideas, always | Hook: commands from features |
| Ask the visionaries | `ui/src/lib/visionaries.ts` (`useAskVisionaries`), `ui/src/lib/teams.ts` (`VISIONARIES`, `addTeam`) | Adds the team from its template if missing, then opens New task with it picked | Own `visionaries.ts`; `addTeam` is core |
| New task preset | `ui/src/lib/new-task.ts` (`useNewTask(kind, epic, team)`, `&newTeam=`), `ui/src/components/organisms/NewTaskDialog.tsx`, `ui/src/components/organisms/NewTaskForm.tsx` (`asksVisionaries`, `visionariesMissing`, the "Theme or question" field, kind forced to `story`) | `?new=task&newTeam=Visionaries` opens New task for a session | Core for `newTeam`; hook for the team-specific form |
| Agents page | `ui/src/pages/AgentsPage.tsx` (uses `addTeam`) | Add a team from a template | Core |
| Form a11y | `ui/src/components/molecules/Field.tsx` (`labelId`), `SegmentedControl.tsx` (`labelledBy`) | Labelled groups, found while building this | Core |
| Docs | `README.md` (team template, Ideas section, Ideas page, Ask the visionaries, MCP tools, profiles) | How to use it | Own a section |

Tests the feature added or changed, which move with it or become generic:
`internal/ideas/ideas_test.go`, `next_test.go`, `export_test.go`;
`internal/coordinator/ideas_test.go`; `internal/web/ideas_api_test.go` and
the profile count in `internal/web/api_test.go`
(`TestThePolicyShowsWithItsProblem`); `internal/mcp/ideas_test.go` and
`server_test.go`; `internal/events/catalogue_test.go`;
`internal/store/store_test.go`; `internal/agents/agents_test.go` and
`prompts_policy_test.go`; `internal/policy/default_test.go` and
`file_test.go`.

Counted by kind: 16 files are the feature's own (Go, prompts and UI); 27
shared files (core Go, the policy file, the UI shell and the README) gained
an ideas-specific line, field or branch; 7 were generic changes made on the
way (`agents_api.go`, `store.go`, `new-task.ts`, `NewTaskDialog.tsx`,
`AgentsPage.tsx`, `Field.tsx`, `SegmentedControl.tsx`).

### Other features that are already team-like

| Feature | What it depends on | Where it hangs off core today |
|---|---|---|
| Shipping column | A team having an agent with `Column: agents.Ships` (`"ship"`) | Deep in core: `agents.Agent.Column`; `tasks.TeamHas` and `Task.UndoesYourColumn` (`internal/tasks/column.go`); `tasks.apply` (`TaskAssigned`'s `"ships"`, `Task.Shipped`, `Handoff.Ships`); `coordinator/next.go` and `checks.go`; `web/board_api.go` refuses a move when "`<team>` has no agent that ships work"; `web/views.go` `workColumn`. The page shows the column always: `ui/src/pages/TasksPage.tsx` lists it statically. |
| Review column | A team having an agent with `Column: agents.Reviews` | Same places as Shipping; every template has a reviewer, so it is always useful. |
| Per-team policy profiles | Each template's agents name a `Profile`; `read-mostly` exists only for Visionaries | `internal/policy/default.toml` (no inheritance, so it copies `read-only`); `web.unknownProfile` refuses the team on an older policy file; task-35 tracks profiles per team. |
| Integrations | Claude Code connections | Core: the `connections` stream (`events.ConnectionInvited`, `ConnectionMade`, `ConnectionRevoked`), `connections.Roster` in `current.State`, `agentry mcp` (`internal/mcp`, a separate process over `client.Client`), `ui/src/pages/IntegrationsPage.tsx`. |
| Design, Research, Writing templates | Nothing but data | Only entries in `agents.Templates` and prompts under `internal/agents/defaults/<folder>/`. They need no code: their agents use `read-only` and `developer`, and their critic has `Column: Reviews`. |
| Server capabilities | How the server was started | `GET /api/config` already carries `hasTerminal`, `hasDesk` and `canAddWorkers`; only `canAddWorkers` is read by the page (`WorkersPage`). This is the one precedent for "the page follows what the server says is on". |

So the line between a team and a feature is already visible: Design,
Research and Writing are pure data, Shipping is a capability of an agent's
role that core logic depends on, and Ideas is a feature with its own data.

## 3. What an extension is

An extension is a named unit that can contribute any of the kinds below.
Most contribute a few. A team template is the commonest way to turn one
on, but not the only one (T-019 is turned on by a GitHub token, not a
team).

| Kind | Example | What core must offer |
|---|---|---|
| Agents and prompts | The Visionaries template and its five prompts | A template list it can add to (`agents.Templates`), prompts by folder, and a way to say which feature a template turns on |
| Policy profiles | `read-mostly` | A way to declare a needed profile and its rules, and the existing `unknownProfile` check when the file in force lacks it (part 2, with task-35) |
| Event types and streams | `idea.suggested`, `idea.accepted`, `idea.dismissed` in `idea-N` | Registering types with their fields and writers (`events.spec`, today unexported), and a stream prefix the core treats as not a task (`IsTaskStream`) |
| Rules | `checkIdea`: once, while open, from a real task | Check 3 for its own streams, plus a guard on core types it cares about (T-024 needs one on approvals), run inside `Store.Propose` like `rosterRules` |
| State slice | `current.State.ideas` | A slot in `current.State` for its streams, built by `Load` and changed by `Apply`, under the same lock, guard and broken-stream handling |
| Coordinator hooks | On done, keep ideas; in the brief, name dismissed ideas and the ideas format | Points in `Loop.apply` and `coordinatorBrief`, given the task and the coordinator, returning text or proposals |
| HTTP endpoints | `/api/ideas` and accept, dismiss | Mounting routes behind `requireUI` (or `requireUIOrDesk`), with `reply`, `decode`, `failed` and `propose` |
| MCP tools | `list_ideas`, `accept_idea`, `dismiss_idea` | The tool list (`mcp.tool`, today unexported), a waiting source, and lines in the server instructions |
| UI: pages and nav | `/ideas`, its sidebar item and Ctrl K command | A list of routes, nav items and commands the shell iterates, shown when the manifest says so |
| UI: task page tabs, cards, settings | T-019 would show the PR on the task; a feature might add a settings section | Slots on the task page and settings that iterate the same list |
| UI: board columns | Shipping, when a team ships | The manifest's list of columns; the board already takes its columns from a list (`TasksPage.tsx` `columnsFor`) |
| Notifications | "Agentry: a new idea" (`mcp/notify.go`); T-019's email | A waiting source with a `Kind` and a title |
| Background jobs | T-019: after `review.approved`, open the PR | A subscriber that reads the log after a position and writes proposals (`Store.NewReader`, as the loop does) |

Two things are not extensions. A supervisor is an ordinary agent, as the
project decided early. And a pure template (Design, Research, Writing) is
data: it needs no feature unless it brings data or behaviour of its own.

## 4. How the page learns what is on

### Installed, enabled, visible

Three words, kept apart:

- Installed: the feature's code is in this binary and registered.
- Enabled: its trigger is present, so it can produce new data. For Ideas,
  some team's coordinator is a Visionaries coordinator. For T-019, a GitHub
  token is set.
- Visible: the page shows it. A feature is visible while it is enabled or
  while it has data you can still look at or act on. Ideas stays visible
  while any idea exists, even after you remove the team: your open ideas
  must still be accepted or dismissed, and the history must stay readable.

Turning a feature off never hides data. Its route also stays: a bookmark
to `/ideas` on an install with neither the team nor ideas shows the page
with an empty state that offers "Ask the visionaries" (which adds the team),
not a 404.

### The manifest

The server computes the manifest from the current state on every request,
which is a map lookup per feature (`current.State` holds the roster and
every idea already). One new endpoint, kept apart from `GET /api/config`,
because config is how the server was started and the manifest is state
that changes with events:

```
GET /api/features
{
  "features": [
    {
      "name": "ideas",
      "enabled": true,
      "visible": true,
      "why": "the Visionaries team suggests ideas",
      "badge": 3
    }
  ],
  "board": { "columns": ["backlog", "todo", "in-progress", "review", "shipping", "done"] }
}
```

- `name` matches the page's own registry (section 6); the page decides how a
  feature looks (label, icon, page), the server decides whether it shows.
- `why` is one line for the page to show where the feature is off or kept
  only for its data ("kept while 4 ideas remain; the Visionaries team was
  removed").
- `badge` is optional: open ideas for Ideas, so the sidebar can count them
  as Tasks counts what needs you.
- `board.columns` is core's own entry: `shipping` is listed while any team
  has an agent with `Column: agents.Ships` or any task is in Shipping, so
  the column follows the same rule as Ideas.

A feature the page doesn't know is ignored; a feature the server doesn't
list is hidden. That makes an old page with a new server, and the reverse,
safe.

### Staying fresh

No new mechanism. The page already long-polls `GET /api/changes?after=`
(`ui/src/live/LiveProvider.tsx`) and every `useResource` refetches when the
log position moves (`ui/src/live/use-resource.ts`). Adding a team writes
`agent.created` events, and a new idea writes `idea.suggested`, so the
manifest is refetched exactly when it can change. `AdminShell` reads it
once, through `useResource`, and passes it down by context to the sidebar,
Ctrl K and the board.

## 5. Options compared

- (a) Compiled-in. A feature is a Go package that registers with a small,
  explicit core API: event types, rules, a state slice, coordinator hooks,
  HTTP routes, MCP tools, and a manifest entry. Its UI is a folder in
  `ui/src/features/` built into the one bundle.
- (b) Out of process. A feature is a separate program that talks to the
  server over its API and event stream, and adds UI through a manifest
  that points at pages shown in sandboxed iframes.
- (c) Hybrid. Built-in features use (a); third parties use (b).

| Weighed against | (a) Compiled-in | (b) Out of process | (c) Hybrid |
|---|---|---|---|
| Single Go binary | Kept: one `agentry`, UI by `go:embed` | A second program to install, run, upgrade and supervise per extension | Both, but the second kind only when someone needs it |
| Event log as the truth | Events go through `Store.Propose` like any other | Same, over HTTP: an extension writes proposals; it can't write past the checks | Same |
| Rules (extensions "add rules") | A rule runs in `rules.Check`, inside `Store.Propose`, under `Store.mu`, in microseconds | Can't, in practice: a rule would need a network call while the store's one write lock is held, so one slow extension stalls every proposal and every agent. It could only react after the fact. | Rules only from (a) |
| In-memory read model | A state slice applied by `current.State.Apply` in the one applier, rebuilt at `Load`, checked by `current.Equal` | Keeps its own copy from the event stream: its own replay, its own position, its own bugs, and nothing the core rules can read | Slices only from (a) |
| Replay of old logs | The feature's code replays its streams; without the code they are skipped (part 2) | The core replays nothing of it; the extension replays its streams itself on every start | Both cases |
| Workers on other machines | No change: workers run agents, not features | No change, but the extension must reach the server like a worker does | Same |
| Security: agents | Its agents go through the gate and policy as any agent (`Profile`, the hook) | Same | Same |
| Security: who writes what | The catalogue's `writers` and the feature's rules decide, in process | Needs its own credential and a list of types it may write; today `carriesUIToken` lets a connection's credential do anything the page can, which is too much for third-party code | (a) for writes that need rules; (b) behind scoped credentials |
| Readability, separation | One folder per feature, one registration list; core files lose their ideas branches | Clear process boundary, but every feature also needs a client, a protocol and a deployment story | Two models to learn |
| Testability | `storetest.Open` and the existing equality check cover it; one `go test ./...` | Integration tests across processes | Both |
| UI bundle | Typed React in the one bundle, shares components, theme and `useResource` | Iframes, like a question's page today (`ui/src/lib/sandbox.ts`: own origin, `allow-scripts`, no network): safe but can't use the app's components, routes or live data without a message bridge | Both |
| Who can add one | Someone who builds Agentry | Anyone with the API | Anyone, for (b)'s narrower powers |

### Recommendation

Build (a) now, and keep (b) as what it already is: a client of the public
API. Reasons:

1. The user's own definition of an extension includes adding rules, and
   only (a) can add a rule without holding the store's write lock across a
   process boundary.
2. Agentry is one binary with one applier. (a) keeps both: a feature's state
   is one more slot in `current.State`, built and checked the same way, and
   the equivalence test covers it for free.
3. Readability first: today ideas reach into 27 shared files. Under (a)
   they are one Go package and one UI folder, and each core file has a
   generic loop where it had an ideas branch. That is the separation the
   user ranks first.
4. Every feature anyone has asked for (Ideas, T-019, T-024, Shipping,
   profiles) is first-party. A plugin protocol for third parties would be
   built for nobody yet.
5. (b) costs nothing to keep open. `agentry mcp` is already an out-of-process
   extension in all but name: it reads the API (`client.Client`), waits on
   changes and writes through the same endpoints. When a third party
   arrives, the work is scoped credentials (which types a credential may
   propose) and a manifest entry that points at a sandboxed page. That is
   (c), reached when it is needed, on the same manifest.

## 6. Core API sketch

Small and explicit: one package, `internal/features`, with one struct that
lists everything a feature can contribute. Every field is optional. Core
packages import `features` (types only); feature packages import core types
(`events`, `tasks`, `agents`, `store`); `cmd/agentry/serve.go` puts the list
together. No package registers itself from `init`: the list is written out
once, so a reader sees every feature in one place.

### Go

```go
// Package features is what a feature, such as ideas, adds to Agentry, and
// the list of them. Core reads the list; it never names a feature.
package features

// A Feature is one capability a team or a setting turns on. Every field but
// Name may be left out.
type Feature struct {
	Name string // "ideas": its manifest entry, routes and UI folder use it

	Streams *Streams                   // the streams it owns, if any
	Guards  map[events.Type]Rule       // extra checks on core types, as rules.rosterRules (T-024)
	Coordinator *Coordinator           // its part in coordinators' briefs and decisions
	Routes  func(Core) []Route         // its HTTP API
	Tools   func(client.Client, string) []mcp.Tool // its MCP tools, for the session's repo
	Waiting func(client.Client) (mcp.Waiting, bool) // what of it waits for you
	Manifest func(Reader) Entry        // whether the page shows it, and why
}

// Streams are the feature's own streams: every one starts with Prefix and
// holds only Types. Core treats them as not a task's (events.IsTaskStream).
type Streams struct {
	Prefix string                     // "idea-"
	Types  map[events.Type]events.Spec // fields and writers, checked as the core's are
	// FromEvents builds a stream's state at Open; Next is the state after one
	// more event, on a copy. Both as tasks.FromEvents and tasks.Next.
	FromEvents func(accepted []events.Event) any
	Next       func(prev any, e events.Event) any
	Check      Rule // check 3 for a proposal on one of its streams
}

// Rule is check 3 for one proposal, given the state it would be appended to.
type Rule func(p events.Proposal, state rules.State) *events.Rejection

// Coordinator is a feature's part in the loop, for coordinators For says.
type Coordinator struct {
	For      func(coordinator agents.Agent) bool                // a Visionaries coordinator
	Brief    func(task *tasks.Task) []Section                    // more sections for its brief
	Decision string                                             // what its done decision may add, for "Your decision"
	Field    string                                             // the decision's field it reads: "ideas"
	OnDone   func(task *tasks.Task, field json.RawMessage) string // acts on it; returns a note for the summary
}

// Entry is the feature's line in GET /api/features.
type Entry struct {
	Enabled bool   `json:"enabled"`
	Visible bool   `json:"visible"`
	Why     string `json:"why,omitempty"`
	Badge   int    `json:"badge,omitempty"`
}
```

`Core` is what a feature's routes and hooks may use: the `*store.Store` to
propose and `Create`, a `Reader` over `*current.State`, and the server's
`Config.Budget`. `Reader` is the read side of `current.State` that features
see: `Task`, `TaskIDs`, `Roster`, and two new generic reads for owned
streams, `Owned(stream) (any, error)` and `OwnedIDs(prefix) []string`. A
feature wraps them in typed helpers (`ideas.Get(r, id) (*Idea, error)`), so
the `any` stays inside its package.

How each core package changes, with the real types:

- `events`: `spec` becomes `Spec`; `Register(prefix string, types
  map[Type]Spec)` adds to `catalogue` before `store.Open` and panics after,
  so the catalogue is fixed while the log is open. `IsTaskStream` excludes
  every registered prefix instead of `IsIdeaStream`. `ideaTypes`,
  `IdeaPrefix` and the three `Idea*` constants move to `internal/ideas`.
- `current`: `State.ideas` becomes `owned map[string]any` plus the
  `*features.Streams` for each prefix. `Load` and `Apply` keep their
  `switch` for the core streams; the `default` asks the owning `Streams`
  (found by prefix) before falling back to `tasks`. The lock, `guard`,
  `breaks` and `broken` are unchanged, so a feature's panic breaks only its
  stream, as a task's does. There is still one applier. `Equal` loops over
  every prefix with `FromEvents`.
- `rules`: `State.Idea` goes; `State` gains `Owned`. `Check` runs
  `Guards[p.Type]` beside `rosterRules`, and the owning `Streams.Check`
  where it now calls `checkIdea`.
- `tasks`: `Task.Idea` becomes `Task.MadeFrom` (any feature's stream that
  made the task), read from the same `"idea"` data on old events and a
  feature-neutral `"from"` on new ones, so old logs replay unchanged.
- `store`: `IdeaIDs` becomes `StartedBy(events.Type)`; `Create` is already
  generic.
- `coordinator`: `Decision.Ideas` goes; `Decision` keeps the raw reply so a
  feature's `Field` can be read from it. `coordinatorBrief` appends the
  sections and decision text of each feature whose `For` matches; `Loop.apply`'s
  `case Finish` calls `OnDone` for it. The "only a Visionaries coordinator
  suggests them" note becomes generic: a field no feature of this
  coordinator claims is reported, not kept.
- `web`: `Handler` mounts each feature's `Route{Pattern, Handler, Desk
  bool}` with `api` or `deskToo`; `reply`, `decode`, `failed` and `propose`
  move to a small exported kit both use. `GET /api/features` calls every
  `Manifest`.
- `mcp`: `tool` becomes `Tool`; `tools()` appends every feature's; `WaitingFor`
  calls every `Waiting`. The MCP process reads `GET /api/features` at
  `initialize` and lists only the tools of enabled or visible features.

Ideas then is `internal/ideas` plus `feature.go`:

```go
// Feature is ideas: the features a Visionaries session suggests.
func Feature() features.Feature {
	return features.Feature{Name: "ideas",
		Streams:     &features.Streams{Prefix: Prefix, Types: types, FromEvents: fromEvents, Next: next, Check: check},
		Coordinator: &features.Coordinator{For: isVisionaries, Brief: dismissedSection, Decision: Format, Field: "ideas", OnDone: suggest},
		Routes:      routes, Tools: tools, Waiting: openIdeas, Manifest: manifest}
}

// manifest shows Ideas while a Visionaries team can suggest them, or any
// idea is left to see.
func manifest(r features.Reader) features.Entry {
	enabled := hasVisionaries(r.Roster()) // a team whose coordinator isVisionaries (Roster.Teams, Roster.Coordinator)
	ids := r.OwnedIDs(Prefix)
	return features.Entry{Enabled: enabled, Visible: enabled || len(ids) > 0, Badge: open(r, ids), Why: why(enabled, len(ids))}
}
```

and `serve.go` lists it: `all := []features.Feature{ideas.Feature()}`.

### TypeScript

A feature's UI is one folder, `ui/src/features/<name>/`, whose `index.ts`
exports one object. The shell, the router and the board iterate a list of
them, and the manifest decides which show.

```ts
// ui/src/features/index.ts
export interface FeatureUI {
  name: string; // the server's feature name in GET /api/features
  routes?: RouteObject[]; // always mounted, so a bookmark works when the feature is off
  nav?: { section: "work" | "setup"; to: string; label: string; icon: ReactNode }[];
  commands?: (navigate: NavigateFunction) => PaletteItem[]; // Ctrl K, under "Go to" or "Commands"
}

export const FEATURES: FeatureUI[] = [ideas];

// useFeatures reads GET /api/features (refetched when the log moves) and
// says which features show. Unknown names on either side are ignored.
export function useFeatures(): { visible: (name: string) => boolean; badge: (name: string) => number; columns: BoardColumn[] };
```

```ts
// ui/src/features/ideas/index.ts
export const ideas: FeatureUI = {
  name: "ideas",
  routes: [{ path: "/ideas", element: <IdeasPage /> }],
  nav: [{ section: "work", to: "/ideas", label: "Ideas", icon: <Lightbulb /> }],
  commands: (navigate) => [{ id: "go-ideas", group: "Go to", label: "Ideas", icon: <Lightbulb />, run: () => navigate("/ideas") }],
};
```

- `App.tsx` spreads `FEATURES.flatMap((f) => f.routes ?? [])` into the
  shell's children. Routes are static because `createBrowserRouter` runs
  once; the page itself shows the empty state when its feature is off.
- `Sidebar.tsx` keeps its core items and appends `nav` items of visible
  features to their section, with `badge` as the count.
- `AdminShell.tsx` appends `commands` of visible features to
  `paletteItems`.
- `TasksPage.tsx` keeps its column definitions and shows those in
  `columns`.
- `IdeasPage.tsx`, `IdeaCard.tsx`, `IdeaDialogs.tsx`, `lib/visionaries.ts`
  and the `Idea` types move into `ui/src/features/ideas/`. The Visionaries
  part of `NewTaskForm.tsx` stays where it is in phase one: the `newTeam`
  preset is core; the form's theme field is the next thing to move, through
  a "new task for this team" slot, once a second team needs one.

## 7. Enable, disable and replay (part 2)

## 8. Versioning, and templates that declare what they need (part 2)

## 9. Security (part 2)

## 10. What moves first, and what stays core (part 2)

## 11. Phased plan (part 2)

## 12. Diagram (part 2)

## Surprises for review

- `ideas.List` and `ideas.Load` read the log (`store.IdeaIDs`, then
  `Store.Stream` and `FromEvents` per idea) on every `GET /api/ideas`, every
  accept and dismiss (`ideaExists`), and every Visionaries brief
  (`dismissedFor`), although `current.State` already holds every idea.
  `State` has `Idea(id)` but no list, so the read model stops halfway for
  ideas. The `OwnedIDs` read above closes it.
- The feature is bound to a team by name, in three places that must agree:
  `coordinator.VisionariesTeam`, `VISIONARIES` in `ui/src/lib/teams.ts`, and
  the template's `Team`. A team you name "Visionaries" by hand gets ideas;
  the template's team under another name would not. Part 2 proposes
  recording which template, and so which features, a team came from.
- The brief's "addTeam's missingProfiles" does not exist; the check is
  `web.unknownProfile` in `internal/web/agents_api.go`, used by `addTeam`
  and by `agentFrom` (for `createAgent` and `changeAgent`).
- The Shipping column has the same problem as Ideas: `TasksPage.tsx` always
  shows it, even on an install whose teams have no agent that ships.
- `GET /api/config` sends `hasTerminal` and `hasDesk`, and the page reads
  neither.
- Replay already survives a feature's code being absent, by accident: the
  store loads events without checking their types, an `idea-N` stream
  would then count as a task stream with no `task.created`, so
  `tasks.Replay` gives nil and the stream is skipped. Part 2 makes that a
  rule rather than a coincidence.
- The principle that "extensions read events, write proposals and add
  rules" is not written in PLAN.md or anywhere in the repo; it is in the
  task's brief only. Part 2 could put it in PLAN.md's decisions.
