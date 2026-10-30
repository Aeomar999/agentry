Feature: Performance
  Measured numbers: how long things take.

  @PERF-01 @benchmark
  Scenario: Writes to the log are not held up by a reader
    Given a log with 1 reader that is not reading
    When 1,200 events are written
    Then the writes take 0.07 seconds, as measured on 2026-10-16
    And the reader then reads all 1,200 back, in order
    # go test ./internal/core/store -run Reader

  @PERF-02 @benchmark
  Scenario: A fresh server answers its page within 0.1 second of starting
    Given an empty data dir, the agentry binary built from the repo, and a machine with 32 CPUs
    When agentry serve starts on a loopback port with -budget 0 -max-turns 0 -archive-after 0, and the page at / is polled every 50 ms
    Then the first HTTP 200 comes within 0.1 second: 83, 87 and 86 ms in 3 runs, median 86 ms, at the poll's 50 ms resolution
    And the fresh agentry.db is 86016 bytes afterwards, every run
    # measured 2026-10-25 at commit 7478feb3 with bench/startup.sh in the task's shared files: timeout 30 ./startup.sh <binary> <datadir> 7913; the log is written by the server on its first start

  @PERF-03 @benchmark
  Scenario: The task list on a fresh server is answered in under a millisecond
    Given the fresh server of the scenario above, with 0 tasks, and its UI token from the data dir
    When GET /api/tasks is requested 10 times in a row with the token as a bearer token, one after the other
    Then every answer is an empty list of 3 bytes
    And the median wall time of a request is 0.2 ms, the slowest 0.3 ms, as curl's total time measures it
    # measured 2026-10-25 at commit 3c1976b5 with bench/tasklist.sh in the task's shared files: timeout 60 ./tasklist.sh <binary> <datadir> 7913 10; the list is built from the state's summaries in internal/domain/tasks/tasksapi/tasks_api.go listTasks, not from the log

  @PERF-04 @test
  Scenario: Every API read says the log position it was read at
    Given a server with 2 tasks created through the API
    When the task list, a task, its events, the questions, the agents, the config, the policy, the settings, the features and a task that does not exist are read
    Then every read's position header is the log's last position
    And a POST and the page carry no position header
    And the position is taken before the read, so an event written during the read is not in the position it reports
    # internal/web/api_test.go TestAPIReadsSayThePositionTheyWereReadAt and TestThePositionIsTakenBeforeTheRead

  @PERF-05 @test
  Scenario: A restart goes on from the snapshot instead of replaying the log
    Given a data dir with no snapshot and a leftover temporary file of a snapshot write that never finished
    When agentry serve starts, stops cleanly, and starts again
    Then the first start logs "there is no snapshot", so the whole log is replayed, and the leftover file is not there afterwards
    And the snapshot state.snapshot is in the data dir with mode 0600 after the first stop
    And the second start does not log "replayed": it goes on from the snapshot
    # cmd/agentry/serve_test.go TestServeKeepsTheSnapshotAcrossARestart; internal/core/current/resume.go Restore logs the fallbacks

  @PERF-06 @test
  Scenario: A snapshot is written after enough events, or after the interval with at least one
    Given a log opened as serve opens it, with a snapshot writer set to write every 3 events, and a clock the test ticks for the interval
    When the start replays the whole log, then 3 events are applied, then the interval passes with no event, then 1 event is applied and the interval passes, then the writer closes
    Then the writer has written exactly 2 snapshots after the replay and the 3 events: one at the start, one for the events
    And the interval with no event adds 0 writes
    And after 1 event, the interval and Close there are exactly 3 writes in all
    And the next start goes on from the last snapshot's position and does not log "replayed"
    # internal/core/current/writer_test.go TestTheWriterWritesAfterEnoughEventsOrTimeWithAnEvent; the shipped counts are in the scenario below

  @PERF-07 @manual
  Scenario: The shipped snapshot writer writes every 10,000 events or every 10 minutes
    Given agentry serve running, and the file state.snapshot in its data dir with its modification time noted
    When 10,000 events are applied since the last snapshot, or 10 minutes pass with at least 1 event applied in them
    Then the snapshot file's modification time moves exactly once for that run of events or that interval
    And 10 minutes with 0 events applied moves it 0 times
    # internal/core/current/writer.go snapshotEvery = 10_000 and snapshotInterval = 10 minutes; no test asserts the shipped numbers, writer_test.go sets its own

  @PERF-08 @test
  Scenario: A proposal is not held up by a snapshot write under way
    Given a log with a snapshot writer whose write is held part way through, after the state was copied
    When a proposal is made while the write is held
    Then the proposal commits within 5 seconds, without waiting for the writer
    And the state's position moves by exactly 1
    And once the write is released a snapshot at the new event's position is written: yes
    # internal/core/current/writer_test.go TestAProposalCommitsWhileTheWriterIsMidWrite; the writer holds no lock of the store's, internal/core/current/writer.go

  @PERF-09 @manual
  Scenario: Startup on a real log is measured by the user with the startup benchmark
    Given a read-only copy of a real agentry.db, which no agent may read, and AGENTRY_BENCH_DB naming the copy
    When the user runs, from the repo root, AGENTRY_BENCH_DB=<copy>/agentry.db go test ./internal/core/current -run TestStartupBench -v -count=1 -timeout 30m
    Then the test grows a synthetic log of 1 GB from the copy, 10 with AGENTRY_BENCH_GB=10, and fails if its mix of events differs from the copy's by more than 5 percent
    And it reports the time from store.Open to Loaded on a fresh state, the fastest of 3 runs, against the full replay limit of 2.5 seconds at 1 GB and 5 seconds up to 10 GB
    And it reports the live file's size and the heap the state holds against their formulas, the snapshot tail of 10,000 events, startup from a snapshot of the working set on 1 and 2 cores, and a snapshot write on 1 core
    And it fails on no target: every target is logged, not asserted; it fails only on the mix, a broken stream, a synthetic log with no working set, or an AGENTRY_BENCH_GB or AGENTRY_BENCH_WORKING value that is not a number above 0
    # docs/design/log-growth.md section 6; internal/core/current/startup_bench_test.go TestStartupBench, skipped without AGENTRY_BENCH_DB; the limits are logged in internal/core/current/snapshot_bench_test.go benchSnapshots

  @PERF-10 @benchmark
  Scenario: The state holds under 1 KB of heap per archived task
    Given 1,000 archived tasks, each kept in the state as its summary alone, made from a task with every kind of event
    When the state holding only those summaries is measured after a garbage collection
    Then the heap per archived task is under 1,000 bytes, the limit the test logs it against: 818, 829 and 824 bytes in 3 runs
    And the state lists exactly 1,000 summaries
    # docs/design/log-growth.md section 6, Target 5; measured 2026-10-25 with GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=mod timeout 600 go test ./internal/core/current -run TestTheSummaryHeapBenchRunsOnAFixture -v -count=1, which prints the bytes after runtime.GC: 818 B at commit 3c1976b5 (engineer), 829 B (reviewer) and 824 B at 6d3f2dcb (engineer); the number varies by run, the test fails on the count only, not the bytes

  @PERF-11 @test
  Scenario: A command with far too many values is judged within a second
    Given the shipped policy, and a 7,300-byte command: a loop over 64 values, 600 assignments from it, then git $c
    When the read-only profile judges it
    Then the verdict comes within 1 second, 30 under the race detector
    And the call is asked, as a variable with too many values is unknown, under read-only, read-mostly, network-diagnosis and developer
    # README.md, The policy; internal/domain/policy/default_test.go TestAVariableWithTooManyValuesIsJudgedQuickly and TestAVariableWithTooManyValuesIsUnknown

  @PERF-12 @test
  Scenario: A word with too many braces is judged within a tenth of a second
    Given the shipped policy, and ls with a word of 20,000 nested braces, 40,001 bytes, or with 257 brace groups, 771 bytes
    When the read-only profile judges each
    Then each verdict comes within 0.1 second, 3 seconds under the race detector
    And each call is asked
    # README.md, The policy; internal/domain/policy/default_test.go TestAWordWithManyBracesIsJudgedQuickly; the cap of 256 brace groups is internal/domain/policy/shell_braces.go maxBraces

  @PERF-13 @test
  Scenario: A wait for changes answers as soon as the log moves
    Given a client waiting on GET /api/changes with after set to the log's last position
    When an event is written 100 ms later
    Then the answer comes within 5 seconds
    And it names the new position, exactly 1 past the one waited on
    # internal/web/api_test.go TestChangesAnswerAsSoonAsTheLogMoves

  @PERF-15 @test
  Scenario: The page's resource cache reads each path once per move of the log, however many watchers share the path
    # ui/src/live/resource-cache.test.ts "a refresh reads each path once, and the poll's jump to the newest position reads nothing again", "one event reads each shown path once more", "events arriving during a read give one more read, not one each, and none is aborted" and "a failed read shows its error, keeps the last data, and is tried again when the log moves"
    Given a resource cache watched as the tasks page mounts: the shell and the page both watch the tasks and the agents, and the shell the questions
    When the watchers first read, then 1 event arrives, then events 42 to 50 arrive while the read for event 41 is out
    Then each of the 3 paths is read exactly once for all its watchers on the first read, and once more for the 1 event
    And events 42 to 50 give exactly 1 more read once the read for 41 answers, not 1 each, and the read out is not aborted
    And a failed read shows its error, keeps the last data and is read again when the log moves: yes
    # not under test, checked by hand: until its first read answers, the tasks list says "Loading the tasks…" instead of showing an empty table (ui/src/pages/TasksPage.tsx line 287)

  @PERF-16 @test
  Scenario: Listing ideas reads the state in memory and sends no query to the database
    # internal/extensions/ideas/routes_test.go TestListingIdeasSendsNoQuery
    Given a server with 2 ideas suggested in 2 repos, 1 of them then dismissed through the API
    When GET /api/ideas and GET /api/ideas?repo=<the open idea's repo> are read
    Then the first lists both, newest first, and the second only the open one
    And the database is sent exactly 0 queries for the 2 reads
    # README.md, Ideas

  @PERF-14 @manual
  Scenario: A wait for changes with no event ends after 25 seconds
    Given a running server with no agent working and no one at the page, and the log's last position from the position header of any API read
    When the user requests GET /api/changes with after set to that position, with the UI token, and times it
    Then the answer comes after 25 seconds, within 1 second of it
    And the position it names is the same one waited on
    # internal/web/api.go waitForChanges, a 25 second timeout on the wait; no test asserts the timeout
