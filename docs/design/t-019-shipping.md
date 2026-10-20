# T-019: shipping through GitHub and GitLab

Status: proposed, for task-121 (story 19 of the extensions epic,
`docs/design/extensions.md` section 11, phase 4).
Design only: nothing in the server or the page changes with this document.
It covers the feature's event types, what starts it, its job, how it fits
with the deployer, how it asks the host every two minutes, its tool, its
waiting entry, its tab, its webhook (later), its settings, security, the
build stories and your decisions.

## In short

PLAN.md's T-019 is "open the PR after `review.approved`". Today, a task's
work leaves Agentry when the deployer merges the task branch into dev and
pushes (`internal/agents/defaults/deployer.md`). Nothing records the
pipeline that push starts, beyond one line in the deployer's report, and
the Visionaries found three Backlog tasks (41, 44, 45) that all say "Done
isn't shipped".

The design is a feature, `git`, in a package `internal/githost`, turned
on by a token in `features.toml`, for GitLab first (GitHub is a later
story). When it is on for a task's repo, the deployer pushes the task
branch instead of dev, and the feature's job opens a merge request from
`agentry/task-N` into dev. You merge it on GitLab. Agentry has no public
address, so GitLab can't call it: every two minutes the job asks GitLab
about each open request, its state and its pipelines, and writes what
changed as `git.*` events in the task's stream, as System. A webhook can
write the same events later, once Agentry has a public https address. The
task page gets a Merge request tab, Claude Code gets a `show_pipeline`
tool, and a failed pipeline waits for you. A task is done once its request
is open, as today it is done once pushed: in fact at the push, with the
request opening just after it, and a refusal waiting for you (section 4).

There is no `review.approved` event, and none is needed. A reviewer's
verdict is read from its report when the coordinator decides
(`internal/coordinator/checks.go`), and the deployer only gets the task
once the work is checked or you moved it to Shipping. So the job starts
from what the log already holds: the deployer's approved, finished push of
the task branch (section 2).

One core change is proposed that the extensions note didn't plan: a
task slice. The feature's events sit in task streams, and `current.State`
keeps nothing for types it doesn't know, so every read would rescan
streams. A task slice is the owned-stream mechanism applied to a feature's
types inside task streams, with one applier, and a check that can refuse
(section 1).

![How a reviewed task becomes a merge request and comes back](t-019-shipping.svg)

## 1. The git.* task types

Each type is one of the feature's `TaskTypes` (`events.Registration`), in
the `git.` namespace, which no core type uses (`NewCatalogue` refuses a
task type in a core namespace). Every one goes in a task's stream. Required
fields are the `Spec.Fields`; the rest are optional from the start, as the
core's comments list them. A field is only ever added as an optional one,
and a type whose shape must change gets a new name (extensions section 8).

| Type | Written by | Required fields | Optional fields |
|---|---|---|---|
| `git.pr_opened` | System, the job (`{System, "git"}`) | `host` (`github` or `gitlab`), `number`, `url`, `branch`, `base`, `sha` | `found`: true when the job found the request already open and recorded it, rather than opening it; `at`: the host's update time in its reply |
| `git.pr_changed` | System: the job, when it polls (section 3) or checks a request on a push (step 4); later a webhook (`httpapi.System`, section 8) | `number`, `state` (`open`, `closed` or `merged`), `at` (the host's update time of the request) | `sha` (a new head), `delivery` (a webhook's), `push`: when the job wrote it on a push, the log position of that push (section 3) |
| `git.pipeline` | System: the job when it polls; later a webhook | `id`, `status` (`pending`, `running`, `success`, `failed`, `canceled`, `skipped`, `manual`), `url`, `sha`, `at` (the host's time: the latest of the pipeline's `updated_at` and its jobs' `started_at` and `finished_at`, below) | `ref`, `attempt`, `jobs` (each `{name, status, url}`), `delivery` |
| `git.refused` | System, the job | `action` (`open` or `reopen`), `status` (the host's HTTP status), `reason` | `sha` |
| `git.retry` | Human, the tab's "Try again", while a refusal is outstanding | none | none |

What each means:

- `git.pr_opened`: the request for this task's branch is open on the host.
  It becomes the task's current request. A task has at most one open at a
  time (the check below).
- `git.pr_changed`: the host says the request `number` names was merged,
  closed, reopened or got a new head. It changes only that request, and
  only if its `at` is later than that request's last one (below).
- `git.pipeline`: one pipeline's status, as of `at`. A later event for the
  same `id` and `attempt` replaces the earlier only if its `at` is later,
  so a delivery that arrives late can't undo a newer one. Jobs merge by
  name. GitLab has no attempt (a retried job reruns in the same pipeline),
  so for an event without one the slice counts it: attempt 1, and one more
  each time the pipeline goes from `success`, `failed`, `canceled` or
  `skipped` back to `pending` or `running`. Going from `manual` to
  `running` is the same run carrying on (you played the manual job), not
  a new attempt.
- `git.refused`: the host refused to open or reopen the request, for a
  reason a retry won't fix by itself (a bad token, no access, no such
  branch). The job tries no more until the next push, or a `git.retry`.
- `git.retry`: you asked the job to try again, after fixing the token.
  The Rule refuses one while no refusal is outstanding.

### Pipeline statuses

The status list is part of `git.pipeline`'s shape, so it is settled here.
`success`, `failed`, `canceled`, `skipped` and `manual` are finished:
polling stops on them (section 3) and none of them but `failed` keeps a
waiting entry (section 6). `pending` and `running` are not. GitLab's
statuses map in full, for a pipeline and for each of its `jobs` alike:

| GitLab | `status` |
|---|---|
| `created`, `waiting_for_resource`, `preparing`, `scheduled`, `pending` | `pending` |
| `running`, `canceling` | `running` |
| `success` | `success` |
| `failed` | `failed` |
| `canceled` | `canceled` |
| `skipped` | `skipped` |
| `manual` | `manual` |

`manual` is its own status, not mapped onto another. A pipeline waiting on
a manual job (a deploy gate, say) waits for a person on GitLab, and may
wait for weeks: as `pending` it would keep a merged request polled for
ever, and as `success` or `skipped` the tab would say something false. So
it counts as finished, for stopping polling and for clearing the waiting
entry, and the tab shows it as "waiting on a manual job". While the
request is open, polling goes on and sees the run carry on once you play
the job; once it is merged or closed, polling has stopped, so a manual job
played after that shows on GitLab, not in Agentry. `canceling` is on its way to `canceled`, so it stays `running` until
GitLab says `canceled`. A status the table doesn't name (one a later
GitLab adds) writes nothing for that pipeline and logs one line naming
it; the slice keeps what it had. The same goes for a job: one job with a
status the table doesn't name drops its whole pipeline from that pass,
not only the job, because a pipeline written without one of its jobs
would show a partial list, and the waiting line could name the wrong
failed jobs. The unknown status is logged once per pipeline (the job
remembers which it logged, until a restart), not on every pass. GitHub's statuses get their own table in
story 13, onto the same seven.

The job and the webhook store only these fields, never the host's reply
or payload: those carry names and emails the log has no use for. Strings
are cut to a length (a URL to 2 KB, a job name to 200 characters) and
`jobs` to 100.

### The git slice

A task can have more than one request over its life: a merged one, then a
new one after the task is reopened and shipped again. The host still sends
events for an old one (an edit, a label, a late delivery of its merge). So
the slice keeps its requests by number:

- `requests`: each request it has seen, by `number`, with its state, head
  `sha`, `base`, `url`, `found` and `at`, the host's time of the last
  change the slice took for it.
- `current`: the number of the last `git.pr_opened`. The tab, the waiting
  entry, the Rule's "one open request" and the due rule (section 3) read
  this request only.
- `pipelines`: by `id` and `attempt`, as above. A request's pipeline is the
  latest one on its head `sha`, so a pipeline on an old request's head
  never shows on the current one. `git.pipeline` needs no `number`. Two
  requests on one head sha (one closed and another opened on the same
  commit) share its pipelines, so the new one shows the old one's result
  until a new pipeline runs; it is that commit's result, so it is true.
