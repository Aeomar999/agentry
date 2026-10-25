Feature: Reliability
  What Agentry does when a turn, a process or a machine fails.

  @REL-01 @test
  Scenario: A dead process ends its turn and the coordinator gets the issue
    Given a turn whose agent process has exited
    When the watchdog's 1 second tick finds the process gone, after the 3 second start grace
    Then the watchdog raises exactly 1 issue, saying the process exited, and stops the session
    And no turn is left open, so the coordinator gets the issue
    # PLAN.md T-014; internal/watchdog/watchdog_test.go

  # Every number below comes from the code; the comment on each names its
  # file and constant. The watchdog checks every task with an open turn
  # once a second (internal/watchdog/watchdog.go Run), so each limit is
  # met within about a second of passing.

  # The watchdog

  @REL-02 @test
  Scenario: A dead agent process is an issue for the coordinator after the start grace
    # internal/watchdog/watchdog.go DefaultLimits.StartGrace = 3 seconds
    Given a task whose open turn started 1 second ago and whose agent process is not running
    When the watchdog checks the turn
    Then it raises 0 issues, because the process may not be up yet within 3 seconds of the start
    When 4 seconds have passed since the turn started and the process is still not running
    Then it stops the task's session: yes
    And it raises 1 issue saying the agent's process exited (crashed or was killed)
    And the open turn has ended, so the task goes back to its coordinator: yes
    # PLAN.md T-014; internal/watchdog/watchdog_test.go

  @REL-03 @test
  Scenario: A turn with no event for 10 minutes is a stall
    # internal/watchdog/watchdog.go DefaultLimits.Silence = 10 minutes
    Given a task whose open turn took its prompt and has no approved tool call running
    And nothing has happened on the turn since its last event
    When 11 minutes have passed since that event
    Then the watchdog raises 1 issue saying the agent has been silent for over 10m0s
    And it stops the task's session: yes
    # PLAN.md T-014; internal/watchdog/watchdog_test.go

  @REL-04 @test
  Scenario: A turn waiting for the user is not a stall
    # internal/watchdog/watchdog.go findProblem isWaitingForYou
    Given a task whose open turn asked for a command and the policy asked the user about it
    And the user has not answered
    When 1 hour has passed with no event on the turn
    Then the watchdog raises 0 issues, because a held action waits for the user, not the agent
    # PLAN.md T-014; internal/watchdog/watchdog_test.go

  @REL-05 @test
  Scenario: An approved tool call with no result for 20 minutes is an issue
    # internal/watchdog/watchdog.go DefaultLimits.Tool = 20 minutes
    Given a task whose open turn has 1 approved tool call, npm test, with no result yet
    When 15 minutes have passed since the call was requested
    Then the watchdog raises 0 issues, because a running tool is not silence and is within its limit
    When 21 minutes have passed since the call was requested
    Then the watchdog raises 1 issue naming the command, npm test, as a tool call with no result for over 20m0s
    # PLAN.md T-014; internal/watchdog/watchdog_test.go

  @REL-06 @test
  Scenario: The same command requested 5 times in a row is a loop
    # internal/watchdog/watchdog.go DefaultLimits.Repeats = 5
    Given a task whose open turn requested the same tool and command 5 times in a row, each approved and each with a result
    When the watchdog checks the turn
    Then it raises 1 issue saying the agent requested the same command 5 times in a row and naming the command
    And it stops the task's session: yes
    # PLAN.md T-014; internal/watchdog/watchdog_test.go

  @REL-07 @test
  Scenario: Different commands with the same description are not a loop
    # internal/watchdog/watchdog.go repeatedCommand compares the tool and the command, never the description
    Given a turn with 5 WebSearch calls whose commands all differ
    When the watchdog counts the repeats
    Then the count is 1
    Given a turn with 5 calls whose commands Agentry could not read
    When the watchdog counts the repeats
    Then the count is 0, because an unreadable command says nothing about a loop
    # PLAN.md T-014; internal/watchdog/repeat_test.go

  @REL-08 @test
  Scenario: A prompt not acknowledged within 60 seconds asks the user to open the terminal
    # internal/watchdog/watchdog.go DefaultLimits.Acknowledge = 60 seconds
    Given a task whose open turn started but whose engine has not echoed its prompt
    When 61 seconds have passed since the turn started
    Then the watchdog records agent.waiting with a question saying the agent has not received its prompt after 1m0s and to open its terminal
    And the task's status shown is Needs you
    When the engine acknowledges the prompt
    Then the task's status shown is Working, not Needs you
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  @REL-09 @test
  Scenario: A resumed or forked session whose process exited before taking the prompt is retried fresh once
    # internal/watchdog/watchdog.go resumeFailed, DefaultLimits.StartGrace = 3 seconds
    Given a task whose open turn went on with the agent's last session, or a fork of it, or a sibling story's session
    And the turn has not acknowledged its prompt and its process is not running
    When 4 seconds have passed since the turn started
    Then the watchdog stops the session and raises 1 issue with retryFresh set to yes, saying the resumed session didn't start, or the forked session when the turn forked
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  @REL-45 @test
  Scenario: A fresh session, or one that took its prompt, whose process exited is a dead process, not retried fresh
    # internal/watchdog/watchdog.go resumeFailed is only for a resumed session that never took its prompt
    Given a task whose open turn was fresh, or had acknowledged its prompt before its process exited
    And its process is not running
    When 4 seconds have passed since the turn started
    Then the issue says the process exited, with retryFresh set to no
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  # A usage limit

  @REL-10 @test
  Scenario: An engine stopped at its usage limit leaves the turn open and limited, with the reset time
    # internal/events/catalogue.go TurnLimited fields: agent, error, details, resetsAt
    Given a running turn whose engine reports a StopFailure with error rate_limit and a message saying the limit resets at 3am UTC
    When the hook call is handled
    Then the last event of the task is turn.limited with error rate_limit and resetsAt at 3:00 UTC
    And the turn is still open: yes
    And the task's count of issues in a row is 0
    # internal/hooks/handler.go; internal/hooks/handler_test.go

  @REL-11 @test
  Scenario: A limited turn is retried once, 1 minute after the engine's reset time
    # internal/watchdog/watchdog.go ResetSlack = 1 minute, CarryOn
    Given a turn limited with a reset time 30 minutes away
    When 30 minutes have passed
    Then 0 lines have been typed into the session and the turn is still limited
    When 31 minutes and 1 second have passed
    Then the watchdog records turn.resumed with reason reset, and then types 1 line into the session: "Your Claude usage limit has reset. Carry on from where you stopped."
    And it stops the session: no
    When 33 minutes have passed
    Then the line has been typed exactly 1 time
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  @REL-12 @test
  Scenario: A limit whose reset time the engine did not say is retried after 1 hour
    # internal/watchdog/watchdog.go UnknownReset = 1 hour, counted from the turn's last event
    Given a turn limited with no reset time
    When 59 minutes and 59 seconds have passed
    Then 0 lines have been typed into the session
    When 1 hour and 1 second have passed
    Then 1 line has been typed into the session
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  @REL-13 @test
  Scenario: A limit said to reset days away is retried after 24 hours at most
    # internal/watchdog/watchdog.go MaxLimitWait = 24 hours
    Given a turn limited with a reset time 3 days away
    When 23 hours, 59 minutes and 59 seconds have passed
    Then 0 lines have been typed into the session
    When 24 hours and 1 second have passed
    Then 1 line has been typed into the session
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  @REL-14 @test
  Scenario: A turn that hits the limit again straight after its retry is an issue, not another wait
    # internal/hooks/handler.go: a second limit with no work since the retry ends the turn as an issue
    Given a turn retried by the watchdog when its limit reset
    When its engine reports the usage limit again before any tool call
    Then the turn ends as 1 issue, and no second wait is started
    # internal/hooks/handler.go; internal/hooks/handler_test.go

  @REL-46 @test
  Scenario: A retried turn that worked before hitting the limit again waits for the next reset
    # internal/hooks/handler.go: a tool call after the retry means the retry worked
    Given a turn retried by the watchdog when its limit reset, which made a tool call after the retry
    When its engine reports the usage limit again
    Then the turn is kept open and limited, to be waited out as the first limit was
    # internal/hooks/handler.go; internal/hooks/handler_test.go

  @REL-15 @test
  Scenario: A limited turn that the log refuses to resume is a stall 10 minutes after the reset
    # internal/watchdog/watchdog.go checkLimited, DefaultLimits.Silence = 10 minutes
    Given a limited turn whose turn.resumed proposal the log refuses when its reset time comes
    When the reset time has passed
    Then 0 lines are typed into the session
    When more than 10 minutes have passed since the retry was due
    Then the watchdog raises 1 issue saying the agent has been silent since its usage limit was due to reset
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  @REL-16 @test
  Scenario: A tool open across a usage limit is timed from the resume, not from its start
    # internal/watchdog/watchdog.go toolClock, DefaultLimits.Tool = 20 minutes
    Given a turn with 1 approved tool call open when its engine hit the usage limit
    And the turn was resumed when the limit reset
    When 20 minutes have passed since the tool call was requested but fewer than 20 minutes since the resume
    Then the watchdog raises 0 issues for the tool
    When more than 20 minutes have passed since the resume
    Then the watchdog raises 1 issue naming the tool call
    # internal/watchdog/watchdog.go; internal/watchdog/watchdog_test.go

  # A stop and start keeping sessions

  @REL-17 @test
  Scenario: An agent's session lives in its own host process and keeps its last screen when the agent dies
    # internal/sessions/host.go: the host runs the command in a pty and outlives the server that started it
    Given a session started with the task's environment, 120 columns by 36 rows
    When the program in it is killed with Ctrl-C
    Then the session still exists: yes
    And its screen still holds the program's last output: yes
    And the session reports the program as running: no
    And a viewer attached to it is told the program died within 5 seconds: yes
    # PLAN.md T-011; internal/sessions/session_test.go

  @REL-18 @manual
  Scenario: Stop Agentry stops the server and the running sessions carry on
    # cmd/agentry/open.go runStopServer: procs.Terminate, then up to 50 waits of 100 ms = 5 seconds
    # internal/procs/procs_unix.go Terminate sends SIGTERM; internal/procs/procs_windows.go Terminate kills the process, since Windows has no polite signal
    # internal/sessions/session.go Start: each session's host is detached from the server
    # Steps:
    # 1. Start Agentry and make a task, so an agent's session is running.
    # 2. Run Stop Agentry (agentry stop).
    # 3. Read what it prints and check the agent's process is still running (ps, or Task Manager on Windows).
    # 4. Start Agentry again and open the task's terminal.
    Given Agentry running from Start Agentry, with 1 agent session running
    When Stop Agentry is run
    Then the server process has ended within 5 seconds: yes
    And it prints that agents' sessions that were running carry on, and the next start picks them up: yes
    And the agent's process is still running: yes
    When Start Agentry is run again
    Then the task's terminal shows the same session: yes
    # README.md, What Start Agentry does; cmd/agentry/open.go

  @REL-19 @test
  Scenario: An agent's next turn on a task goes on with its last session
    # cmd/agentry/serve.go -resume-sessions default true; internal/turns/turns.go Config.NoResume
    Given a task whose agent ended a turn in session s-1
    And the server runs with -resume-sessions at its default, true
    When the agent's next turn starts
    Then the engine is launched with --resume s-1: yes
    And the turn records s-1 as the session it came from
    # internal/turns/turns.go; internal/turns/turns_test.go

  @REL-47 @test
  Scenario: With -resume-sessions=false every turn starts a fresh session
    # internal/turns/turns.go Config.NoResume, ResumeOff = "resume is off"
    Given a task whose agent ended a turn in session s-1
    And a server started with -resume-sessions=false
    When the agent's next turn starts
    Then the engine is launched with --resume: no
    And the reason the turn records for starting fresh is exactly "resume is off"
    # internal/turns/turns.go; internal/turns/turns_test.go

  @REL-20 @test
  Scenario: An action left asking the user when the server stops is held for the user
    # internal/hooks/orphans.go HoldOrphanedAsks runs at every start
    # cmd/agentry/serve.go serveUntilDone: requests share the server's context, with 2 seconds to shut down
    Given an action the policy asked the user about, with its hook call waiting
    When the server stops, so the hook call's context ends
    Then the action is recorded as held: yes
    Given an action asked and never answered in the log, with no hook call waiting, as after the server was killed outright
    When the server starts
    Then the action is held for the user: yes
    And its decision is still undecided, and 0 hooks are waiting on it
    # internal/hooks/orphans.go; internal/hooks/handler_test.go

  @REL-21 @test
  Scenario: After a start, a worker that was online is waited for 30 seconds before its agent is refused
    # internal/workerlink/server.go ForgetConnections records every worker offline at start
    # internal/machines/fleet.go OfflineGrace = 30 seconds
    Given a fleet that expects a worker which has not dialed in since the start
    And an agent that may run only on that worker
    When a turn for the agent is started and the worker connects 300 milliseconds later
    Then the turn starts on that worker: yes
    # README.md, Work across machines; internal/machines/fleet_test.go

  @REL-48 @test
  Scenario: An agent whose only worker has never been seen is refused a turn
    # internal/machines/fleet.go: the grace is for a worker that was online, never for one unseen
    Given a fleet whose worker has never dialed in
    And an agent that may run only on that worker
    When a turn for the agent is started
    Then the fleet refuses it with an error: yes
    # README.md, Work across machines; internal/machines/fleet_test.go

  # Startup replay of the log and the snapshot

  @REL-22 @test
  Scenario: A second start goes on from the snapshot the first one wrote
    # cmd/agentry/serve.go snapshotFile = state.snapshot, in the data folder beside the log
    # internal/current/writer.go: the last snapshot is written when the server closes
    Given an empty data folder with an unfinished snapshot write left in it
    When the server starts
    Then it logs that there is no snapshot, so the whole log is replayed
    And the unfinished write is removed: yes
    And when it stops, the data folder holds state.snapshot with mode 0600
    When the server starts again
    Then it logs "replayed": no, because the state went on from the snapshot
    # PLAN.md T-066; cmd/agentry/serve_test.go

  @REL-23 @test
  Scenario: A snapshot that cannot be trusted is thrown away and the whole log is replayed
    # internal/current/resume.go Restore and Resume; a torn or corrupt file is kept as state.snapshot.bad
    Given a log of 12 events and a snapshot written at position 12 by build-1
    When the snapshot is missing, or written by build-2, or is a folder, or cannot be read
    Then the state resumed from the snapshot: no, and the log says why
    When the snapshot is truncated, or 1 byte of its body is flipped
    Then the state resumed from the snapshot: no, and the file is kept as state.snapshot.bad
    When the log is shorter than the snapshot's position, or has no event at that position, or the event there is not the one it was written at
    Then the state resumed from the snapshot: no
    When an event after the snapshot breaks a stream the snapshot holds
    Then the whole log is replayed and the stream is broken afterwards: no
    # PLAN.md T-066; internal/current/snapshot_test.go

  @REL-24 @test
  Scenario: Starting from a snapshot gives the same state as replaying the whole log
    # internal/current/equal.go Equal compares every stream
    Given a fixture log, with a story's news among its events
    When a snapshot is written at every position of the log and a state is resumed from each
    Then each resumed state equals the state replayed from the whole log: yes
    # PLAN.md T-066; internal/current/snapshot_test.go

  @REL-25 @test
  Scenario: The snapshot is written after 10,000 events, or after 10 minutes with at least 1 event, and at close
    # internal/current/writer.go snapshotEvery = 10,000 events, snapshotInterval = 10 minutes
    Given a snapshot writer running beside the state
    When 10,000 events have been applied since the last snapshot
    Then 1 snapshot is written
    When 10 minutes pass with at least 1 event applied
    Then 1 snapshot is written
    When 10 minutes pass with 0 events applied
    Then 0 snapshots are written
    When the writer closes with events applied since the last snapshot
    Then it writes 1 last snapshot, which the next start goes on from: yes
    When the writer closes with 0 events applied since the last snapshot
    Then it writes 0 snapshots
    # PLAN.md T-066; internal/current/writer_test.go

  @REL-26 @benchmark
  Scenario: Starting from a snapshot is fast however long the log is
    # internal/current/snapshot_bench_test.go startupLimit = 500 ms, startupLimitOne = 750 ms, tailLimit = 100 ms, tailEvents = 10,000
    Given a log of 1 GB of history, its done tasks archived, and a snapshot of the working set
    When the state is opened from the snapshot on 2 or more cores
    Then the open takes at most 500 ms, and the snapshot body's decode, the main part of it, measured 0.10 s on 2 cores and 0.08 s on 8
    When the state is opened from the snapshot on 1 core
    Then the open takes at most 750 ms, and the body's decode measured 0.14 s
    When the snapshot is 10,000 events behind the log
    Then applying the tail adds at most 100 ms
    # Measured by TestStartupBench in internal/current/startup_bench_test.go on a copy of a real log grown to 1 GB
    # of history, run with AGENTRY_BENCH_DB=<copy>/agentry.db go test ./internal/current -run TestStartupBench -v -count=1 -timeout 30m.
    # Measured in October 2026 during the log-growth build, on a 1 GB synthetic log grown from the 19 October copy:
    # a 201.7 MB body (128.5 MB of working-set tasks, 90,000 summaries), written as gob (127.7 MB, 64 streams),
    # decodes in 0.14 / 0.10 / 0.08 s on 1 / 2 / 8 cores. That decode is the main part of the open; the numbers are
    # in docs/design/log-growth.md, section "Measured in build step 2: gob".
    # The limits are the constants in internal/current/snapshot_bench_test.go; TestTheSnapshotBenchRunsOnASmallLog
    # runs the same measurement on a small log in every go test run.
    # PLAN.md T-066; internal/current/startup_bench_test.go

  # Bounded disk use and archiving

  @REL-27 @test
  Scenario: Tasks done for 30 days are archived unless -archive-after is 0
    # cmd/agentry/serve.go defaultArchiveAfter = 30 days (720h)
    Given a server started with no -archive-after
    Then it logs that it archives tasks done for 720h0m0s
    Given a server started with -archive-after 0
    Then it logs that archiving is off, with no age
    # README.md, Archiving, erasing and redacting; cmd/agentry/serve_test.go

  @REL-49 @test
  Scenario: A negative -archive-after is refused at start
    # cmd/agentry/serve.go: "-archive-after is 0 or above, 0 to never archive"
    Given a server started with -archive-after -1h
    When it starts
    Then it refuses to start, saying -archive-after is 0 or above
    # README.md, Archiving, erasing and redacting; cmd/agentry/serve_test.go

  @REL-28 @test
  Scenario: The archiver runs a round at start and then every hour
    # internal/archive/archive.go roundEvery = 1 hour
    Given an archiver started with the server
    When it starts
    Then it runs 1 round at once
    When 1 hour passes
    Then it runs 1 more round
    When the server closes during a round
    Then the task under way finishes and the rest wait: yes
    # README.md, Archiving, erasing and redacting; internal/archive/archive_test.go

  @REL-29 @test
  Scenario: A round archives only tasks done for the age with no turn open, and an epic after its stories
    # internal/archive/archive.go chosen
    Given a task done 30 days ago and another done 29 days ago
    When a round runs
    Then 1 task is archived
    Given a task done 30 days ago whose turn is still open
    When a round runs
    Then it is archived: no, until the round after the turn ends
    Given an epic done 30 days ago with 1 story still in the live log
    When a round runs
    Then the epic is archived: no, and it is archived in a round after the story is
    Given a task archived, then unarchived 1 day ago and not reopened
    When a round runs
    Then it is archived: no, because it waits the whole 30 days again from its unarchiving
    # README.md, Archiving, erasing and redacting; internal/archive/archive_test.go

  @REL-30 @test
  Scenario: Archiving moves a task's rows into one file and leaves a marker with its summary
    # internal/store/archive.go Archive; the blob is kept by sharedfiles.Disk under the data folder's files
    Given a done task with N rows in the live log
    When it is archived
    Then the live log holds 0 of those rows and 1 task.archived marker in their place
    And the marker names exactly 1 blob, which holds the N rows, and the task's summary
    When the delete would remove other than N rows
    Then the whole archive is undone, marker and all: yes
    # README.md, Archiving, erasing and redacting; internal/store/archive_test.go

  @REL-52 @test
  Scenario: A delete of rows no task.archived marker covers is refused by the database
    # internal/store/archive.go: the log is append-only by trigger; only a marker in the same transaction lets its own rows go
    Given a live log with a task's rows in it and no task.archived marker for them
    When a statement deletes all of the log's rows, or the row at position 1
    Then each delete is refused by the database as append-only: yes
    And 0 rows are gone
    # README.md, Archiving, erasing and redacting; internal/store/archive_test.go

  @REL-31 @test
  Scenario: An archived task still opens as it was, and takes no new events
    # internal/web/archives.go reads the blob and the live rows back; internal/rules refuse every event but task.unarchived
    Given a task archived with its history
    When its page is read
    Then it reads as before archiving, marked archived with the date: yes
    When its blob is missing
    Then the read fails with an error: yes
    # README.md, Archiving, erasing and redacting; internal/web/archives_test.go

  @REL-53 @test
  Scenario: An archived task takes no event but task.unarchived
    # internal/rules/rules.go: on an archived stream every event but task.unarchived is refused; event.redacted is refused with its own reason
    Given a task archived with its history, read back once
    When a message is sent to it, or its labels or its rank are set
    Then each request is refused: yes, and none is answered 200
    And the task is still archived in the state: yes
    # README.md, Archiving, erasing and redacting; internal/web/archives_test.go

  @REL-32 @test
  Scenario: Reopening an archived task brings its rows back first
    # internal/store/unarchive.go Unarchive inserts the rows at their own positions and appends task.unarchived
    Given an archived done task
    When it is moved off Done on the board, or its coordinator is told something
    Then task.unarchived is appended and its rows are back at their own positions: yes
    And it reopens as any done task does: yes
    # README.md, Archiving, erasing and redacting; internal/web/unarchive_test.go

  @REL-50 @test
  Scenario: A reopen whose unarchiving is refused writes nothing
    # internal/store/unarchive.go Unarchive
    Given an archived done task whose unarchiving the store refuses
    When it is moved off Done on the board
    Then 0 events are written
    # README.md, Archiving, erasing and redacting; internal/web/unarchive_test.go

  @REL-33 @test
  Scenario: After a round that archived, the space goes back to the disk and a snapshot is asked for
    # internal/archive/archive.go afterArchiving; internal/store/vacuum.go GiveBackSpace and VacuumOnce
    Given a round that archived at least 1 task
    Then the store gives its free pages back and empties the WAL 1 time
    And 1 snapshot is asked for
    Given a round that archived 0 tasks
    Then the store gives space back 0 times and 0 snapshots are asked for
    Given a log file made before incremental vacuum whose one full VACUUM failed
    When the next round runs
    Then the VACUUM is tried again: yes
    # README.md, Archiving, erasing and redacting; internal/archive/archive_test.go

  @REL-34 @benchmark
  Scenario: An archived task opens in time
    # internal/web/archived_bench_test.go coldRead = 100 ms, warmRead = 10 ms
    Given an archived task of about 4,000 events
    When its page is read for the first time since the server started
    Then the read takes at most 100 ms, and measured 16.9 ms
    When it is read again
    Then the read takes at most 10 ms from the cache of the last few opened, and measured 2.4 ms
    # Measured by TestAnArchivedTaskOpensInTime in internal/web/archived_bench_test.go, which runs in every
    # go test ./internal/web: a task of about 4,000 events is archived and GET /api/tasks/{task} is timed cold
    # (gunzip the blob and fold its events) and warm (from the cache); the test logs both times.
    # Measured on 2026-10-25 on a development machine, 4,001 events: cold 16.9 ms, warm 2.4 ms.
    # TestArchivedTaskOpensBench in the same file measures the largest task of a real log when AGENTRY_BENCH_DB is set.
    # README.md, Archiving, erasing and redacting; internal/web/archived_bench_test.go

  # A timed-out approval kept open and retried

  @REL-35 @test
  Scenario: An ask the user does not answer in time is held for the user, and the agent is told no
    # internal/hooks/handler.go hold and Held
    Given an agent's command the policy asks the user about
    When no answer comes before the hook's wait runs out
    Then the hook's reply is deny, saying the command is held for the user's approval and not to retry it now
    And the action is recorded as held, with no decision yet
    And the task's status shown is Needs you
    And 0 hooks are waiting on the task
    # README.md, What happens to one action; internal/hooks/handler_test.go

  @REL-36 @test
  Scenario: A late approval hands the command back and lets that exact command through once
    # internal/hooks/handler.go usePass, reason "the user approved this earlier, after it timed out"
    Given a held command, git push origin HEAD:dev, that the user approves after it timed out
    Then the task's coordinator is handed the full command back: yes
    When the agent asks for the same command again before its next turn
    Then it is allowed: no, because the pass waits for the agent's next turn
    When the coordinator hands the work back to the agent and it asks for another command
    Then it is allowed by the pass: no
    When the agent asks for git push origin HEAD:dev in that turn
    Then it is allowed with the reason "the user approved this earlier, after it timed out": yes
    When the agent asks for the same command once more
    Then the pass is used: yes, and the user is asked again
    # README.md, What happens to one action; internal/hooks/handler_test.go

  @REL-37 @test
  Scenario: A pass never overrides a denial
    # internal/hooks/handler.go: the policy is judged first, the pass skips only the question to the user
    Given a held command the user approved late, and a policy that now denies that command
    When the agent asks for it in its next turn
    Then it is denied: yes, and the pass is kept for a later ask
    # README.md, What happens to one action; internal/hooks/handler_test.go

  @REL-51 @test
  Scenario: A late denial gives no pass
    # internal/hooks/handler.go: only a late approval makes a pass
    Given a held command the user denied late
    When the agent asks for it in its next turn
    Then it passes: no
    # README.md, What happens to one action; internal/hooks/handler_test.go

  # A held reply before a hook's deadline

  @REL-38 @test
  Scenario: The hook's deadline is under the engine's own timeout, so the gate fails closed
    # internal/engines/engine.go GateTimeoutSeconds = 1800; internal/turns/turns.go environment: deadline = 1800 - 10 = 1790 seconds
    # internal/hookcall/hookcall.go EnvDeadline = AGENTRY_HOOK_DEADLINE
    Given a turn's environment as the runner builds it
    Then AGENTRY_HOOK_DEADLINE is 1790 seconds, which is 10 seconds under the engine's 1800 second hook timeout: yes
    # internal/engines/engine.go; internal/turns/turns_test.go

  @REL-39 @test
  Scenario: The server stops waiting on an ask 5 seconds before the hook's deadline, or a quarter of a short one
    # internal/hooks/handler.go replyMargin = 5 seconds, answerWithin
    Given a hook call with a deadline of 20 seconds
    Then the server waits at most 15 seconds
    Given a hook call with a deadline of 1 second
    Then the server waits at most 750 milliseconds
    Given a hook call with a deadline of 29 minutes and 50 seconds
    Then the server waits at most 29 minutes and 45 seconds
    Given a hook call with no deadline, or a negative one
    Then the server's wait has no limit of its own: yes
    # internal/hooks/handler.go; internal/hooks/handler_test.go

  @REL-40 @test
  Scenario: An ask is held and answered before the deadline the hook sent
    # internal/hooks/handler.go Handle and hold
    Given a hook call asking about a command with a deadline of 1 second and no answer from the user
    When the server handles it
    Then the reply comes in under 1 second: yes
    And the reply is deny and says not to retry: yes
    And the action is held: yes
    # internal/hooks/handler.go; internal/hooks/handler_test.go

  # A worker going offline during a turn

  @REL-41 @test
  Scenario: A worker that went offline is taken to still run its turns for 30 seconds
    # internal/machines/offline.go Offline.IsRunning; internal/machines/fleet.go OfflineGrace = 30 seconds
    Given a worker whose link dropped less than 30 seconds ago, with an agent's turn running on it
    When the watchdog asks whether the task's agent is running
    Then the answer is yes, so the watchdog raises 0 issues for it
    Given a worker whose link dropped 30 seconds ago or more
    When the watchdog asks whether the task's agent is running
    Then the answer is no, so the turn is judged as a dead process would be
    # README.md, Work across machines; internal/machines/fleet_test.go

  @REL-42 @manual
  Scenario: A worker dials the server again, waiting 1 second, then twice as long each time, up to 30 seconds
    # internal/workerlink/worker.go Run: wait starts at 1 second, doubles, min(2*wait, 30 seconds); back to 1 second after a link that was up
    # internal/workerlink/worker.go stream: each write to the terminal stream gives up after 10 seconds
    # Steps:
    # 1. Join a worker and check its link is up on the Workers page.
    # 2. Stop the server.
    # 3. Run agentry worker logs -f on the worker and note each "dialing again in" line.
    # 4. Start the server again and check the Workers page shows the worker online.
    Given a worker whose link to the server was up
    When the server stops
    Then the worker's log says the link is down and it dials again in 1s
    And each later wait is twice the one before, up to 30s and never more
    When the server starts again
    Then the worker is online again within 30 seconds of the start: yes
    And a link that drops after being up starts the waits at 1s again: yes
    # README.md, Add machines; internal/workerlink/worker.go

  @REL-43 @test
  Scenario: The coordinator asks the user when the agent's only machines are offline
    # internal/coordinator/loop.go startTurn: the fleet is waited for OfflineGrace, then the user is asked
    Given a worker that joined and is offline, and an engineer who may run only on it
    When the coordinator assigns the task to the engineer
    Then the task asks the user exactly 1 question, saying the engineer can't run anywhere now because the worker is offline
    And the task has 1 turn, the coordinator's: no engineer turn was started
    # README.md, Work across machines; internal/coordinator/loop_test.go

  @REL-44 @test
  Scenario: A join code works once, for 1 hour
    # internal/workers/workers.go InviteLifetime = 1 hour
    Given a join code made now
    When a machine joins with it within 1 hour
    Then it joins: yes
    # README.md, Add machines; internal/workers/workers_test.go

  @REL-54 @test
  Scenario: A used, wrong or expired join code is refused
    # internal/workers/workers.go InviteLifetime = 1 hour; expired means 1 second past the hour in the test
    Given a join code made now and used once by a machine
    When the same code is used again, or a code that was never made
    Then the join is refused: yes
    When a fresh code is used 1 hour and 1 second after it was made
    Then the join is refused: yes
    # README.md, Add machines; internal/workers/workers_test.go
