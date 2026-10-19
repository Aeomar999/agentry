# Schedules: tasks Agentry makes on a clock

Status: proposed, for task-123 (story 21 of `docs/design/extensions.md`
section 11, phase 6). Design only: nothing in the server or the page
changes with this document. It covers what a schedule is, its events, its
state, its rules, the job that fires it, its routes, tools and page, how a
task links back to it, what happens when a run waits on you, the open
questions and the build stories.

Checked against the code at `e54470b` (branch `agentry/task-123`, from
`dev`).

## In short

A schedule is a task you would otherwise write again and again: "continue
Portside" every weekday at 02:00, "a Visionaries pass on agentry" every
Monday at 09:00. It names the task (title, description, repo, team) and
when it runs (daily, weekdays, or some days of the week, at a time, in a
time zone), and it is on or paused.

Schedules are the second feature after Ideas, built the same way: a Go
package, `internal/schedules`, and a folder, `ui/src/features/schedules`.
It owns the `schedule-` streams and seven event types, keeps a slice of
`current.State`, adds rules, routes, MCP tools and a page, and is the first
feature with a job. The job is the clock: once a minute it reads the state,
works out which schedules are due, and for each makes the task as Agentry
(`task.created` by `System`, `"from": "schedule-N"`), then records the run
(`schedule.fired`). The task is made first, as an accepted idea's is, so a
crash in between leaves a task the next wake finds and records, never a
recorded run with no task.

Three things decide how safe an unattended run is:

- A schedule never piles up. While a task from it is open, a due run is
  skipped and says why (`schedule.skipped`), so a run that stalls on an
  approval overnight blocks only the next runs of its own schedule. A rule
  refuses the clock's task while one from the schedule is open, so the
  clock makes a task only when no task from the schedule is open, even if
  two firings race. Run now is yours to ask for and isn't held back (open
  question 7), and nor is reopening an old task from the schedule
  (`internal/rules/task_rules.go:29` checks only that it is done).
- Missed runs catch up once. A machine that was off or asleep runs the
  latest missed slot when it wakes, however old, and counts the rest; it
  never runs a week of nights back to back.
- `System` is already among `task.created`'s writers
  (`internal/events/catalogue.go:134`), but the rules let Agentry make a
  task only about an epic you closed or from an idea it may accept
  (`internal/rules/rules.go:91`). Schedules add a third case, for a
  schedule that is on and due, and nothing wider.

An unattended run that needs an approval waits as any task does: the gate
holds the action after 30 minutes and the task stays in Needs you. Until
Web Push sends (phase 3, story 15, not built yet) you learn of it only
from the page or Claude Code. That is why schedules came last.

## 1. What a schedule is

From your side, a schedule is a line on the Schedules page:

> **Continue Portside** · weekdays at 02:00 (Europe/London) · on
> next: Tue 20 Oct 02:00 · last: Mon 19 Oct 02:00, made task-412 (done)

What it holds:

| Field | Meaning | Example |
|---|---|---|
| `title` | the task's title | "Continue Portside" |
| `description` | the task's description, what the coordinator reads | "Pick the next story from PLAN.md and do it." |
| `repo` | the repo the task works in, as a new task takes it | `/home/you/portside` |
| `team` | whose coordinator gets the task; `""` for the first team | "Software" |
| `every` | `day`, `weekdays` or `week` | `weekdays` |
| `days` | for `week`: which days | `["mon"]` |
| `at` | the local time, `HH:MM`, 24-hour | `02:00` |
| `zone` | an IANA time zone, read with `time.LoadLocation` | `Europe/London` |
| `start` | true: the task starts at once; false: it waits in your Backlog | true |
| `budget`, `maxTurns` | the task's limits; absent takes the server's defaults (`features.Core.Defaults`) | 200000 |
| on or paused | a paused schedule makes nothing | on |

A run is one firing: the slot it was for (the wall-clock time it was due,
in its zone), the task it made, or why it was skipped.

The simple form is the whole rule in this version: no cron string. It
covers both examples in `extensions.md` section 13 and every schedule the
Visionaries named. A cron rule can come later as an optional field
(`"cron"`, with `every: "cron"`), which an old binary's state would still
read as a schedule it can't compute and so never fires; see open question
1.

What a schedule is not: it is not a recurring epic, and it does not carry
state from one run to the next. Each run is a fresh task; continuity lives
where it already lives, in the repo (a `PLAN.md`, the git history) and in
the agents' knowledge files.

## 2. Streams and event types

Each schedule is one stream, `schedule-N`, named by `Store.Create`
(`internal/store/store.go:220`), which counts the `schedule.created`
events under the store's lock, so two made at once never share a name.
The prefix passes `events.NewCatalogue`'s checks
(`internal/events/catalogue.go:77`): it doesn't overlap `task-`, a core
stream or `idea-`.

