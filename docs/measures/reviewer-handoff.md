# Measure: the reviewer's re-review with a full brief, a resumed session or a compact handoff

Story 4 of the session-reuse epic gives a fresh reviewer a compact handoff instead of a resumed
session. This measures the same re-review three ways on a scratch server, with the scripts in
`tools/measure`. Measured on 2026-10-25.

The task is the same each run: add `format_duration` to a two-file Python module in a scratch
repo. A Measure team (lead, engineer, reviewer; no tester, no deployer) builds and reviews it.
After the first review, `plant.sh` commits a fix round with the same defect every run and the
task is sent back to review with the note "I committed the fix round on the branch myself. Have
the reviewer review the whole change again before anything else." The re-review is the measured
turn.

## Table 1: the re-review

| Configuration | Build | Switch or setting | Turns table | Re-review turn | New input | Cache writes | Cache reads | Output | Total | Brief | Found the planted defect |
|---|---|---|---|---|---|---|---|---|---|---|---|
| A: full fresh brief | dev at 80cf9e0b (`agentry-dev`) | `-resume-sessions=false` | fresh: resume is off | turn-7 | 130 | 25,866 | 232,645 | 3,169 | 261,810 | 8,263 bytes, 68 lines | yes, "Verdict: changes needed" |
| B: resumed session | dev at 80cf9e0b (`agentry-dev`) | defaults (resuming on) | resumed from its own session: fromSession set, no freshBecause | turn-9 | 66 | 11,204 | 172,210 | 1,857 | 185,337 | 8,238 bytes, 79 lines | yes, "Verdict: changes needed" |
| C: compact handoff | this branch (`agentry-214`) | defaults: the reviewer's resume setting unset, so a Reviews-column agent is fresh | fresh: reviewers start fresh; no fromSession | turn-10 | 98 | 25,090 | 179,644 | 1,988 | 206,820 | 12,121 bytes, 153 lines | yes, "Verdict: changes needed" |

C's re-review total against A: -21.0% (54,990 fewer tokens). Against B: +11.6% (21,483 more).

Brief sizes are those of the saved `turn-N.md` the reviewer was given. What each holds:

- A, dev's fresh worker brief: the task, the lead's instructions, the worktree and guard rails,
  the plan. It carries no trail at all: on dev only the coordinator's fresh brief has "What has
  happened so far"; a fresh worker never had it.
- B, dev's "going on" brief: "What has happened since your last turn" (the trail steps since the
  reviewer's first turn, each cut to its first lines) and the new instructions. The session
  itself carries the rest.
- C, the handoff: the task, the instructions, "What to review" (the range, the touched files, the
  git commands; never the diff), "The builder's last report" (quoted whole), "Your earlier
  findings" (the reviewer's last report, quoted whole) and "Changed since your last verdict" (the
  commits and files since, with the git command). Two whole reports make it the largest brief.

So the handoff is a superset of A's brief, not a cut-down trail. The saving against A is in the
turn, not the brief: A's reviewer read more (232k cache reads against 180k) and wrote more
(3,169 output tokens against 1,988) to find the same thing. Every fresh reviewer turn in these
runs, first review or re-review, cost 200k to 207k; A's re-review at 262k is the outlier. B's
resumed turn is cheaper mainly in cache writes (11k against 25k), since the session's context was
already written.

## Table 2: the first review and the whole task

| Configuration | First review turn | Turns table | New input | Cache writes | Cache reads | Output | Total | Brief | Task total | Turns |
|---|---|---|---|---|---|---|---|---|---|---|
| A | turn-4 | fresh: resume is off | 98 | 23,380 | 175,274 | 2,448 | 201,200 | 7,208 bytes, 65 lines | 1,380,186 | 9 |
| B | turn-6 | first turn | 98 | 23,182 | 175,711 | 1,680 | 200,671 | 6,826 bytes, 63 lines | 2,357,332 | 11 |
| C | turn-4 | fresh: reviewers start fresh | 68 | 24,123 | 176,134 | 1,861 | 202,186 | 9,551 bytes, 124 lines | 1,906,840 | 12 |

