Feature: The coordinator
  An ordinary agent that reads every report and ends each turn with a decision the loop carries out.

  @COORD-01 @test
  Scenario: An assign decision hands the task to one agent
    Given a task whose coordinator has a turn open
    When the coordinator ends its turn with an assign decision that names the engineer and its instructions
    Then the loop starts exactly 1 turn, the engineer's
    And the coordinator's instructions are shown in the engineer's brief
    # README.md, How a task moves; internal/coordinator/loop_test.go

  # --- A task's life

  @COORD-02 @test
  Scenario: A new task goes to its coordinator first
    # internal/coordinator/next_test.go TestNext
    Given a task was created for a team, with no turns yet
    When its worktree is ready
    Then the first turn is the coordinator's, and no other agent has had a turn
    And while an agent's turn is open, no other turn starts on the task

  @COORD-03 @test
  Scenario: The coordinator reads the task and the trail before anyone builds
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the coordinator's first turn on a task
    When its brief is written
    Then the brief shows its own prompt, the task, the agents it can hand work to, and every step of the trail so far
    And its prompt says it reads the task, the trail and the code and changes no code
    # the prompt: internal/agents/defaults/tech-lead.md and internal/agents/defaults/custom/coordinator.md

  @COORD-04 @test
  Scenario: The agent's plan for the turn is kept as it last wrote it
    # internal/hooks/handler_test.go TestThePlanFollowsTheFile
    Given a turn whose brief asked for a plan in .agentry/turn-N-plan.md before anything else, as a Markdown checklist
    When the agent writes the file and then ticks steps in it
    Then the plan kept on the turn is the same as the file's last content
    And the plan is kept when the turn ends
    # the plan file name: internal/machines/local.go PlanFile; the brief's wording: internal/coordinator/brief.go plan

  @COORD-05 @test
  Scenario: An unfinished plan carries over to the agent's next turn
    # internal/tasks/plan_test.go TestAnUnfinishedPlanCarriesOn
    Given an agent's last turn on the task left a plan with 1 or more steps not ticked
    When the agent's next turn on the task starts
    Then that plan is already in the new turn's plan file, kept as it was, and the brief says which turn it came from
    And a plan with every step ticked is not carried over

  @COORD-06 @test
  Scenario: Any decision may retitle the task
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given a task titled "Remove the TODOs"
    When the coordinator's assign decision also carries "title": "Remove the TODO comments in src/"
    Then the task's title is the new one and its first title is kept as "Remove the TODOs"
    And the decision format asks for 1 plain imperative line under 70 characters
    # 70 characters: internal/coordinator/decision.go DecisionFormat

  @COORD-07 @test
  Scenario: An assign decision hands the task to the named agent
    # internal/coordinator/next_test.go TestNext
    Given the coordinator's turn ended with {"action": "assign", "agent": "engineer", "instructions": "Remove them."}
    When the loop applies the decision
    Then the next turn is the engineer's, started on exactly those instructions
    And the step from the coordinator to the engineer is shown in the trail

  @COORD-08 @test
  Scenario: An assign to an agent that does not exist is an issue
    # internal/coordinator/next_test.go TestNext
    Given the coordinator's turn ended with an assign to "designer", and the roster has no agent of that name
    When the loop reads the decision
    Then no turn starts
    And an issue goes back to the coordinator saying there is no agent by that name

  # --- Briefs and reports

  @COORD-09 @test
  Scenario: An agent starts fresh with only its brief and the repository
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the coordinator assigned the engineer with the instructions "Remove every TODO in src/."
    When the engineer's turn starts in a fresh session
    Then its own prompt, the task, exactly those instructions, where the work is, the shared files and the plan file are shown in its brief
    And the brief's prompt is the engineer's own, not the coordinator's

  @COORD-10 @test
  Scenario: The brief is a file and the prompt is one line pointing at it
    # internal/turns/turns_test.go TestStartRunsTheAgentOnAOneLinePrompt
    Given a turn is about to start
    When Agentry starts the agent's session
    Then the brief is written to .agentry/turn-N.md and the prompt is exactly 1 line: "Read .agentry/turn-N.md and do what it says."
    And the turn records the hash of that line, which the engine echoes back once it has the prompt
    # the prompt line: internal/turns/turns.go start

  @COORD-11 @test
  Scenario: Every agent's report returns to the coordinator
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the engineer's turn ended with the report "Removed 3 TODOs and committed."
    When the loop reads the task
    Then the next turn is the coordinator's, and no other agent's turn starts first
    And the report is shown in the coordinator's brief as the step "engineer → tech-lead: Removed 3 TODOs and committed."

  @COORD-12 @test
  Scenario: The coordinator's reply ends with exactly one JSON decision
    # internal/coordinator/decision_test.go TestParseDecision
    Given a coordinator's reply
    When Agentry reads the JSON object at its end
    Then the action is exactly 1 of these 9: assign, done, ask, transfer, request, wait, fetch, move, split
    And a decision missing what its action needs, such as an assign with no agent, is refused with the reason
    # the 9 actions: internal/coordinator/decision.go, the Action constants and check

  @COORD-13 @test
  Scenario: A reply with no usable decision is an issue
    # internal/coordinator/next_test.go TestNext
    Given the coordinator's turn ended with "I think the engineer." or with no reply at all
    When the loop reads the task
    Then no agent's turn starts
    And an issue goes back to the coordinator saying the reply had no usable decision

  # --- Issues

  @COORD-14 @test
  Scenario: An issue goes back to the coordinator
    # internal/coordinator/next_test.go TestNext
    Given the watchdog raised an issue "the agent's process exited" on a task
    When the loop reads the task
    Then the next turn is the coordinator's and its handoff is exactly the issue's reason
    And the task's count of issues in a row goes up by 1
    # IssuesInARow + 1 on IssueRaised: internal/tasks/apply.go

  @COORD-15 @test
  Scenario: A crashed agent is an issue within seconds
    # internal/watchdog/watchdog_test.go TestACrashIsFlaggedAfterTheGrace
    Given an open turn whose process has exited
    When more than 3 seconds have passed since the turn started
    Then the watchdog raises exactly 1 issue saying the agent's process exited (crashed or was killed)
    # StartGrace = 3 * time.Second: internal/watchdog/watchdog.go DefaultLimits; checked every second: Run

  @COORD-16 @test
  Scenario: A silent agent is a stall
    # internal/watchdog/watchdog_test.go TestASilentTurnIsAStall
    Given an open turn with no tool call running and no action waiting for the user
    When the turn has had no events for more than 10 minutes
    Then the watchdog raises exactly 1 issue saying the agent has been silent for over 10 minutes
    # Silence = 10 * time.Minute: internal/watchdog/watchdog.go DefaultLimits

  @COORD-17 @test
  Scenario: An agent waiting for the user's answer is not stalled
    # internal/watchdog/watchdog_test.go TestWaitingForYouIsNotAStall
    Given an open turn with an action the policy asked the user about, not yet decided
    When more than 10 minutes pass with no events
    Then the watchdog raises no issue

  @COORD-18 @test
  Scenario: A tool call with no result for too long is an issue
    # internal/watchdog/watchdog_test.go TestAToolOpenTooLong
    Given an approved tool call in an open turn, with no result yet
    When it has run for more than 20 minutes
    Then the watchdog raises exactly 1 issue naming the command
    # Tool = 20 * time.Minute: internal/watchdog/watchdog.go DefaultLimits

  @COORD-19 @test
  Scenario: The same command requested again and again is an issue
    # internal/watchdog/watchdog_test.go TestTheSameCommandRepeated
    Given an open turn
    When the agent requests the same command 5 times in a row
    Then the watchdog raises exactly 1 issue saying the agent may be stuck in a loop, naming the command
    # Repeats = 5: internal/watchdog/watchdog.go DefaultLimits

  @COORD-20 @test
  Scenario: A denied action is shown to the coordinator, not raised as an issue
    # internal/coordinator/resumed_brief_test.go TestAResumedCoordinatorBriefKeepsTheDecisionChecksAndRefusals
    Given the policy denied an agent's command, such as git push origin HEAD:dev
    When the coordinator's next brief is written
    Then the command and the reason are shown under "Actions Agentry refused"
    And no issue is raised for the denial
    # the gate writes action.denied, no issue: internal/hooks/handler.go decide; the brief section: internal/coordinator/brief.go refusedActions

  @COORD-21 @test
  Scenario: A resumed session that did not start is retried fresh once
    # internal/coordinator/retry_fresh_test.go TestAFailedResumeIsRetriedFreshOnce
    Given an agent's turn went on with its last session, and the process exited before taking the prompt
    When the watchdog sees it after the 3 second grace
    Then it raises exactly 1 issue saying the session didn't start, and the same agent gets the same step again, fresh
    And the coordinator is only told, with no turn of its own
    And the retry still counts as 1 issue towards the issues in a row
    # retryFresh: internal/watchdog/watchdog.go checkTurn; the same step handed back: internal/tasks/apply.go IssueRaised

  @COORD-59 @test
  Scenario: An agent's report of a blocker is a report, not an issue
    # internal/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task with the default limit of 24 turns, where the coordinator assigns the engineer and the engineer ends its turn with the report "still broken", 12 times over
    When the loop reads the task after those 24 turns
    Then the 12 reports raised 0 issues, and the stop is the "turns" limit, with the words "24 of its 24 turns"
    And the issues limit is reached in the same test only by 3 issues raised with no turn ending between them
    # an issue is raised only for a failed turn or a usage limit that lasts: internal/hooks/handler.go, the UsageLimited and TurnFailed cases
    # a turn that ends with a report sets IssuesInARow to 0: internal/tasks/apply.go turn.ended

  # --- Safety stops

  @COORD-22 @test
  Scenario: The turn limit asks the user
    # internal/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task with the default limit of 24 turns
    When it has had 24 turns with no answer from the user
    Then no turn starts
    And the user is asked, with the words "24 of its 24 turns", and the stop is marked as the "turns" limit
    # DefaultMaxTurns = 24: internal/tasks/task.go; the stop at TurnsSinceAnswer >= MaxTurns: internal/tasks/task.go AtTurnLimit

  @COORD-23 @test
  Scenario: Each task has its own limit of turns, set when it is made
    # internal/coordinator/next_test.go TestEachTaskHasItsOwnLimitOfTurns
    Given a task created with maxTurns 4
    When it has had 4 turns with no answer from the user
    Then the user is asked, with the words "4 of its 4 turns"
    And a task created before each had its own limit is kept at 24
    # the server's default comes from the -max-turns flag, default coordinator.DefaultMaxTurns: cmd/agentry/serve.go

  @COORD-24 @test
  Scenario: A limit of 0 is no limit
    # internal/coordinator/next_test.go TestEachTaskHasItsOwnLimitOfTurns
    Given a task created with maxTurns 0
    When it has had 40 turns with no answer from the user
    Then the next turn starts and the user is not asked
    And a task stopped at its limit, then changed to 0, runs on
    # MaxTurns > 0 is the condition: internal/tasks/task.go AtTurnLimit; "0 for no limit": cmd/agentry/serve.go -max-turns and -budget

  @COORD-25 @test
  Scenario: The budget stop asks the user once the task has used it
    # internal/coordinator/next_test.go TestTheBudgetSendsTheTaskToYou
    Given a task with a budget of 100,000 tokens
    When its ended turns have used 110,000 tokens, and 1 or more turns have run since the user last answered
    Then no turn starts
    And the user is asked, with the words "110k tokens of its 100k budget", and the stop is marked as the "budget" limit
    And after the user answers, exactly 1 more turn runs, the coordinator's, so it can wrap up
    # AtBudget is Budget > 0 and TokensUsed >= Budget: internal/tasks/task.go; the stop needs TurnsSinceAnswer > 0: internal/coordinator/next.go Next
    # the server's default budget is 5,000,000 tokens, flag -budget: cmd/agentry/serve.go

  @COORD-26 @test
  Scenario: 3 issues in a row ask the user
    # internal/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task with 3 issues raised and no turn ending normally between them
    When the loop reads the task
    Then no turn starts
    And the user is asked "3 issues in a row, the last: the engine is missing. Fix what's wrong and answer, and the coordinator carries on."
    # MaxIssuesRun = 3: internal/coordinator/next.go

  @COORD-27 @test
  Scenario: The user's answer resets the counts
    # internal/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task stopped at its turn limit
    When the user answers
    Then the turns since the answer and the issues in a row are both 0, and the next turn starts
    # task.answered sets TurnsSinceAnswer and IssuesInARow to 0: internal/tasks/apply.go

  @COORD-28 @manual
  Scenario: A turn that ends normally resets the issues in a row
    Given a task with 2 issues in a row
    When an agent's turn ends with a report
    Then the issues in a row are 0, and 3 more issues are needed before the user is asked
    # turn.ended sets IssuesInARow to 0: internal/tasks/apply.go
    # Steps: on a task, let 2 turns crash (kill the agent's process), then let 1 turn end with a report;
    # read the task's JSON from the API and check IssuesInARow is 0.

  # --- Decisions

  @COORD-29 @test
  Scenario: An ask decision puts a question to the user, with optional options and a page
    # internal/coordinator/decision_test.go TestParseDecision
    Given the coordinator's reply ends with {"action": "ask", "question": "Which layout?", "options": ["Tabs", "One page"], "page": "<svg></svg>"}
    When the decision is read
    Then it is an ask with exactly that question, 2 options and the page
    And once applied, the task waits with the question open and no turn starts until the user answers
    # the wait while a question is open: internal/coordinator/next.go Next

  @COORD-30 @test
  Scenario: Only an epic with stories may wait
    # internal/coordinator/next_test.go TestWhenAnEpicMayWait
    Given a task that is not an epic, or an epic with no stories yet
    When its coordinator decides {"action": "wait"}
    Then the decision is refused as an issue saying only an epic with stories can wait
    And an epic whose stories are all done is refused too, with "finish it with a summary", until the user has answered once about closing it

  @COORD-31 @test
  Scenario: A fetch from inside a machine's worktrees comes at once, from anywhere else the user is asked first
    # internal/coordinator/files_test.go TestSharedFilesFollowTheWork
    Given the coordinator decides {"action": "fetch", "machine": "local", "path": "<a path>", "as": "notes.txt"}
    When the path is outside that machine's worktrees
    Then the user is asked whether to copy it, with the options "Copy it" and "Don't copy it"
    And the file is shared only once the user answers "Copy it"
    And a path inside that machine's worktrees is shared with no question asked
    # CopyFile: internal/coordinator/files.go fetch

  @COORD-32 @test
  Scenario: A move decision takes the task to a repo the user named
    # internal/coordinator/repos_test.go TestTheCoordinatorMovesATaskToTheRepoYouNamed
    Given the user named a git address or a folder for the task's work in its description or in what they told the coordinator
    When the coordinator decides {"action": "move", "repo": "<that address, as they wrote it>"}
    Then the task's repo is the new one, cloned once if it is an address, and its next turn runs in a new worktree of it
    And a move to a repo the user did not name is refused as an issue, and the task's repo is kept
    # namedByYou: internal/coordinator/next.go; TestARepoIsNamedOnlyWhole in the same file covers the whole-name match

  @COORD-33 @test
  Scenario: A transfer passes the task to another team's coordinator
    # internal/coordinator/teams_test.go TestATaskWithNoTeamChosenGoesToTheRightOne
    Given a task whose team the user did not choose, so the brief says to decide whose work it is
    When the coordinator decides {"action": "transfer", "team": "Writing", "reason": "It is a blog post."}
    Then the next turn is the Writing team's coordinator's, and the task's coordinator is now exactly that agent
    And "This task is your team's work: It is a blog post." is shown in its brief

  @COORD-34 @test
  Scenario: A request opens a task for another team and waits for its summary
    # internal/coordinator/teams_test.go TestACoordinatorAsksAnotherTeamAndCarriesOnWithTheAnswer
    Given a task of the Software team, and the Writing team is listed under "Other teams" in the brief
    When the coordinator decides {"action": "request", "team": "writing", "request": "Write the user guide for the TODO tool."}
    Then exactly 1 new task is made for the Writing team, with the request as its title, in the same repo, with this task as its parent
    And this task waits, with no turn starting, until that task is done
    And then the other team's summary comes back to this coordinator as exactly 1 step in the trail

  @COORD-35 @manual
  Scenario: A transfer or request goes only to a team named in the brief
    Given a task of the Software team
    When the coordinator decides a transfer or request to its own team, or to a team that does not exist
    Then the decision is refused as an issue naming the teams there are
    And a request from a task already 3 requests deep is refused
    # MaxRequestDepth = 3: internal/coordinator/next.go otherTeam
    # Steps: end a coordinator turn with {"action": "request", "team": "software", "request": "x"} on a Software task and
    # read the issue in the trail; repeat with "team": "nobody"; then chain 3 requests team to team and try a 4th.

  @COORD-36 @test
  Scenario: Done is refused until a checker has approved the last change and no checker objects
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the engineer's turn changed the work and no checker has looked at it since
    When the coordinator decides {"action": "done", "summary": "TODOs removed."}
    Then the task is not done
    And an issue goes back to the coordinator saying no one has checked it since it last changed, naming the turn and the agent
    And done is allowed once at least 1 checker has approved since the last change and no checker's latest check is older than that change, asks for changes or gives no verdict
    And a checker that never looked is not required: the test's team has a reviewer and a tester, and the reviewer's approval alone allows done
    # unchecked: internal/coordinator/checks.go; the team's checkers, reviewer and tester: internal/agents/defaults.go, Column Reviews

  @COORD-37 @test
  Scenario: A change after a review needs another review
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the reviewer ended a turn with "Verdict: changes needed", and the engineer then changed the work
    When the coordinator decides done
    Then the task is not done, and the issue says the reviewer checked it in its turn and it changed after that
    And once the reviewer's verdict is "approved" on the last change, done is allowed, and a deployer turn after it is not counted as a change, so the approval holds
    # the verdict line: internal/coordinator/checks.go VerdictFormat and verdictOf; a ship turn is not a change: checks.go recordOf

  @COORD-60 @manual
  Scenario: Work the user had shipped from Shipping needs no new check to be done
    Given the engineer changed the work after the last check, and the user moved the task to Shipping
    When the deployer's turn that the move asked for pushes the work, and the coordinator then decides done
    Then the task is done with no new check: the push counts as the user taking the work as it was
    # the turn is marked shipped: internal/tasks/task.go Turn.Shipped; shipped in workRecord, and a change before it raises no objection: internal/coordinator/checks.go recordOf, unchecked
    # Steps: on a task whose branch was reviewed and then changed, drag it to Shipping; let the deployer's turn push;
    # end the coordinator's next turn with {"action": "done", "summary": "Shipped."} and check the task is Done with no reviewer turn between.

  # --- Epics

  @COORD-38 @test
  Scenario: A split turns the task into an epic whose stories wait in the Backlog
    # internal/coordinator/epics_test.go TestAnEpicRunsItsStoriesAndFinishesWithThem
    Given an epic with no stories yet, whose brief says to split it before anything else
    When the coordinator decides a split with 3 stories, 1 of them for the Writing team
    Then exactly 3 tasks are made, each parked in the user's Backlog with 0 turns, each saying it is a story of the epic
    And each story is kept at the same budget and limit of turns as the epic
    And the Writing team's story goes to that team's coordinator once the user starts it
    # a story's limits: internal/tasks/limits.go NewLimits; "parked": "backlog": internal/coordinator/loop.go split

  @COORD-39 @manual
  Scenario: A split adds 1 to 12 stories
    Given an epic
    When the coordinator decides a split with 0 stories, or with 13
    Then the decision is refused with the reason "a "split" decision needs "stories": 1 to 12 of them"
    And a split with 12 stories, each with a title and a brief, is allowed
    # MaxStories = 12: internal/coordinator/decision.go checkStories
    # Steps: end an epic's coordinator turn with a split of 13 one-line stories and read the issue in the trail;
    # then with 12 and check 12 stories appear in the Backlog.

  @COORD-40 @test
  Scenario: A story's after list says which stories it builds on
    # internal/coordinator/epics_test.go TestASplitStoryBuildsOnAnExistingStoryOrAnEarlierOne
    Given a split whose third story has "after": [1], and an epic with an existing story task-N
    When the stories are made
    Then the third story's after list names the first story's id, and an "after" naming task-N is kept as that id
    And a story that builds on others is not started until every one of them is done, so its branch starts from a dev that has their work
    And a story whose "after" names a story later in the list, or one not in this epic, is refused
    # the refusal: internal/coordinator/epics_test.go TestASplitStoryAfterAStoryNotBeforeItIsRefused; blocked: internal/coordinator/loop.go

  @COORD-41 @test
  Scenario: A story cannot itself be split, and an epic is not assigned
    # internal/coordinator/epics_test.go TestAStoryCantBeSplitAndAnEpicIsntAssigned
    Given a story of an epic
    When its coordinator decides a split
    Then the decision is refused as an issue saying a story can't be split, and to say in its summary what else the epic needs
    And an assign on an epic is refused as an issue saying its work is done in its stories
    And a task the user made a story is never split either

  @COORD-42 @test
  Scenario: The epic's coordinator waits while stories run and asks the user to close it when all are done
    # internal/coordinator/epics_test.go TestAnEpicStaysOpenWithEveryStoryDone
    Given an epic whose stories are all running
    When its coordinator decides {"action": "wait"}
    Then no turn starts on the epic until a story's summary comes back to it
    And once every story is done, a done decision asks the user "Close it?" with exactly 1 option, "Close the epic", and the epic is not done until they do
    And if the user leaves it open, it stays open with no further question until they close it or add a story
    # CloseEpic: internal/coordinator/epics.go; the question: internal/coordinator/loop.go apply, Finish

  @COORD-43 @test
  Scenario: Closing a done epic starts one Visionaries follow-up session about it
    # internal/coordinator/followups_test.go TestClosingAnEpicStartsOneVisionariesSessionAboutIt
    Given a team made from the Visionaries template, and the Ideas page setting for sessions on epic close is on
    When the user answers "Close the epic" on a done epic
    Then exactly 1 new task is made for that team, titled "Follow-ups after <the epic's title>", with the epic's and each story's summary in its brief
    And a second pass over the same close makes no second session
    And the session takes the server's limits, not the epic's
    # "about" and "close" mark the session: internal/coordinator/followups.go followUp

  @COORD-44 @test
  Scenario: No follow-up session when turned off, when the epic was dragged to Done, or for a close before the loop started
    # internal/coordinator/followups_test.go TestNoSessionWhenTheSettingIsOff
    Given the Ideas page setting for sessions on epic close is off
    When the user closes a done epic
    Then no session is made and no note is left on the epic
    And an epic moved to Done on the board gets no session either
    And an epic closed before the server's loop started gets no session, only closes after it do
    # TestAnEpicMovedToDoneGetsNoSession and TestTheLoopFollowsUpOnlyCloseAfterItStarted in the same file; startedAt: internal/coordinator/loop.go NewLoop

  # --- Worktrees and branches

  @COORD-45 @test
  Scenario: Each task works in its own worktree on branch agentry/<task>
    # internal/worktrees/worktree_test.go TestCreateSetsTheWorktreeUp
    Given a repo and a task task-1
    When Agentry prepares the task's worktree
    Then the worktree is on branch agentry/task-1, started from the repo's current commit, with 1 port of its own
    And the repo's untracked .env files are copied in, node_modules is linked, and Agentry's own files under .agentry/ never show as changes
    And a second task's worktree adds the ignore lines exactly once

  @COORD-46 @test
  Scenario: Each worktree leases a port between 7900 and 7999
    # internal/worktrees/worktree_test.go TestLeasePortSkipsTakenAndBusyPorts
    Given port 7900 is held by another unfinished task and 7901 has something listening on it
    When a new task's worktree is prepared
    Then its port is exactly 7902, and that port is in the PORT environment variable of every turn on the task
    # FirstPort = 7900, LastPort = 7999: internal/worktrees/ports.go; PORT: internal/turns/turns.go environment

  @COORD-47 @test
  Scenario: A finished task's worktree is removed only once its branch is merged into dev
    # internal/worktrees/worktree_test.go TestRemoveOnlyOnceMerged
    Given a done task whose branch has commits not yet in dev
    When Agentry removes its worktree
    Then the worktree and the branch are kept, so no work is lost
    And once the branch is in origin/dev, or a local dev with no remote, the worktree and the branch are removed

  @COORD-48 @test
  Scenario: The task's work follows its branch to the next machine
    # internal/worktrees/handoff_test.go TestWorkMovesThroughTheHubAndBack
    Given a task's worktree on one machine with 1 commit and 1 file left uncommitted
    When the next turn's agent runs on another machine
    Then what was left uncommitted is committed first, the branch goes to the repo Agentry runs from, and the other machine makes its worktree from that branch
    And each worktree exists on exactly 1 machine at a time
    # the coordinator's choice of machine: internal/coordinator/machines_test.go TestTheCoordinatorChoosesTheMachine

  @COORD-49 @test
  Scenario: Shared files in .agentry/files follow the task
    # internal/coordinator/files_test.go TestSharedFilesFollowTheWork
    Given an agent's turn added a file under .agentry/files in the worktree
    When the turn ends
    Then the file is read back and kept with the task, recorded with the agent that left it and when
    And the coordinator's next brief lists it as "out/report.csv (9 bytes, from engineer)"
    And a file the user shares is in the next agent's .agentry/files when its turn starts, and a file a turn removed is shared no more
    # a shared file is Name, Hash, Size, By, At, From: internal/tasks/files.go SharedFile

  @COORD-58 @manual
  Scenario: A story's worktree has its epic's files under .agentry/files/epic, to read
    Given a story of an epic, and the epic has exactly 1 shared file, brief.md
    When the story's turn starts
    Then the story's .agentry/files/epic holds exactly that file, and the brief says changes there aren't kept
    And a file written under .agentry/files/epic during the turn is not shared when the turn ends
    # the delivery under epic/: internal/coordinator/files.go filesFor; the folder, left out of what a turn shares: internal/sharedfiles/folder.go EpicFolder, Manifest
    # the folder left out of the manifest: internal/sharedfiles/sharedfiles_test.go TestFilesGoRoundBetweenWorktrees; the brief's words: internal/coordinator/brief.go sharedText
    # Steps: drop a file on an epic's Shared files panel, start one of its stories, and in the story's worktree read
    # .agentry/files/epic/<the file>; write a new file beside it and end the turn; the story's Details list no such file.

  # --- Sessions

  @COORD-50 @test
  Scenario: A turn goes on with the agent's last session on the task when it can
    # internal/tasks/resume_test.go TestATurnResumesOnlyTheSameAgentsSessionOnTheSameEngineAndMachine
    Given the agent's last turn on the task ended with a report, on the same engine and the same machine, with a session id
    When its next turn on the task starts
    Then it resumes exactly that session, and its brief is only what has happened since
    And it starts fresh, saying why, when any of these holds: first turn, last turn failed, last turn stopped by the user, work moved to another machine, another engine, no session id, or the session's context is over 150,000 tokens
    # MaxResumedContext = 150_000: internal/tasks/resume.go; the shorter brief: internal/coordinator/resumed_brief_test.go TestAResumedBriefIsShorterThanTheWholeOne

  @COORD-51 @test
  Scenario: A story's first turn can go on with a sibling story's session
    # internal/tasks/share_test.go TestAFirstTurnOnAStoryGoesOnWithTheLatestSessionOfASibling
    Given an epic with stories, and the agent has had no turn on this story yet
    When the agent's first turn on this story starts
    Then it goes on with the agent's latest ended turn's session on any sibling story, held to the same rules as its own
    And the brief says this is a new task, with its own worktree and branch, and only this task's trail is shown, from its start
    And once the agent has had a turn on this story, its own latest session is used, even when a sibling's is newer
    # the brief: internal/coordinator/carried_brief_test.go TestABriefFromASiblingsSessionSaysItIsANewTask; own first: internal/tasks/share_test.go TestAStorysOwnSessionWinsOverANewerSiblings

  @COORD-52 @test
  Scenario: A busy session is forked, never resumed in place
    # internal/turns/share_test.go TestAFirstTurnOnAStoryGoesOnWithASiblingsSessionAndForksItWhenBusy
    Given a session that another turn is open in, or that an open turn started from
    When a turn on a sibling story goes on with it
    Then the turn goes on with a copy of the session, marked forked, and the original is kept by the open turn
    And 2 stories starting at once from 1 session never both resume it in place: the later one forks
    # SessionBusy: internal/tasks/resume.go; the race: internal/turns/share_test.go TestTwoStoriesStartingAtOnceFromOneSessionNeverBothResumeIt

  @COORD-53 @test
  Scenario: With session resume off, every turn starts fresh
    # internal/turns/share_test.go TestResumeOffKeepsEveryStoryFresh
    Given the server was started with -resume-sessions=false
    When any turn starts, on a story with sibling sessions or not
    Then it starts fresh, and the reason kept on the turn is exactly "resume is off"
    # the flag, default true: cmd/agentry/serve.go; ResumeOff: internal/turns/turns.go

  # --- Cannot cases

  @COORD-54 @test
  Scenario: The coordinator never edits code
    # internal/policy/default_test.go TestTheReadOnlyProfile
    Given the coordinator runs under the read-only profile
    When it calls the Edit tool on main.go in the workspace, or runs git commit -am 'x', or git push origin HEAD:dev
    Then each is denied by the rule "this agent changes nothing" or the rule for its command
    And reading a file, git diff, and go test ./... are allowed
    And writing its own notes in .agentry/knowledge.md is allowed
    # tech-lead and every template's coordinator: Profile "read-only", internal/agents/defaults.go and internal/agents/teams.go (the Visionaries' lead is read-mostly)
    # the profile: internal/policy/default.toml, [profiles.read-only], line 198

  @COORD-55 @test
  Scenario: The gate judges each action by the working agent's own profile
    # internal/hooks/handler_test.go TestTheGateUsesTheWorkingAgentsProfile
    Given the engineer, whose profile is developer, is the agent working on the task
    When it asks to run a command
    Then the policy is asked with exactly the profile "developer", not the coordinator's

  @COORD-56 @test
  Scenario: The coordinator cannot assign the task to itself
    # internal/rules/rules_test.go TestATaskGoesRoundTheCoordinator
    Given a coordinator's turn has ended
    When it assigns the task to its own name
    Then the assignment is refused by the rules
    And an assignment by any agent other than the coordinator is refused too
    # notToTheCoordinator and byTheCoordinator: internal/rules/task_rules.go

  @COORD-57 @test
  Scenario: After the user moves a task to Done, the coordinator's one wrap-up turn can only summarise
    # internal/coordinator/wrapup_test.go TestAWrapUpCantStartMoreWork
    Given the engineer is working and the user moves the task to Done on the board
    When the coordinator's wrap-up turn ends with an assign to the engineer
    Then the task stays done, with the summary "Moved.", and the trail says the coordinator decided "assign" and only its summary is kept
    And an assignment proposed after that is refused with "the user moved this task to Done; it stays done"
    And no more turns run on the task
