Feature: Sessions
  A turn goes on with an agent's earlier session, starts fresh or forks one,
  and its live terminal.
  # Sources: internal/sessions, internal/terminal, internal/tasks/resume.go,
  # internal/turns, ui/src/components/organisms/TerminalView.tsx and their tests.

  # ---- Going on with a session -----------------------------------------------

  @SESSION-01 @coordinator @test
  Scenario: A turn goes on with the agent's last session on the task when it can
    # internal/tasks/resume_test.go TestATurnResumesOnlyTheSameAgentsSessionOnTheSameEngineAndMachine
    Given the agent's last turn on the task ended with a report, on the same engine and the same machine, with a session id
    When its next turn on the task starts
    Then it resumes exactly that session
    And its brief is the resumed one, with no "## The task" section, shorter than the whole brief
    And it starts fresh, saying why, when any of these holds: first turn, last turn failed, last turn stopped by the user, work moved to another machine, another engine, no session id, or the session's context is over 150,000 tokens
    # MaxResumedContext = 150_000: internal/tasks/resume.go; the shorter brief: internal/coordinator/resumed_brief_test.go TestAResumedBriefIsShorterThanTheWholeOne and TestTheLoopBriefsATurnAsItStarts

  @SESSION-02 @coordinator @test
  Scenario: A story's first turn can go on with a sibling story's session
    # internal/tasks/share_test.go TestAFirstTurnOnAStoryGoesOnWithTheLatestSessionOfASibling and TestASiblingsSessionIsHeldToTheSameRules
    Given an epic with stories, and the agent has had no turn on this story yet
    When the agent's first turn on this story starts
    Then it goes on with the agent's latest ended turn's session on any sibling story, held to the same rules as its own
    And the brief says this is a new task, with its own worktree and branch, and only this task's trail is shown, from its start
    And once the agent has had a turn on this story, its own latest session is used, even when a sibling's is newer
    # the brief: internal/coordinator/carried_brief_test.go TestABriefFromASiblingsSessionSaysItIsANewTask; own first: internal/tasks/share_test.go TestAStorysOwnSessionWinsOverANewerSiblings

  @SESSION-03 @coordinator @test
  Scenario: A busy session is forked, never resumed in place
    # internal/turns/share_test.go TestAFirstTurnOnAStoryGoesOnWithASiblingsSessionAndForksItWhenBusy
    Given a session that another turn is open in, or that an open turn started from
    When a turn on a sibling story goes on with it
    Then the turn goes on with a copy of the session, marked forked, and the original is kept by the open turn
    And 2 stories starting at once from 1 session never both resume it in place: 1 goes on in place and 1 forks
    # not under test, checked by hand: the one that forks is the later
    # SessionBusy: internal/tasks/resume.go; the race: internal/turns/share_test.go TestTwoStoriesStartingAtOnceFromOneSessionNeverBothResumeIt

  @SESSION-04 @coordinator @test
  Scenario: With session resume off, every turn starts fresh
    # internal/turns/share_test.go TestResumeOffKeepsEveryStoryFresh
    Given the server was started with -resume-sessions=false
    When any turn starts, on a story with sibling sessions or not
    Then it starts fresh, and the reason is exactly "resume is off"
    # not under test, checked by hand: the reason is kept on the turn; the test reads the runner's choice
    # the flag, default true: cmd/agentry/serve.go; ResumeOff: internal/turns/turns.go

  # ---- The live terminal -----------------------------------------------------

  @SESSION-05 @user @test
  Scenario: The live terminal: everyone watches, only the one in control types
    # internal/terminal/terminal_test.go TestOnlyTheViewerInControlCanType
    # the page's words for the seats, Watching, "You have control: typing goes to the session" and "Session ended", and its buttons "Take control" and "Hand back": ui/src/components/organisms/TerminalView.tsx lines 18 to 24 and 108 to 116
    # the route: internal/web/server.go line 181
    Given the engineer's session is running and 2 viewers watch its terminal
    When a viewer types while only watching
    Then the keys are dropped: 0 of them reach the session
    When the first viewer clicks "Take control"
    Then that viewer is the 1 in control, and its keys reach the session
    When the second viewer asks to take control
    Then it is refused: exactly 1 viewer has control at a time
    When the session ends
    Then every viewer is disconnected
