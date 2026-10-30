Feature: The coordinator and its handoffs
  An ordinary agent that reads every report and ends each turn with a decision the loop carries out:
  a task's life, briefs, reports, issues, safety stops and limits.
  # Sources: internal/runtime/coordinator, internal/runtime/watchdog, internal/domain/tasks,
  # internal/domain/tasks/tasksapi/chat_api.go, ui/src/components/organisms/CoordinatorChat.tsx and
  # their tests.

  # ---- A task's life ---------------------------------------------------------

  @HANDOFF-01 @coordinator @test
  Scenario: An assign decision hands the task to one agent
    Given a task whose coordinator has a turn open
    When the coordinator ends its turn with an assign decision that names the engineer and its instructions
    Then the next turn, turn-2, is the engineer's
    And the coordinator's instructions are shown in the engineer's brief
    # README.md, How a task moves; internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator

  @HANDOFF-02 @coordinator @test
  Scenario: A new task goes to its coordinator first
    # internal/runtime/coordinator/next_test.go TestNext
    Given a task was created for a team, with no turns yet
    When its worktree is ready
    Then the first turn is the coordinator's, and no other agent has had a turn
    And while an agent's turn is open, no other turn starts on the task

  @HANDOFF-03 @coordinator @test
  Scenario: The coordinator reads the task and the trail before anyone builds
    # internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the coordinator's first turn on a task
    When its brief is written
    Then the brief shows its own prompt, the task, the agents it can hand work to, and every step of the trail so far
    # not under test, checked by hand: its prompt says it reads the task, the trail and the code and changes no code: internal/domain/agents/defaults/tech-lead.md lines 1 and 5, internal/domain/agents/defaults/custom/coordinator.md lines 1 and 5

  @HANDOFF-04 @coordinator @test
  Scenario: An unfinished plan carries over to the agent's next turn
    # internal/domain/tasks/plan_test.go TestAnUnfinishedPlanCarriesOn
    Given an agent's last turn on the task left a plan with 1 or more steps not ticked
    When the agent's next turn on the task starts
    Then that plan is the one the next turn carries on, kept as it was, with the turn it came from
    And a plan with every step ticked is not carried over
    # not under test, checked by hand: the plan is already in the new turn's plan file and the brief says which turn it came from: internal/runtime/turns/turns.go line 136, internal/runtime/coordinator/brief_plan.go lines 19 and 20

  @HANDOFF-05 @coordinator @test
  Scenario: Any decision may retitle the task
    # internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given a task titled "Remove the TODOs"
    When the coordinator's assign decision also carries "title": "Remove the TODO comments in src/"
    Then the task's title is the new one and its first title is kept as "Remove the TODOs"
    # not under test, checked by hand: the decision format asks for 1 plain imperative line under 70 characters: internal/runtime/coordinator/decision.go DecisionFormat, line 105

  @HANDOFF-06 @coordinator @test
  Scenario: An assign decision hands the task to the named agent
    # internal/runtime/coordinator/next_test.go TestNext, for the instructions in the next turn's handoff; internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator, for the engineer's turn and its brief
    Given the coordinator's turn ended with {"action": "assign", "agent": "engineer", "instructions": "Remove them."}
    When the loop applies the decision
    Then the next turn is the engineer's, and its brief has those instructions: yes
    # not under test, checked by hand: the step from the coordinator to the engineer is shown in the trail

  @HANDOFF-07 @coordinator @test
  Scenario: An assign to an agent that does not exist is an issue
    # internal/runtime/coordinator/next_test.go TestNext; goes back to the coordinator: internal/domain/tasks/retry_fresh_test.go TestAPlainIssueStillGoesToTheCoordinator
    Given the coordinator's turn ended with an assign to "designer", and the roster has no agent of that name
    When the loop reads the decision
    Then no turn starts
    And an issue goes back to the coordinator saying there is no agent by that name

  # ---- Briefs, reports and messages ------------------------------------------

  @HANDOFF-08 @user @test
  Scenario: A message to the coordinator stops the turn in progress, and its reply says what it changed
    # internal/domain/tasks/tasksapi/chat_api_test.go TestAMessageStopsTheTurnForTheCoordinator and TestTheCoordinatorsReplyComesBeforeItsDecision
    # internal/domain/tasks/tasksapi/chat_api.go lines 55 to 63
    Given the engineer's turn is in progress on task-1
    When the user sends "Use Redis, not memory." in the Coordinator tab
    Then the engineer's turn is stopped, not failed: 0 issues in a row
    And the trail ends with exactly 2 steps: the user's message, then a step to the coordinator saying the engineer stopped so it can carry on
    And the coordinator is the 1 agent next
    When the coordinator hands the work back saying "Switched to Redis."
    Then the trail shows "Switched to Redis." to the user, then exactly 1 handoff to the engineer
    Given no turn is in progress
    When the user sends "And keep it simple."
    Then the trail's last step is exactly the message

  @HANDOFF-09 @user @manual
  Scenario: The chat's field, its status line and the message it refuses
    # ui/src/components/organisms/CoordinatorChat.tsx lines 107 to 124, the status and the hint; line 37, the 3 suggestions
    # a blank message: internal/domain/tasks/tasksapi/chat_api.go lines 27 to 30
    Given task-5 is open on the Coordinator tab while the engineer works
    Then the status under the coordinator's name reads exactly "engineer is working on it"
    And the hint reads exactly "Sending pauses engineer so the coordinator reads it now; engineer's work and plan carry on after."
    And exactly 3 suggestions are shown: "Where are we?", "What's left before it's done?" and "Wrap up what's done and finish."
    When the user presses Enter in the field "Message tech-lead"
    Then the message is sent once, and Shift+Enter adds a line instead of sending
    When a message of only spaces is sent
    Then it is refused with 400 "say something"
    Given the coordinator's turn is in progress
    Then the status reads exactly "Thinking…" and the hint "Sending makes the coordinator decide again with what you said."

  @HANDOFF-10 @coordinator @test
  Scenario: The brief is a file and the prompt is one line pointing at it
    # internal/runtime/turns/turns_test.go TestStartRunsTheAgentOnAOneLinePrompt
    Given a turn is about to start
    When Agentry starts the agent's session
    Then the brief is written to .agentry/turn-N.md and the prompt is exactly 1 line: "Read .agentry/turn-N.md and do what it says."
    And the turn records the hash of that line: yes
    # the prompt line: internal/runtime/turns/turns.go start
    # not under test here, checked by hand: the engine echoes the hash back once it has the prompt: internal/runtime/turns/turns.go line 118

  @HANDOFF-11 @coordinator @test
  Scenario: Every agent's report returns to the coordinator
    # internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the engineer's turn ended with the report "Removed 3 TODOs and committed."
    When the loop reads the task
    Then the next turn is the coordinator's, and no other agent's turn starts first
    And the report is shown in the coordinator's brief as the step "engineer → tech-lead: Removed 3 TODOs and committed."

  @HANDOFF-12 @coordinator @test
  Scenario: The coordinator's reply ends with exactly one JSON decision
    # internal/runtime/coordinator/decision_test.go TestParseDecision
    Given a coordinator's reply
    When Agentry reads the JSON object at its end
    Then an assign, a done and an ask are read with their fields, and an unknown action such as "deploy" is refused
    And a decision missing what its action needs, such as an assign with no agent, is refused
    # not under test, checked by hand: the action is 1 of these 9: assign, done, ask, transfer, request, wait, fetch, move, split, and a refusal says why: internal/runtime/coordinator/decision.go, the Action constants and check

  @HANDOFF-13 @coordinator @test
  Scenario: A reply with no usable decision is an issue
    # internal/runtime/coordinator/next_test.go TestNext
    Given the coordinator's turn ended with "I think the engineer." or with no reply at all
    When the loop reads the task
    Then no agent's turn starts
    And an issue goes back to the coordinator saying the reply had no usable decision

  # ---- Issues ----------------------------------------------------------------

  @HANDOFF-14 @coordinator @test
  Scenario: An issue goes back to the coordinator
    # internal/runtime/coordinator/next_test.go TestNext; internal/domain/tasks/retry_fresh_test.go TestAPlainIssueStillGoesToTheCoordinator
    # the count: internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou, where 3 issues raised read "3 issues in a row"
    Given the watchdog raised an issue "the agent's process exited" on a task
    When the loop reads the task
    Then the next turn is the coordinator's and its handoff is exactly the issue's reason
    And the task's count of issues in a row goes up by 1
    # IssuesInARow + 1 on IssueRaised: internal/domain/tasks/apply_turns.go

  @HANDOFF-15 @coordinator @test
  Scenario: A crashed agent is an issue within seconds
    # internal/runtime/watchdog/watchdog_test.go TestACrashIsFlaggedAfterTheGrace
    Given an open turn whose process has exited
    When more than 3 seconds have passed since the turn started
    Then the watchdog raises an issue saying the agent's process exited: yes
    # the words "(crashed or was killed)": internal/runtime/watchdog/watchdog.go line 134; the test asserts "process exited"
    # StartGrace = 3 * time.Second: internal/runtime/watchdog/watchdog.go DefaultLimits; checked every second: Run

  @HANDOFF-16 @coordinator @test
  Scenario: A silent agent is a stall
    # internal/runtime/watchdog/watchdog_test.go TestASilentTurnIsAStall
    Given an open turn with no tool call running and no action waiting for the user
    When the turn has had no events for more than 10 minutes
    Then the watchdog raises an issue saying the agent has been silent: yes
    # the words "has been silent for over 10m0s (stalled: a hung network call or a stuck tool)": internal/runtime/watchdog/watchdog.go line 150; the test asserts "silent"
    # Silence = 10 * time.Minute: internal/runtime/watchdog/watchdog.go DefaultLimits

  @HANDOFF-17 @coordinator @test
  Scenario: An agent waiting for the user's answer is not stalled
    # internal/runtime/watchdog/watchdog_test.go TestWaitingForYouIsNotAStall
    Given an open turn with an action the policy asked the user about, not yet decided
    When more than 10 minutes pass with no events
    Then the watchdog raises no issue

  @HANDOFF-18 @coordinator @test
  Scenario: A tool call with no result for too long is an issue
    # internal/runtime/watchdog/watchdog_test.go TestAToolOpenTooLong
    Given an approved tool call in an open turn, with no result yet
    When it has run for more than 20 minutes
    Then the watchdog raises an issue naming the command: yes
    # Tool = 20 * time.Minute: internal/runtime/watchdog/watchdog.go DefaultLimits

  @HANDOFF-19 @coordinator @test
  Scenario: The same command requested again and again is an issue
    # internal/runtime/watchdog/watchdog_test.go TestTheSameCommandRepeated
    Given an open turn
    When the agent requests the same command 5 times in a row
    Then the watchdog raises an issue saying the command came 5 times in a row
    # the words "so it may be stuck in a loop: <command>": internal/runtime/watchdog/watchdog.go line 140; the test asserts "5 times in a row"
    # Repeats = 5: internal/runtime/watchdog/watchdog.go DefaultLimits

  @HANDOFF-20 @coordinator @test
  Scenario: A denied action is shown to the coordinator under Actions Agentry refused
    # internal/runtime/coordinator/resumed_brief_test.go TestAResumedCoordinatorBriefKeepsTheDecisionChecksAndRefusals
    Given the policy denied an agent's command, such as git push origin HEAD:dev
    When the coordinator's next brief is written
    Then the command and the reason are shown under "Actions Agentry refused"
    # not under test, checked by hand: no issue is raised for the denial
    # the gate writes action.denied, no issue: internal/domain/policy/hooks/handler.go decide; the brief section: internal/runtime/coordinator/brief_decision.go refusedActions

  @HANDOFF-21 @coordinator @test
  Scenario: A resumed session that did not start is retried fresh once
    # internal/runtime/coordinator/retry_fresh_test.go TestAFailedResumeIsRetriedFreshOnce; internal/domain/tasks/retry_fresh_test.go TestARetryFreshIssueHandsTheSameStepBack
    Given an agent's turn went on with its last session, and the process exited before taking the prompt
    When the watchdog sees it after the 3 second grace
    Then it raises an issue saying the resumed session didn't start, and the same agent gets the same step again, fresh
    And the coordinator has no turn of its own before the retry
    And the coordinator is only told
    And the retry still counts as 1 issue towards the issues in a row
    # retryFresh: internal/runtime/watchdog/watchdog.go checkTurn; the same step handed back: internal/domain/tasks/apply_turns.go IssueRaised

  @HANDOFF-22 @coordinator @manual
  Scenario: An agent's report of a blocker is a report, not an issue
    Given a task with the default limit of 24 turns, where the coordinator assigns the engineer and the engineer ends its turn with the report "still broken", 12 times over
    When the loop reads the task after those 24 turns
    Then the 12 reports raised 0 issues, and the stop is the "turns" limit, with the words "24 of its 24 turns"
    And the issues limit is reached only by 3 issues raised with no turn ending between them
    # an issue is raised only for a failed turn or a usage limit that lasts: internal/domain/policy/hooks/handler.go, the UsageLimited and TurnFailed cases
    # a turn that ends with a report sets IssuesInARow to 0: internal/domain/tasks/apply_turns.go turn.ended
    # the stop on "turns" alone: internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou, which writes the reports to the log and runs no hook
    # Steps: on a task with Max turns 24, have the coordinator assign the engineer and the engineer end each turn with
    # the report "still broken", 12 times over. Open the task's Events tab: no issue.raised row, and the last
    # task.needs_you row has "limit":"turns" and the question "This task has had 24 of its 24 turns without finishing"
    # (internal/runtime/coordinator/next.go). The page says Waiting for you and Turns since your answer reads "24 of 24 turns", in red (internal/domain/tasks/summary.go atTurnLimit; ui/src/pages/TaskPage.tsx line 386).

  # ---- Safety stops and limits -----------------------------------------------

  @HANDOFF-23 @user @test
  Scenario: Raising the budget or the turn limit while stopped at it is the answer
    # internal/domain/tasks/tasksapi/tasks_api_test.go TestRaisingTheBudgetLetsATaskGoOn and TestRaisingTheTurnLimitLetsATaskGoOn
    # internal/domain/tasks/tasksapi/limits_api.go lines 15 to 62; internal/domain/tasks/task.go lines 145 to 163, StoppedAt
    Given task-1 stopped at its budget of 200,000 tokens, with 210,000 used
    When the user sets the budget to 1,000,000
    Then the reply is exactly {"answered":true,"budget":1000000}
    And the task is working again and the coordinator read "I raised the budget to 1M tokens. Carry on."
    When the user makes another such task's budget unlimited
    Then it carries on with "I made the budget unlimited. Carry on." and a budget of 0
    Given task-2 stopped at its 2 turns, with 2 had
    When the user sets the turn limit to 2
    Then the reply says answered: false, the limit is kept at 2, and the task still needs you
    When the user sets it to 5
    Then the task is working again and the coordinator read "I raised the turn limit to 5. Carry on."

  @HANDOFF-24 @user @test
  Scenario: A limit answers only its own stop, and a done task's limits stay
    # internal/domain/tasks/tasksapi/tasks_api_test.go TestALimitAnswersOnlyItsOwnStop, TestADoneTasksLimitsStay and TestALimitIsKeptThoughItsAnswerIsRefused
    Given task-1 stopped at its turn limit, over its budget too
    When the user raises the budget
    Then the budget is kept and the task still needs you: the turn stop is not answered
    When the user raises the turn limit
    Then the task is working again, no longer waiting
    Given the coordinator's own question, not a limit's stop
    When the user changes a limit
    Then the task still needs you and the question is not answered
    Given a done epic with a budget of 200,000 and 5 turns
    When the user changes either limit
    Then it is refused with 409 "is done" and both limits are kept
    Given the rules refuse the answer, as the stop was answered first
    Then the limit is kept with 200 and the reply names the refusal

  @HANDOFF-25 @user @test
  Scenario: The stop's question names its limit and the numbers it stopped at
    # internal/domain/tasks/tasksapi/tasks_api_test.go TestQuestionsShowWhichLimitStopped
    Given task-1 stopped at its 2 turns, task-2 at its budget of 200,000 with 210,000 used, and task-3 asked to copy a file
    When Questions is read
    Then task-1's question has kind turns, with exactly 2 of 2 turns
    And task-2's has kind budget, with exactly 210,000 tokens used of 200,000
    And task-3's has kind question and no limit, budget 0

  @HANDOFF-26 @coordinator @test
  Scenario: The turn limit asks the user
    # internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task with the default limit of 24 turns
    When it has had 24 turns with no answer from the user
    Then no turn starts
    And the user is asked, with the words "24 of its 24 turns", and the stop is marked as the "turns" limit
    # DefaultMaxTurns = 24: internal/domain/tasks/task.go; the stop at TurnsSinceAnswer >= MaxTurns: internal/domain/tasks/task.go AtTurnLimit

  @HANDOFF-27 @coordinator @test
  Scenario: Each task has its own limit of turns, set when it is made
    # internal/runtime/coordinator/next_test.go TestEachTaskHasItsOwnLimitOfTurns
    Given a task created with maxTurns 4
    When it has had 4 turns with no answer from the user
    Then the user is asked, with the words "4 of its 4 turns"
    And a task created before each had its own limit is kept at 24
    # the server's default comes from the -max-turns flag, default coordinator.DefaultMaxTurns: cmd/agentry/serve.go

  @HANDOFF-28 @coordinator @test
  Scenario: A limit of 0 is no limit
    # internal/runtime/coordinator/next_test.go TestEachTaskHasItsOwnLimitOfTurns
    Given a task created with maxTurns 0
    When it has had 40 turns with no answer from the user
    Then the next turn starts and the user is not asked
    And a task stopped at its limit, then changed to 0, runs on
    # MaxTurns > 0 is the condition: internal/domain/tasks/task.go AtTurnLimit; "0 for no limit": cmd/agentry/serve.go -max-turns and -budget

  @HANDOFF-29 @coordinator @test
  Scenario: The budget stop asks the user once the task has used it
    # internal/runtime/coordinator/next_test.go TestTheBudgetSendsTheTaskToYou
    Given a task with a budget of 100,000 tokens
    When its ended turns have used 110,000 tokens, and 1 or more turns have run since the user last answered
    Then no turn starts
    And the user is asked, with the words "110k tokens of its 100k budget", and the stop is marked as the "budget" limit
    And after the user answers, the stop is no longer marked and the next turn is the coordinator's, so it can wrap up
    # AtBudget is Budget > 0 and TokensUsed >= Budget: internal/domain/tasks/task.go; the stop needs TurnsSinceAnswer > 0: internal/runtime/coordinator/next.go Next
    # the server's default budget is 5,000,000 tokens, flag -budget: cmd/agentry/serve.go

  @HANDOFF-30 @coordinator @test
  Scenario: 3 issues in a row ask the user
    # internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task with 3 issues raised and no turn ending normally between them
    When the loop reads the task
    Then no turn starts
    And the user is asked "3 issues in a row, the last: the engine is missing. Fix what's wrong and answer, and the coordinator carries on."
    # MaxIssuesRun = 3: internal/runtime/coordinator/next.go

  @HANDOFF-31 @user @test
  Scenario: The user's answer lifts the stop
    # internal/runtime/coordinator/next_test.go TestTheSafetyStopsSendTheTaskToYou
    Given a task stopped at its turn limit
    When the user answers
    Then the next turn starts, no longer stopped by the limit
    # not under test, checked by hand: the turns since the answer and the issues in a row are both 0
    # task.answered sets TurnsSinceAnswer and IssuesInARow to 0: internal/domain/tasks/apply_handoffs.go

  @HANDOFF-32 @coordinator @manual
  Scenario: A turn that ends normally resets the issues in a row
    Given a task with 2 issues in a row
    When an agent's turn ends with a report
    Then the issues in a row are 0, and 3 more issues are needed before the user is asked
    # turn.ended sets IssuesInARow to 0: internal/domain/tasks/apply_turns.go
    # Steps: on a task, let 2 turns crash (kill the agent's process), then let 1 turn end with a report;
    # let 2 more turns crash: the task stays in its column, the Tasks page does not list it under Needs you
    # and its page does not say Waiting for you; a 3rd crash in a row lists it under Needs you, its page says
    # Waiting for you and the Coordinator tab reads Coordinator · asks you, with "3 issues in a row" as the reason
    # (MaxIssuesRun = 3: internal/runtime/coordinator/next.go). 2 crashes after the report not asking proves the count was 0.

  # ---- Decisions -------------------------------------------------------------

  @HANDOFF-33 @coordinator @test
  Scenario: A move decision takes the task to a repo the user named
    # internal/runtime/coordinator/repos_test.go TestTheCoordinatorMovesATaskToTheRepoYouNamed
    Given the user named a git address or a folder for the task's work in its description or in what they told the coordinator
    When the coordinator decides {"action": "move", "repo": "<that address, as they wrote it>"}
    Then the task's repo is the new one, a clone if it is an address, and its next turn runs in a new worktree of it: yes
    And a move to a repo the user did not name is refused as an issue, and the task's repo is kept
    # namedByYou: internal/runtime/coordinator/next.go; TestARepoIsNamedOnlyWhole in the same file covers the whole-name match

  @HANDOFF-34 @coordinator @test
  Scenario: A transfer passes the task to another team's coordinator
    # internal/runtime/coordinator/teams_test.go TestATaskWithNoTeamChosenGoesToTheRightOne
    Given a task whose team the user did not choose, so the brief says to decide whose work it is
    When the coordinator decides {"action": "transfer", "team": "Writing", "reason": "It is a blog post."}
    Then the next turn is the Writing team's coordinator's, and the task's coordinator is now exactly that agent
    And "This task is your team's work: It is a blog post." is shown in its brief

  @HANDOFF-35 @coordinator @test
  Scenario: A request opens a task for another team and waits for its summary
    # internal/runtime/coordinator/teams_test.go TestACoordinatorAsksAnotherTeamAndCarriesOnWithTheAnswer
    Given a task of the Software team, and the Writing team is listed under "Other teams" in the brief
    When the coordinator decides {"action": "request", "team": "writing", "request": "Write the user guide for the TODO tool."}
    Then a new task is made for the Writing team, with the request's first line as its title and this task as its parent: yes
    And this task waits, with no turn starting, until that task is done
    And then the other team's summary is shown in this coordinator's brief as "<that task> is done: <its summary>"
    # not under test, checked by hand: the new task is in the same repo, and the summary is 1 step in the trail

  @HANDOFF-36 @coordinator @manual
  Scenario: A transfer or request goes only to a team named in the brief
    Given a task of the Software team
    When the coordinator decides a transfer or request to its own team, or to a team that does not exist
    Then the decision is refused as an issue naming the teams there are
    And a request from a task already 3 requests deep is refused
    # MaxRequestDepth = 3: internal/runtime/coordinator/next.go otherTeam
    # Steps: end a coordinator turn with {"action": "request", "team": "software", "request": "x"} on a Software task and
    # read the issue in the trail; repeat with "team": "nobody"; then chain 3 requests team to team and try a 4th.

  @HANDOFF-37 @coordinator @test
  Scenario: Done is refused until a checker has approved the last change and no checker objects
    # internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the engineer's turn changed the work and no checker has looked at it since
    When the coordinator decides {"action": "done", "summary": "TODOs removed."}
    Then the task is not done
    And an issue goes back to the coordinator saying no one has checked it since it last changed, naming the turn and the agent
    And done is allowed once at least 1 checker has approved since the last change and no checker's latest check is older than that change
    # not under test, checked by hand: done is also refused while a checker's latest check asks for changes or gives no verdict: internal/runtime/coordinator/checks.go unchecked, line 140
    And a checker that never looked is not required: the test's team has a reviewer and a tester, and the reviewer's approval alone allows done
    # unchecked: internal/runtime/coordinator/checks.go; the team's checkers, reviewer and tester: internal/domain/agents/defaults.go, Column Reviews

  @HANDOFF-38 @coordinator @test
  Scenario: A change after a review needs another review
    # internal/runtime/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the reviewer ended a turn with "Verdict: changes needed", and the engineer then changed the work
    When the coordinator decides done
    Then the task is not done, and the issue says the reviewer checked it in its turn and it changed after that
    And once the reviewer's verdict is "approved" on the last change, done is allowed, and a deployer turn after it is not counted as a change, so the approval holds
    # the verdict line: internal/runtime/coordinator/checks.go VerdictFormat and verdictOf; a ship turn is not a change: checks.go recordOf

  @HANDOFF-39 @coordinator @manual
  Scenario: Work the user had shipped from Shipping needs no new check to be done
    Given the engineer changed the work after the last check, and the user moved the task to Shipping
    When the deployer's turn that the move asked for pushes the work, and the coordinator then decides done
    Then the task is done with no new check: the push counts as the user taking the work as it was
    # the turn is marked shipped: internal/domain/tasks/task.go Turn.Shipped; shipped in workRecord, and a change before it raises no objection: internal/runtime/coordinator/checks.go recordOf, unchecked
    # Steps: on a task whose branch was reviewed and then changed, drag it to Shipping; let the deployer's turn push;
    # end the coordinator's next turn with {"action": "done", "summary": "Shipped."} and check the task is Done with no reviewer turn between.

  # ---- What the coordinator cannot do ----------------------------------------

  @HANDOFF-40 @coordinator @test
  Scenario: The coordinator never edits code
    # internal/domain/policy/default_test.go TestTheReadOnlyProfile
    Given the coordinator runs under the read-only profile
    When it calls the Edit tool on main.go in the workspace, or runs git commit -am 'x', or git push origin HEAD:dev
    Then each is denied
    # not under test, checked by hand: the rule that denies it, "this agent changes nothing" or the rule for its command: internal/domain/policy/default.toml line 219
    And reading a file, git diff, and go test ./... are allowed
    And writing its own notes in .agentry/knowledge.md is allowed
    # tech-lead and every template's coordinator: Profile "read-only", internal/domain/agents/defaults.go and internal/domain/agents/teams.go (the Visionaries' lead is read-mostly)
    # the profile: internal/domain/policy/default.toml, [profiles.read-only], line 198

  @HANDOFF-41 @coordinator @test
  Scenario: The coordinator cannot assign the task to itself
    # internal/core/rules/rules_test.go TestATaskGoesRoundTheCoordinator
    Given a coordinator's turn has ended
    When it assigns the task to its own name
    Then the assignment is refused by the rules
    And an assignment by any agent other than the coordinator is refused too
    # notToTheCoordinator and byTheCoordinator: internal/core/rules/task_rules.go

  @HANDOFF-42 @coordinator @test
  Scenario: After the user moves a task to Done, the coordinator's one wrap-up turn can only summarise
    # internal/runtime/coordinator/wrapup_test.go TestAWrapUpCantStartMoreWork
    Given the engineer is working and the user moves the task to Done on the board
    When the coordinator's wrap-up turn ends with an assign to the engineer
    Then the task stays done, with the summary "Moved.", and the trail says the coordinator decided "assign": yes
    # not under test, checked by hand: the trail's words that only its summary is kept
    And an assignment proposed after that is refused with "the user moved this task to Done; it stays done"
    And no more turns run on the task
