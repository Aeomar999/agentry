Feature: Limits
  Budgets, turn limits, sizes and caps.

  @LIM-01 @test
  Scenario: A task stops at its token budget
    Given a task with a token budget of 100k tokens
    When its turns have used 110k tokens
    Then the coordinator's next move asks the user, not a turn, the question saying 110k tokens of its 100k budget: yes
    And after the user's answer the next move is the coordinator's turn: yes
    # not under test, checked by hand: the question goes on "Raise the budget to go on, or tell the coordinator to wrap up." (internal/runtime/coordinator/next.go line 70), and after that one turn the budget asks again, as it waits for a turn since the answer (line 67)
    # README.md, Use it, Budgets and turn limits; internal/runtime/coordinator/next_test.go TestTheBudgetSendsTheTaskToYou

  @LIM-02 @manual
  Scenario: A new task's budget is 5,000,000 tokens unless the flags say otherwise
    Given agentry serve runs with no -budget flag
    When the user runs agentry serve -h and then creates a task from the page without a budget
    Then the help shows -budget with the default 5000000 and says 0 is no limit
    And the task's Details show a budget of 5,000,000 tokens
    # README.md, Use it; the flag is set in cmd/agentry/serve.go

  @LIM-03 @manual
  Scenario: A negative -budget or -max-turns flag is refused at start
    Given the user runs agentry serve -budget -1, and then agentry serve -max-turns -1
    When the flags are parsed
    Then each run exits 2 and says "-budget and -max-turns are 0 or above, 0 for no limit"
    And no server runs
    # README.md, Use it; the check is in cmd/agentry/serve.go

  @LIM-04 @test
  Scenario: On the API and the MCP tool, 0 takes the default and -1 is no limit
    Given a server whose default budget is 5,000,000 tokens and default turn limit is 2
    When tasks are created with no limits given, with budget 200,000 and maxTurns 50, with budget -1 and maxTurns -1, and with budget 0 and maxTurns 0, and through create_task with no limits
    Then the first and the fourth task get a budget of 5,000,000 and 2 turns
    And the second task gets a budget of 200,000 and 50 turns
    And the third task has no budget and no turn limit
    And a task created with maxTurns -5 is refused with HTTP 409
    And the config endpoint shows a budget of 5,000,000 and 2 turns
    And the MCP tool's create_task with no limits sends budget 0 and maxTurns 0 to the API: yes
    # not under test, checked by hand: the MCP tool sends a budget or maxTurns of -1 to the API as given (internal/mcp/tools.go line 308)
    # README.md, Use it from Claude Code; internal/domain/tasks/tasksapi/tasks_api_test.go TestANewTaskHasItsLimits; the MCP tool: internal/mcp/server_test.go TestTheBoardAndHandingOverWork

  @LIM-05 @test
  Scenario: A task that comes from another takes that task's limits
    Given the server's defaults of 5,000,000 tokens and 24 turns, a task with a budget of 700 tokens and 3 turns, and a task with no limits
    When a new task takes its limits from the first task, another from the unlimited task, and a third from no task
    Then the one from the first task has a budget of 700 and 3 turns, the same as its source
    And the one from the unlimited task has no budget and no turn limit
    And the one from no task takes the defaults, 5,000,000 tokens and 24 turns
    And with unlimited defaults, the one from no task has no budget and no turn limit
    # README.md, Use it; internal/domain/tasks/limits_test.go TestANewTaskTakesItsSourcesLimitsElseTheDefaults

  @LIM-06 @test
  Scenario: Raising the budget on the page answers the budget stop
    Given a task stopped at its budget
    When the user raises the budget above what the task has used, or sets it to no limit
    Then the raised task is answered with "I raised the budget to <N> tokens. Carry on.", N being the new budget, and is working again: yes
    And the unlimited task is answered with "I made the budget unlimited. Carry on." and is working again: yes
    # README.md, Use it; internal/domain/tasks/tasksapi/tasks_api_test.go TestRaisingTheBudgetLetsATaskGoOn

  @LIM-07 @test
  Scenario: A task has 24 turns without the user's answer before it asks
    Given a task created with the default turn limit
    When its 24th turn since the user last answered ends without finishing
    Then the coordinator's next move asks the user, not a turn: yes
    And the question says "24 of its 24 turns"
    And after the user's answer the next move is a turn: yes
    # not under test, checked by hand: the loop records the ask as task.needs_you, shown in Needs you (internal/runtime/coordinator/loop.go line 325); the question goes on " without finishing. Raise the limit, or answer with any guidance, and the coordinator carries on." (internal/runtime/coordinator/next.go line 65); the answer sets the count of turns since it to 0 (internal/domain/tasks/apply.go line 463); the answer hands the task to the coordinator (internal/domain/tasks/apply.go line 464)
    # README.md, Use it; internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou

  @LIM-08 @test
  Scenario: A negative budget or turn limit is refused by the rules
    Given a task on the board
    When an event sets its budget to -1, and another sets its turn limit to -3
    Then the budget event is refused: yes
    And the turn limit event is refused: yes
    # not under test, checked by hand: the refusals say "a budget is a number of tokens, or 0 for no limit" and "a limit of turns is a number of turns, or 0 for no limit" (internal/core/rules/task_rules.go lines 135 and 143)
    # README.md, Use it; internal/core/rules/rules_test.go TestBudgetsAndLimitsOfTurns

  @LIM-09 @test
  Scenario: The most tasks running at once is a setting from 0 to 100
    Given the setting maxRunningTasks, 0 by default, which is no limit
    When the user sets it to 2 and then to 100
    Then the settings answer with maxRunningTasks 2, then 100, the highest allowed
    # README.md, Use it; internal/domain/settings/settingsapi/settingsapi_test.go TestYouSetTheMostTasksRunningAtOnce; the default: TestYouTurnTheVisionariesAfterAnEpicOffAndOn

  @LIM-10 @test
  Scenario: A most-tasks setting out of range is refused
    Given maxRunningTasks is 2
    When the user sets it to -1, 101, 1.5, "3", true or null
    Then each is refused with HTTP 400
    And the setting stays 2
    # README.md, Use it; internal/domain/settings/settingsapi/settingsapi_test.go TestYouSetTheMostTasksRunningAtOnce

  @LIM-11 @test
  Scenario: Tasks over the limit wait in the user's priority order
    Given maxRunningTasks is 2, and 2 tasks on the board have a turn open
    When a third task wants a turn
    Then the loop starts no turn for it
    And waiting tasks are admitted in priority order, the newer task first on a tie, never the older
    And only tasks on the board with a turn open take a slot, so a wrap-up turn takes none
    And when one running task ends its turn exactly 1 waiting task is started
    # README.md, Use it; internal/runtime/coordinator/slots_test.go TestOnlyTasksInProgressTakeASlot, TestSlotsGoInYourPriorityOrder, TestATieOfRankAdmitsTheNewerTask, TestALimitUnderWhatRunsQueuesEveryOneWaiting and TestTheLoopRunsNoMoreTasksThanTheLimit

  @LIM-12 @manual
  Scenario: A team may ask another team at most 3 requests deep
    Given two custom teams, and a task that is itself the 3rd request in a chain of teams asking teams
    When the user asks the task's coordinator in its chat to request work from the other team
    Then the decision is refused with "this task is already 3 requests deep; do the work in this team, or ask the user"
    And the refusal is shown in the task's Activity
    # README.md, Use it; the depth is set in internal/runtime/coordinator/next.go

  @LIM-13 @manual
  Scenario: One split adds 1 to 12 stories
    Given an epic whose coordinator is deciding
    When the user asks the coordinator in its chat to split the epic into 13 one-line stories, and then into 12
    Then the split of 13 is refused with 'a "split" decision needs "stories": 1 to 12 of them', shown in Activity
    And the split of 12 files exactly 12 stories in the Backlog
    # README.md, Use it; the cap is set in internal/runtime/coordinator/decision.go

  @LIM-14 @test
  Scenario: Three issues in a row stop the task
    Given a task whose last 2 turns ended in an issue, with no turn ending normally in between
    When a 3rd issue is raised
    Then the coordinator's next move asks the user, not a turn: yes
    And the question is "3 issues in a row, the last: " followed by the issue's reason and ". Fix what's wrong and answer, and the coordinator carries on."
    And a retry still counts as an issue, so retries are bounded at 3
    # not under test, checked by hand: the loop records the ask as task.needs_you, shown in Needs you (internal/runtime/coordinator/loop.go line 325); a turn that ends normally, or the user's answer, sets the count of issues in a row to 0 (internal/domain/tasks/apply.go lines 386 and 463)
    # README.md, How a task moves; internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou; the retry: internal/domain/tasks/retry_fresh_test.go TestARetryFreshIssueHandsTheSameStepBack and internal/runtime/coordinator/next_test.go TestIssuesInARowEndingInARetryQuoteTheIssue

  @LIM-15 @test
  Scenario: Done tasks are archived after 30 days unless the flag says otherwise
    Given agentry serve with no -archive-after flag, and another with -archive-after=48h
    When the flags are parsed
    Then the first archives tasks done for exactly 720h
    And the second archives tasks done for exactly 48h
    # README.md, Archiving, erasing and redacting; cmd/agentry/serve_test.go TestServeParsesArchiveAfter

  @LIM-16 @test
  Scenario: With -archive-after=0 nothing is archived
    Given agentry serve runs with -archive-after=0 and a task done long ago
    When the server starts
    Then no task is archived, however long it has been done
    And the log says "archiving is off (-archive-after=0)"
    # README.md, Archiving, erasing and redacting; cmd/agentry/serve_test.go TestServeArchivesOnlyWithArchiveAfter

  @LIM-17 @test
  Scenario: A negative -archive-after is refused at start
    Given the user runs agentry serve -archive-after=-1h
    When the flags are parsed
    Then the run exits 2 and says "-archive-after is 0 or above"
    # README.md, Archiving, erasing and redacting; cmd/agentry/serve_test.go TestServeParsesArchiveAfter

  @LIM-18 @test
  Scenario: The archiver runs at start and then every hour
    Given tasks done for more and for less than the age, one of them with a turn open, and an epic with a story still in the state
    When the archiver starts
    Then one round runs at once, and one more every 60 minutes
    And every done task older than the age is archived
    And the task with an open turn is not archived in that round
    And the epic is not archived while any of its stories is still in the state
    And a task brought back is not archived again until the age has passed since it was brought back
    # not under test, checked by hand: a round takes the tasks in creation order (internal/runtime/archive/archive.go line 148)
    # README.md, Archiving, erasing and redacting; internal/runtime/archive/archive_test.go TestTheArchiverRunsAtStartAndThenHourly, TestATaskIsArchivedOnceDoneForTheAge, TestATaskWithATurnRunningWaitsForTheRoundAfterItEnds, TestAnEpicWaitsForItsStories and TestAnUnarchivedTaskWaitsTheAgeAgain; at the server's start: cmd/agentry/serve_test.go TestServeArchivesOnlyWithArchiveAfter

  @LIM-19 @test
  Scenario: A signed webhook's request body is at most 1 MiB
    Given a signed webhook route
    When a request's body is exactly 1,048,576 bytes, and another's is 1,048,577 bytes
    Then the body of 1,048,576 bytes is accepted
    And the body of 1,048,577 bytes is refused with HTTP 400
    # not under test, checked by hand: a route that takes JSON has the same cap, MaxBody = 1 << 20 (internal/core/httpapi/httpapi.go lines 23 and 28)
    # README.md, Features' settings and secrets; internal/web/extensions_api_test.go TestASignedRouteTakesABodyOfExactlyMaxBody and TestASignedRouteRefusesAnOversizedBody

  @LIM-20 @manual
  Scenario: A shared file is at most 2 GiB
    Given a task with a shared files folder, and a file of 2,147,483,649 bytes made with truncate in a scratch folder
    When the user uploads it from the task's Workspace, and then a file of 2,147,483,648 bytes
    Then the first upload is refused with "a shared file is at most 2 GB"
    And the second file is kept
    And no cap holds on the number of files or their total size
    # README.md, Use it; the cap is set in internal/domain/sharedfiles/blobs.go

  @LIM-21 @test
  Scenario: A task's events page takes a limit of at most 2,000
    Given a task with events
    When the page asks for the task's events with ?limit=2000, with ?limit=2001 or ?limit=0, and with 100 or 101 ?type= filters
    Then ?limit=2000 is answered: yes
    And ?limit=2001 and ?limit=0 are refused with HTTP 400
    And 100 ?type= filters are taken, the 101st refused with HTTP 400
    # not under test, checked by hand: a page with no limit holds at most 500 events (internal/domain/tasks/tasksapi/events_api.go line 18), and the refusal says "limit is from 1 to 2000" (line 92)
    # README.md, Use it; internal/domain/tasks/tasksapi/events_api_test.go TestAnEventPageOfNoTaskOrWithBadBoundsIsRefused

  @LIM-22 @test
  Scenario: A live terminal is sized between 20 by 5 and 500 by 200
    Given a task's session with a viewer in control
    When the viewer in control asks for 9999 columns by 1 row
    Then the other viewer sees the session sized at 500 columns by 5 rows: yes
    # not under test, checked by hand: the bounds are 20 to 500 columns and 5 to 200 rows (internal/platform/terminal/viewers.go lines 117 and 118)
    # README.md, Use it; internal/platform/terminal/terminal_test.go TestAViewerResizesTheSession

  @LIM-23 @manual
  Scenario: A viewer too slow for the live terminal is disconnected
    Given a viewer attached to a busy task's terminal with a websocket client that never reads
    When 4,096 output chunks queue for it unread, or 16 state messages
    Then that viewer is disconnected, exactly once
    And the session and the other viewers are not affected, so the page's terminal keeps streaming
    # README.md, Use it; the backlog sizes are set in internal/platform/terminal/viewers.go

  @LIM-24 @test
  Scenario: A push notification fits one 4,096-byte record
    Given a device subscribed to push notifications
    When Agentry sends a notification
    Then its message is at most 3,993 bytes, sealed into a 4,096-byte record
    And a message of 3,994 bytes is refused as too long for a push
    And the push lives 86,400 seconds, 24 hours, at the push service
    # README.md, Use it; internal/domain/notifications/webpush/send_test.go TestAMessageTooLongForAPushIsRefused; the lifetime: TestAPushReachesThePushServiceAndOpensForTheBrowser

  @LIM-25 @test
  Scenario: Long text quoted into a brief is cut with a marker
    Given text longer than the place it is quoted into
    When it is cut
    Then it ends with " [...]", exactly once
    And text that fits is kept whole, with no marker
    # README.md, How a task moves; internal/domain/tasks/shorten_test.go TestShortenCutsBetweenCharacters

  @LIM-26 @manual
  Scenario: What a brief quotes is cut at a fixed size
    Given an engineer's report of over 4,000 bytes, a story summary of over 1,500 bytes and an epic description of over 2,000 bytes
    When the coordinator's next brief is written in the task worktree, the story's summary reaches its epic, and a story's brief quotes its epic
    Then the trail entry in the coordinator's brief is at most 4,000 bytes and ends with " [...]"
    And a refused command quoted in the brief is at most 200 bytes
    And the story's summary quoted to its epic is at most 1,500 bytes
    And the epic's description quoted to a story is at most 2,000 bytes
    And an epic's closing summary quoted to a follow-up is at most 4,000 bytes
    # README.md, How a task moves; the sizes are set in internal/runtime/coordinator/brief.go, epics.go and followups.go

  @LIM-27 @manual
  Scenario: A task has at most 12 labels of at most 32 bytes
    Given a task on the board with its Details open
    When the user adds 13 labels, and then one label of 33 characters
    Then the 13 labels are refused with "at most 12 labels"
    And the label of 33 characters is refused
    And an empty label, or one with spaces around it, is refused
    # README.md, Use it; the caps are set in internal/core/rules/task_rules.go

  @LIM-28 @test
  Scenario: The watchdog's limits on a turn
    Given a turn is open and the watchdog ticks every second
    When the watchdog measures the turn
    Then a process not up 3 seconds after the start is an issue
    And a prompt not echoed after 60 seconds puts the task in Needs you, on a question that names trusting the repository
    And no event at all for 10 minutes is a stall, unless the turn waits for the user
    And one approved tool call with no result after 20 minutes is an issue
    And the same command requested 5 times in a row is an issue
    # not under test, checked by hand: the watchdog checks once a second (internal/runtime/watchdog/watchdog.go line 73), the tests call Check with the clock moved on; the prompt's question says to open the task's terminal (internal/runtime/watchdog/watchdog.go lines 116 to 118)
    # README.md, How a task moves; internal/runtime/watchdog/watchdog_test.go TestACrashIsFlaggedAfterTheGrace, TestAPromptThatNeverArrivesWaitsForYou, TestASilentTurnIsAStall, TestWaitingForYouIsNotAStall, TestAToolOpenTooLong and TestTheSameCommandRepeated

  @LIM-29 @test
  Scenario: A worker join code works once, until it expires
    Given the user makes a worker join code
    When a machine joins with it, and another tries the same code
    Then the first machine joins: yes
    And the second try is refused as unknown, used or expired
    And the code no longer works once its lifetime has passed: yes
    And the join's event holds neither the code nor the worker's credential as they are: yes
    # not under test, checked by hand: the lifetime, InviteLifetime, is 1 hour (internal/domain/workers/workers.go line 28)
    # README.md, Add machines; internal/domain/workers/workers_test.go TestAMachineJoinsOnceWithItsCode; the refusal's words: internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  @LIM-30 @manual
  Scenario: A Claude Code invite works for 15 minutes
    Given the user makes a Claude Code connection invite on the Integrations page
    When a Claude Code session connects with it 16 minutes later
    Then the connection is refused
    And the invite works for exactly 15 minutes from when it was made
    # README.md, Use it from Claude Code; the lifetime is set in internal/domain/connections/connections.go

  @LIM-31 @manual
  Scenario: A worker is offline after 20 seconds of silence
    # internal/domain/workers/workerlink/server.go StatusEvery = 2 seconds, silentFor = 10 * StatusEvery
    Given a worker that reports its status every 2 seconds
    When it is silent for 20 seconds
    Then the server hangs up its link and the worker is shown offline
    And a worker heard from within 20 seconds is shown online
    # Steps: on <worker>, run agentry worker run in a terminal and open the server's Workers page: the worker's Status reads online;
    # pause the worker with kill -STOP <pid>, its pid from pgrep -f "agentry worker run": within about 22 seconds (the link is
    # checked every 2 seconds) the Status reads offline; kill -CONT <pid> and the Status reads online again once the worker dials back
    # README.md, Add machines

  @LIM-32 @test
  Scenario: Each task's worktree gets the lowest free dev port from 7900
    Given tasks that hold dev ports
    When a port is leased for a new worktree
    Then its PORT is the lowest from 7900 that no task holds and nothing listens on: yes
    # not under test, checked by hand: the range ends at 7999, so 100 ports are the most worktrees that can hold one at once (internal/platform/worktrees/ports.go lines 12 and 13)
    # README.md, Use it; internal/platform/worktrees/worktree_test.go TestLeasePortSkipsTakenAndBusyPorts

  @LIM-33 @manual
  Scenario: A turn's start and a clone time out
    Given a task whose repo is a git address that accepts the connection and never answers
    When the task starts and the clone runs
    Then the clone is stopped after exactly 10 minutes and the turn is an issue
    And a session that has not started 2 minutes after its turn began is an issue
    # README.md, Use it; the timeouts are set in internal/platform/worktrees/repo.go and internal/runtime/turns/turns.go

  @LIM-34 @manual
  Scenario: The server waits at most 10 seconds for a request's headers
    Given agentry serve is running
    When a client opens a connection to it and sends nothing
    Then the server closes the connection after 10 seconds
    And the server has no other read, write or idle timeout
    # README.md, Use it; the timeout is set in cmd/agentry/serve.go

  @LIM-35 @test
  Scenario: A handoff lists at most 40 touched files by name
    Given a commit under review that touches 41 files, and another that touches 40
    When the coordinator briefs a checking agent with the handoff
    Then for the 41 files the handoff does not list them one by one: it says "It touches <n> files; list them with `git diff --stat <base>..HEAD`.", n being 41
    And for the 40 files they are listed one by one, the 40th by name: yes
    # not under test, checked by hand: each file is one line (internal/runtime/coordinator/brief.go line 325)
    # README.md, How it works, How a task moves; the cap is mostFilesListed in internal/runtime/coordinator/brief.go; internal/runtime/coordinator/handoff_brief_test.go TestAHandoffCapsALongFileList