- `marked`: the position of the last open or push mark: a `git.pr_opened`,
  or the job's own `git.pr_changed`, at its `push` - 1 (section 3, step 4).
- `refused`: the position of the outstanding `git.refused`, or 0 when none
  is. The due rule (section 3, step 3) reads `marked` and `refused` apart,
  so clearing a refusal falls back to `marked`.

How each event changes it:

- `git.pr_opened`: adds or replaces the request with its number, makes it
  `current`, sets `marked` to its position and `refused` to 0. Its `at`,
  when set and later, becomes the request's `at`; a reopened request keeps
  its own `at` otherwise.
- `git.pr_changed` naming the current request: sets its state or head if
  its `at` is later than the request's `at`, and else changes nothing. So a
  late `open` after `merged`, or a late `closed` after a reopen, can't flip
  the request back, whichever source wrote it. One the job wrote with
  `push` sets `marked` to `push` - 1 either way.
- `git.pr_changed` naming an older request: sets that request's state or
  head, by the same `at` rule, so its record stays true. It never changes
  `current` or the current request's state, and it is never a mark.
- `git.pr_changed` naming a number the slice doesn't hold: the Rule
  refuses it ("task-121 has no request !7"), and `Next` ignores one
  anyway, so a log written another way can't change the slice either.
- `git.refused`: sets `refused` to its position.
- `git.retry`: sets `refused` to 0. The Rule refuses one while `refused`
  is 0 ("task-121 has no refusal to try again"), and `Next` ignores one
  then, so a stray retry can't make an old push count again or reopen a
  closed request.

What counts as a change, for the job's poll and for the webhook alike: a
`git.pr_changed` when the host's state or head for a request differs from
that request's record in the slice and the host's `at` is later than the
record's, and a `git.pipeline` when a pipeline is new to the slice, or its
status or jobs differ and its `at` is later. Both are needed, because
`Next` takes only a later `at`: a difference that isn't later (a late
delivery, a host answer older than the slice) would be written and
ignored, and written again on every pass. So equal writes nothing, and
neither does different but not later. A polled pipeline's `at` is the
latest of its `updated_at` and its jobs' `started_at` and `finished_at`,
so a job that finishes inside a running pipeline is a later change
(the poll asks for a pending or running pipeline's jobs on every pass,
section 3).
The slice is built from the log, so a poll after a restart, or a poll and
a webhook seeing the same change, writes it once; two writes that race
both carry the host's same `at`, and the second changes nothing.

### The task slice

