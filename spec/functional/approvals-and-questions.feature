Feature: Approvals and questions
  What waits for the user: actions the policy held, and the questions coordinators ask.
  # Sources: internal/domain/policy/hooks, internal/domain/tasks/waitlist, internal/domain/tasks/waiting.go,
  # internal/domain/tasks/tasksapi/questions_api.go, internal/web/waiting_api.go,
  # ui/src/components/organisms/NeedsYouPanel.tsx,
  # ui/src/components/organisms/QuestionsPanel.tsx and their tests.

  # ---- Held actions and approvals --------------------------------------------

  @APPROVE-01 @user @test
  Scenario: Needs you: approve or deny an action the policy held
    # internal/domain/tasks/tasksapi/tasks_api_test.go TestApproveOnlyWhatWasAsked
    # the panel "Needs you", reading "engineer asks to run Bash:" with the command, Approve and Deny: ui/src/components/organisms/NeedsYouPanel.tsx lines 77 to 99
    # the routes: internal/domain/tasks/tasksapi/tasksapi.go lines 97 and 98
    Given the engineer asked to run "git push origin HEAD:dev" and the policy asked the user
    When the task is read
    Then its needsYou reads exactly "Waiting for you to approve the push to dev" and the action is shown in its approvals
    When the user clicks Approve
    Then the approval is recorded with 200
    When the same action is denied afterwards
    Then it is refused with 409 "already approved"

  @APPROVE-02 @user @test
  Scenario: An approval nobody answered in time is held, and stays open for the user
    # internal/web/held_approval_test.go TestATimedOutApprovalStaysOpenUntilYouAnswer, and TestTheHookHearsItsRequestIsHeldBeforeItsDeadline for the agent being told no
    # the panel's line "no answer came in time, so engineer was told no and carried on": ui/src/components/organisms/NeedsYouPanel.tsx lines 83 to 88; its toast "Approved: back to the coordinator": line 67
    Given the engineer's hook gave up waiting for an answer on "git push origin HEAD:dev"
    When the wait runs out
    Then the action is held: the agent was told no and carried on, and the task still needs you
    And Questions lists exactly 1 approval, held, with the full command
    When the user approves it late
    Then the action is recorded approved and the coordinator is handed the command back: yes

  @APPROVE-03 @agent @test
  Scenario: A push the user does not answer is held for them, and the deployer told to carry on
    # internal/domain/policy/hooks/handler_test.go TestAnAskThatTimesOutIsHeldForTheUser; "Don't retry": TestAnAskIsHeldBeforeTheDeadlineTheHookSent in the same file
    Given the deployer asked to push and the user gave no answer before the hook's deadline
    When the wait runs out
    Then the agent is told no, with "the command is held for the user's approval. Don't retry it now"
    And the action is held and undecided, the task is in Needs you, and 0 hook calls still wait on it
    And the task's waiting line reads exactly "Waiting for you to approve the push to dev"
    # the hook's deadline is 1790 seconds, GateTimeoutSeconds 1800 less 10: internal/runtime/turns/turns.go environment, internal/platform/engines/engine.go;
    # the server answers 5 seconds before it: internal/domain/policy/hooks/handler.go replyMargin; the line: internal/domain/tasks/waiting_test.go TestAHeldPushNamesItsBranch
    # the test holds a curl command, which goes through the same hold as any ask

  @APPROVE-04 @agent @test
  Scenario: A late approval lets the same command through once, in the agent's next turn
    # internal/domain/policy/hooks/handler_test.go TestALateApprovalLetsTheSameCommandThroughOnce
    Given a held push the user approved after the turn moved on
    When the coordinator is handed the command back and hands the deployer its next turn
    Then exactly that command, from that agent, in that turn, runs once without asking, and git push origin HEAD:main does not use the pass
    And the same push asked for again after that is asked about again
    And a policy denial still denies it, another agent's identical request is asked, and a pass not used in that turn is gone
    # TestAPassDoesntOverrideADenial, TestAPassIsOnlyForTheAgentThatAsked, TestAPassEndsWithTheAgentsNextTurn, TestALateDenialGivesNoPass in the same file;
    # PassFor: internal/domain/tasks/task.go

  # ---- Questions to the user -------------------------------------------------

  @APPROVE-05 @user @test
  Scenario: Questions in the dock gathers what waits for the user, oldest first
    # internal/domain/tasks/tasksapi/tasksapi_test.go TestQuestionsGatherWhatWaitsForYou
    # Ctrl+J: ui/src/lib/shortcuts.ts line 16; the tab "Questions N": ui/src/components/organisms/TerminalDock.tsx line 57
    # oldest first: internal/domain/tasks/tasksapi/questions_api.go line 54; the 5 kinds, labelled Approve, Question, Budget, "Max turns" and Waiting: ui/src/components/organisms/QuestionsPanel.tsx lines 20 to 26
    Given task-1's coordinator asked "Which layout?" with 2 options and a page, and task-2's policy asks about "git push"
    When the user presses Ctrl+J and opens Questions
    Then exactly 2 questions are listed, the older first
    And the first has kind question, the text "Which layout?", exactly 2 options and the page
    And the second has kind approval, its action, the command "git push" and the rule "every push asks"

  @APPROVE-06 @user @manual
  Scenario: Answering a question with options, and with the page the coordinator made
    # ui/src/components/organisms/QuestionsPanel.tsx lines 102 to 136; ui/src/components/organisms/NeedsYouPanel.tsx lines 111 to 157
    # the page's answer is cut at 4000 characters: ui/src/components/molecules/QuestionPage.tsx line 75; its frame 240 px to 75% of the window: line 76; sandbox allow-scripts only: line 86
    # a limit's stop: ui/src/components/organisms/NeedsYouPanel.tsx lines 208 to 220; its wrap-up answer: line 198
    Given a question with the options Tabs and "One page", and a page
    When the user opens it in Questions
    Then exactly 2 option buttons are shown, the field "Your answer goes to the coordinator", and Answer
    And "The coordinator made a page for this question." is shown over the page, with Expand
    When the user clicks an option
    Then that 1 answer is sent, and a toast "Answer sent to the coordinator" is shown once
    When the page calls answer("Tabs, with a sidebar")
    Then the field is filled with it and "The page filled in your answer below. Check it, then send it." is shown; nothing is sent yet
    And a text over 4000 characters is cut at 4000
    Given the chat on the task's page
    Then the question's card is shown reading "Asks you" with "Answer in Questions · it made a page for this", and opens the dock
    Given a task stopped at its budget or its turn limit
    Then its question offers exactly 3 choices: "Raise the budget" or "Raise the turn limit", "Make it unlimited" and "Wrap up"
    When the user clicks "Wrap up"
    Then exactly 1 answer is sent, "Stop here: wrap the task up with what is done, and say what is left."

  @APPROVE-07 @coordinator @test
  Scenario: An ask decision puts a question to the user, with optional options and a page
    # internal/runtime/coordinator/decision_test.go TestParseDecision; the wait: internal/runtime/coordinator/next_test.go TestNext, "wait for your answer" and "your answer goes to the coordinator"
    Given the coordinator's reply ends with {"action": "ask", "question": "Which layout?", "options": ["Tabs", "One page"], "page": "<svg></svg>"}
    When the decision is read
    Then it is an ask with exactly that question, 2 options and the page
    And once applied, the task waits with the question open and no turn starts until the user answers
    # the wait while a question is open: internal/runtime/coordinator/next.go Next