Task totals are not comparable: B's engineer got an extra fix round before the first review
(two engineer turns), and C had an extra reviewer turn and two turns with no tokens (see below).
C's extra reviewer turn, turn-7, was also a handoff: 203,195 tokens (98 / 23,599 / 178,221 /
1,277), brief 12,140 bytes, 148 lines.

## Review quality

All three re-reviews found the planted defect and gave the same must-fix findings: the early
`return '0'` breaks the `0 -> '0s'` spec and the round trip, it sits above the type guard so
`0.0` and `False` no longer raise, and the test changes hide the regression. All three ended
"Verdict: changes needed".

That is weaker evidence than it looks: each lead's re-review instructions described the planted
change and its effect before the reviewer ran. A's lead listed "whether format_duration(0) still
meets the spec ('0s')" among points to verify; B's lead wrote that "parse_duration('0') raises
ValueError, so the round trip no longer holds at zero"; C's lead wrote "the lead probed:
format_duration(0) == '0' ... parse_duration('0') raises ValueError". The re-review note names the
commit as a "fix round", and the lead reads the diff before assigning, so this will happen in
every run. The reviewers still verified each point themselves (A ran a line tracer, C probed
`0.0` and `False`), but the measure cannot say whether a reviewer without the lead's hints
would have found it from the handoff alone.

## The planted defect

`tools/measure/plant.sh` adds `if seconds == 0: return '0'` at the top of `format_duration`
(the docstring still says `0 -> '0s'`), changes the zero test to expect `'0'` and drops 0 from
any round-trip loop, so the suite stays green. It commits as "fix: review round" with an
engineer author and writes the hash to `<cfg>-plant.hash`. Each run's test suite was OK after the
plant (`<cfg>-plant-tests.txt`).

## What varied between runs

- The engineer's output differs run to run: different commit hashes, a different zero path (A
  returned `'0s'` early; C fell through to `return text or "0s"`), six or seven tests.
- The lead's instructions differ in wording and length; see "Review quality" for the hints.
- B's lead sent the engineer a fix round before the first review, so B has two engineer turns.
- C's first review verdict was not registered: the reviewer's report had notes after its last
  verdict line. The lead sent it back for a re-confirmation (turn-7, also a fresh handoff), so
  C's planted re-review is the reviewer's third turn, not its second.
- C was interrupted. After the first review, the lead's turn-5 hit a usage limit and the server
  died with the tester's turn. C was restarted later on the same data dir; the log replayed, the
  lead went on in turn-6 with the reason "its last turn failed", and the run continued from
  there. Turn-5 and turn-11 (stopped by the move to Done) show 0 tokens.
- The scratch reviewer's turn-10 report began with a line about the user's own board, which
  shows the scratch agents could see the user's live Agentry through the user-level Claude
  configuration. It is the same for every configuration and is not part of the change measured.

## How to repeat it

Everything lives under `.agentry/files/measure` (git ignores it) and the scripts take relative
paths from the worktree root. Build the two binaries into that folder first, `agentry-dev` from
dev at 80cf9e0b and `agentry-214` from this branch, with the UI dist built so `serve` prints the
page token. Then, once, `tools/measure/scratch.sh` makes the scratch repo. Per configuration:

    tools/measure/start.sh A agentry-dev -resume-sessions=false     # B: agentry-dev; C: agentry-214
    tools/measure/team.sh A
    tools/measure/run.sh A task
    tools/measure/wait.sh A task-1 m-reviewer 1      # exit 3 = still waiting, call again
    tools/measure/run.sh A plant
    tools/measure/run.sh A rereview
    tools/measure/wait.sh A task-1 m-reviewer 2      # 3 if an extra reviewer turn ran
    tools/measure/run.sh A finish                    # saves A-task.json, A-events.json
    tools/measure/briefs.sh A turn-4 turn-7          # the reviewer turns' briefs into A-briefs/
    tools/measure/summary.sh A                       # splits, modes, reviewer reports
    tools/measure/stop.sh A

`run.sh finish` renames the scratch branch to `measure/<cfg>-task-1` so the next run's task-1
can use the name again. The lead's instructions are the `task.assigned` events in
`<cfg>-events.json`.
