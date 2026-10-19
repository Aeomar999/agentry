# Extensions: how teams bring features and UI

Status: proposed, for task-46 ("Research how teams contribute features and
UI: the shape of Agentry's extensions").
Research only: nothing in the server or the page changes with this document.
It covers the inventory, what an extension is, how the page learns what
is on, the options, the core API, replay, versioning, security, what moves
first, and a phased plan ready to file as Backlog stories.

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

Turning a feature off never touches the log. Disabled (no team), its data
stays readable and actionable; absent (no code), its streams and types are
skipped on replay, refused for new proposals and counted, by a stated rule
in place of today's accident (section 7). Event types change only by adding
optional fields, or by a new name (section 8). Team templates gain an ID
and a list of features, and each agent records the template it came from,
which fixes the name matching that misses a second "Visionaries 2" team.
A feature's agents go through the gate like any other; a feature owns its
prefix and types, names the core types it writes, and its rules can only
refuse (section 9). Tasks, the gate, agents and machines stay core
(section 10).

The plan (section 11) starts small: phase 1 is five stories that make
Ideas show only while a Visionaries team or an idea exists, through
`internal/features`, `GET /api/features` and the page's feature registry.
Phase 2 moves the rest of Ideas onto the API, phase 3 brings the Shipping
column and team profiles, phase 4 T-019, T-024 and T-036.

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
| Lists | `internal/ideas/list.go` (`Load`, `List`, `Dismissals`) | Reads ideas from the log (`store.IdeaIDs`, `Store.Stream`), not from `current.State` | Own (and should read the state: section 11, story 1) |
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
| Policy profiles | `read-mostly` | A way to declare a needed profile and its rules, and the existing `unknownProfile` check when the file in force lacks it (section 8, with task-35) |
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
| Replay of old logs | The feature's code replays its streams; without the code they are skipped and counted (section 7) | The core replays nothing of it; the extension replays its streams itself on every start | Both cases |
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

## 7. Enable, disable and replay

### What turning a feature off means

A feature is turned off in one of two ways, and neither touches the log:

- Disabled: its code is in the binary but its trigger is gone (you removed
  the Visionaries team, or the GitHub token). Its streams still load, its
  state is still in `current.State`, its routes and tools still answer, and
  its manifest entry says `enabled: false` and stays `visible` while it has
  data. Nothing new is made: no Visionaries coordinator means no done
  decision with ideas, so no `idea.suggested`.
- Absent: the binary has no code for it (a build without the feature, or an
  older binary on a newer log). Its events stay in the log untouched; the
  core skips them on replay, refuses new ones, and counts them.

Events are never deleted or rewritten to turn a feature off, as
`docs/design/log-growth.md` requires of every change. Archiving there moves
only a done task's rows, under a `task.archived` marker; a feature's
streams are not tasks and are never archived by it.

### How the core classifies a stream

Today the rule is accidental. `current.Load` sends any stream that is not
`agents`, `workers`, `connections` or `policy` and does not start with
`idea-` to `tasks.Replay`, and `Replay` returns nil for a stream with no
`task.created`, so without the ideas code an `idea-N` stream would load as
nothing. Three other places would not be so quiet: `events.IsTaskStream`
would call `idea-3` a task's stream, so `rules.Check` would let a
`task.renamed` into it; `coordinator.touchedTasks` would hand `idea-3` to
`Loop.act`, which logs "reading the task" as an error for every idea event;
and `current.Equal` would never look at it.

The stated rule, in `current` and `events` both:

1. A core stream is one of the fixed names: `agents`, `workers`,
   `connections`, `policy`.
2. An owned stream starts with a prefix a feature registered (`idea-`). Its
   events go to that feature's `Streams`.
3. A task stream is one whose first accepted event is `task.created`. That
   is what `tasks.Replay` already checks, so every old log replays the same.
4. Anything else is a foreign stream: events whose stream or type this
   binary doesn't know. They stay in the log and are skipped in apply.

`IsTaskStream` changes to match: a stream is a task's when `current.State`
holds a task for it, and a proposal for a stream that is neither core,
owned nor an existing task must be `task.created` made by `Store.Create`
with the `task-` prefix. So `rules.Check` refuses any proposal into a
foreign stream ("stream idea-3 belongs to a feature this build doesn't
have"), and the coordinator acts only on task streams. A new proposal of an
unknown type is refused already, by `events.CheckShape` ("unknown event
type"); only replay meets unknown types, and it skips them.

The same rule covers a feature's types inside a core stream. T-019 would
write something like `pr.opened` into a task's stream. `tasks.apply` has no
case for it and ignores it (it only moves `LastActiveAt`, as every event
does); that stays true and gets a test: a task stream with an unknown type
in the middle replays to the same task as without it, apart from
`LastActiveAt`.

### Counting what was skipped

`current.State` keeps a small tally while it loads and applies: per foreign
stream prefix (the stream name up to its last `-`) and per unknown type in
a known stream, the number of events and the last position. It is never
an error and never breaks a stream. `GET /api/features` returns it beside
the features:

```
"unknown": [
  { "prefix": "idea-", "streams": 12, "events": 31, "last": 48211 },
  { "type": "pr.opened", "events": 4, "last": 48190 }
]
```

The page shows it in one place, as one line ("31 events from a feature
this build doesn't have: idea-"), so a downgrade is explained rather than
silent. There is no Settings page yet, so where that line goes is open. The server logs one line per prefix at
startup, not one per event.

### Re-enabling

Re-enabling is never a migration. Disabled to enabled: the state was kept
all along, and the manifest flips on the next `agent.created` that brings
the team back. Absent to present: the next start replays the log, and the
feature's `FromEvents` builds its streams from every event it ever wrote,
including the ones the older binary only counted. A feature must therefore
replay any event it ever wrote, in every version (section 8), exactly as
`tasks.apply` must.

### The applied gate, snapshots and the equivalence test

- The applied position (`docs/design/read-model.md`, section 4) moves for
  every committed event, skipped or not, as it does for a
  `proposal.rejected` today. A reader of the log after position N still
  sees foreign events, as raw events: `GET /api/changes` and
  `Store.NewReader` return what the log holds, and each consumer ignores
  types it doesn't know (the page's `LiveProvider` only uses positions).
- `current.Equal` loops over every registered prefix with that feature's
  `FromEvents`, beside tasks and rosters, so a feature's slice is proven
  equal to a replay like any other. Foreign streams are compared by their
  tally only.
- Snapshots (`docs/design/log-growth.md`, section 2) are tagged with the
  binary's hash, and any doubt means a full replay. That already covers a
  feature arriving or leaving: a different binary is a different hash. The
  snapshot also lists the feature names and versions it holds slices for,
  so a later change to the tag rule can't load a slice its code can't read.
  A snapshot never holds a foreign stream's state, only its tally.

### Task.Idea becomes Task.MadeFrom

`task.created` carries `"idea": "idea-7"` when you accept an idea, read by
`tasks.apply` into `Task.Idea`. The field becomes `Task.MadeFrom`, the
stream of whatever made the task, so the task package names no feature:

- New events carry `"from": "idea-7"`; `apply` reads `"from"`, and falls
  back to `"idea"` when `"from"` is absent, so every old `task.created`
  replays to the same task. The fallback stays for as long as old logs
  exist, which is always.
- The page shows "Made from" with a link the owning feature supplies (the
  feature whose prefix starts the stream), and nothing when that feature
  is absent: the stream name alone.
- `rules.madeFromIt` (accepting an idea needs the task that says it was
  made from it) reads `MadeFrom`.
- The `catalogue` comment on `TaskCreated` says "may carry the stream it
  was made from".

## 8. Versioning, and templates that declare what they need

### Event types

An event type is a contract with every log ever written: a stored event is
never rewritten, so `apply` reads every shape a type has had. The rules:

- A type's meaning never changes. Adding an optional data field is a minor
  change: old events lack it, and `apply` gives it the old default, as
  `task.created`'s `maxTurns` does today ("a task made before keeps the
  old limit").
- A required field is never added to an existing type, because
  `events.CheckShape` would only enforce it on new proposals while replay
  still meets events without it. If a type must change its shape, it gets a
  new name (`idea.suggested` and, say, `idea.proposed`), the old one stays
  registered for replay with no writers, and `apply` handles both.
- `events.Spec` gains a `Since` (the feature version that added the type)
  and a `Retired` flag (registered for replay only: `CheckWriter` refuses
  it). No version is stored on each event: the type name and the fields
  present already say which shape it is, and the `events` table stays as
  it is.

T-036 (a JSON Schema per event type) fits here: the `Spec` a feature
registers is the place to generate it from, and a schema test per type
checks that every fixture event still validates, so an incompatible change
fails in CI rather than on someone's log. With features writing events,
T-036 moves from "later" to the phase that makes `Spec` public.

### Features

A `Feature` gains `Version int`, raised when its events or state change.
The manifest carries it, so a page built for a newer version can say so,
and the snapshot tag lists it (section 7). There is no `feature.enabled`
event: enabled is worked out from state (a team, a token) on every
request, so it can't drift from what is there.

### Templates declare what they need

`agents.Template` today has `Team`, `Description`, `Folder` and `Agents`.
It gains two fields:

```go
type Template struct {
	ID          string   // "visionaries": stable, recorded on the agents it makes
	Team        string   // "Visionaries": the default name, which you may change
	Description string
	Folder      string
	Agents      []Agent
	Features    []string // "ideas": what a team made from it turns on
}
```

- Profiles are already declared, per agent, by `Agent.Profile`. `addTeam`
  checks each with `web.unknownProfile` and refuses the team with "copy
  `[profiles.read-mostly]` from `internal/policy/default.toml`". That
  stays the gate; task-35 (profiles per team) decides how the profile gets
  into the user's `policy.toml` in one step, for example a button that
  appends the shipped profile to the file, which reloads it with a
  `policy.loaded` as any save does. The feature does not bring policy of
  its own: the policy file stays the user's, and the only thing a template
  may do is name a profile.
- `Features` is what makes the manifest's `enabled` true. A feature's
  `Manifest` asks the roster for a team made from a template that lists
  it, instead of matching a team name.

### Recording which template a team came from

The feature is bound to a team by name today, in three places that must
agree: `coordinator.VisionariesTeam` ("Visionaries", compared by
`isVisionaries` with `strings.EqualFold` on `TeamName()`), `VISIONARIES` in
`ui/src/lib/teams.ts`, and the template's `Team`. Two bugs follow:
`agents.AddTeam` names a second copy "Visionaries 2", which `isVisionaries`
doesn't match, so that team's sessions suggest nothing; and a team you
build by hand and call "Visionaries" does get ideas.

The fix: `AddTeam` puts `"template": "visionaries"` in each
`agent.created` it proposes, and `agents.Agent` gains `Template string`.
`Roster.TemplateOf(team)` is the template of the team's coordinator.
`isVisionaries` becomes a check that the coordinator's template lists the
`ideas` feature. `Agent.Proposal` writes the key on every `agent.created` and
`agent.changed`, empty for an agent you make by hand, so a missing key
means an event from before the field. Only for those does the roster fall
back, at replay: a team whose name equals a template's `Team`, or that name
plus " N", counts as made from it. That keeps today's
Visionaries teams working with no new event. On the page, "Ask the
visionaries" finds the team by `template` on `GET /api/agents` and adds one
from the `visionaries` template when none exists; `VISIONARIES` becomes the
template ID.

## 9. Security

### A feature's agents

They are agents: every tool call goes through the hook gate and is judged
with the agent's own `Profile`, as T-041 made it. A feature cannot add
allow rules, cannot widen a profile and cannot run anything outside an
agent's turn. The Visionaries' `read-mostly` profile is in the user's
`policy.toml`, where they can read and change it. Agentry's own files stay
protected in code, whatever profile is in force.

### Who may write which events

Compiled-in code is trusted code: it is reviewed and built into the
binary, and nothing in Go stops it calling `Store.Propose` with any type.
The point of the rules below is legibility and catching mistakes, so a
reader can see from the registration what a feature can write.

- A feature owns its prefix and its types. `events.Register` panics at
  start if a prefix or a type name is already taken (by core or another
  feature) or if a type's name does not start with the feature's
  namespace (`idea.`). Types it registers for its own streams can go only
  there (`belongsIn`, as today for ideas).
- Each type's `writers` (`Human`, `Agent`, `System`, ...) work as now:
  `events.CheckWriter` refuses an `idea.accepted` from anyone but you.
- A feature names the core types it writes. Ideas writes `task.created`
  when you accept one (`ideas.makeTask`). `Core` hands a feature a
  proposer that refuses any type the feature neither registered nor listed
  in `Writes`, with the rejection rule "features". A refusal is stored as
  `proposal.rejected` like any other, so a mistake shows in the log.
- Guards can only refuse, never change a proposal. A feature may guard
  core types (T-024 guards approvals) and its own, but not another
  feature's: one feature depending on another is a reason to merge them or
  move the shared part to core. Each refusal names its feature ("ideas:
  ..."), so it is clear whose rule said no.
- A feature's rule, guard, `FromEvents` and `Next` run under `recover`.
  Today a panic in `rules.Check` is not recovered: it unwinds through
  `Store.Propose` and takes down the goroutine that proposed (the loop, or
  a request). For a feature's code, a panic in a rule is a refusal, and a
  panic in apply breaks only that stream, as `current.guard` already does.

### HTTP and MCP

A feature's routes are mounted by the core through the same wrappers as
every route: `api` (behind `requireUI`) or, if the route says `Desk: true`,
`deskToo` (behind `requireUIOrDesk`). A `Route` can't choose to skip them,
and no feature route can sit behind `requireRealUIToken`, which hands out
the token. Every HTTP write is made as `you` (`web.you`), so a feature's
endpoint has your authority and no more.

That includes connections. A Claude Code session's credential passes
`carriesUIToken`, so `accept_idea` through MCP writes `idea.accepted` as
you. The MCP instructions say "never accept or dismiss an idea on your
own", but that is a prompt, not a check. For first-party code that is the
accepted trade today; it is the first thing to close for (b).

### What (b) and (c) would need

An out-of-process extension must not get the page's authority. It would
need:

- A credential of its own (a new `ActorKind`, `Extension`, with an ID), so
  its events say who wrote them and `writers` can allow or refuse it.
- A scope on that credential: the types it may propose and the streams it
  may read, checked in the core before `Store.Propose`. A connection's
  credential could take the same scopes, which would also close the
  `accept_idea` gap above.
- No rules: its checks run after the fact, as proposals of its own.
- Its UI only in a sandboxed frame (`ui/src/lib/sandbox.ts`), with a
  message bridge that exposes reads, never the token.

## 10. What moves first, and what stays core

| Feature | When | Why then |
|---|---|---|
| Ideas and Visionaries | Phase 1 (visibility), phase 2 (the rest) | The trigger, the clearest case, and the one that touched 27 shared files; everything the API needs, it uses |
| Shipping column | Phase 3 | Already "on" only when a team has `Column: agents.Ships`, but the column always shows. The manifest's `board.columns` fixes it without moving code: the column logic in `tasks` and `coordinator` stays core, because a task's column is how the board and the coordinator agree |
| Per-team policy profiles | Phase 3, with task-35 | Template `ID` and `Features` from section 8, plus one-step profile setup; the gate stays core |
| Integrations (T-019) | Phase 4 | The first feature with a background job and types in a core stream; proves those parts of the API |
| Senior approval (T-024) | Phase 4 | The first guard on a core type; proves guards |
| Design, Research, Writing templates | Not features | Data only: they gain an `ID` and no `Features` |

What stays core, and why:

- Tasks, epics and stories, the board's columns and the trail: every
  feature reads them, and the coordinator loop, the rules and the read
  model are written around them. A feature adds to a task; it is never one.
- The gate, the policy and profiles: security must not depend on which
  features are installed.
- Agents, teams, templates and the roster: a feature is turned on by a
  team, so teams can't be a feature.
- Workers and machines, connections, files and worktrees: how agents run
  at all.
- The log, `Store.Propose`, `current.State` and the applied gate: the
  thing every feature plugs into.
- Questions and approvals, the waiting list and notifications as
  mechanisms; features add sources to them.

## 11. Phased plan

Each story is one reviewable change, with a title and what done looks like,
ready to file as Backlog stories under one epic.

### Phase 1: Ideas shows only when it is useful, on the new mechanism

1. **List ideas from the read model.** `current.State` gains `Ideas()` (in
   log order, broken ones left out, as `Tasks()`); `ideas.List`, `Load`,
   `ideaExists` and `dismissedFor` read it; `store.IdeaIDs` leaves the
   request path. Done: `GET /api/ideas` makes no SQL query (a test with a
   counting store), the existing ideas tests pass, and `current.Equal`
   still covers ideas.
2. **Record the template a team came from.** `Template.ID` and
   `Template.Features`; `AddTeam` writes `"template"` on each
   `agent.created`; `Agent.Template`, `Roster.TemplateOf`, the name
   fallback for old events; `isVisionaries` uses the template. Done:
   tests for a second team ("Visionaries 2") suggesting ideas, a hand-made
   team called "Visionaries" not suggesting them, and an old log with no
   `template` field still treating its Visionaries team as one.
3. **The features package and `GET /api/features`.** `internal/features`
   with `Feature{Name, Version, Manifest}` only, the `Reader` over
   `current.State`, the list in `serve.go`, and Ideas' `manifest`. Done:
   API tests for no team and no ideas (hidden), a Visionaries team
   (enabled and visible), the team removed with ideas left (visible, not
   enabled, `why` says so), and the badge counting open ideas.
4. **The page's feature registry.** `ui/src/features/` with `FeatureUI`,
   `FEATURES`, `useFeatures` (through `useResource`, so it refetches as the
   log moves); Ideas' page, card, dialogs, types and `visionaries.ts` move
   into `ui/src/features/ideas/`; `App.tsx` mounts feature routes;
   `Sidebar.tsx` and `AdminShell.tsx`'s `paletteItems` add nav items and
   commands of visible features. Done: `npm run lint` and `npm run build`
   clean; on a scratch install, Ideas is absent from the sidebar and Ctrl K,
   appears when the Visionaries team is added, and stays after removing it
   while ideas exist; `/ideas` always opens.
5. **The empty Ideas page.** With the feature off, `/ideas` shows what
   Ideas is and an "Ask the visionaries" button that adds the team.
   "Ask the visionaries" finds the team by template, not by name. Done: a
   bookmark to `/ideas` on a fresh install shows the empty state, not a
   404, and the button adds the team and opens New task.

### Phase 2: Ideas as a feature package

6. **Event types from features.** `events.Spec` and `events.Register`
   (prefix, types, namespace check, panic after `store.Open`); ideas
   registers its three types; `IsIdeaStream` and `IdeaPrefix` leave
   `events`. Done: the catalogue tests pass with ideas registered in the
   test setup, and registering a taken prefix or type panics.
7. **The stream rule and the unknown tally.** Section 7's four kinds of
   stream in `current`, `rules` and `coordinator.touchedTasks`; the tally in
   `GET /api/features`; one log line per prefix at startup. Done: a fixture
   log with `idea-N` streams replays on a build without ideas registered,
   with the same tasks as before, the tally counting them, no
   "reading the task" errors, and a proposal into `idea-3` refused.
8. **Owned state slices.** `current.State.ideas` becomes the owned map;
   `Load`, `Apply` and `Equal` go through `Streams`; `rules.State.Idea`
   becomes `Owned`; feature code runs under `recover`. Done: the
   equivalence test passes over every fixture log, and a feature whose
   `Next` panics breaks only its stream.
9. **Task.MadeFrom.** `"from"` on new `task.created` events, `"idea"` read
   on old ones. Done: an old log's accepted idea and a new one both show
   "Made from idea-N" on the task.
10. **Coordinator hooks.** `features.Coordinator`; `Decision.Ideas` and
    the ideas lines in `coordinatorBrief` and `Loop.apply` go. Done: the
    coordinator's ideas tests pass through the hook, and a done decision
    with `ideas` from a non-Visionaries coordinator is reported, not kept.
11. **Routes, tools and waiting from features.** `Route`, the exported
    reply kit, `mcp.Tool`, `Waiting`, the MCP instructions per feature,
    the proposer that checks `Writes`. Done: `internal/web` and
    `internal/mcp` name no ideas type; the MCP process lists the ideas
    tools only while the feature is enabled or visible.

### Phase 3: the board and teams

12. **Board columns from the manifest.** `board.columns` lists Shipping
    while a team ships or a task is in Shipping; `TasksPage.tsx` shows
    those. Done: an install of only Writing has no Shipping column; adding
    Software brings it.
13. **One-step profiles for a template** (with task-35). Done: adding the
    Visionaries team to an install whose `policy.toml` lacks
    `read-mostly` offers to add it, and does so with one click.
14. **Use or drop `hasTerminal` and `hasDesk`.** `GET /api/config` sends
    both and the page reads neither. Done: either the terminal dock and the
    desk entry hide when the server has none, or the fields are gone; no
    flag is sent that nothing reads.

### Phase 4: the next features on the API

15. **T-019 as a feature** (background job after `review.approved`, a
    type in a task stream, a task page slot). 16. **T-024 as a feature**
    (a guard on approvals). 17. **T-036 schemas** generated from the
    registered `Spec`s, checked against every fixture log. Each gets its
    own design note before it is built.

## 12. Diagram

![How a feature plugs into the core](extensions.svg)

The core (grey panel) owns the log, `Store.Propose` with the rules,
`current.State`, the coordinator loop, the web server, the MCP process and
the UI shell. The Ideas feature (right) registers once, in `serve.go`, and
each of its parts lands in one core slot: its types in the catalogue, its
rule in `Check`, its state slice in `current.State`, its hook in the loop,
its routes in `web`, its tools in `mcp`, its manifest entry in
`GET /api/features`, and its page, nav item and command in the UI
registry. Nothing in the core names it. The source is
`docs/design/source/figures.py`.

## 13. What the Visionaries think

To come: a short session with the Visionaries team on this recommendation.

## Still open

- Whether a connection's credential should be scoped now (section 9), or
  only when (b) arrives. It would stop a Claude Code session from
  accepting ideas on its own, which today only the MCP instructions do.
- Where the page shows the unknown tally (section 7): there is no Settings
  page yet; the Policy page or a small diagnostics view are candidates.
- The principle is not written in the repo. A proposed line for PLAN.md's
  decisions: "Extensions are compiled-in features that read events, write
  proposals and add rules through `internal/features`. They are not
  read-only, and they never write past the rules. Turning one off never
  deletes or hides its data, and a log replays whether or not its code is
  present."
