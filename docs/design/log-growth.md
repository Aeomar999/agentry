# Log growth: bounded startup and disk use

Status: measurements only, for task-23. The design sections follow in a
later turn. Research only: nothing in the server changes with this
document.

## In short

To be written with the design. What the measurements already say:

- The log grows about 10 to 12 MB on a busy day, and faster each day,
  because `agent.learned` copies an agent's whole knowledge file every
  time, and the files keep growing. Knowledge is 58% of all bytes.
- Startup with the read model (`docs/design/read-model.md`) replays the
  log in 0.21 s now, about 0.9 s per 100k events. The state it keeps holds
  9.2 MB of heap, most of it done tasks' actions and turns.
- Done tasks hold 77% of the events but only 35% of the bytes. Archiving
  them alone takes the file from 26.5 MB to 15.3 MB; line diffs of knowledge and plans alone take it
  from 26.5 MB to 7.2 MB plus 4.9 MB of blobs. Both together leave a live
  log of 1.9 MB.

## 1. Measurements

### The data

A copy of the live database, made read-only with `VACUUM INTO` from a
`mode=ro` URI (it is not in git: it holds the user's data). The live file
was not written. Only counts, sizes and timings are quoted here. The log
starts on 16 October at 00:51 and ends on 18 October at 22:24, local time
(EDT): 69.5 hours.

| | |
|---|---|
| Events | 23,418, of which 33 are `proposal.rejected` |
| Streams | 27: 23 tasks, `agents`, `workers`, `connections`, `policy` |
| Database | 26.5 MB (6,478 pages of 4 KB, none free); 21.8 MB of it is event `data` |
| Rest of a row | 87 bytes on average (time, stream, actor, type, correlation) |
| Biggest task stream | `task-8`, 7,726 events, 4.0 MB |
| Busiest hour | 3,683 events, 4.1 MB |

Bytes by stream kind:

| Streams | Events | Data |
|---|---|---|
| `agents` | 516 | 12.67 MB |
| 23 task streams | 22,354 | 9.09 MB |
| `workers` | 537 | 42 KB |
| `connections`, `policy` | 11 | 1.4 KB |

### Growth per day

By local day. No day was quiet: the first was setting up, the last ends
at 22:24.

| Day | Hours with events | Events | Data | Data per active hour |
|---|---|---|---|---|
| 16 October | 5 | 968 | 0.37 MB | 0.07 MB |
| 17 October | 18 | 13,375 | 11.71 MB | 0.65 MB |
| 18 October | 5 | 9,075 | 9.73 MB | 1.95 MB |

Activity is bursty: 24 of the 28 hours with events have more than 100,
and hold 99% of the bytes. A busy day is 10 to 12 MB. If agents worked all
24 hours at the mean rate of an active hour (0.78 MB), a day would be
19 MB.

The rate grows even at the same activity. The mean `agent.learned` event
was 1.8 KB on 16 October, 20.6 KB on the 17th and 46.9 KB on the 18th: each
event copies a knowledge file that only gets longer. 18 October has half
the events of the 17th, and more knowledge bytes (6.56 MB against 5.98 MB).

### Bytes and counts by event type

Top ten by bytes (the top five are 96% of all data):

| Type | Events | Data | Share | Mean | Biggest |
|---|---|---|---|---|---|
| `agent.learned` | 446 | 12.57 MB | 57.6% | 28 KB | 59 KB |
| `action.requested` | 6,156 | 3.72 MB | 17.0% | 604 B | 32 KB |
| `turn.ended` | 510 | 2.00 MB | 9.2% | 3.9 KB | 8.6 KB |
| `plan.updated` | 951 | 1.78 MB | 8.1% | 1.9 KB | 14 KB |
| `task.assigned` | 239 | 0.89 MB | 4.1% | 3.7 KB | 6.3 KB |
| `action.approved` | 6,147 | 0.42 MB | 1.9% | 67 B | 68 B |
| `agent.changed` | 44 | 67 KB | | 1.5 KB | 2.4 KB |
| `turn.started` | 534 | 50 KB | | 93 B | 116 B |
| `task.created` | 23 | 39 KB | | 1.7 KB | 4.0 KB |
| `task.needs_you` | 43 | 38 KB | | 885 B | 7.6 KB |

Top ten by count: `action.requested` 6,156, `action.approved` 6,147,
`action.result` 6,064, `plan.updated` 951, `turn.started` 534,
`session.started` 532, `prompt.acknowledged` 531, `turn.ended` 510,
`agent.learned` 446, `worker.connected` 255. The three action events are
78% of the count and 19% of the bytes.

### Startup replay

`internal/current` does not exist yet (task-21 builds it), so this times
what its `Load` will do with today's code: open the store, read every task
stream with `TaskIDs` and `Store.Stream`, plus the `agents`, `workers` and
`connections` streams, and fold each with its `FromEvents`. Each run opens
a fresh store. Min of 3, on an idle 32-core Linux machine:

| Step | Time |
|---|---|
| Open, read and decode every stream | 156 ms |
| Fold every stream | 50 ms |
| Whole startup | 207 ms, 0.89 s per 100k events |

The state this builds holds **9.2 MB of heap** (9,242 KB), about 400
bytes per event.
That is seven times the 1.3 MB in `read-model.md`, which measured the state
while the decoded events stayed alive, so the texts the state shares with
them were not counted. The state keeps every action's command, every
turn's plan and report, and the knowledge files.

### Snapshot cost

A throwaway snapshot of the whole folded state: every task, the agents
roster and the workers. `tasks.Task` has two unexported fields,
`collected` and `storyNews`, so each task is encoded as a mirror struct
holding the `*Task` and those two, read and set back through reflection.
Min of 3:

| Format | Size | Encode | Decode | gzip size | gzip write | gunzip |
|---|---|---|---|---|---|---|
| JSON | 10.25 MB | 17.6 ms | 30.1 ms | 2.45 MB | +68 ms | +36 ms |
| gob | 9.06 MB | 9.6 ms | 4.9 ms | 2.39 MB | +71 ms | +31 ms |

With `collected` and `storyNews` set back, both round trips are
`reflect.DeepEqual` to the replay for every task and the agents roster.
The workers are not: `Worker.credential` (the hash of its credential) is
unexported and is lost, and so are the unexported `invites` of
`workers.Roster` and `connections.Roster`. A real snapshot must carry all
of them, so each package needs its own exported snapshot form.

Where the snapshot's bytes are (JSON):

| Part | Size |
|---|---|
| Actions of every task | 5.04 MB, about 818 B per action |
| Turns of every task (plans, reports) | 3.04 MB, about 5.7 KB per turn |
| The rest of the tasks (trail, files, descriptions) | 1.98 MB |
| Agents roster, knowledge included | 0.20 MB |
| Workers | 0.7 KB |

Per task:

| | Tasks | Mean | Largest |
|---|---|---|---|
| Open | 6 | 271 KB | 500 KB |
| Done | 17 | 496 KB | 4.04 MB (`task-8`) |
| Any task, without its actions | 23 | 218 KB | 2.13 MB |

So the snapshot is dominated by history, not open work: done tasks are 82%
of it, and a task's size grows with its actions and turns, open or not. A
snapshot of every task is about the size of the task streams' data. It
saves the replay time, but not memory or disk, unless done tasks leave the
state (archiving).

### Archiving potential

Which tasks are done, from the folded state, and when, from their last
`task.done` event, counted back from the log's last event. The data is
under 3 days old, so nothing is 3 or 7 days done yet.

| Done for at least | Tasks | Events | Data |
|---|---|---|---|
| any time | 17 | 18,100 (77.3%) | 7.74 MB (35.5%) |
| 1 day | 12 | 7,189 (30.7%) | 2.83 MB (13.0%) |
| 3 days | 0 | 0 | 0 |
| 7 days | 0 | 0 | 0 |

Done tasks hold most events but a third of the bytes, because the biggest
bytes, knowledge, are in the `agents` stream, which belongs to no task.
Archiving tasks can't shrink knowledge; only slimmer events can.

A one-row summary per archived task, sized as the task list's JSON
(`summaryOf`), is 624 bytes on average and 829 at most. For 17 tasks that
is 11 KB in place of 7.7 MB.

### Slimmer events

Each big text, decoded from its event: `knowledge`, `command`, `report`,
`plan` and `instructions`. Exact repeats are texts whose SHA-256 was seen
before. Line diffs are an ed-style diff from the previous version in the
same series (an agent's knowledge, one turn's plan): every added line plus
12 bytes per changed hunk, with the first version whole. Commands, reports
and instructions are not versions of anything, so a diff is the whole
text.

| Type | Texts | Bytes | Exact repeats | As line diffs |
|---|---|---|---|---|
| `agent.learned` | 446 | 12.39 MB | 0.47 MB (3.8%) | **0.36 MB (2.9%)** |
| `action.requested` | 6,156 | 3.34 MB | 26 KB (0.8%) | n/a |
| `turn.ended` | 510 | 1.95 MB | 0 | n/a |
| `plan.updated` | 951 | 1.74 MB | 0.76 MB (43.3%) | **1.02 MB (58.4%)** |
| `task.assigned` | 239 | 0.86 MB | 8 KB (0.9%) | n/a |

Knowledge changes by a few lines at a time: diffs keep 3% of its bytes.
Plans are often written again unchanged (43% are repeats), and diffs keep
58%. The other texts are unique, so a hash store gains nothing on them
but moving them out of the rows.

Texts over a threshold moved to the content-addressed store, the event
keeping `{"sha256": ..., "size": N}` (91 bytes), each unique text stored
once:

| Threshold | Data left in the log | Blobs | Blob bytes | On disk, 4 KB blocks | gzipped one by one |
|---|---|---|---|---|---|
| 1 KB | 2.97 MB (13.6%) | 2,108 | 17.49 MB | 21.97 MB | 7.68 MB |
| 4 KB | 5.98 MB (27.4%) | 867 | 14.64 MB | 16.89 MB | 6.30 MB |

Blob bytes by type at 1 KB: knowledge 11.91 MB, commands 2.07 MB, reports
1.94 MB, instructions 0.86 MB, plans 0.71 MB.

A hash store alone moves bytes; it hardly removes any. At 1 KB the log
file falls to 6.1 MB but 17.5 MB of blobs (22 MB in 4 KB blocks) come on
top: more disk than today, unless blobs are compressed (7.7 MB gzipped).
Only the repeats are saved.

### The resulting live database

The copy's rows written into new database files, each with a change
applied, then `VACUUM`:

| | Log file | Blobs beside it |
|---|---|---|
| As it is | 26.53 MB | |
| Texts over 4 KB moved | 10.14 MB | 14.64 MB |
| Texts over 1 KB moved | 6.11 MB | 17.49 MB |
| Knowledge and plans as line diffs, the rest over 1 KB moved | 7.22 MB | 4.87 MB |
| Done tasks archived (one 600-byte summary each) | 15.35 MB | |
| Archived, texts over 1 KB moved | 1.45 MB | the live part of 17.49 MB |
| Archived, diffs, the rest over 1 KB moved | 1.86 MB | the live part of 4.87 MB |

Diffs are what shrink the total: 26.5 MB becomes 12.1 MB (7.2 MB log plus
4.9 MB of blobs) with every event kept. Archiving then takes the live log
under 2 MB.

### VACUUM

On a second scratch copy, never the live file: drop the delete trigger,
delete every event of the streams archiving would move, check the free
pages, `VACUUM`, check the file.

| Archived | Streams | Events left | File after the delete | Free pages | After `VACUUM` |
|---|---|---|---|---|---|
| Done at all | 17 | 5,318 | 26.53 MB | 2,657 | **15.34 MB** |
| Done for 1 day | 12 | 16,229 | 26.53 MB | 1,002 | **22.26 MB** |

Deleting frees pages inside the file but the file does not shrink; only
`VACUUM` gives the space back. A `VACUUM` of the whole 26.5 MB copy takes
0.13 s (min of 3), about 5 s per GB, and needs free disk as big as the
database while it runs. The database has `auto_vacuum` off (0), so
`PRAGMA incremental_vacuum` is not available without first rebuilding
with `auto_vacuum = INCREMENTAL`.

### Projection inputs

The numbers the 1, 10 and 100 GB projections will use, all from the
measurements above:

- **Events per GB of database file:** about 880,000 (23,418 events in
  26.5 MB, 1,133 bytes per event in the file, 931 of them `data`).
- **Startup replay:** 8.9 µs per event, so about 7.8 s per GB of file.
  Of that, reading and decoding is 75% (scales with bytes), folding 25%
  (scales with events).
- **Heap held by the state:** about 400 bytes per event, so about 350 MB
  per GB of file, if done tasks stay in the state.
- **Snapshot:** gob 0.34 bytes per byte of file now (9.06 MB for 26.5 MB),
  decoded at about 0.54 ms per MB, plus 3.4 ms per MB to gunzip.
- **Per open task in the state:** 271 KB on average, 500 KB at most, and
  growing with its history: about 818 bytes per action and 5.7 KB per
  turn. Per done task: 496 KB on average.
- **Archive summary:** about 625 bytes per archived task.
- **Growth:** 10 to 12 MB on a busy day today, up to 19 MB if agents worked
  around the clock, and rising with knowledge size: about 4 GB a year at a
  busy day every day, 7 GB at all hours, before knowledge growth.
- **Share by kind:** knowledge 57.6% of bytes, other task texts 38%;
  done tasks 77.3% of events and 35.5% of bytes (with all tasks counted as
  they are now).
- **Slimming factors on the whole log:** knowledge as diffs keeps 2.9% of
  its bytes, plans 58%; the log file with diffs is 0.27 of today's, plus
  0.18 in blobs; archiving done tasks after that leaves 0.07.
- **VACUUM:** about 5 s per GB, with free disk equal to the file.

### Rerun the measurements

```sh
# a read-only copy of the live database (never write to the live one)
mkdir -p .agentry/files/bench
sqlite3 'file:'"$HOME"'/.agentry/agentry.db?mode=ro' \
  "VACUUM INTO '$PWD/.agentry/files/bench/agentry.db'"

# totals, days and types
sqlite3 -column 'file:.agentry/files/bench/agentry.db?mode=ro' "
  SELECT COUNT(*), SUM(type = 'proposal.rejected'), COUNT(DISTINCT stream),
    SUM(length(CAST(data AS BLOB))) FROM events;
  SELECT substr(datetime(at, 'localtime'), 1, 10) AS day, COUNT(*),
    SUM(length(CAST(data AS BLOB))),
    COUNT(DISTINCT substr(datetime(at, 'localtime'), 1, 13)) FROM events GROUP BY day;
  SELECT type, COUNT(*), SUM(length(CAST(data AS BLOB))) AS bytes,
    MAX(length(CAST(data AS BLOB))) FROM events GROUP BY type ORDER BY bytes DESC LIMIT 10;"

# replay, snapshot, archiving and slimmer events
AGENTRY_BENCH_DB=$PWD/.agentry/files/bench/agentry.db \
  go test ./internal/web -run TestLogGrowthBench -v -count=1

# VACUUM after archiving, on a scratch copy (1 for "done for a day", 0 for "done")
cp .agentry/files/bench/agentry.db .agentry/files/bench/vacuum.db
sqlite3 .agentry/files/bench/vacuum.db "
  DROP TRIGGER events_no_delete;
  CREATE TEMP TABLE archived AS SELECT stream FROM events GROUP BY stream
    HAVING MAX(CASE WHEN type = 'task.done' THEN position END)
         > COALESCE(MAX(CASE WHEN type = 'task.reopened' THEN position END), 0)
       AND julianday((SELECT MAX(at) FROM events))
         - julianday(MAX(CASE WHEN type = 'task.done' THEN at END)) >= 0;
  DELETE FROM events WHERE stream IN (SELECT stream FROM archived);
  SELECT COUNT(*) FROM events; PRAGMA freelist_count;"
ls -l .agentry/files/bench/vacuum.db
sqlite3 .agentry/files/bench/vacuum.db VACUUM
ls -l .agentry/files/bench/vacuum.db
```

The benchmark is `internal/web/loggrowth_bench_test.go`; it is skipped
unless `AGENTRY_BENCH_DB` is set, copies the file again so the run never
writes to it, and takes about 5 s. The rebuilt databases go to the test's
temporary folder.

The figures above are from a copy taken at 22:24. A rerun from a fresh
copy 8 minutes later had 762 more events (24,180) and a 27.8 MB file, and
every share above moved by under 3 points: knowledge diffs kept 2.8%, done
tasks held 35.6% of the bytes, and the archived, diffed live log was
1.7 MB.
