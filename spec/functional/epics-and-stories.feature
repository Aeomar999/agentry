Feature: Epics and stories
  A goal too big for one change is an epic, split into stories that wait in the
  Backlog, ordered in Plan, and closed once every story is done.
  # Sources: internal/domain/tasks, internal/runtime/coordinator/epics.go,
  # internal/web/epics_api.go, ui/src/pages/PlanPage.tsx, ui/src/lib/epics.ts and
  # their tests.

  # ---- Making epics and stories ----------------------------------------------

  @EPIC-01 @user @manual
  Scenario: New epic and New story open from Plan and from their addresses
    # the dialog's title: ui/src/components/organisms/NewTaskDialog.tsx line 28; the buttons: ui/src/components/organisms/NewTaskForm.tsx lines 262 and 343
    Given the Plan page is open with the epic epic-7 in it
    When the user clicks New epic in the page header
    Then a dialog titled "New epic" is shown with ?new=epic in the address
    And the Kind field is set to Epic and the primary button reads exactly "Add epic"
    When the user clicks Story at the foot of epic-7's column
    Then a dialog titled "New story of epic-7" is shown with ?new=story&newIn=task-7 in the address
    And the Epic field is shown reading epic-7 and the Kind field is hidden
    And the repo choice offers "The epic's repo" and the primary button reads exactly "Add story"

  @EPIC-02 @coordinator @test
  Scenario: A split turns the task into an epic whose stories wait in the Backlog
    # internal/runtime/coordinator/epics_test.go TestAnEpicRunsItsStoriesAndFinishesWithThem
    Given an epic with no stories yet, whose brief says to split it before anything else
    When the coordinator decides a split with 3 stories, 1 of them for the Writing team
    Then exactly 3 tasks are made, each parked in the user's Backlog with 0 turns
    And each story is kept at the same budget and limit of turns as the epic
    And the Writing team's story goes to that team's coordinator once the user starts it, its description and brief saying it is a story of the epic
    # not under test, checked by hand: the other stories say so too; the test reads the Writing team's story only
    # a story's limits: internal/domain/tasks/limits.go NewLimits; "parked": "backlog": internal/runtime/coordinator/loop.go split

  @EPIC-03 @coordinator @manual
  Scenario: A split adds 1 to 12 stories
    Given an epic
    When the coordinator decides a split with 0 stories, or with 13
    Then the decision is refused with the reason "a "split" decision needs "stories": 1 to 12 of them"
    And a split with 12 stories, each with a title and a brief, is allowed
    # MaxStories = 12: internal/runtime/coordinator/decision.go checkStories
    # Steps: end an epic's coordinator turn with a split of 13 one-line stories and read the issue in the trail;
    # then with 12 and check 12 stories appear in the Backlog.

  @EPIC-04 @coordinator @test
  Scenario: A story's after list says which stories it builds on
    # internal/runtime/coordinator/epics_test.go TestASplitStoryBuildsOnAnExistingStoryOrAnEarlierOne; the wait: TestAnEpicRunsItsStoriesAndFinishesWithThem
    Given a split whose second story has "after": [1], and an epic with an existing story task-N
    When the stories are made
    Then the second story's after list names the first story's id, and an "after" naming task-N is kept as that id
    And a story that builds on others is not started until every one of them is done
    And a story whose "after" names itself, a story later in the list, or a number outside it, is refused
    # not under test, checked by hand: the waiting story's branch starts from a dev that has their work, and an "after" naming a task not in this epic is refused
    # the refusal: internal/runtime/coordinator/epics_test.go TestASplitStoryAfterAStoryNotBeforeItIsRefused; blocked: internal/runtime/coordinator/loop.go

  @EPIC-05 @coordinator @test
  Scenario: A story cannot itself be split
    # internal/runtime/coordinator/epics_test.go TestAStoryCantBeSplitAndAnEpicIsntAssigned
    Given a story of an epic
    When its coordinator decides a split
    Then the decision is refused as an issue saying a story can't be split
    And a task the user made a story is never split either
    # not under test, checked by hand: the issue says to put what else the epic needs in its summary, and an assign on an epic is refused as an issue saying its work is done in its stories: internal/runtime/coordinator/next.go line 151

  # ---- Plan ------------------------------------------------------------------

  @EPIC-06 @user @test
  Scenario: Plan has a column for the tasks of no epic, then one for each epic
    # ui/src/pages/PlanPage.test.tsx
    Given the epic task-21 with its story task-22, and the task task-30 of no epic
    When the Plan page is shown
    Then the ids are shown in the order task-30, epic-21, task-22
    And the epic's column has a button "Open epic-21"

  @EPIC-07 @user @test
  Scenario: Filing a task under an epic, moving it and taking it out
    # internal/web/api_test.go TestTasksAreFiledUnderEpics
    Given 2 stories and an epic made with 1 of them
    When the user files the second story under the epic
    Then the epic lists exactly 2 stories
    When the user files the epic under a story
    Then it is refused with 409 "isn't an epic"
    When the user moves a story to another epic and takes the other out
    Then the first epic lists exactly 0 stories and the moved story names the second epic

  @EPIC-08 @user @manual
  Scenario: Dragging a card in Plan sets its priority or files it under another epic
    # ui/src/pages/PlanPage.tsx drop
    Given Plan with the epic epic-7 and 3 unstarted tasks in "No epic"
    When the user drags the third task to the top of "No epic"
    Then it is shown first, and the order is kept after a reload
    When the user drags it into epic-7's column
    Then a toast "task-N is a story of epic-7" is shown once and the card is shown in epic-7
    When the user drags it back to "No epic"
    Then a toast "task-N is out of its epic" is shown once
    Given a finished epic, folded behind "Finished · 1"
    Then its column accepts no drop and shows no Story button

  @EPIC-09 @user @manual
  Scenario: Start on a story, and Start the ready ones on an epic
    # ui/src/pages/PlanPage.tsx start, PlanCard and PlanColumn
    Given epic-7 with 3 stories in the Backlog, none started and none waiting on another
    Then each card reads "Not started" and shows Start
    And the epic's header shows "Start the ready ones · 3"
    When the user clicks Start on 1 card
    Then a toast "Started task-N" with "On the Tasks board now." is shown once
    And that card no longer shows Start, and the header shows "Start the ready ones · 2"
    When the user clicks "Start the ready ones"
    Then a toast "Started 2 stories" is shown once and the header button is hidden

  # ---- Stories that build on others ------------------------------------------

  @EPIC-10 @user @test
  Scenario: A story that builds on another waits until that one is done
    # internal/runtime/coordinator/wrapup_test.go TestAStoryAfterAnArchivedTaskStarts
    # a blocked story gets no turn: internal/runtime/coordinator/loop.go line 134
    Given a story that builds on a task not yet done
    When the loop looks at it
    Then the story is counted as blocked and cannot start
    Given a story that builds on a task that is done and archived
    When the loop looks at it
    Then the loop does not count it as blocked, as an archived task counts as done

  @EPIC-11 @user @manual
  Scenario: A waiting story says what it waits for, and stays parked when that is done
    # ui/src/lib/epics.ts waitingLine; the ready count: ui/src/pages/PlanPage.tsx line 175
    # a parked story gets no turn: internal/runtime/coordinator/next.go line 55
    # what blocks it empties when task-8 is done: internal/web/views.go lines 135 to 145
    # "Not started" and Start: ui/src/pages/PlanPage.tsx lines 298 to 304 and 341
    Given epic-7 with task-8 not started and task-9 that builds on task-8, both in the Backlog
    When the Plan page is shown
    Then task-9's card reads "Starts when task-8 is done"
    And the epic's header counts only task-8: "Start the ready ones · 1"
    When task-8 is done
    Then task-9 stays in the Backlog, its card reads "Not started" and shows Start
    And the epic's header reads "Start the ready ones · 1", counting task-9 now

  @EPIC-12 @user @manual
  Scenario: A story started before the one it builds on waits, then starts once
    # a blocked story gets no worktree: internal/runtime/coordinator/loop.go line 134
    # the epic wakes its stories without a worktree: internal/runtime/coordinator/loop.go lines 532 to 541
    Given epic-7 with task-8 not done and task-9 that builds on task-8
    When the user clicks Start on task-9 while task-8 is not done
    Then task-9 is shown on the Tasks board and gets no worktree
    And its card still reads "Starts when task-8 is done"
    When task-8 is done
    Then task-9 starts exactly once, from dev
    And its card no longer shows the waiting line

  # ---- Waiting and closing an epic -------------------------------------------

  @EPIC-13 @user @test
  Scenario: An epic onto Done closes it and parks its open stories
    # internal/web/api_test.go TestYourMovesOnTheBoardStand
    Given an epic with 2 stories, 1 of them with a turn in progress and 1 not started
    When the user moves the epic to Done
    Then the epic is done in the same request
    And the story at work is parked in the Backlog with its turn stopped and its wrap-up turn given: yes
    And the story not started stays parked and gets no wrap-up turn

  @EPIC-14 @user @test
  Scenario: An epic with every story done asks once, and waits quietly if left open
    # internal/runtime/coordinator/epics_test.go TestAnEpicStaysOpenWithEveryStoryDone
    # the question with the option "Close the epic": internal/runtime/coordinator/loop.go lines 375 to 378
    Given an epic whose 1 story is done
    When its coordinator finishes
    Then the epic asks the user exactly once whether to close it
    When the user answers "Leave it open, there may be more."
    Then the epic stays open and working, asked exactly 1 time, with no question waiting
    And nothing more runs: labelling it starts no turn
    When the user adds a story and it is done
    Then the epic hears it and asks again: asked exactly 2 times

  @EPIC-15 @user @test
  Scenario: Close the epic from its page, and one Visionaries session follows
    # internal/web/api_test.go TestYouCloseAnEpicYouLeftOpen and TestYouCloseAnEpic
    # internal/runtime/coordinator/followups_test.go TestClosingAnEpicStartsOneVisionariesSessionAboutIt
    # ui/src/components/organisms/OpenEpic.tsx lines 34 to 45; internal/web/api.go lines 418 to 427
    # the words "that is approved on the page": internal/web/api.go line 421; the test asserts the 403 only
    Given an epic left open with every story done, whose page shows the panel "All 1 stories done" with the button "Close the epic"
    Then the epic is not in Questions
    When the user clicks "Close the epic"
    Then the epic is done with its closing summary, and its closing is empty
    When Claude Code's desk sends "Close the epic" instead
    Then it is refused with 403
    Given a Visionaries team and the setting on
    When the user closes an epic
    Then exactly 1 Visionaries session starts, titled "Follow-ups after <epic title>", and a second pass starts no other

  @EPIC-16 @coordinator @test
  Scenario: Only an epic with stories may wait
    # internal/runtime/coordinator/next_test.go TestWhenAnEpicMayWait
    Given a task that is not an epic, or an epic with no stories yet
    When its coordinator decides {"action": "wait"}
    Then the decision is refused as an issue saying only an epic with stories can wait
    And an epic whose stories are all done is refused too, with "finish it with a summary", until the user has answered once about closing it

  @EPIC-17 @coordinator @test
  Scenario: The epic's coordinator waits while stories run and asks the user to close it when all are done
    # internal/runtime/coordinator/epics_test.go TestAnEpicStaysOpenWithEveryStoryDone; the wait while stories run, and the question: TestAnEpicRunsItsStoriesAndFinishesWithThem
    Given an epic whose stories are all running
    When its coordinator decides {"action": "wait"}
    Then no turn starts on the epic until a story's summary comes back to it
    And once every story is done, a done decision asks the user "Close it?" with the option "Close the epic", and the epic is not done until they do
    And if the user leaves it open, it stays open with no further question until they close it or add a story
    # CloseEpic: internal/runtime/coordinator/epics.go; the question: internal/runtime/coordinator/loop.go apply, Finish

  @EPIC-18 @user @test
  Scenario: Closing a done epic starts one Visionaries follow-up session about it
    # internal/runtime/coordinator/followups_test.go TestClosingAnEpicStartsOneVisionariesSessionAboutIt
    Given a team made from the Visionaries template, and the Ideas page setting for sessions on epic close is on
    When the user answers "Close the epic" on a done epic
    Then exactly 1 new task is made for that team, titled "Follow-ups after <the epic's title>", with the epic's and each story's summary in its brief
    And a second pass over the same close makes no second session
    And the session takes the server's limits, not the epic's
    # "about" and "close" mark the session: internal/runtime/coordinator/followups.go followUp

  @EPIC-19 @user @test
  Scenario: No follow-up session when turned off, when the epic was dragged to Done, or for a close before the loop started
    # internal/runtime/coordinator/followups_test.go TestNoSessionWhenTheSettingIsOff
    Given the Ideas page setting for sessions on epic close is off
    When the user closes a done epic
    Then no session is made and no note is left on the epic
    And an epic moved to Done on the board gets no session either
    And an epic closed before the server's loop started gets no session, only closes after it do
    # TestAnEpicMovedToDoneGetsNoSession and TestTheLoopFollowsUpOnlyCloseAfterItStarted in the same file; startedAt: internal/runtime/coordinator/loop.go NewLoop
