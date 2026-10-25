Feature: Reliability: Agentry keeps going when an agent, a worker or the server itself fails

  # Every number below comes from the code; the comment on each names its
  # file and constant. The watchdog checks every task with an open turn
  # once a second (internal/watchdog/watchdog.go Run), so each limit is
  # met within about a second of passing.

  # The watchdog

  @REL-02 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A dead agent process is an issue for the coordinator after the start grace
    # internal/watchdog/watchdog.go DefaultLimits.StartGrace = 3 seconds
    Given a task whose open turn started 1 second ago and whose agent process is not running
    When the watchdog checks the turn
    Then it raises 0 issues, because the process may not be up yet within 3 seconds of the start
    When 4 seconds have passed since the turn started and the process is still not running
    Then it stops the task's session: yes
    And it raises 1 issue saying the agent's process exited (crashed or was killed)
    And the open turn has ended, so the task goes back to its coordinator: yes

  @REL-03 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A turn with no event for 10 minutes is a stall
    # internal/watchdog/watchdog.go DefaultLimits.Silence = 10 minutes
    Given a task whose open turn took its prompt and has no approved tool call running
    And nothing has happened on the turn since its last event
    When 11 minutes have passed since that event
    Then the watchdog raises 1 issue saying the agent has been silent for over 10m0s
    And it stops the task's session: yes

  @REL-04 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A turn waiting for the user is not a stall
    # internal/watchdog/watchdog.go findProblem isWaitingForYou
    Given a task whose open turn asked for a command and the policy asked the user about it
    And the user has not answered
    When 1 hour has passed with no event on the turn
    Then the watchdog raises 0 issues, because a held action waits for the user, not the agent

  @REL-05 @test
  # internal/watchdog/watchdog_test.go
  Scenario: An approved tool call with no result for 20 minutes is an issue
    # internal/watchdog/watchdog.go DefaultLimits.Tool = 20 minutes
    Given a task whose open turn has 1 approved tool call, npm test, with no result yet
    When 15 minutes have passed since the call was requested
    Then the watchdog raises 0 issues, because a running tool is not silence and is within its limit
    When 21 minutes have passed since the call was requested
    Then the watchdog raises 1 issue naming the command, npm test, as a tool call with no result for over 20m0s

  @REL-06 @test
  # internal/watchdog/watchdog_test.go
  Scenario: The same command requested 5 times in a row is a loop
    # internal/watchdog/watchdog.go DefaultLimits.Repeats = 5
    Given a task whose open turn requested the same tool and command 5 times in a row, each approved and each with a result
    When the watchdog checks the turn
    Then it raises 1 issue saying the agent requested the same command 5 times in a row and naming the command
    And it stops the task's session: yes

  @REL-07 @test
  # internal/watchdog/repeat_test.go
  Scenario: Different commands with the same description are not a loop
    # internal/watchdog/watchdog.go repeatedCommand compares the tool and the command, never the description
    Given a turn with 5 WebSearch calls whose commands all differ
    When the watchdog counts the repeats
    Then the count is 1
    Given a turn with 5 calls whose commands Agentry could not read
    When the watchdog counts the repeats
    Then the count is 0, because an unreadable command says nothing about a loop

  @REL-08 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A prompt not acknowledged within 60 seconds asks the user to open the terminal
    # internal/watchdog/watchdog.go DefaultLimits.Acknowledge = 60 seconds
    Given a task whose open turn started but whose engine has not echoed its prompt
    When 61 seconds have passed since the turn started
    Then the watchdog records agent.waiting with a question saying the agent has not received its prompt after 1m0s and to open its terminal
    And the task's status is Needs you
    When the engine acknowledges the prompt
    Then the task's status is Working

  @REL-09 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A resumed or forked session whose process exited before taking the prompt is retried fresh once
    # internal/watchdog/watchdog.go resumeFailed, DefaultLimits.StartGrace = 3 seconds
    Given a task whose open turn went on with the agent's last session, or a fork of it, or a sibling story's session
    And the turn has not acknowledged its prompt and its process is not running
    When 4 seconds have passed since the turn started
    Then the watchdog stops the session and raises 1 issue with retryFresh set to yes, saying the resumed session didn't start
    Given instead a turn that was fresh, or one that had acknowledged its prompt before its process exited
    When 4 seconds have passed since the turn started
    Then the issue says the process exited, with retryFresh set to no

  # A usage limit

  @REL-10 @test
  # internal/hooks/handler_test.go
  Scenario: An engine stopped at its usage limit leaves the turn open and limited, with the reset time
    # internal/events/catalogue.go TurnLimited fields: agent, error, details, resetsAt
    Given a running turn whose engine reports a StopFailure with error rate_limit and a message saying the limit resets at 3am UTC
    When the hook call is handled
    Then the last event of the task is turn.limited with error rate_limit and resetsAt at 3:00 UTC
    And the turn is still open: yes
    And the task's count of issues in a row is 0

  @REL-11 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A limited turn is retried once, 1 minute after the engine's reset time
    # internal/watchdog/watchdog.go ResetSlack = 1 minute, CarryOn
    Given a turn limited with a reset time 30 minutes away
    When 30 minutes have passed
    Then nothing has been typed into the session and the turn is still limited
    When 31 minutes and 1 second have passed
    Then the watchdog records turn.resumed with reason reset, and then types 1 line into the session: "Your Claude usage limit has reset. Carry on from where you stopped."
    And it stops the session: no
    When 33 minutes have passed
    Then the line has been typed exactly 1 time

  @REL-12 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A limit whose reset time the engine did not say is retried after 1 hour
    # internal/watchdog/watchdog.go UnknownReset = 1 hour, counted from the turn's last event
    Given a turn limited with no reset time
    When 59 minutes and 59 seconds have passed
    Then 0 lines have been typed into the session
    When 1 hour and 1 second have passed
    Then 1 line has been typed into the session

  @REL-13 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A limit said to reset days away is retried after 24 hours at most
    # internal/watchdog/watchdog.go MaxLimitWait = 24 hours
    Given a turn limited with a reset time 3 days away
    When 23 hours, 59 minutes and 59 seconds have passed
    Then 0 lines have been typed into the session
    When 24 hours and 1 second have passed
    Then 1 line has been typed into the session

  @REL-14 @test
  # internal/hooks/handler_test.go
  Scenario: A turn that hits the limit again straight after its retry is an issue, not another wait
    # internal/hooks/handler.go: a second limit with no work since the retry ends the turn as an issue
    Given a turn retried by the watchdog when its limit reset
    When its engine reports the usage limit again before any tool call
    Then the turn ends as 1 issue, and no second wait is started
    Given instead a retried turn that made a tool call after the retry
    When its engine reports the usage limit again
    Then the turn stays open and limited, to be waited out as the first limit was

  @REL-15 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A limited turn that the log refuses to resume is a stall 10 minutes after the reset
    # internal/watchdog/watchdog.go checkLimited, DefaultLimits.Silence = 10 minutes
    Given a limited turn whose turn.resumed proposal the log refuses when its reset time comes
    When the reset time has passed
    Then 0 lines are typed into the session
    When more than 10 minutes have passed since the retry was due
    Then the watchdog raises 1 issue saying the agent has been silent since its usage limit was due to reset

  @REL-16 @test
  # internal/watchdog/watchdog_test.go
  Scenario: A tool open across a usage limit is timed from the resume, not from its start
    # internal/watchdog/watchdog.go toolClock, DefaultLimits.Tool = 20 minutes
    Given a turn with 1 approved tool call open when its engine hit the usage limit
    And the turn was resumed when the limit reset
    When 20 minutes have passed since the tool call was requested but fewer than 20 minutes since the resume
    Then the watchdog raises 0 issues for the tool
    When more than 20 minutes have passed since the resume
    Then the watchdog raises 1 issue naming the tool call

  # A stop and start keeping sessions

  @REL-17 @test
  # internal/sessions/session_test.go
  Scenario: An agent's session lives in its own host process and keeps its last screen when the agent dies
    # internal/sessions/host.go: the host runs the command in a pty and outlives the server that started it
    Given a session started with the task's environment, 120 columns by 36 rows
    When the program in it is killed with Ctrl-C
    Then the session still exists: yes
    And its screen still holds the program's last output: yes
    And the session reports the program as running: no
    And a viewer attached to it is told the program died within 5 seconds: yes

  @REL-18 @manual
  Scenario: Stop Agentry stops the server and the running sessions carry on
    # cmd/agentry/open.go runStopServer: SIGTERM, then up to 50 waits of 100 ms = 5 seconds
    # internal/sessions/session.go Start: each session's host is detached from the server
    # Steps:
    # 1. Start Agentry and make a task, so an agent's session is running.
    # 2. Run Stop Agentry (agentry stop).
    # 3. Read what it prints and check the agent's process is still running (ps, or Task Manager on Windows).
    # 4. Start Agentry again and open the task's terminal.
    Given Agentry running from Start Agentry, with 1 agent session running
    When Stop Agentry is run
    Then the server process has ended within 5 seconds: yes
    And it prints that agents' sessions that were running carry on, and the next start picks them up
    And the agent's process is still running: yes
    When Start Agentry is run again
    Then the task's terminal shows the same session: yes

  @REL-19 @test
  # internal/turns/turns_test.go
  Scenario: An agent's next turn on a task goes on with its last session
    # cmd/agentry/serve.go -resume-sessions default true; internal/turns/turns.go Config.NoResume
    Given a task whose agent ended a turn in session s-1
    And the server runs with -resume-sessions at its default, true
    When the agent's next turn starts
    Then the engine is launched with --resume s-1: yes
    And the turn records s-1 as the session it came from
    Given instead a server started with -resume-sessions=false
    When the agent's next turn starts
    Then the engine is launched with --resume: no
    And the turn records why it is fresh: resume is off

  @REL-20 @test
  # internal/hooks/handler_test.go
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

  @REL-21 @test
  # internal/machines/fleet_test.go
  Scenario: After a start, a worker that was online is waited for 30 seconds before its agent is refused
    # internal/workerlink/server.go ForgetConnections records every worker offline at start
    # internal/machines/fleet.go OfflineGrace = 30 seconds
    Given a fleet that expects a worker which has not dialed in since the start
    And an agent that may run only on that worker
    When a turn for the agent is started and the worker connects 300 milliseconds later
    Then the turn starts on that worker: yes
    Given instead an agent whose only worker has never been seen
    When a turn for the agent is started
    Then the fleet refuses it with an error: yes

  # Startup replay of the log and the snapshot

  @REL-22 @test
  # cmd/agentry/serve_test.go
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

  @REL-23 @test
  # internal/current/snapshot_test.go
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

  @REL-24 @test
  # internal/current/snapshot_test.go
  Scenario: Starting from a snapshot gives the same state as replaying the whole log
    # internal/current/equal.go Equal compares every stream
    Given a fixture log, with a story's news among its events
    When a snapshot is written at every position of the log and a state is resumed from each
    Then each resumed state equals the state replayed from the whole log: yes

  @REL-25 @test
  # internal/current/writer_test.go
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

  @REL-26 @benchmark
  # Measured by TestStartupBench in internal/current/startup_bench_test.go on a copy of a real log grown to 1 GB
  # of history, run with AGENTRY_BENCH_DB=<copy>/agentry.db go test ./internal/current -run TestStartupBench -v -count=1 -timeout 30m.
  # The limits are the constants in internal/current/snapshot_bench_test.go; TestTheSnapshotBenchRunsOnASmallLog
  # runs the same measurement on a small log in every go test run.
  Scenario: Starting from a snapshot is fast however long the log is
    # internal/current/snapshot_bench_test.go startupLimit = 500 ms, startupLimitOne = 750 ms, tailLimit = 100 ms, tailEvents = 10,000
    Given a log of 1 GB of history, its done tasks archived, and a snapshot of the working set
    When the state is opened from the snapshot on 2 or more cores
    Then the open takes at most 500 ms
    When the state is opened from the snapshot on 1 core
    Then the open takes at most 750 ms
    When the snapshot is 10,000 events behind the log
    Then applying the tail adds at most 100 ms

  # Bounded disk use and archiving

  @REL-27 @test
  # cmd/agentry/serve_test.go
  Scenario: Tasks done for 30 days are archived unless -archive-after is 0
    # cmd/agentry/serve.go defaultArchiveAfter = 30 days (720h)
    Given a server started with no -archive-after
    Then it logs that it archives tasks done for 720h0m0s
    Given a server started with -archive-after 0
    Then it logs that archiving is off
    Given a server started with -archive-after -1h
    Then it refuses to start, saying -archive-after is 0 or above

  @REL-28 @test
  # internal/archive/archive_test.go
  Scenario: The archiver runs a round at start and then every hour
    # internal/archive/archive.go roundEvery = 1 hour
    Given an archiver started with the server
    When it starts
    Then it runs 1 round at once
    When 1 hour passes
    Then it runs 1 more round
    When the server closes during a round
    Then the task under way finishes and the rest wait: yes

  @REL-29 @test
  # internal/archive/archive_test.go
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

  @REL-30 @test
  # internal/store/archive_test.go
  Scenario: Archiving moves a task's rows into one file and leaves a marker with its summary
    # internal/store/archive.go Archive; the blob is kept by sharedfiles.Disk under the data folder's files
    Given a done task with N rows in the live log
    When it is archived
    Then the live log holds 0 of those rows and 1 task.archived marker in their place
    And the marker names the blob that holds the N rows and the task's summary
    And a delete of any row the marker does not cover is refused by the database: yes
    When the delete would remove other than N rows
    Then the whole archive is undone, marker and all: yes

  @REL-31 @test
  # internal/web/archives_test.go
  Scenario: An archived task still opens as it was, and takes no new events
    # internal/web/archives.go reads the blob and the live rows back; internal/rules refuse every event but task.unarchived
    Given a task archived with its history
    When its page is read
    Then it reads as before archiving, marked archived with the date: yes
    And a message sent to it is refused: yes
    When its blob is missing
    Then the read fails with an error: yes

  @REL-32 @test
  # internal/web/unarchive_test.go
  Scenario: Reopening an archived task brings its rows back first
    # internal/store/unarchive.go Unarchive inserts the rows at their own positions and appends task.unarchived
    Given an archived done task
    When it is moved off Done on the board, or its coordinator is told something
    Then task.unarchived is appended and its rows are back at their own positions: yes
    And it reopens as any done task does: yes
    Given a reopen whose unarchiving is refused
    Then nothing is written: yes

  @REL-33 @test
  # internal/archive/archive_test.go
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

  @REL-34 @benchmark
  # Measured by TestAnArchivedTaskOpensInTime in internal/web/archived_bench_test.go, which runs in every
  # go test ./internal/web: a task of about 4,000 events is archived and GET /api/tasks/{task} is timed cold
  # (gunzip the blob and fold its events) and warm (from the cache); the test logs both times.
  # Measured on 2026-10-25 on a development machine, 4,001 events: cold 16.9 ms, warm 2.4 ms.
  # TestArchivedTaskOpensBench in the same file measures the largest task of a real log when AGENTRY_BENCH_DB is set.
  Scenario: An archived task opens in time
    # internal/web/archived_bench_test.go coldRead = 100 ms, warmRead = 10 ms
    Given an archived task of about 4,000 events
    When its page is read for the first time since the server started
    Then the read takes at most 100 ms, and measured 16.9 ms
    When it is read again
    Then the read takes at most 10 ms from the cache of the last few opened, and measured 2.4 ms

  # A timed-out approval kept open and retried

  @REL-35 @test
  # internal/hooks/handler_test.go
  Scenario: An ask the user does not answer in time is held for the user, and the agent is told no
    # internal/hooks/handler.go hold and Held
    Given an agent's command the policy asks the user about
    When no answer comes before the hook's wait runs out
    Then the hook's reply is deny, saying the command is held for the user's approval and not to retry it now
    And the action is recorded as held, with its decision undecided
    And the task's status is Needs you
    And 0 hooks are waiting on the task

  @REL-36 @test
  # internal/hooks/handler_test.go
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

  @REL-37 @test
  # internal/hooks/handler_test.go
  Scenario: A pass never overrides a denial, and a late denial gives no pass
    # internal/hooks/handler.go: the policy is judged first, the pass skips only the question to the user
    Given a held command the user approved late, and a policy that now denies that command
    When the agent asks for it in its next turn
    Then it is denied: yes, and the pass is kept for a later ask
    Given a held command the user denied late
    When the agent asks for it in its next turn
    Then it passes: no

  # A held reply before a hook's deadline

  @REL-38 @test
  # internal/turns/turns_test.go
  Scenario: The hook's deadline is under the engine's own timeout, so the gate fails closed
    # internal/engines/engine.go GateTimeoutSeconds = 1800; internal/turns/turns.go environment: deadline = 1800 - 10 = 1790 seconds
    # internal/hookcall/hookcall.go EnvDeadline = AGENTRY_HOOK_DEADLINE
    Given a turn's environment as the runner builds it
    Then AGENTRY_HOOK_DEADLINE is 1790 seconds, which is 10 seconds under the engine's 1800 second hook timeout: yes

  @REL-39 @test
  # internal/hooks/handler_test.go
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

  @REL-40 @test
  # internal/hooks/handler_test.go
  Scenario: An ask is held and answered before the deadline the hook sent
    # internal/hooks/handler.go Handle and hold
    Given a hook call asking about a command with a deadline of 1 second and no answer from the user
    When the server handles it
    Then the reply comes in under 1 second: yes
    And the reply is deny and says not to retry: yes
    And the action is held: yes

  # A worker going offline during a turn

  @REL-41 @test
  # internal/machines/fleet_test.go
  Scenario: A worker that went offline is taken to still run its turns for 30 seconds
    # internal/machines/offline.go Offline.IsRunning; internal/machines/fleet.go OfflineGrace = 30 seconds
    Given a worker whose link dropped less than 30 seconds ago, with an agent's turn running on it
    When the watchdog asks whether the task's agent is running
    Then the answer is yes, so the watchdog raises 0 issues for it
    Given a worker whose link dropped 30 seconds ago or more
    When the watchdog asks whether the task's agent is running
    Then the answer is no, so the turn is judged as a dead process would be

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

  @REL-43 @test
  # internal/coordinator/loop_test.go
  Scenario: The coordinator asks the user when the agent's only machines are offline
    # internal/coordinator/loop.go startTurn: the fleet is waited for OfflineGrace, then the user is asked
    Given a worker that joined and is offline, and an engineer who may run only on it
    When the coordinator assigns the task to the engineer
    Then the task asks the user a question saying the engineer can't run anywhere now because the worker is offline
    And the task has 1 turn, the coordinator's: no engineer turn was started

  @REL-44 @test
  # internal/workers/workers_test.go
  Scenario: A join code works once, for 1 hour
    # internal/workers/workers.go InviteLifetime = 1 hour
    Given a join code made now
    When a machine joins with it within 1 hour
    Then it joins: yes
    When the same code is used again, or a wrong code
    Then it joins: no
    When the code is used 1 hour and 1 second after it was made
    Then it joins: no