`current.State` builds a task with `tasks.apply`, which ignores types it
doesn't know (only `LastActiveAt` moves). So a `git.*` event changes no
state the feature can read, and every read of it (the tab, the waiting
list, the job's "is one open already?") would read the task's stream again.
That is the cost `docs/design/read-model.md` removed.

The fix is the owned-stream mechanism, for a feature's types in task
streams:

```go
// TaskSlice is a feature's state inside each task's stream, built from its
// task types alone: as Streams, but keyed by task. Its task types are the
// ones in its namespace: "git." for the feature "git".
type TaskSlice struct {
	FromEvents func(accepted []events.Event) any // the feature's events of one task, in order
	Next       func(prev any, e events.Event) any
	Check      Rule // optional: refuses a proposal of one of its task types
}

// Rule is a feature's order check for one of its task types: the reason to
// refuse p, or "". slice is the task's slice as Next left it, nil when it
// holds nothing yet.
type Rule func(task *tasks.Task, slice any, p events.Proposal) string
```

`Rule` is new: `extensions.md` only sketches it, and `rules.Check` still
names ideas directly. A feature's task types all sit in one namespace,
named after the feature (`git.pr_opened` is the `git` feature's).
`NewCatalogue` already reads a task type's namespace to refuse a core one;
`events.Namespace` exports that, and the state and the rules find a type's
feature by it.

How the parts change:

- **Passing it in.** `Feature` gains `TaskSlice *TaskSlice`. `current`
  gains `NewFor`, `RestoreFor` and `EqualThroughSnapshotsFor`, which take
  `...features.Feature` and keep each feature's `Streams` and `TaskSlice`.
  `New`, `Restore` and `EqualThroughSnapshots` keep their
  `...*features.Streams` and become one-line wrappers that make a
  `Feature` of each. They are called as `current.New` and the like 23
  times in 15 files (in `cmd/agentry`, `coordinator`, `hooks`, `rules`,
  `storetest`, `tasks`, `watchdog`, `web` and three of `current`'s
  benchmarks), more often inside `current`, with 56 `ideas.Streams()`
  arguments. None of them needs a slice, so they stay
  as they are and story 1 stays a change you can review. `serve.go` calls
  `RestoreFor` with its installed list (story 2), and `storetest.OpenWith`
  calls `NewFor` with `ideas.Feature()`.
- **Load and Apply.** `fold` and `Apply` pass an event of a feature's task
  type to its `Next`, beside `tasks.apply`, which is unchanged. The slices
  sit in `slices map[string]map[string]any` (by task, then feature).
- **A broken slice.** `broken` is keyed by stream, so a slice can't use it
  without breaking the task. A slice's `guard` writes to its own
  `brokenSlices map[string]map[string]error` (by task, then feature)
  instead: the task reads as before, `TaskSlice` returns the error, and the
  slice takes no more events, as a broken stream takes none. A slice that
  breaks in the tail after a snapshot sets `tail.broke`, so Resume falls
  back to a whole replay, as a broken stream does.
- **Archiving.** `archive` drops the task's slices and broken slices with
  the rest of the task. `task.unarchived` refolds the stream (`Refolds`),
  and `fold` builds the slices again from its events. So `TasksWith`, the
  manifest's count and the waiting list never list an archived task, and
  all list it again once it is back.
- **Snapshots.** Owned streams aren't in the snapshot, and Resume rebuilds
  them from the log (`rebuilt`). Slices are the same: `savedState` gains
  `Sliced map[string][]string`, `TasksWith` per feature, broken ones
  included. Resume reads each of those tasks' streams up to the snapshot's
  position and builds just its slices (a `foldSlices`, which leaves the
  task the snapshot holds as it is), before it applies the tail.
  `EqualThroughSnapshots` then compares the slices through every cut, and
  the startup benchmark (`startup_bench_test.go`) gets tasks with slices,
  so the cost of rebuilding them is measured.
- **Reading.** `Reader` gains `TaskSlice(task, feature string) (any, error)`
  and `TasksWith(feature string) []string` (tasks holding a slice, in log
  order, broken ones too, as `OwnedIDs`). `githost.Of(r, task)` wraps
  them, so the `any` stays in the package, as `ideas.Get` does.
- **Equality.** `current.Equal` compares every task's slices with a
  replay, so the equivalence test covers them.
- **The rules.** `rules.Check(p, state, catalogue, installed)` gains the
  installed features; `serve.go` passes them in the check it gives
  `store.Open`, so a feature's `Rule` runs inside `Store.Propose`, under the
  store's write lock, like every other order check. For a proposal of a
  feature's task type, `rules.Check` refuses a foreign or archived task and
  a task that doesn't exist, as now, then runs the feature's `Rule` (none:
  allowed) and returns, before the core's task rules. It reads the slice
  with `TaskSlice` first. A broken slice refuses the proposal with "could
  not read the git slice: ...", as a broken agents or workers stream
  refuses today, so a `Rule` never sees nil for a slice that may hold an
  open request. It calls the `Rule` under `recover`: a panic refuses with
  "its rule failed: ...", as `entryOf` does for a manifest, and never
  reaches whoever called `Propose`. It lets the type into
  a done task: the coordinator finishes a task once the push shows, so
  `git.pr_opened` and every pipeline and merge after it arrive on a done
  task. The `afterDone` list stays the core's. `taskRules[p.Type]` gains a
  nil check, so a type with no entry is allowed instead of calling a nil
  func. The git `Rule` refuses a `git.pr_opened` while the slice's current
  request is open: never two at once, enforced under the lock rather than
  hoped for in the job. It also refuses a `git.pr_changed` for a number the
  slice doesn't hold, and a `git.retry` while no refusal is outstanding.
- **The API.** `GET /api/tasks/{task}` gains `slices`: the names of the
  features holding a slice for the task. The page uses it to show a
  feature's tab (section 7). The core names no feature.

Why not an owned stream per task (`git-121` beside `task-121`): its events
would sit beside the task, not in it, so archiving would leave them
behind, the task's Events tab and `LastActiveAt` wouldn't see them, the
rules couldn't refuse a write into an archived task without reading another
stream, and `Store.Create` names a new stream by a count, not after a task.

### Old logs, and a build without the feature

- A log from before the feature has no `git.*` events: every task's slice
  is empty, and nothing changes.
- A build without the feature (an older binary, or one built without it)
  meets `git.*` events as unknown types in known task streams. Story 7 of
  the extensions plan already handles that: `tasks.apply` ignores them,
  `current` counts them in the unknown tally ("31 events of a type this
  build doesn't know: git.pipeline"), and nothing fails. A new proposal of
  one is refused by `CheckShape` ("unknown event type"). No feature code
  runs, so nothing can crash.
- Re-adding the feature replays every `git.*` event into the slices,
  including those the older build only counted.
- Each `git.*` event wakes the coordinator loop for its task
  (`touchedTasks` takes any event in a task stream). `Loop.act` works from
  state, so it starts nothing, but story 3 tests that: a pipeline event on
  a done task and on a working one starts no turn. The event does move the
  task's `LastActiveAt`, which is right: a pipeline result is news on the
  task.

## 2. What starts it

There is no `review.approved` in the log. A checking agent (a reviewer, a
tester: `Column: agents.Reviews`) ends its report with "Verdict: approved"
or "Verdict: changes needed", and `verdictOf` reads it from the
`turn.ended` report when the coordinator decides. `unchecked` (in
`checks.go`) refuses a done decision until every checker approved since the
last change. Nothing is written when the work becomes checked: it is worked
out from the turns each time.

The job doesn't need that moment. A request can't open until the branch is
on the host, and only the deployer's push puts it there. The deployer
(`Column: agents.Ships`) is handed the task only when the work is checked,
or when you moved the task to Shipping and so took it as it was
(`Turn.Shipped`). So the job reads what is already in the log:

- `task.assigned` to an agent in the Ships column, then its `turn.started`
  and `turn.ended`.
- In that turn, an `action.requested` that pushes to the task's branch,
  with `action.approved` and `action.result`.

`Task.Pushed(turn)` already exists, but it matches any approved, finished
command that starts with `git push`, whatever its target. Only its comment
and `deployer.md` talk about the exact command. So the feature reads the
target with `tasks.plainPush` (`internal/tasks/waiting.go`), the parser the
waiting line already uses, and the push qualifies only when the branch it
returns equals the task's `Worktree.Branch`. Story 7 exports that as
`Task.PushTo(turn, branch string) *Action`: the turn's approved, finished
push to the branch, or nil. `Pushed` stays as it is for the coordinator. A push `plainPush` can't read (a quote, an unknown option, a
bare `git push`) doesn't qualify: the deployer's instructions give the
exact command, `git push origin HEAD:agentry/task-N`.

Two kinds of push still look finished and pushed nothing:

- A dry run. `plainPushOption` accepts `-n` and `--dry-run`, which is
  right for the waiting line (it names where the push would go). So story
  7 splits the reading out: `readPush(command) (push, bool)`, where `push`
  holds the branch and `dryRun`, and `plainPush` becomes a wrapper that
  returns `push.branch`, so the waiting line reads as before. `PushTo`
  skips a push with `dryRun` set.
- A push that failed. An `Action` only has `HasResult`, and with Codex a
  failed push still gets one (the comment above `Task.Pushed`). The log
  can't tell, so the host does: before it opens or reopens, the job asks
  the host for the branch (section 3, step 6). If the branch isn't there,
  it writes `git.refused` with `status` 404 and the reason "the branch
  agentry/task-N isn't on GitLab" (or, when the token can't see the
  project at all, "the token can't see group/name on GitLab"), which the
  tab and the waiting entry show, with "Try again". A failed push onto a branch already there from
  an earlier push opens or keeps a request on the old head, which the tab
  shows by its sha; the next push updates it.

A push to dev, which every task shipped before this feature made, never
qualifies, so turning the feature on opens nothing for old tasks.

No core event is needed. A `task.checked` event, written by System when
`unchecked` turns "", would freeze in the log a rule that lives in code and
has changed before (the ship retry in `recordOf`), and it would still not
put the branch on the host. If a later change needs the moment the work is
checked (say, a draft request opened at review), that event is the minimal
addition: `task.checked {turn}`, System, written by the coordinator, with
no other change.

## 3. The job

`Feature.Jobs`, started by `features.StartJobs` under `recover`. It never
holds the store: it reads through `Core.Read` and `Core.Events`, and writes
through `Core.Log`, which refuses any type the feature didn't register.

`Core.Events` today has only `Stream(name)`. The job needs to follow the
log, as the loop does with `Store.NewReader`, so `features.EventLog` gains
`Follow(position int64) Follower` (`Next(ctx)`, `Close()`) and `Head()`,
and `ReadOnly` wraps them, still with no way to write.

What it does:

1. It reads its settings on each pass (`Settings` gives the version in
   force on every call; nothing tells the job the file changed). With no
   token or no project it does nothing that pass.
2. It looks at every task once (`Read.TaskIDs()`), then follows the log
   from `Head()`, and looks again at each task a batch touched.
3. A task has a new push when it is in the project (its
   `Worktree.Repo`'s origin names the project; the job reads
   `git remote get-url origin` there once per repo and keeps it), and its
   latest qualifying push (section 2) comes later in the log than the
   slice's `marked`, and later than its `refused` when that isn't 0. The
   marks are a `git.pr_opened`, the job's own `git.pr_changed` for the
   current request (step 4), and the outstanding `git.refused`. A
   `git.retry` clears the refusal, so the push before it counts again,
   unless a `git.pr_opened` or the job's own change came after that push;
   with no refusal outstanding the Rule refuses a `git.retry`. A polled or
   webhook `git.pr_changed` is never a mark, for any request: a late
   delivery of an old merge, or a poll that lands just after a new push,
   would otherwise stop that push from opening a request. It isn't needed
   as one either: the push that opened a request is already before its
   `git.pr_opened`. The push's position is its `action.result`'s. So a
   request you close or merge on the host is left alone until the deployer
   pushes again.
   The job finds the push's position and time in the task's stream.
   `Task.PushTo` gives the push from state, and the job reads
   `Core.Events.Stream(task)` for the `action.result` whose `Correlation`
   is that action's `ID`; the event carries both `Position` and `At`. It
   reads the stream only for a task in the project with a push to its
   branch, which no task shipped to dev has, and keeps what it found by
   action, so it reads each push once. The other way was to add
   `ResultPosition` and `ResultAt` to `tasks.Action`, set in
   `tasks.apply`. That changes replay for every task to serve one
   feature, and needs the real-log replay test, for a read the job can
   make by itself, so the job reads the stream and the core stays as it
   is.
4. A new push onto the current request, which the slice holds open, may
   be onto one you merged or closed on GitLab since the last poll, up to
   two minutes ago (below). So the job asks the host for that request
   (GitHub `GET /repos/{owner}/{repo}/pulls/{number}`, GitLab
   `GET /projects/{id}/merge_requests/{iid}`). Still open: the push only
   updated it, and the job writes nothing and remembers, in memory, that
   it checked that push; after a restart it asks once more. Merged or
   closed: it writes `git.pr_changed` with that state, the host's `at`,
   and `push` set to the push's position, and goes on to step 5 in the
   same pass. The slice counts that change as a mark just before `push`
   (at `push` - 1), not at its own position, so the push still comes
   after it: if the job stops
   between the two, the next pass goes on from step 5. It names the
   current request, the one the job asked about, so it is the one it
   changes. It is one more call per push, and it covers a change no poll
   or delivery has brought yet.
5. For a task with a new push and no open request, it asks the host for
   the most recent request from the branch, in any state (GitHub
   `GET /repos/{owner}/{repo}/pulls?head={owner}:{branch}&state=all&sort=created&direction=desc`,
   GitLab
   `GET /projects/{id}/merge_requests?source_branch={branch}&state=all&order_by=created_at&sort=desc`).
   If it is open, or was created after the push's `action.result`, it is
   one the job opened and didn't record (a crash between the host's reply
   and the write): it writes `git.pr_opened` with `found: true`, then a
   `git.pr_changed` with its state if it is closed or merged, and opens
   nothing.