The types, registered by `schedules.Registration()` as Ideas registers its
own (`internal/ideas/events.go:25`). `Fields` are required, so
`CheckShape` (`internal/events/catalogue.go:279`) refuses a proposal
without them; the rest are optional. Writers are checked by `CheckWriter`
(`catalogue.go:321`).

| Type | Writers | Fields (required) | Optional data | Meaning |
|---|---|---|---|---|
| `schedule.created` | Human | `title`, `repo`, `team`, `every`, `at`, `zone` | `description`, `days`, `start`, `budget`, `maxTurns` | you made it, on |
| `schedule.changed` | Human | the same six | the same | you edited it: the whole schedule as it now is, as `agent.changed` carries the whole agent |
| `schedule.paused` | Human | none | `note` | it makes nothing until resumed |
| `schedule.resumed` | Human | none | none | on again, from now: slots during the pause are not due |
| `schedule.removed` | Human | none | `note` | gone from the list; the stream takes nothing more |
| `schedule.fired` | System, Human | `task`, `slot` | `missed` (count of earlier slots not run), `manual` | a run made `task`; `slot` is `""` for a run you asked for ("Run now") |
| `schedule.skipped` | System | `slot`, `reason` | `task` (the open one, or the unrecorded one), `why`, `missed` | a due run made nothing: `reason` is `open` (a task from this schedule is still open) or `refused` (the task was refused; `why` says so); or, with `slot` `""` and reason `unrecorded`, step 1 of section 5 couldn't record a run a crash cut short (`why` says so) |

`slot` is an RFC 3339 time with its offset (`2026-10-20T02:00:00+01:00`),
the instant the run was due. Slots are compared as instants, never as
strings: across a clock change the offsets differ.

The clock's actor is `events.Actor{Kind: events.System, ID: "schedules"}`,
beside the other `System` actors (`policy`, `watchdog`, `archiver`...).
Agents write no schedule type: an agent that wants a schedule tells you,
and you make it (open question 4).

These types are a contract with every log (`extensions.md` section 8): they
change only by adding optional fields. `schedule.changed` carrying the
whole schedule rather than a patch keeps that simple: a field added later
is absent from old events and takes its default, in `created` and
`changed` alike.

## 3. The state slice

`schedules.Streams()` gives `current.New` the prefix and the two folds, as
`ideas.Streams()` does (`internal/ideas/feature.go:26`). The state keeps
one `*Schedule` per stream in its owned map
(`internal/current/current.go:88`, `Apply` at `:246`), and the feature
reads it through `Reader.Owned` and `OwnedIDs`
(`internal/features/features.go:72`), wrapped in typed helpers
(`schedules.Get(r, id)`, `schedules.All(r)`), as `ideas.Get` does
(`internal/ideas/list.go:14`).

```go
// A Schedule is one schedule as its stream leaves it.
type Schedule struct {
	ID                 string
	Rule               Rule      // every, days, at, zone
	Title, Description string
	Repo, Team         string
	Start              bool
	Budget, MaxTurns   *int      // nil: the server's defaults
	State              string    // "on", "paused", "removed"
	Since              time.Time // the last created, changed or resumed: slots at or before it are not due
	CreatedAt          time.Time
	LastSlot           time.Time // the last slot fired or skipped; zero for none
	LastTask           string    // the task its last run made, "" for none
	LastRun            Run       // what the last firing or skip said, for the page
	Runs               int       // runs made, Run now included
	Named              map[string]bool // every task a schedule.fired or an unrecorded skip named
}
```

`Named` is how the job tells a recorded run from one a crash cut short
(section 5, step 1). A task number alone can't: `Store.Create` numbers
tasks in order, but a run the job made and didn't record can sit below a
later Run now's task. It grows by one per run, a few hundred a year for a nightly schedule, and `Next` copies it
on each event that adds to it: an estimate of microseconds per event,
unmeasured.

How `Next` folds each type (on a copy, as `features.Streams` requires):

- `created`: every field, `State` "on", `Since` and `CreatedAt` the
  event's time.
- `changed`: the fields again; `Since` moves to the event's time, so a
  rule changed at 01:59 to "daily at 01:00" first fires tomorrow, not
  in a minute for a slot that has passed.