6. Before it opens or reopens, it asks the host for the branch (GitHub
   `GET /repos/{owner}/{repo}/branches/{branch}`, GitLab
   `GET /projects/{id}/repository/branches/{branch}`). A 404 there means
   the push failed, or that the token can't see the project: GitHub
   answers 404 for both, and GitLab tells them apart only in its message
   ("404 Branch Not Found", "404 Project Not Found"). So on a 404 the job
   asks for the project (GitHub `GET /repos/{owner}/{repo}`, GitLab
   `GET /projects/{id}`). Found: it writes `git.refused` with the reason
   "the branch agentry/task-N isn't on GitLab". A 404 again: the reason is
   "the token can't see group/name on GitLab". Either way `action` is
   `reopen` when step 7 would reopen, else `open`, `status` is 404, and it
   opens nothing (section 2). Otherwise it goes on.
7. Otherwise, if the current request is closed and not merged, it
   reopens that one (GitHub `PATCH .../pulls/{n}` with `state: open`,
   GitLab `PUT .../merge_requests/{iid}` with `state_event: reopen`) and
   writes `git.pr_opened` for it.
8. Otherwise it opens one from the branch into `base` (dev by default),
   titled with the task's title, its body the task's link and its summary
   when it has one, and writes `git.pr_opened`.

Every host call has its own timeout, 30 seconds, through its context, so
a host that hangs can't hold the job, and a call that times out counts as
a network error (below).

Every host path is built from escaped parts: GitLab's `{id}` is the
project's path through `url.PathEscape` (`group%2Fname`), a branch in a
path is escaped the same way (`agentry%2Ftask-121`), and query values go
through `url.Values`. Numbers (`{iid}`, `{number}`) come from the slice's
ints.

Never two requests for one task:

- The slice's check refuses a second `git.pr_opened` while the current
  request is open. An event for an older request never makes the current
  one read closed (section 1).
- The job asks the host first (step 5), in every state, so a crash after
  the host opened the request and before `git.pr_opened` was written finds
  it on restart and records it, even if it was merged or closed meanwhile.
- There is one job, which handles one task at a time.
- A host that says the request exists (GitHub 422 "A pull request already
  exists", GitLab 409) is treated as step 5: look it up and record it.
- A merged request stays merged. If the task is reopened and shipped
  again, the new push opens a new request: one open at a time, never two.
  This holds between polls too: step 4 finds the merge on the host and
  records it before step 8 opens the new one.

When the host says no:

- No token, or no project: the feature is off (section 9). The job writes
  nothing, and the Settings page says how to turn it on.
- 401, 403, 404, a 422 that is not "already exists", or no branch (step
  6): `git.refused` with
  the status and the host's message, cut to 300 characters. The tab shows
  it with "Try again", and it waits for you (section 6). It isn't retried
  by itself: a bad token stays bad.
- A network error, a timeout, 429 or 5xx: nothing is written. It tries
  again after 1, 2, 4 and up to 30 minutes, or after the host's
  `Retry-After`, and logs one line per task and status, never the token or
  a header.

### Asking the host every two minutes

Agentry has no public address, so GitLab can't deliver a webhook to it.
Polling is how the feature learns what happened on the host. It is part of
the same job, not a second one, so it still handles one task at a time.
The job's loop calls the `Follower`'s `Next` with a context whose
deadline is the next poll. Events before then come back as a batch, as
now; at the deadline `Next` returns with the context's error, the job
polls, and calls `Next` again with the next deadline. The reader keeps its
position across those returns (`internal/store/reader.go`, `Next`), so no
event is missed or read twice.

- How often: every `poll` in the settings, two minutes when unset, never
  less than 30 seconds. The job reads the settings on each pass, so a new
  interval, a new token or a removed one applies at the next pass. No
  token, or no project: it makes no call to the host.
- Which requests: for each task in `TasksWith("git")`, its current request
  while it is open, or while it is merged or closed and the latest pipeline
  on its head is `pending` or `running` (`manual` is finished, section
  1). Once it is merged or closed and its last pipeline finished (or it
  has none), the job stops asking about
  it, until a new `git.pr_opened` makes another request current. Older
  requests aren't asked about. An archived task isn't in `TasksWith`.
- What it asks, for each: the request
  (`GET /projects/{id}/merge_requests/{iid}`: state, head `sha`,
  `updated_at`), its pipelines
  (`GET /projects/{id}/merge_requests/{iid}/pipelines`, the newest 20),
  and the jobs of each pipeline that is new to the slice, whose status
  differs from the slice's, or that is `pending` or `running` on the host
  (`GET /projects/{id}/pipelines/{pipeline_id}/jobs`, the latest try of
  each). Asking for a pending or running pipeline's jobs on every pass
  is what lets a job that finishes inside it show as a later `at`
  (section 1); a finished
  pipeline whose status hasn't changed isn't asked again. That is two
  calls per request per pass, and one more per such pipeline: 20 open
  requests, each with one running pipeline, make 60 calls every two
  minutes, and 40 once their pipelines finish.
- What it writes: by the comparison in section 1, against the slice. A
  state or head that differs from the request's record, with a later
  `updated_at`, is a `git.pr_changed` with that `updated_at` as `at`; a
  pipeline that is new, or whose status or jobs differ with a later `at`
  (section 1), is a `git.pipeline`. Nothing differs, or nothing later,
  nothing is written. `Store.Propose`
  applies a write before it returns, so the next comparison in the pass
  sees it. After a restart the slice is the log's, so the first pass
  writes nothing that was already written.
- A polled `git.pr_changed` is never a mark (step 3). So a merge the poll
  records doesn't stop a later push from opening a request, and a poll
  that lands just after a push doesn't either.
- When GitLab says no: a network error, a timeout, 429 or 5xx backs that
  request off as the job does (1, 2, 4, up to 30 minutes, or
  `Retry-After`), and the others are still asked. A 401 or 403 is the
  token, for every request: the whole poll backs off to 30 minutes, and the
  job logs one line when the status changes, never the token. It writes
  nothing, since there is no request action to refuse; the next open or
  reopen writes `git.refused`, which waits for you. A 404 for a request
  (deleted on the host) is logged, and that request is left until a new
  push.
- A retry inside one poll: a pipeline that is retried and fails again
  between two polls was never seen `running`, so the slice counts no new
  attempt. It keeps the same attempt number, and the waiting entry stays
  the one already there (its key is the same), with the new jobs if they
  differ. A webhook, later, would see the run and count it.
- What it keeps: nothing outside the log and the slice but its backoff
  timers, step 4's memory of the pushes it checked, and the pipelines
  whose unknown status it logged (section 1). A restart forgets
  them and asks at once, which is safe: an answer that changed nothing
  writes nothing.

## 4. How it fits with today's deployer

The deployer merges `origin/dev` into the task branch, runs the tests and
the build, pushes the branch to dev with `git push origin HEAD:dev` (the
push asks you first), and reports the pipeline once with `glab ci list` or
`gh run list`. The coordinator finishes the task once a ship turn pushed
(`recordOf`, `Task.Pushed`).

The options:

| Option | What happens | For | Against |
|---|---|---|---|
| A. The request replaces the push to dev | The deployer merges `origin/dev` in, tests, and pushes the task branch (`git push origin HEAD:agentry/task-N`), still asked first. The job opens the request; the host merges it | Dev only moves through a request and its pipeline; the host's checks and branch protection apply; no agent holds a token | Merging becomes a step on the host (yours, or the host's auto-merge) |
| B. The request beside the push | The deployer pushes the task branch, the job opens the request, then the deployer pushes dev as today, and the host marks the request merged | The request records the work and runs the branch pipeline | Two pushes and two approvals per task; the request is merged before anyone could look at it, so it adds a record but no check |
| C. The deployer merges through the request itself | The deployer runs `gh pr create` and `gh pr merge` (or `glab mr`) in its turn | No job, no webhook | The token sits in the agent's environment; state lives only in its report; nothing comes back after its turn ends |

Decided: A, for a repo the feature is on for, and today's push to dev
for every other repo (section 12). The deployer learns which through its
instructions: the feature's `Coordinator.Brief` adds a section for a coordinator whose
team ships ("This repo ships through merge requests: tell the deployer to
push the task branch, not dev"), and `deployer.md` gains the matching
paragraph. The push is still asked of you, and `Task.Pushed` still counts
it, so the coordinator's "pushed, so it can be done" rule is unchanged.

The task is done when the request is open (section 12, 5). How it works:
the coordinator finishes the task at the push (`recordOf`,
`Task.Pushed`), and the job opens the request in its next pass, seconds
later. So for that moment the task is done with no request, and if GitLab
refuses to open it (a bad token, no access, the branch not there), it
stays that way:

- the job writes `git.refused`, which waits for you (section 6) with
  GitLab's status and message;
- the Merge request tab shows the refusal with "Try again", which writes
  `git.retry` once you have fixed the cause, and the job tries at its next
  pass (section 3, step 3);
- the task stays done. It isn't reopened and no agent is started: the
  work was checked and pushed, and only opening the request is left, which
  is the job's, not an agent's.

The deployer's pipeline report stays as it is: it is one line in its
report, and the feature's events replace it as the record.

## 5. The show_pipeline tool

An MCP tool in `agentry mcp`, from the feature's `Tools`, listed while the
feature is enabled or visible, as Ideas' tools are.

- Input: `{ "task": "task-121" }`. Required.
- Output, as text: the request (`!412 open: https://...`, or "no merge
  request yet" with why: off, not shipped, refused), then the latest
  pipeline on its head (`failed 3 minutes ago, 4f2c1a9: https://...`),
  then its jobs, failed first (`test: failed https://...`). When the
  feature is off it says so and how to turn it on.
- Data: `GET /api/git/tasks/{task}` (a UI route), which reads the task's
  slice. It reads the log, not the host: what the job's last poll (or,
  later, a webhook) wrote, at most one poll old. It makes no call to the
  host, so the tool is fast and needs no token in `agentry mcp`.

## 6. A waiting entry when a pipeline fails

The feature's `Waiting`, read over `GET /api/git/waiting` until the
server lists what waits (extensions story 13), then worked out in the
server from the slices.

- Kind: `git`.
- Line: `task-121's pipeline failed on agentry/task-121: test, lint
  (https://...)`. For a refusal: `task-121: GitLab refused to open the merge
  request (401): <its message>`.
- Key: `git/<task>/pipeline/<id>/<attempt>` for a failure (for GitLab,
  the attempt the slice counts, section 1), so a new pipeline or a retried
  one that fails again is news, and the same failure
  delivered twice is not. `git/<task>/refused/<position>` for a refusal.
- It clears when a later pipeline on the current request's head is
  anything but failed (a retry that is running counts), when the current
  request is merged or closed, or when the task is archived. A refusal clears on the next
  `git.pr_opened` or `git.retry`.

Only the latest pipeline on the current request's head counts: a failure
on an older head, since replaced, or on an older request, waits for no
one. Likewise only the current request's merge or close clears it.

## 7. The Merge request tab

The extensions note's `taskTabs` slot, added with this feature:

```ts
// ui/src/features/index.ts
export interface TaskTab {
  id: string; // in the URL's ?tab=; the core's TABS and other features' ids may not repeat it
  label: string;
  element: ComponentType<{ task: TaskDetail }>;
}

export interface FeatureUI {
  // ...as now
  taskTabs?: TaskTab[]; // tabs on the task page, for a task the feature holds a slice for
}
```

`TaskPage.tsx` appends the tabs of each visible feature whose name is in
the task's `slices`, after the core's last tab, Details. `TabId` widens to the core's ids plus
feature tab ids. A test checks that no feature tab id repeats a core one.

`ui/src/features/git/` brings one tab, `merge-request` ("Merge request"),
not "Shipping", which is the name of a board column:

- The current request: its number and link, its state (open, merged, closed) and
  base, and `found` as "found open on GitLab" when the job recorded one it
  didn't open.
- The latest pipeline on its head: status, link, short sha, when, and its
  jobs with their status, failed first. A `manual` pipeline shows as
  "waiting on a manual job", naming the manual jobs, not as passed or
  skipped (section 1).
- A refusal, with the host's message and "Try again" (`POST
  /api/git/tasks/{task}/retry`, a UI route that writes `git.retry` as you).

The tab shows when the manifest lists `git` as visible and the task has a
git slice. The manifest's entry:

- `enabled`: a token, a host and a project are in `features.toml`.
- `visible`: enabled, or any task holds a git slice (`TasksWith("git")`),
  so the history stays readable after the token is removed.
- `why`: "opens merge requests on gitlab.com for group/name and checks
  them every 2 minutes", naming the host of `api` instead of gitlab.com
  for a self-hosted GitLab ("on gitlab.example.com"), or "add a
  token to features.toml to open merge requests", or "kept while 12 tasks
  have merge requests; no token is set".

## 8. The webhooks (later, optional)

Not built now: Agentry has no public address (section 12, 4). This is for
when it has an https one. A webhook writes exactly what polling writes, by
the same comparison (section 1), so both can run together: whichever sees
a change first writes it, and the other then finds nothing changed. It
only brings the news sooner than the next poll. The GitLab route is story
12; the GitHub one comes with GitHub, story 13.

Two routes, both `features.Signed(verify)`, so the core's `signed` wrapper
reads the body (up to `httpapi.MaxBody`, 1 MB), runs `verify`, and answers
401 with nothing written when it refuses. No token opens them.

| Route | Host | Events it takes |
|---|---|---|
| `POST /api/git/hooks/github` | GitHub | `pull_request`, `workflow_run`, `workflow_job`, `ping` |
| `POST /api/git/hooks/gitlab` | GitLab | `Merge Request Hook`, `Pipeline Hook` |

Each `verify` reads the secret from the settings file in force, at the
time of the request, so a changed secret applies at once. No secret set
means every request is refused: a missing secret never opens a route.

- GitHub signs the body. `X-Hub-Signature-256` is `sha256=` and the hex
  HMAC-SHA256 of the body with the secret. `verify` decodes the hex,
  computes the HMAC over the bytes it was given, and compares with
  `hmac.Equal`. A missing or malformed header is refused. The older
  `X-Hub-Signature` (SHA-1) is ignored.
- GitLab doesn't sign the body. It sends the secret itself in
  `X-Gitlab-Token`. `verify` compares the header with the secret using
  `subtle.ConstantTimeCompare` and ignores the body. That proves only that
  the sender knows the secret, so it is as safe as the channel: the route
  needs an https public address, and the Settings page says the GitLab hook
  is off without one.

What a verified delivery writes, always as System (`httpapi.System`):

- Its project: GitHub's `repository.full_name` or GitLab's
  `project.path_with_namespace` must equal the configured `project`
  (GitHub's ignoring case, as GitHub treats names). Otherwise nothing is
  written: an organisation's or group's hook, or a second project with the
  same `agentry/task-N` branch names, can't write into a task here.
- Its task: the branch (`pull_request.head.ref`, `workflow_run.head_branch`,
  GitLab's `object_attributes.source_branch` or `ref`, or a merge request
  pipeline's `merge_request.source_branch`) must be `agentry/task-N`, and
  task N's `Worktree.Branch` must equal it. Otherwise nothing is written.
- `pull_request` and `Merge Request Hook`: the event names its request
  (`pull_request.number`, GitLab's `object_attributes.iid`). The handler
  compares against that request in the slice, not the current one, and
  writes `git.pr_changed` by the comparison in section 1 (closed with
  `merged: true` is `merged`). A number the slice doesn't hold
  (a request someone opened by hand from the branch) writes nothing: 202,
  "not a request of task-121". One for an older request is written and
  changes only that request's record (section 1).
- `workflow_run` and `Pipeline Hook`: `git.pipeline`, with GitLab's
  `builds` as `jobs`. A Pipeline Hook's builds carry no `web_url`, so each
  job's `url` is the hook's `project.web_url` + `/-/jobs/<id>`, the form
  of the `web_url` the polled jobs carry: a poll after a delivery finds
  the same values and writes nothing. `workflow_job`: a `git.pipeline` for its run with one
  job, merged by name.
- Its `at`. A Merge Request Hook has `object_attributes.updated_at`, as
  the polled request has, and GitHub's events carry `updated_at`. GitLab's
  Pipeline Hook has none, so its `at` is `object_attributes.finished_at`,
  else `started_at`, else `created_at`, which every Pipeline Hook carries,
  so Agentry's clock never enters the slice. All three are GitLab's clock, as polling's `updated_at` is, and a
  pipeline's `updated_at` is never earlier than them. So a delivery of a
  status the poll already wrote is not later and writes nothing, and a
  poll of a status a delivery wrote is equal and writes nothing. A
  delivery whose `at` isn't later than the slice's (a retried pipeline
  running again can carry an old `created_at`) writes nothing, and the
  next poll brings that change with its `updated_at`: the webhook only
  brings news sooner, never different news.
- `ping`, and any other event: nothing.

Replay and duplicate deliveries:

- Each event carries `delivery`: GitHub's `X-GitHub-Delivery`, GitLab's
  `X-Gitlab-Event-UUID`. A delivery already in the task's slice is
  answered 200 and not written again. Two copies racing past that check
  write the same state twice, which changes nothing: each event sets
  state, none counts, and the second's `at` is not later than the
  first's.
- A delivery that arrives late, or an old one sent again, carries its own
  `at`, and the slice keeps the newest per pipeline, so it can't undo a
  later status.
- Replaying the log replays these events like any other: nothing calls the
  host on replay.

What is refused, and the answer:

- A bad or missing signature or token: 401, from the wrapper, nothing
  written.
- A body over 1 MB, or a verified body that isn't JSON: 202 with a
  one-line reason, nothing written. The sender can't fix either, and a 4xx
  counts toward GitLab turning the hook off. The core's `signed` wrapper
  answers 400 for an over-long body today, before `verify` runs, so story
  12 changes it to answer 202 for that on every signed route; the handler
  answers 202 for a body it can't decode. GitHub's deliveries for these
  events are well under 1 MB.
- An event for another project, no task, a branch that isn't a task's, an archived task
  (the rules refuse a write into it), an event the feature doesn't take:
  202 with a one-line reason, nothing written. These are 2xx on purpose:
  GitLab turns a hook off after it fails a few times in a row, and none of
  these is the sender's fault.

## 9. Settings and secrets

In `features.toml`, beside `policy.toml`, read by `internal/settingsfile`.
`cmd/agentry/policy.go` already lists the file as protected, so no agent can read or
change it. Nothing in it reaches the log, a snapshot, the page or the API.

```toml
[git]
host = "gitlab"            # "github" once story 13 lands
project = "group/name"     # GitHub: "owner/repo"
base = "dev"               # the branch requests go into; dev when unset
api = "https://gitlab.example.com/api/v4"  # only for a self-hosted host
poll = "2m"                # how often the job asks the host; 2m when unset, 30s at least

[git.secrets]
token = "..."              # sent as PRIVATE-TOKEN (GitLab) or a Bearer token (GitHub)
webhook = "..."            # optional, only for the webhook (section 8, later)
```

- On: `host`, `project` and `token` are set. That is enough for all of
  it: the job opens requests, and polling brings back their state and
  pipelines. The webhook secret is optional and only matters once the
  webhook is built and Agentry has a public address; without one the
  webhook route refuses every request (section 8) and polling carries on
  alone.
- The job, its polling, the routes' `verify`, the manifest and the
  coordinator brief read the table in force on every call, so an edit
  applies without a restart.
- Today only jobs get `Core.Settings` (`startJobs`). `Manifest` is
  `func(Reader) Entry` and `Coordinator.Brief(state, task)` has no `Core`,
  so neither can reach them that way. The design: the feature is built
  with its settings, `githost.Feature(settings features.Settings)`, and its
  `Manifest`, `Brief`, routes' `verify` and job read them from the
  closure. Story 2 makes that possible for any feature:
  `installedFeatures(settings func(feature string) features.Settings)`.
  `serve.go` opens `features.toml` once (`settingsfile.Open`, moved out of
  `startJobs`) and passes its `For`; `cmd/agentry/mcp.go` and
  `cmd/agentry/claudecode.go`, which also call `installedFeatures`, pass
  one that gives the zero `Settings`. They need none: they use only
  `Tools`, `Instructions` and `Waiting`, which read the server's API.
- While the feature is off, the Settings page lists it (it is installed
  and not visible) with its `why` and its `turnOn`: the TOML above, with
  the token's scopes (section 10), and the path of the file. The page
  never takes the token: you put it in the file yourself.

## 10. Security

- The token's scope. GitLab: a project access token with the Developer
  role and the `api` scope (`read_api` can't open a merge request; a
  project token keeps `api` to the one project). GitHub, from story 13: a
  fine-grained token for the one repository, with Pull requests read and
  write, Contents read and Actions read. The token can open and reopen
  requests and read pipelines; you merge on the host (section 12, 3).
- Only the server uses the token. It is never in an agent's environment,
  so an agent's `gh` or `glab`, which the deployer still runs for its
  pipeline line, uses the machine's own sign-in, as today.
- A forged webhook writes nothing: `signed` refuses it before the handler
  runs. A genuine delivery can only set the state of a task whose branch
  it names, and only to what the host says.
- No secrets in logs or snapshots: secrets are only in `features.toml`
  and the server's memory. `settingsfile` already never logs the file's
  content. The job's log lines name the host, the task and the status.
  The webhook's lines name the event and the delivery, never a header. The
  slices hold only the event fields above. Story 5 adds a test that writes
  a known token and secret and finds neither in the log, the snapshot or
  the server's output; stories 7, 9, 12 and 13 extend it to the job,
  polling and the webhooks, run against fakes.
- The `api` address is the file's, which only you can change, so the job
  and its polling call no address an agent or a webhook chose. Polling
  asks only about the configured project and the request numbers in the
  slice.

## 11. Build stories

In build order. Each is one reviewable change. Stories 1 to 11 are GitLab
only, which keeps each of them smaller: the types and the tab already name
both hosts, so GitHub adds no type. Stories 12 and 13 come later: the
webhook only once Agentry has a public https address, and GitHub once the
GitLab path works. Three stories change replay or apply (1, 3 and 5); each
of those, before it lands, runs the real-log replay test
(`internal/current/real_log_test.go`) on a fresh read-only `VACUUM INTO`
copy of the live log. The rest, polling included, only write events of the
types story 5 registers, through `Core.Log`, and need no replay test. The
file lists come from grepping the real callers.

1. **Task slices in current and the snapshot.** `features.TaskSlice`,
   `features.Rule` (the type only), `Feature.TaskSlice`,
   `events.Namespace`; `current.NewFor`, `RestoreFor` and
   `EqualThroughSnapshotsFor` take `...features.Feature`, and `New`,
   `Restore` and `EqualThroughSnapshots` become wrappers over them, so
   their callers don't change (section 1); `fold`, `Apply`, `archive` and
   `Equal` keep a slice per task and feature, with `brokenSlices`;
   `savedState.Sliced` and Resume's `foldSlices`; `Reader.TaskSlice` and
   `TasksWith`; `slices` on `GET /api/tasks/{task}`.
   Files: `internal/features/features.go`, `internal/events/catalogue.go`,
   `internal/current` (`current.go`, `equal.go`, `snapshot.go`,
   `resume.go`, `startup_bench_test.go`, and a new `slices_test.go`),
   `internal/storetest/storetest.go` (`OpenWith` calls `NewFor` with
   `ideas.Feature()`), `internal/web/api.go` and `views.go` (`showTask`'s
   view), `ui/src/api/types.ts`. `current.State` is the only `Reader`
   outside tests; the test fakes (`ideas/feature_test.go`'s `board`) embed
   the interface and still build. `serve.go` doesn't change here: it moves
   to `RestoreFor` in story 2.
   Done: a test feature's task types replay to the same slices as `Apply`
   (the equivalence test, and `EqualThroughSnapshotsFor` through every
   cut); a state resumed from a snapshot holds the same slices as a whole
   replay; without the feature, the task is the same and the tally counts
   them; a panic in its `Next` breaks only that slice, and the task still
   reads; archiving drops the slices from `TasksWith` and unarchiving
   brings them back; the startup benchmark runs with slices; the existing
   callers of `New`, `Restore` and `EqualThroughSnapshots` build and pass
   unchanged. It changes replay and apply, so it runs the real-log replay
   test first. After: none.
2. **Settings reach the features list.** `installedFeatures(settings)`,
   and `serve.go` reordered so the features exist before the state: it
   opens `features.toml` once (`settingsfile.Open`, moved out of
   `startJobs`) and starts its `Watch`, builds
   `installedFeatures(settings.For)` before the state, calls
   `current.RestoreFor(snapshot, build, catalogue, installed...)` in place
   of `Restore(..., ideas.Streams())`, and gives `startJobs` that same
   `*settingsfile.Live` instead of a path. `agentry mcp` and Claude Code's
   hooks pass a `settings` that gives the zero `Settings`. Files:
   `cmd/agentry/serve.go` (the `Restore` call, `installedFeatures`,
   `startJobs`), `cmd/agentry/jobs_test.go` (calls `startJobs` with a
   path), `cmd/agentry/mcp.go`, `cmd/agentry/claudecode.go` (five calls),
   `internal/features/settings.go`. Done: a test feature's manifest and
   route read a value from the file and see an edit without a restart;
   the job and the manifest read the same `Live` (one file watcher, not
   two); a secret in the file is not in `GET /api/features`;
   `agentry mcp` starts with no settings file. It doesn't change replay or
   apply: the features `RestoreFor` gets hold the same `Streams` as before
   and no slice yet, so it needs no replay test. After: 1.
3. **Feature task rules in rules.Check.** `rules.Check(p, state,
   catalogue, installed)` runs a feature's `Rule` for its task types
   before the core's task rules, and lets them into a done task, never an
   archived one; a broken slice refuses with "could not read the git
   slice: ..."; a panic in a `Rule` refuses with "its rule failed: ...";
   the nil check on `taskRules`. Files: `internal/rules/rules.go` and its
   tests; the callers: `cmd/agentry/serve.go` (passes the `installed`
   that story 2 builds before `store.Open`), `internal/storetest/storetest.go`,
   `internal/coordinator/epics_test.go`, `internal/watchdog/watchdog_test.go`,
   `internal/web/api_test.go`, `internal/web/readmodel_bench_test.go`.
   Two of those pass `rules.Check` as a value, and the new parameter
   changes its type: `splittingWith(t, rules.Check, ...)` in
   `epics_test.go` and `limitsServerChecked(t, rules.Check)` in
   `api_test.go`, so both helpers' parameter types change with it. Done:
   tests that a test feature's `Rule` refuses with its reason, runs under
   `Store.Propose`'s lock, allows when it is nil, lets its type into a
   done task, and refuses it in an archived one and in a task that doesn't
   exist; a `Rule` that panics refuses with "its rule failed: ..." and
   `Propose` returns; a broken slice refuses with "could not read the git
   slice: ..." and never calls the `Rule`; a core task type with no
   `taskRules` entry no longer panics; a test event on a done task and on
   a working one starts no turn. It changes which events the log accepts
   and so what replay and apply meet, so it runs the real-log replay test
   first. After: 1, 2.
4. **Jobs follow the log.** `features.EventLog` gains `Follow` and `Head`.
   `ReadOnly` takes the `*store.Store` and makes them from
   `Store.NewReader` and `LastPosition`, so the store gains no method and
   doesn't import `features` (`features` imports `store`, which imports
   only `events`: no cycle). Files: `internal/features/features.go`, and
   the three callers of `ReadOnly`, which already pass the store:
   `cmd/agentry/serve.go`, `internal/coordinator/loop.go`,
   `internal/web/server.go`. The fakes in `internal/ideas`
   (`accept.go`, `routes.go`, `ideas_test.go`) embed `EventLog` and still
   build. Done: a job sees events written after it started, and a type
   assertion on its `Follower` gets no store. After: none.
5. **The git feature: types, slice, rule and manifest.** `internal/githost`
   with the five types (their `Spec`s in `events.go`, as
   `internal/ideas/events.go` has Ideas'), the slice with its `marked` and
   `refused` positions, the `at` rule and the comparison of section 1
   (`githost.changes`, which polling and the webhook share), its `Rule`
   (one open request, known numbers, no stray retry), the manifest (on for
   `host = "gitlab"`; `"github"` reads as off, "GitHub comes in a later
   story", until story 13), and `ui/src/features/git` with `turnOn`,
   listed in `FEATURES` (`ui/src/features/index.ts`), then the UI rebuilt
   into `internal/web/dist`; a fixture log under `testdata`. The wiring in
   `cmd/agentry/serve.go`: `githost.Registration()` in `NewCatalogue`,
   `githost.Feature(settings("git"))` in `installedFeatures`, and its
   `features.Checked(feature, githost.Registration(), log)` proposer in the
   `logs` map (without it, the feature gets `features.Unwritable`). Done:
   the catalogue builds with the feature; the rule refuses a second
   `git.pr_opened`, and allows one after a `git.pr_changed` to `merged`;
   it refuses a `git.pr_changed` for a number the slice doesn't hold;
   a job-written `git.pr_changed` marks just before its `push`; a
   `git.pr_changed` for an older number leaves the current request open
   and adds no mark; a late merge of the old request, after a push, doesn't
   stop that push from opening a request; a `git.pr_changed` whose `at`
   isn't later than its request's (a late `open` after `merged`, a late
   `closed` after a reopen) changes nothing; a `git.retry` with no refusal
   outstanding is refused, and one in a fixture log leaves a closed request
   closed and its old push not due; a `git.retry` after a refusal makes the
   push before it due again, falling back to `marked`; a GitLab pipeline
   that goes from failed back to running is attempt 2, and one that goes
   from `manual` to `running` keeps the same attempt; `changes` finds
   nothing for an answer equal to the slice, nor for one that differs with
   an `at` that isn't later; a pipeline on an old
   request's head doesn't show on the current one; a build without the
   feature replays the fixture with the tally counting it; the Settings
   page shows Git with its `turnOn` while off; the no-secrets test
   (section 10). It registers new types that apply reads, so it runs the
   real-log replay test first. After: 2, 3.
6. **The GitLab client.** Over `net/http`: find the most recent request by
   branch in any state, get one by number, get a branch, get the project,
   open, reopen, a request's pipelines, a pipeline's jobs. Files:
   `internal/githost/gitlab.go`. Done: tests against `httptest` fakes for
   opened, found (open, merged and closed), one got by number in each
   state, a branch there and not there, a project not there (404), "already
   exists" (409) as found, a request's pipelines and a pipeline's jobs (the
   latest try of each), the project and branch escaped in the path
   (`group%2Fname`, `agentry%2Ftask-1`), 401, 404, 429 with `Retry-After`,
   and 5xx; every GitLab pipeline and job status maps as section 1's table
   says, and a status it doesn't name, for the pipeline or any of its
   jobs, gives no pipeline; each call ends at
   its timeout against a fake that never answers. After: none.
7. **The job (GitLab).** `readPush`, with `plainPush` as its wrapper, and
   `Task.PushTo` over it, skipping dry runs; the push's position and
   time read from `Core.Events.Stream`; the check of an open request on a
   new push; `due` by log position, look up, the branch check with the
   project check after a 404, record, reopen or open, refusals, backoff.
   Files: `internal/tasks/waiting.go` (`readPush`),
   `internal/tasks/task.go` (`PushTo`, a read of state: `apply` doesn't
   change), their tests, `internal/githost/job.go`. Done: tests with a fake
   GitLab: one request per task across a restart between the host's reply
   and the write, also when the request was merged or closed before the
   restart; a request closed on the host isn't reopened until a new push;
   a push onto an open request opens nothing; before any poll, a request
   merged on the host and a task shipped again writes `git.pr_changed`
   merged and opens a new request, also across a restart between the two;
   a push to dev, a push `plainPush` can't read, and a dry-run push (`-n`,
   `--dry-run`) open nothing; the waiting line for a dry-run push reads as
   before; a push whose branch isn't on GitLab writes one `git.refused`
   with "the branch agentry/task-N isn't on GitLab", and one for a project
   the token can't see writes "the token can't see group/name on GitLab",
   both opening nothing; an old log opens nothing; no token writes
   nothing; a 401 writes one `git.refused` and no more until `git.retry`;
   an edit to the settings file reaches the next pass; each push's stream
   is read once; the no-secrets test (section 10) runs the job against
   the fake and finds the token in no event, log line or output. It changes no replay or apply, so it needs no replay
   test. After: 4, 5, 6.
8. **Read route, show_pipeline and the waiting entry.**
   `GET /api/git/tasks/{task}`, `GET /api/git/waiting`, the retry route,
   the tool and `Waiting`. Files: `internal/githost/routes.go`, `tools.go`.
   Done: tool tests on a fixture (request and failed pipeline, refusal, no
   request, feature off, a `manual` pipeline printed as waiting on a
   manual job); a failed pipeline makes one entry, a retry that fails
   again makes a new one, a later success clears it, and so does a later
   `manual` pipeline; the retry
   route with no refusal outstanding writes nothing. After: 5.
9. **Polling GitLab.** The job's timer: every `poll` (two minutes when
   unset), for each task whose current request is open, or merged or
   closed with an unfinished last pipeline, it asks for the request, its
   pipelines and the jobs of new, changed, pending and running
   pipelines, and writes by `changes`;
   backoff per request, and the 30-minute backoff on 401 and 403 (section
   3). Files: `internal/githost/poll.go`, `poll_test.go`, and
   `internal/githost/job.go` (the timer in its loop). Done: tests with a
   fake GitLab: a merged request found by polling writes one
   `git.pr_changed`, which is not a mark (a push after it still opens a
   request); a failed pipeline found by polling writes `git.pipeline`, and
   the waiting entry appears; a job that finishes while its pipeline
   stays `running` writes one `git.pipeline` with a later `at`, and the
   next pass with nothing new writes nothing; a finished pipeline whose
   status hasn't changed gets no jobs call; a job with a status the table
   doesn't name writes nothing for its pipeline and logs one line over two
   passes; a second poll with nothing changed writes
   nothing; a restart writes no duplicates; a merge step 4 already
   recorded isn't written again; polling stops for a merged or closed
   request whose pipelines finished (no call for it on the next pass); no
   token means no host calls; a 429 backs off that request only; a
   401 or a 403 backs the whole poll off to 30 minutes, with one log line,
   and makes no call for any request until then; a `poll` under 30 seconds
   is held at 30; a merged request whose pipeline waits on a manual job
   stops being polled; a retry that fails again between two polls keeps
   the same attempt; an edit to `poll` applies at the next pass; the
   no-secrets test (section 10) runs polling against the fake. It writes only story 5's types
   through `Core.Log` and changes no apply, so it needs no replay test.
   After: 7, 8.
10. **The deployer ships through the request.** The feature's
    `Coordinator.Brief` section and the paragraph in `deployer.md`. Files:
    `internal/githost/coordinator.go`, `internal/agents/defaults/deployer.md`.
    Done: a ship brief for a repo the feature is on for says to push the
    task branch, and one for any other repo doesn't; `Task.Pushed` counts
    the branch push; `internal/agents/prompts_policy_test.go` passes with
    the new command. After: 7, 9, so a shipped task's pipelines come back
    from the first request on.
11. **The taskTabs slot and the Merge request tab.** `TaskTab`,
    `FeatureUI.taskTabs`, `TaskPage.tsx`, the tab component. Files:
    `ui/src/features/index.ts`, `ui/src/pages/TaskPage.tsx` (`TABS`,
    `TabId`), `ui/src/features/git/`, and `internal/web/dist` rebuilt.
    Done: `npm run lint` and `npm run build` clean; render tests for a
    merged request with a passed pipeline, a failed one, a `manual` one
    shown as "waiting on a manual job", a refusal with "Try again", and a task with no git slice (no tab); the feature's tab
    comes after Details; a feature tab id equal to a core one fails a
    test. After: 1, 8.
12. **The GitLab webhook (later, optional).** Only once Agentry has a
    public https address. `POST /api/git/hooks/gitlab`, its `verify`, the
    project and task checks, `changes`, duplicates and refusals; the
    core's `signed` answers 202 for a body over 1 MB. Files:
    `internal/githost/hooks.go`, `internal/web/server.go` (`signed`) and
    its tests. Done: tests for GitLab's token right and wrong, and no
    secret set (refused); another project with the same branch name, a
    branch that is no task's, an archived task, a body that isn't JSON and
    one over 1 MB (each 202, nothing written); the same delivery twice
    (written once); an older status after a newer (the newer stays); a
    merge request event for an older request writes nothing that changes
    the current one; one for a number the task never had writes nothing;
    a change the poll already wrote writes nothing, and a poll after a
    delivery writes nothing; a Pipeline Hook's `at` comes from
    `finished_at`, else `started_at`, else `created_at`; the no-secrets
    test (section 10) sends deliveries with the secret and finds it in no
    event, log line or output. After: 9.
13. **GitHub (later).** Once the GitLab path works.
    `internal/githost/github.go`: story 6's calls over GitHub's API (pulls,
    branches, the repo, and for polling the workflow runs on a request's
    head and their jobs), "already exists" as GitHub's 422; `host =
    "github"` turns the feature on, and the brief, the manifest and the
    Settings page name GitHub. If story 12 has landed, also
    `POST /api/git/hooks/github` with its `X-Hub-Signature-256` verify and
    `pull_request`, `workflow_run`, `workflow_job` and `ping`. Files:
    `internal/githost/github.go`, `hooks.go`, the manifest. Done: story
    6's client tests and story 9's polling tests against a GitHub fake; a
    404 on the branch with the repo visible and with it not; for the route,
    a good signature, a bad one and a missing one; the no-secrets test
    (section 10) runs the GitHub client, and the route if built, against
    the fake. It changes no replay or apply. After: 9, 10, and 12 for the
    route.

## 12. Decisions

Your answers to the open questions:

1. **Shipping:** A. The request replaces the deployer's push to dev, for
   the repos the feature is on for; every other repo ships to dev as today
   (section 4).
2. **Host:** GitLab first. Stories 1 to 11 are GitLab only; GitHub is
   story 13, its client and its webhook's `verify`, once the GitLab path
   works.
3. **Merging:** you merge on the host. Auto-merge when the pipeline passes
   could be a later setting (`merge = "auto"`); no story is planned for it.
4. **Reaching Agentry:** there is no public address, so polling (section
   3) is how the feature learns about the host. The webhook (section 8) is
   later and optional, for when Agentry has a public https address.
5. **Done:** a task is done when its request opens, as today it is done
   once pushed. The tab and the waiting entry follow the merge and the
   pipeline after that, and you reopen the task if the pipeline fails.
   In practice the task is done at the push and the request opens just
   after it. If GitLab refuses to open it, the task stays done with no
   request: `git.refused` waits for you, and the tab's "Try again"
   (`git.retry`) has the job try again (section 4).

PLAN.md's T-019 row also names notification and email extensions. They are
out of this note: the waiting entry reaches Claude Code now, and Web Push
(extensions phase 3) once it lands.