- `paused`, `resumed`, `removed`: `State`; `resumed` also moves `Since`.
- `fired`: `LastSlot` (when `slot` isn't `""`), `LastTask`, `LastRun`,
  `Runs`, and `task` added to a copy of `Named`.
- `skipped`: `LastSlot` and `LastRun`; with reason `unrecorded`, `LastRun`
  and `task` added to a copy of `Named`, and `LastSlot` stays.
- anything else, such as a type a later version adds: nothing.

`Since` and the times come from each event's `At`, never the clock, so a
replay folds to the same schedule however late it runs.

The slice sees only `schedule-` streams. It does not learn of the task a
run made from that task's own `task.created`: `Apply` hands an owned
stream's `Next` only its own events (`current.go:283`), and `Equal`
proves each owned stream from its own events alone
(`internal/current/equal.go:167`). That is why a run is recorded by its
own `schedule.fired`, as an idea records its task with `idea.accepted`.

### Old logs replay without this code

A binary without schedules reads a log that has them as Ideas' absence is
read today (`extensions.md` section 7):

- Its catalogue has no `schedule-` prefix, so `KindOf` calls the streams
  foreign (`internal/events/catalogue.go:242`). `current.State.Apply`
  counts their events in the unknown tally and skips them
  (`internal/current/unknown.go`, `current.go:256`); serve prints one line
  per prefix at startup (`cmd/agentry/serve.go:110`); the Settings page
  shows the tally.
- `rules.Check` refuses any proposal into them (`internal/rules/rules.go:28`).
- The tasks the schedules made replay unchanged: `tasks.apply` reads
  `"from"` into `MadeFrom` with no knowledge of the feature
  (`internal/tasks/apply.go:97`, `MadeFromOf` at `:666`), and ignores
  `"slot"`. The task page shows "made from schedule-3" as a plain name,
  because no feature claims the prefix (`ui/src/features/streams.ts`,
  `featureLink`).
- The job isn't there, so nothing fires. Re-installing the feature
  replays its streams from the log: a schedule is back as it was, and
  its next due slot is worked out from `LastSlot` (section 5's catch-up).

Inside the feature, `Next` never fails on an event it doesn't expect: it
ignores it. A panic in it breaks only that schedule's stream, as
`current.guard` does for any stream (`current.go:353`), and a broken
schedule never fires (the job skips what `Get` can't read).

Snapshots need nothing new. A snapshot holds an owned stream only by its
place in the order and rebuilds it from the log on resume
(`internal/current/snapshot.go:60`), and a new binary has a new hash, so
the first start after the upgrade replays in full anyway
(`log-growth.md` section 2).

## 4. Rules

A feature's rules only refuse. For now they live where Ideas' do, in
`internal/rules`, in a new `schedule_rules.go`, called from `rules.Check`
beside `checkIdea` (`internal/rules/rules.go:56`): the generic slot that
would let a feature carry its own `Check` and guards (`Streams.Check`,
`Guards` in `extensions.md` section 6) isn't built, and building it means
moving Ideas' rules too (open question 6).

On a schedule's own stream, `checkSchedule` refuses:

- `created`, `changed`: an empty title or repo; a `team` with no
  coordinator in the roster (`agents.Roster.Coordinator`,
  `internal/agents/agent.go:223`); an `every` it doesn't know; `days`
  missing, empty or unknown for `week`, or present otherwise; an `at`
  that isn't `HH:MM`; a `zone` `time.LoadLocation` can't load; a budget or
  turn limit the task rules would refuse (`aBudget`, `aTurnLimit`,
  `internal/rules/task_rules.go:124`). `changed` also needs the schedule
  to exist and not be removed. A repo that isn't a git repo on this
  machine is the route's to refuse (with `worktrees.IsRepo`, as
  `internal/web/api.go:97` does for a new task): the rules don't read the
  disk.
- `paused`: a schedule that isn't on. `resumed`: one that isn't paused.
  `removed`: one removed already. Any type into a stream that holds no
  schedule.
- `fired` by System: a paused or removed schedule; a `slot` that doesn't
  parse, isn't after `LastSlot` or isn't after `Since`; a `task` that
  doesn't exist, wasn't made from this schedule (`task.MadeFrom` must be
  the stream, as `madeFromIt` checks for ideas,
  `internal/rules/idea_rules.go:86`) or is in `Named` already. The one
  System `fired` with `slot` `""` is section 5's step 1 recording a Run
  now that a crash cut short: it needs `manual` and a task you made
  (`ByAgentry` false). Step 1's other records carry the slot the task was
  made for and meet the same checks as any run's.
- `fired` by you (Run now): a `slot` that isn't `""`; a `task` not made
  from this schedule; a removed schedule.
- `skipped`: a `reason` it doesn't know. With reason `open` or
  `refused`: a `slot` that isn't after `LastSlot`. With reason
  `unrecorded`, only this list, as its `slot` is `""` and comes after
  nothing: a `slot` that isn't `""`, or a `task` that doesn't exist,
  wasn't made from this schedule or is in `Named` already; a removed
  schedule.

And one rule on a core type. `rules.Check` sends a `task.created` by
System with no `"about"` to `forAnOpenIdea` today
(`internal/rules/rules.go:91`, `idea_rules.go:122`), which would refuse a
schedule's task ("idea schedule-3 doesn't exist"). It becomes a choice by
the owner of the stream the task says it is made from: an idea's goes to
`forAnOpenIdea` as now, a schedule's to `forADueSchedule`, anything else is
refused with today's message. `forADueSchedule` refuses unless:

- the schedule exists, is on, and isn't broken;
- the task is what the schedule says: the same `title` and `repo`, the
  coordinator of its team in the roster now, its limits
  (or the defaults the route was given, which the rules can't see, so
  only checked when the schedule sets them), `parked` `""` when `start`
  and `"backlog"` otherwise;
- no task made from this schedule is open, whoever made it. "Open" is
  not done, read with `Reader.Task` for each of `Reader.TaskIDs()`; an
  archived task is done. So the clock makes a task only when no task from
  the schedule is open. That doesn't make it the only open one: you may
  run it now or reopen an old one beside it.

That last check is what makes a double fire impossible in the log, not
just unlikely in the job (section 5). It walks the live tasks once per
firing, under the store's lock: an estimate of well under a millisecond
for a thousand live tasks, unmeasured. It is the same walk
`ideas.madeFrom` makes outside the lock (`internal/ideas/accept.go:146`).

A `task.created` you make yourself with `"from": "schedule-3"` (Run now)
is not limited: you asked for it. The rules only check that `from` and
`idea` agree, as now (`rules.go:85`).

## 5. The job is the clock

`Feature.Jobs` (`internal/features/features.go:44`) is started once by
`features.StartJobs` (`internal/features/jobs.go:13`) from `serve`'s
`startJobs` (`cmd/agentry/serve.go:465`), in its own goroutine under
`recover`, with the same `Core` the feature's routes get: the checked
proposer from the `logs` map (`serve.go:218`), the read-only log, the
state and the server's defaults. It never holds the store.

### How it wakes

A ticker, once a minute, plus a wake at start. Each wake compares the wall
clock (`time.Now()`) with what the state says, so it doesn't matter how
late a tick is. It doesn't sleep until the next slot with one long
`time.Timer`: Go's timers run on the monotonic clock, which on Linux
doesn't advance while the machine is suspended, so a timer set for 02:00
at midnight on a laptop that sleeps from 01:00 to 07:00 fires near 08:00.
A one-minute tick costs a read of a few schedules; a run starts at most a
minute late (an estimate of the worst case, plus the time the loop takes
to start a turn).

`Run now` and edits don't wait for the tick: the route acts at once
(section 6), and a schedule created or changed is picked up within a
minute.

### What is due

For each schedule that is on and not broken, `Due(schedule, now)` is
every slot of its rule after both `LastSlot` and `Since`, up to and at
`now`. Slots come from one pure function, `Rule.Next(after time.Time)
time.Time`, in the schedule's zone, so tests can drive any clock. Across a
clock change, a local time may not exist (02:30 on the spring night) or
happen twice (01:30 on the autumn one), and Go's `time.Date` documents
only that it picks a time correct in one of the two zones, not which.
`Rule.Next` therefore settles it itself: a skipped time runs at the first
instant after the gap, a repeated one at its first occurrence, so every
rule makes exactly one slot a day, and S1's tests pin both nights.

### Missed runs: catch up once

When more than one slot is due, the job runs the latest one and puts the
number of earlier ones in `missed`. It never runs the others. Agentry was
down, or the machine asleep or off, and:

- Every example is "do the next piece of work", not "process that day's
  data". Seven nightly runs of "continue Portside" back to back make one
  task after another that each continue the same work, and each waits
  for the one before (the guard below), so six would be skipped anyway.
- Skipping them all would lose the one run you wanted: a laptop closed
  over the weekend should still do Monday's work when it opens.

So the trail says exactly what happened: "fired for Mon 02:00, 2 missed
since Sat". A run caught up this way is a normal run in every other way,
and it runs however old its slot is. The alternative, skipping a slot
more than one period old, changes nothing for a daily schedule or a
weekly one on one day: the latest due slot is always less than one
period old, or the next slot would be due. It bites only where the gap
between slots varies: a weekdays schedule (period a day) found on
Saturday morning loses Friday's run; that is open question 2.

### Firing one schedule

Under one mutex in the package, `firing`, held by the job and by Run now
alike, as `ideas.Accept` holds `accepting`
(`internal/ideas/accept.go:68` and `:136`):

1. **Finish a run a crash cut short.** A task made from this schedule
   (`task.MadeFrom` is the stream, read for each of `Reader.TaskIDs()`)
   that isn't in `Named`, open or done, is a run that an earlier firing
   made and that stopped before `schedule.fired`. The job reads that
   task's `task.created` from the log (`Core.Events.Stream`) for its
   `slot` and writes `schedule.fired` for it as System (with `slot` `""`
   and `manual` for a task you made with Run now). That write can be
   refused (the refusal is stored as `proposal.rejected`,
   `internal/store/store.go:260-265`) only when the schedule is paused
   now, or was changed or resumed after that slot, so `Since` has passed
   it and the slot isn't due either. A paused schedule never reaches step
   1, as the job fires only schedules that are on, but it can be paused
   while step 1 runs. If it is refused, the job writes
   `schedule.skipped {slot: "", reason: "unrecorded", task, why}` once
   instead, which puts the task in `Named` and says so in `LastRun`, so no
   later wake tries it again. If even that is refused, the job logs it and
   moves on. That can't loop or fire twice: the skip is refused only for
   a removed schedule, which no later wake fires, or a task already in
   `Named`, which can't be, as Run now and the job, the only writers of
   `fired` and skips, both hold `firing`. Either way it carries on with
   steps 2 to 5 on the same wake.
   This finds only what it should: a Run now made while the clock's task
   is open names its own task, so both are in `Named` and step 1 does
   nothing; reopening an old done scheduled task does nothing, as it was
   named when it ran; and a job that panicked between `task.created` and
   `schedule.fired` (`features.runJob` recovers and stops the job until
   the server restarts, `internal/features/jobs.go:21`) leaves a task
   that may be done by the restart but is still unnamed, so it is
   recorded once, `LastSlot` moves to its slot, and step 2 doesn't fire
   that slot again.
2. **Work out what is due.** None: done.
3. **Guard against piling up.** If a task made from this schedule is
   open (the same walk as step 1, and the same check as section 4's rule;
   usually it is `LastTask`, but a Run now may have made a later one),
   write `schedule.skipped {slot, reason: "open", task, missed}` for the
   latest due slot and stop. `LastSlot` moves on, so the next wake
   doesn't try the same slot again.
4. **Make the task.** `Core.Log.Create("task-", ...)` as
   `{System, "schedules"}`, with `title`, `description`, `repo`,
   `coordinator` (the team's, from the roster now), `budget`, `maxTurns`,
   `"from": "schedule-N"`, `"slot"`, and `"parked": "backlog"` unless
   `start`. Refused (the team has no coordinator now, the schedule was
   paused a moment ago): write `schedule.skipped {slot, reason:
   "refused", why}` and stop.
5. **Record the run.** `schedule.fired {task, slot, missed}`.

The task then is like any other: the coordinator loop picks it up, and
if `maxRunningTasks` is set it waits for a slot
(`internal/coordinator/slots.go:80`).

### Why it is idempotent

Each claim, checked against the store:

- **Every rule reads every event committed before it.** `Store.Propose`
  and `Create` hold `Store.mu` from the check to the end of `propose`
  (`internal/store/store.go:210`, `:220`, `:259`), which commits the
  event, hands it to `current.State.Apply` and moves `applied` before the
  lock is let go. So the second of two proposals is checked against a
  state that has the first. Two firings of one schedule, from any two
  goroutines, can't both make a task as the clock: the second
  `task.created` by System is refused by "no task made from this schedule
  is open".
- **The job's own reads are only hints.** It reads `Core.Read` outside the
  lock, so what it saw may change before it proposes; whatever it then
  proposes is checked by the rules under the lock. A stale read costs a
  refusal, never a duplicate.
- **There is no transaction across two events.** `task.created` and
  `schedule.fired` are two commits (`commit`, `store.go:294`, is one event
  in its own transaction). A crash between them leaves a task with no
  recorded run, which step 1 finds and records. The order is chosen so
  the other gap can't happen: no `schedule.fired` names a task that
  doesn't exist, because the rule needs the task first.
- **A restart forgets nothing.** What has run is `LastSlot` and `Named`,
  rebuilt from the log at start. The job keeps no state of its own.
- **A sleeping machine.** Covered by the wall-clock compare and catch-up
  once above.
- **What the mutex is for.** Not correctness of the log, which the rules
  give. It keeps the job and Run now from interleaving steps 1 to 5 for
  one schedule, so one of them doesn't, for instance, record a skip for a
  slot the other is firing. Without it the clock would still make a task
  only when no task from the schedule is open (Run now's tasks and
  reopened ones, which you ask for, are not limited).
- **Only one server per log.** Two `agentry serve` processes on one
  database are not supported today, and a second clock would be the
  least of it; the rules would still refuse the second task.

The job writes events only as it finds work: on a quiet day, two per
schedule run (the task's `task.created` is the task's).

## 6. Routes, tools and the page

### Routes

`schedules.routes(c features.Core)`, mounted by `web` behind each route's
`Auth` (`internal/web/features_api.go:25`, `wrapperOf` at `:38`). All are
`features.UI`: the page, and Claude Code connections, which carry the UI
token. None is `Desk` (the desk's session has no need), and none is
`Signed`. Every write is made as you (`httpapi.You`,
`internal/httpapi/httpapi.go:16`).

| Route | Does |
|---|---|
| `GET /api/schedules` | every schedule not removed (`?removed=1` too), each with its rule in words, `next` (from `Rule.Next`, worked out here so the page never computes zones), and its last run: the slot, the task, the task's title and status, or the skip and why |
| `POST /api/schedules` | make one; refuses a repo that isn't a git repo here (`worktrees.IsRepo`) |
| `PUT /api/schedules/{schedule}` | change one: the whole schedule, as `schedule.changed` carries it |
| `POST /api/schedules/{schedule}/pause` | `schedule.paused` |
| `POST /api/schedules/{schedule}/resume` | `schedule.resumed` |
| `DELETE /api/schedules/{schedule}` | `schedule.removed` |
| `POST /api/schedules/{schedule}/run` | Run now: under `firing`, a `task.created` as you from the schedule, then `schedule.fired {slot: "", manual: true}` as you. It doesn't move `LastSlot`, so the next due run still runs (subject to the open-task guard) |

A refusal comes back as 409 with the rule's reason, as other routes do
(`httpapi.ProposeOrRefuse`).

### MCP tools

`schedules.tools(agentry features.API, repo string)`, listed by
`agentry mcp` while the feature is enabled or visible
(`internal/features/features.go:38`):

| Tool | Arguments | Does |
|---|---|---|
| `list_schedules` | `repo` (optional; default this session's) | the schedules, with next and last run |
| `add_schedule` | `title`, `description`, `repo`, `team`, `every`, `days`, `at`, `zone`, `start`, `budget` | asks you to confirm (`features.AskOf`), then `POST /api/schedules` |
| `change_schedule` | `schedule` and any of `add_schedule`'s | the same, `PUT` |
| `pause_schedule`, `resume_schedule`, `remove_schedule` | `schedule` | asks, then the route |
| `run_schedule` | `schedule` | asks, then Run now |

The feature's `Instructions` line: "- Schedules: tasks Agentry makes on a
clock (list_schedules). Add, change, pause, run or remove one only when
the user asks in their own words; Claude Code asks the user to confirm."
The same rule as Ideas' tools: an agent's judgement never makes a task
run every night.

`Waiting` (`features.go:40`, read over the API until story 13 brings
`GET /api/waiting`): one entry per schedule whose last run was refused
("schedule-3 couldn't run: there is no coordinator in team Software") or
whose last run step 1 couldn't record (an `unrecorded` skip), kind
`schedules`, until the schedule is changed or runs. A run skipped
because a task from it is open adds nothing: that task is already listed if it
waits for you.

### The page

`ui/src/features/schedules/`, one object in `FEATURES`
(`ui/src/features/index.ts`), as `ui/src/features/ideas/index.tsx` is:

- `routes`: `/schedules`.
- `nav`: "Schedules", section `setup`, a clock icon (`CalendarClock`
  from lucide, as Ideas uses `Lightbulb`).
- `commands`: "Go to Schedules" and "New schedule" in Ctrl K.
- `streams: { prefix: "schedule-", to: (s) => "/schedules#" + s }`, so the
  task page's "made from" links to it (`ui/src/pages/TaskPage.tsx:255`,
  `featureLink` in `ui/src/features/streams.ts`).
- `turnOn`: on Settings, while it is hidden, a "New schedule" button.

`SchedulesPage.tsx` lists the schedules: title, the rule in words, on or
paused, next run, last run (its time, a link to its task and that task's
status, or "skipped: task-412 still waits for you"), and actions: Edit,
Pause or Resume, Run now, Remove (with a confirm). An empty page says what
a schedule is and offers the form. `ScheduleForm.tsx` is the add and edit
form: title, description, repo (as the new-task form takes it), team,
every / days / at, the zone (default the browser's,
`Intl.DateTimeFormat().resolvedOptions().timeZone`), start now or wait in
the Backlog, budget. Square corners, no shadows, flat selection, as the
rest of the page.

The manifest (`schedules.manifest`): `Enabled` always (nothing has to be
present for schedules to work, so the MCP tools are always listed),
`Visible` once any schedule exists, `Badge` the schedules whose last run
was refused. `Why`: "add a schedule to run a task on a clock" while there
are none. Whether to show it before the first schedule is open question 5.

## 7. `task.created` by System, and how the trail shows it

What exists now:

- `System` writes `task.created` already (`internal/events/catalogue.go:134`),
  for a task about an epic you closed (`"about"`, made by the coordinator
  loop, `internal/coordinator/loop.go:469`) and for an idea Agentry
  accepted for you (`internal/ideas/accept.go:211`). No catalogue change
  is needed.
- `"from"` is the provenance field: the stream that made the task, read
  by `tasks.MadeFromOf` into `Task.MadeFrom` (`internal/tasks/apply.go:97`,
  `:666`), sent to the page as `madeFrom` (`internal/tasks/summary.go:41`).
  Ideas writes `"from"` and `"idea"` both, for older binaries; schedules
  write only `"from"`, since no binary that reads `"idea"` alone knows
  schedules.
- `Task.ByAgentry` is true for any `task.created` by System (`apply.go:97`).
- What stands in the way is the rule, not the catalogue: section 4's
  change to `rules.go:91`.

A schedule's task adds one optional field, `"slot"`, the slot it was made
for: step 1 of section 5 reads it, and it goes in the description's first
line so a person sees it too ("Scheduled run for Tue 20 Oct 02:00,
from schedule-3.").

The trail: the task page's header says "made from schedule-3", linked to
the schedule. The first step of the trail, though, says it came from you:
`tasks.apply` records every `task.created` as `t.step(e, You, ...)`
(`internal/tasks/apply.go:99`), whoever wrote it. That is already wrong
for epic follow-ups and auto-accepted ideas. Story S7 makes the step's
`From` "Agentry" when `ByAgentry`. It changes what `apply` builds for old
events (the trail, not the log), so it runs the real-log replay tests;
`Equal` compares two folds by the same code, so it still holds.

## 8. Unattended runs and approvals

What happens today when a scheduled task's agent asks for something the
policy doesn't allow:

1. The gate records `action.asked`, which puts the task in Needs you, and
   waits on the log for your answer (`askTheUser`,
   `internal/hooks/handler.go:331`).
2. After `GateTimeoutSeconds`, 30 minutes (`internal/engines/engine.go:52`),
   the hook gives up: the action is held (`action.held`, `hold` at
   `handler.go:361`), and the agent is told to carry on with other work or
   report back (`Held`, `:324`). The task keeps its place in Needs you.
3. When you approve the held action later, the coordinator hands it back
   and the next request for that command passes with your approval
   (`usePass`, `handler.go:313`).
4. You hear of it from the page, from Claude Code's waiting list
   (`internal/mcp/waiting.go:28`) and its desktop notification
   (`internal/mcp/notify.go`, `notify-send` or `osascript`). Web Push
   subscriptions are kept (`internal/webpush/webpush.go`,
   `internal/web/push_api.go`), but nothing sends a push yet: that is
   story 15 ("Push what waits"). So a run at 02:00 that needs you waits
   until you next look.

The guard against piling up is section 5's step 3 and section 4's rule:
while a task from the schedule is open, every due run is skipped with
`reason: "open"`, and the page says which task it waits on. A run that
stalls costs one task's budget, not one per night. Each run also has the
task limits: a budget and `maxTurns` (the server's defaults unless the
schedule sets its own), so an unattended run that loops stops itself and
asks you.

What a schedule does not do: approve anything for you, or run under a
wider profile. Its task's agents are the team's agents, judged by their
own profiles in your `policy.toml` (`extensions.md` section 9).

Before relying on a nightly run, story 15 (push) should land, and per-team
profiles (phase 5) cut how many approvals arrive at all. Schedules can
ship before them: they are only less useful without them (open question
3).

## 9. Open questions for you

1. **The simple form, or cron too?** Recommendation: the simple form only
   (daily, weekdays, some days of the week, at a time, in a zone). It
   covers every schedule named so far, and the page can say it in words.
   Cron can come later as an optional field.
2. **A missed run: catch up once, always?** Recommendation: yes, the latest
   missed slot, once, however old, with the count of the rest. The
   alternative is to skip a missed slot older than one period. It never
   drops a daily schedule's run (Monday 02:00 is hours old when a laptop
   shut all weekend opens on Monday morning) or a one-day weekly one's,
   since the latest due slot is always less than a period old. It drops
   runs only where the gap between slots varies: a weekdays schedule
   found on Saturday loses Friday's run until Monday.
3. **Build schedules before Web Push sends (story 15)?** Recommendation:
   build them now, and say on the page, beside a schedule, that its runs
   can't reach you while push is off (the same reason
   `webpush.Available` gives). A nightly run that waits until morning is
   still a night's work done up to the question.
4. **May an agent make a schedule?** Recommendation: no. Only you write
   the schedule types; a coordinator that thinks something should recur
   says so in its summary. A schedule is standing authority to spend
   tokens.
5. **Show Schedules in the sidebar before the first one?**
   Recommendation: no: hidden until one exists, with "New schedule" on the
   Settings page and in Ctrl K, as Ideas waits for its team. The MCP tools
   are listed either way.
6. **Where do schedules' rules live?** Recommendation: in `internal/rules`
   beside Ideas' (`schedule_rules.go`), as today's code does, and move
   both into their features in one later story that adds the `Check` and
   guard slots. Adding the slots first would make this epic carry a
   refactor of Ideas.
7. **Should Run now be refused while a task from the schedule is open?**
   Recommendation: no, it is your explicit request; the page says the
   open task is still running and asks you to confirm.
8. **A scheduled task's title.** Recommendation: the schedule's title as
   it is, with the slot in the description's first line; the board then
   shows several "Continue Portside" cards over time, told apart by their
   number and date. The alternative is to append the date to the title.

## 10. Build stories

Each is one reviewable change, in order. Every story marked **apply** or
**replay** must run the real-log replay tests,
`internal/current` `TestARealLogAppliedEventByEventEqualsAReplay`
(`internal/current/real_log_test.go:27`) and `internal/tasks`
`TestARealLogReplays` (`internal/tasks/real_log_test.go:22`), on a fresh
`VACUUM INTO` copy of the live log, before it lands. Sizes are estimates.

**S1. The schedules package: types, rule and state** (apply, replay; M).
`internal/schedules` with `Registration()` (the seven types of section
2), `Streams()`, `Schedule`, `FromEvents`/`Next`, `Get`/`All`, `Rule` and
`Rule.Next`. Registered in `serve.go`'s catalogue (`:119`), `Restore`
(`:133`) and `installedFeatures` (`:486`), and in `storetest`'s catalogue
(`internal/storetest/storetest.go:52`). No routes, job or page yet. Done:
`Rule.Next` tests for daily, weekdays and weekly, both clock changes in
`Europe/London` and a zone with a half-hour offset; fold tests for every
type, including an unknown type ignored and a future optional field
ignored, and `Named` growing by `fired` and by an `unrecorded` skip and
by nothing else; a fixture log with schedule streams replays equal
event-by-event and in one go; the same log opened with `events.Core()`
counts the `schedule-` events in the tally and loads every task the same;
and `TestARealLogAppliedEventByEventEqualsAReplay` and
`TestARealLogReplays` pass on a fresh `VACUUM INTO` copy of the live
log.

**S2. Schedules' rules** (rules only; S). `internal/rules/schedule_rules.go`:
`checkSchedule` for the schedule types, and `forADueSchedule` with
`rules.go:91` choosing by the owner of `"from"`. Done: a test per
refusal in section 4; a System `task.created` from a schedule that is on
passes, a second one while the first is open is refused (and one while
a Run now's task is open), one from a paused schedule is refused; an
`unrecorded` skip with `slot` `""` naming an unnamed task made from the
schedule passes, whatever `LastSlot` is; a `fired` or an `unrecorded`
skip naming a task already in `Named` is refused; an auto-accepted idea's task still passes
and an epic follow-up still passes (Ideas' existing rule tests stay
green).

**S3. The job** (S/M). `Feature.Jobs`: the ticker, `Due`, catch-up once,
the five steps under `firing`, with the clock injected for tests. Its
checked proposer in `serve.go`'s `logs` (`:218`), with `Writes:
[task.created]`. Done: tests with a fake clock for one run; a machine
shut from Friday night to Monday morning, for a daily schedule (one run,
`missed: 2`) and a weekdays one (one run, `missed: 0`); a run skipped
because a task from the schedule is open; a refused task (team removed)
recorded as skipped; a crash between `task.created` and
`schedule.fired` (a test seam as `beforeMarking` in
`internal/ideas/accept.go:140`) recovered on the next wake; a panic at
that seam, with the task done before the restart, recorded once and its
slot not fired again; a Run now while the clock's task is open, after
which the next wake writes no `fired` for either task; an old done
scheduled task reopened, which step 1 ignores; a refused step 1 record
(the schedule changed after the slot) written as one `unrecorded` skip
and not retried on the next wake; and
two firings at once making one task. A panic in the job logs and stops
it, the server goes on (`features.StartJobs`).

**S4. Routes** (S). Section 6's routes in `internal/schedules/routes.go`,
with HTTP tests in `routes_test.go` as Ideas has. Done: each route's
happy path and its refusal (409 with the reason); a repo that isn't a git
repo is refused before anything is written; Run now under `firing` makes
one task and doesn't move `LastSlot`.

**S5. MCP tools, instructions and waiting** (S). Done: the tools listed
while the feature is enabled; each write asks through `AskOf` and does
nothing when you decline; a refused run shows in the waiting list until
the schedule runs or changes.

**S6. The page** (M). `ui/src/features/schedules/`: the page, the form,
nav, commands, `streams`, `turnOn`, and the manifest in Go. Done: render
tests for the empty page, a list with a next and last run, a skipped run
saying which task it waits on, and the form's validation; the task page
of a scheduled task links "made from" to its schedule; built into
`internal/web/dist` in its own `build:` commit.

**S7. The trail says Agentry made it** (apply, replay; S).
`tasks.apply`'s first step uses `From: "Agentry"` (or the loop's name, as
the trail spells Agentry elsewhere) when `ByAgentry`. Done: a test that a
scheduled task, an auto-accepted idea's task and an epic follow-up each
show Agentry as the first step, and yours still shows you; and
`TestARealLogAppliedEventByEventEqualsAReplay` and `TestARealLogReplays`
pass on a fresh `VACUUM INTO` copy of the live log.

**S8. Push for runs that need you** (depends on story 15). Nothing new if
story 15 pushes every new waiting key: a scheduled task in Needs you is
one. Done: a test with a fake push service that a fired task's
`action.asked` sends one push. If story 15 lands first, this is only the
test.

Order: S1, S2, S3 (the job is useless without the rules), then S4 to S6
in any order, S7 any time. A rough total, as an estimate: S1 to S6 are
about the size of Ideas' phase 2 stories 8 to 11.

## Still open

- Whether `Feature` gains `Check` and `Guards` slots before or after
  schedules (open question 6). Either way, the rules' behaviour is the
  same; only where the code lives differs.
- Removing a team a schedule names: the rules can't refuse
  `agent.removed` for a schedule's sake without the feature guarding a
  core type. Today's design records a refused run and lists it as
  waiting; a guard that warns at removal would need the `Guards` slot.
