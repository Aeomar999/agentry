Feature: The user

  The user is the person at the Agentry page: they describe work in their own words,
  watch it move across the board, answer what the agents ask, and shape epics and
  their stories in Plan. Every scenario here is behaviour that exists on dev today.

  @USER-01 @test
  Scenario: A new task goes to its team's coordinator
    Given Agentry runs with its Software team
    When the user adds a task from the page, with a description in their own words
    Then the task is shown with the team's coordinator as its coordinator
    And the path its work took shows exactly 1 handoff, from the user to the coordinator
    # README.md, First steps; internal/web/api_test.go TestCreateListAndShowATask

  @USER-02 @manual
  Scenario: New task opens as a modal from the key n and the New task button
    # n: ui/src/lib/shortcuts.ts line 18, bound in ui/src/components/templates/AdminShell.tsx
    Given the Tasks page is open and no field has the focus
    When the user presses n
    Then a dialog titled "New task" is shown over the page
    And the address carries ?new=task, so a reload or a link opens the same dialog
    When the user closes the dialog with Cancel
    Then the dialog is hidden and ?new is no longer in the address
    When the user clicks New task in the page header
    Then the same dialog is shown

  @USER-03 @manual
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

  @USER-04 @test
  Scenario: A task needs only a description, and its first line names it
    # internal/web/api_test.go TestATaskNeedsOnlyADescription
    # 72 characters: internal/web/api.go line 348, titleFrom
    # the words "describe the task": internal/web/api.go line 127; the test asserts the 400 only
    Given a server with no tasks
    When the user adds a task with the description "Fix the login redirect" followed by a second line, and no title
    Then the task is created with exactly the title "Fix the login redirect"
    And a first line longer than 72 characters is cut at a word and ends in "..."
    When the user adds a task whose description is only spaces
    Then the task is refused with 400

  @USER-05 @test
  Scenario: The token budget and turn limit default to the server's, and Unlimited is allowed
    # internal/web/api_test.go TestANewTaskHasItsLimits
    # 5,000,000 tokens: cmd/agentry/serve.go line 102, the -budget flag's default
    # 24 turns: internal/tasks/task.go line 112, DefaultMaxTurns
    Given a server whose default budget is 5,000,000 tokens
    When the user adds a task without choosing a budget or a turn limit
    Then the task's budget is exactly 5,000,000 tokens and its turn limit is the server's default
    When the user adds a task with a budget of 200,000 tokens and 50 turns
    Then the task keeps exactly those 2 limits
    When the user adds a task with both limits Unlimited
    Then the task's budget and turn limit are both 0, which is no limit
    When a turn limit of -5 is sent
    Then the task is refused with 409

  @USER-06 @manual
  Scenario: The limit lists on the form offer presets, the default, Unlimited and Custom
    # budget presets 500k, 1M, 2M, 5M, 10M: ui/src/lib/limits.ts line 26
    # turn presets 12, 24, 50, 100: ui/src/lib/limits.ts line 33
    # the default is marked "(default)": ui/src/components/molecules/LimitSelect.tsx
    Given the New task dialog is open on a server with the default limits
    When the user opens the Token budget list
    Then it lists exactly 7 choices: 500k, 1M, 2M, 5M (default), 10M, Unlimited and Custom
    When the user opens the Max turns list
    Then it lists exactly 6 choices: 12, 24 (default), 50, 100, Unlimited and Custom
    When the user picks Custom for Max turns and types "abc"
    Then the field is marked invalid and the line "A whole number of turns up to 100,000, such as 30." is shown
    And Add task is refused with "Give the budget and max turns as numbers, or pick a preset."

  @USER-07 @manual
  Scenario: The team and kind fields on the form
    # the Team field shows only with more than 1 team: ui/src/components/organisms/NewTaskForm.tsx, teams.length > 1
    Given 2 teams, Software and Writing, and the New task dialog is open
    Then the Team field is shown with the chips "Let the coordinators decide", Software and Writing
    And "Let the coordinators decide" is the 1 chip pressed
    And the Kind list reads "Let the coordinator decide", with Story and Epic as the other 2 choices
    Given only the Software team
    Then the Team field is hidden

  @USER-08 @test
  Scenario: Another repo named by a git address is cloned for the task
    # internal/web/api_test.go TestATaskInARepoNamedByItsAddressWorksInAClone
    Given a git repository reachable by an address
    When the user adds a task with "Another repo" set to that address
    Then the task's repo is a clone under the repos folder, not the address itself
    And the clone's origin is exactly the address given
    When the user adds a task whose address names no repository
    Then the task is refused with 400

  @USER-09 @test
  Scenario: A new repo is made at the path the user gives
    # internal/web/api_test.go TestATaskCanStartANewRepo
    Given a path where no folder exists
    When the user adds a task titled "Design market data" with "A new repo" at that path
    Then the task's repo is exactly that path
    And the new repo has exactly 1 commit, "Start: Design market data"
    When the user adds another task with "A new repo" at the same path
    Then the task is refused with 400

  @USER-10 @manual
  Scenario: The repo choice on the form asks for a full path
    # "Give the repo's full path, starting with /.": ui/src/components/organisms/NewTaskForm.tsx
    Given the New task dialog is open
    Then "Where the work goes" shows 3 choices: "The default repo", "Another repo" and "A new repo"
    And the repo field is hidden while "The default repo" is picked
    When the user picks "Another repo" and types "projects/app"
    And clicks Add task
    Then the task is not created and the banner reads "Give the repo's full path, starting with /."

  @USER-11 @test
  Scenario: Labels are trimmed, lowercased and kept short
    # internal/web/api_test.go TestTasksCarryLabels
    # at most 12 labels of 1 to 32 characters: internal/rules/task_rules.go lines 156 and 160
    Given a server with no tasks
    When the user adds a task with the labels "Bug", " auth ", "bug" and ""
    Then the task carries exactly 2 labels, bug and auth
    When the user saves the labels as only "frontend"
    Then the task carries exactly 1 label, frontend
    When the user saves a label of 40 characters
    Then the change is refused with 409

  @USER-12 @manual
  Scenario: The label field on the form and in Details
    # ui/src/components/molecules/LabelInput.tsx and ui/src/components/organisms/LabelEditor.tsx
    Given the New task dialog is open and another task already carries the label docs
    When the user types "Bug" in Labels and presses Enter
    Then exactly 1 chip, bug, is shown in the field
    When the user types "ui," with a trailing comma
    Then a second chip, ui, is shown and the field is empty
    When the user presses Backspace in the empty field
    Then the chip ui is removed and only bug is kept
    And the field's suggestions list docs, the 1 label other tasks use
    Given a task's Details tab
    When the user clicks "Add labels" or "Edit labels", adds a label and clicks Save
    Then the label is shown on the task, in the list and on its card

  @USER-13 @manual
  Scenario: Adding a task opens it, and adding a story leaves it in the Backlog
    # a story is parked in the backlog at creation: internal/web/api.go line 185
    Given the New task dialog is open
    When the user describes a task and clicks Add task
    Then a toast "Created task-N" with "The coordinator reads it next." is shown once
    And the page opens the new task, not the list
    Given the New story dialog is open for epic-7
    When the user describes the story and clicks Add story
    Then a toast "Created task-N" with "A story of epic-7, in your Backlog until you start it." is shown once
    And the dialog closes and the user stays on the same page
    And the story is shown in the Backlog, not in To do

  @USER-14 @manual
  Scenario: Tasks as a list or a board, with the keys 1 and 2
    # 1 and 2: ui/src/lib/shortcuts.ts lines 24 and 25, bound in ui/src/pages/TasksPage.tsx
    Given the Tasks page is open as a list
    When the user presses 2
    Then the board is shown and the List / Board control has Board pressed
    And the address carries ?view=board, so a link opens the same view
    When the user presses 1
    Then the list is shown with List pressed
    When the user picks Board and reloads the page without ?view in the address
    Then the board is shown, as the last choice is kept in this browser

  @USER-15 @test
  Scenario: The board's columns, with the Backlog only when asked
    # ui/src/components/organisms/TaskBoard.test.tsx
    # the 6 columns in order: internal/tasks/column.go line 39
    Given a board whose columns are backlog, todo, in-progress, review, shipping and done
    And "Show the backlog" is not pressed
    When the board is shown
    Then exactly 5 columns are shown, in the order To do, In progress, Review, Shipping, Done
    And the Backlog column is hidden
    Given a board whose columns leave out shipping, and a task in Review
    Then exactly 4 columns are shown and no column is called Shipping

  @USER-16 @test
  Scenario: Shipping is a column only while it means something
    # internal/web/features_api_test.go TestTheBoardShowsShippingOnlyWhileItMeansSomething
    Given the Software team, whose deployer ships work
    Then the board lists exactly 6 columns, Shipping among them
    When the user adds the Writing team and removes the Software team
    Then the board lists exactly 5 columns and Shipping is not one of them
    Given a task moved to Shipping and then the deployer removed
    Then Shipping is kept while that task is in it
    When that task is moved to Done
    Then Shipping is no longer listed

  @USER-17 @test
  Scenario: Done finishes the task at once, with one last turn for the coordinator
    # internal/web/api_test.go TestYourMovesOnTheBoardStand
    Given a task with a turn in progress
    When the user moves its card to Done
    Then the task is done in the same request, marked as moved there by the user
    And the turn in progress is stopped, not finished
    And the coordinator gets exactly 1 wrap-up turn, and nothing else runs

  @USER-18 @test
  Scenario: To do and the Backlog park the task
    # internal/web/api_test.go TestYourMovesOnTheBoardStand and TestCardsMoveThroughTheStages
    Given a task with a turn in progress
    When the user moves its card to To do
    Then the task is held, parked in todo, in the same request
    And the turn in progress is stopped and the coordinator gets exactly 1 wrap-up turn
    Given a task just added, with no turn running
    When the user moves it to the Backlog
    Then the task is held, parked in backlog, and shown in the Backlog only
    When the user moves it to To do
    Then the task is parked in todo, and no turn runs until the user moves it on

  @USER-19 @test
  Scenario: An epic onto Done closes it and parks its open stories
    # internal/web/api_test.go TestYourMovesOnTheBoardStand
    Given an epic with 2 stories, 1 of them with a turn in progress and 1 not started
    When the user moves the epic to Done
    Then the epic is done in the same request
    And the story at work is parked in the Backlog with its turn stopped and exactly 1 wrap-up turn
    And the story not started stays parked and gets no wrap-up turn

  @USER-20 @test
  Scenario: In progress starts a parked task, and Review has it checked next
    # internal/web/api_test.go TestCardsMoveThroughTheStages
    Given a task parked in To do
    When the user moves it to In progress
    Then the task is working, no longer parked, and no new step is written, as there is nothing to say
    When the user moves it to Review with the note "Check the redirect on Safari."
    Then the coordinator's next direction reads exactly "Have this task reviewed next. Check the redirect on Safari."

  @USER-21 @test
  Scenario: Shipping ships the task now, and a team with no shipper refuses the move
    # internal/web/api_test.go TestYourMoveToShippingShipsItNow
    Given a task of the Software team in To do
    When the user moves it to Shipping
    Then the task is no longer parked and its column is shipping
    Given a task of the Research team, which has no agent that ships work
    When the user moves it to Shipping
    Then the move is refused with 409 "Research has no agent that ships work"
    And no event is written to the task
    And the task's canShip is false, so the board offers it no Shipping

  @USER-22 @manual
  Scenario: A drag the board does not allow
    # "No agent in this team ships work": ui/src/components/organisms/TaskBoard.tsx line 153
    Given the board with a card of a team that has no agent that ships work
    When the user drags that card over Shipping
    Then the column does not accept the drop and its border is not marked as a target
    And the line "No agent in this team ships work" is shown under the column's heading
    Given a done card in Done
    When the user drags it over Done
    Then the drop is not allowed and no dialog opens

  @USER-23 @test
  Scenario: A move the route refuses
    # internal/web/api_test.go TestCardsMoveThroughTheStages
    # the words: internal/web/board_api.go lines 92 and 112; the test asserts the codes only
    Given a task that is done
    When a move to done is sent for it
    Then the move is refused with 409
    When a move to a column called sideways is sent
    Then the move is refused with 400

  @USER-24 @test
  Scenario: A done task dragged back to In progress is reopened for more work
    # internal/web/api_test.go TestCardsMoveThroughTheStages
    Given a task that is done
    When the user moves it to In progress with the note "The fix broke sign-out."
    Then the task is working again and has no worktree yet
    And the coordinator's next direction reads exactly "Reopen this task. This needs more work: hand it back to an agent that builds it. The fix broke sign-out."

  @USER-25 @manual
  Scenario: Dropping a card on another column asks once, then moves it at once
    # ui/src/components/organisms/TaskBoard.tsx DirectDialog and move
    Given the board with task-5 in In progress
    When the user drops task-5 on Review
    Then a dialog titled "Have task-5 reviewed next?" is shown with a note field and the buttons Send and Cancel
    When the user clicks Send
    Then the card is shown in Review before the server answers
    And a toast "Moved task-5 to Review" is shown once
    When the server refuses a move
    Then a toast "That wasn't sent" is shown and the card goes back to its column

  @USER-26 @test
  Scenario: Dragging within a column or a list row sets the priority order
    # internal/web/api_test.go TestTasksListInPriorityOrder
    Given 3 tasks added in the order first, second, third
    Then the tasks list newest first, in exactly the order third, second, first
    When the user drops the first task between the third and the second
    Then the tasks list in exactly the order third, first, second

  @USER-27 @manual
  Scenario: Done keeps the last 7 days on the board, the list keeps everything
    # 7 days: ui/src/lib/epics.ts line 7, DONE_DAYS
    Given 1 task done 2 days ago and 1 task done 8 days ago
    When the board is shown
    Then the Done column shows exactly 1 card
    And the line "1 more, done over 7 days ago or in a finished epic, are in the list" is shown under it
    When the list is shown with the Done filter
    Then both tasks are shown, 2 rows

  @USER-28 @test
  Scenario: Most running at once is set on the Tasks page
    # internal/web/settings_api_test.go TestYouSetTheMostTasksRunningAtOnce
    # from 1 to 100, 0 for no limit: internal/settings/settings.go line 33, MostRunningTasks
    # presets 1, 2, 3, 4, 5, 8, 10: ui/src/lib/limits.ts line 87
    Given the Tasks page with no limit set
    When the user picks 2 in "Most running at once"
    Then the setting is saved as 2 and the reply says 0 tasks are running
    When a limit of -1 or 101 is sent
    Then the change is refused with 400 and the limit is kept at 2
    When the user picks "No limit"
    Then the setting is saved as 0

  @USER-29 @test
  Scenario: Over the limit, the other tasks wait as queued
    # internal/coordinator/slots_test.go TestTheLoopRunsNoMoreTasksThanTheLimit and TestALimitUnderWhatRunsQueuesEveryOneWaiting
    Given a limit of 2 and 4 tasks started
    When the loop runs
    Then exactly 2 tasks have a turn at once
    And the next task starts by rank once a slot frees, never a third at the same time
    Given a limit of 1 while 2 tasks already run
    Then both keep running, 2 counted as running
    And the 1 task waiting behind them is marked queued

  @USER-30 @test
  Scenario: The Shipping filter chip is there only while the board has the column
    # ui/src/pages/TasksPage.test.tsx
    Given a board without Shipping, and the address ?status=shipping
    Then the Shipping chip is hidden and All is the 1 chip pressed
    Given a board without Shipping but a task in it
    Then the Shipping chip is kept and pressed

  @USER-31 @test
  Scenario: Search finds a task by its id, title, repo, agent or label, and shows epics as epic-N
    # ui/src/pages/TasksPage.test.tsx
    # what the search matches: ui/src/pages/TasksPage.tsx line 211
    Given the list with the epic task-21, its story task-22 and the task task-30
    Then the ids shown are exactly epic-21, task-22 and task-30
    When the user types "epic-21" in "Filter tasks"
    Then exactly 1 row is shown, epic-21
    When the user types "task-21"
    Then the same 1 row, epic-21, is shown

  @USER-32 @manual
  Scenario: Filtering by a label or an epic, and the empty result
    # ui/src/pages/TasksPage.tsx pickLabel, pickEpic and EmptyState; the chips: lines 262 and 263
    # the 7 filters: ui/src/pages/TasksPage.tsx line 46, FILTERS
    Given the list with 1 task labelled bug and 1 without, and the key / to reach the search
    Then the chips are shown in the order All, Needs you, Working, Review, Shipping, Held, Done, each with its count
    When the user clicks the bug chip on a row
    Then only the labelled task is shown, the address carries ?label=bug, and a chip "label: bug  ×" is shown
    When the user clicks that chip
    Then the filter is removed and both tasks are shown
    When the user clicks an epic's name on a Backlog card
    Then only that epic and its stories are shown, with a chip "epic: <title>  ×"
    When no task matches the filters
    Then "No task matches" and "Try another filter." are shown
    Given no tasks at all
    Then "No tasks yet" and "Add one and the coordinator picks it up. Press N." are shown

  @USER-33 @test
  Scenario: Plan has a column for the tasks of no epic, then one for each epic
    # ui/src/pages/PlanPage.test.tsx
    Given the epic task-21 with its story task-22, and the task task-30 of no epic
    When the Plan page is shown
    Then the ids are shown in the order task-30, epic-21, task-22
    And the epic's column has a button "Open epic-21"

  @USER-34 @test
  Scenario: Filing a task under an epic, moving it and taking it out
    # internal/web/api_test.go TestTasksAreFiledUnderEpics
    Given 2 stories and an epic made with 1 of them
    When the user files the second story under the epic
    Then the epic lists exactly 2 stories
    When the user files the epic under a story
    Then it is refused with 409 "isn't an epic"
    When the user moves a story to another epic and takes the other out
    Then the first epic lists exactly 0 stories and the moved story names the second epic

  @USER-35 @manual
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

  @USER-36 @manual
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

  @USER-37 @test
  Scenario: A story that builds on another waits until that one is done
    # internal/coordinator/wrapup_test.go TestAStoryAfterAnArchivedTaskStarts
    # a blocked story gets no turn: internal/coordinator/loop.go line 134
    Given a story that builds on a task not yet done
    When the loop looks at it
    Then the story is counted as blocked and cannot start
    Given a story that builds on a task that is done and archived
    When the loop looks at it
    Then the loop does not count it as blocked, as an archived task counts as done

  @USER-38 @manual
  Scenario: A waiting story says what it waits for, and stays parked when that is done
    # ui/src/lib/epics.ts waitingLine; the ready count: ui/src/pages/PlanPage.tsx line 175
    # a parked story gets no turn: internal/coordinator/next.go line 55
    # what blocks it empties when task-8 is done: internal/web/views.go lines 131 to 141
    # "Not started" and Start: ui/src/pages/PlanPage.tsx lines 298 to 304 and 341
    Given epic-7 with task-8 not started and task-9 that builds on task-8, both in the Backlog
    When the Plan page is shown
    Then task-9's card reads "Starts when task-8 is done"
    And the epic's header counts only task-8: "Start the ready ones · 1"
    When task-8 is done
    Then task-9 stays in the Backlog, its card reads "Not started" and shows Start
    And the epic's header reads "Start the ready ones · 1", counting task-9 now

  @USER-39 @test
  Scenario: The running line counts the queued first
    # ui/src/lib/limits.test.ts
    Given a limit of 2, 2 tasks running and 3 queued
    Then the line reads exactly "Queued: 3 · 2 of 2 running"
    Given a limit of 3, 1 task running and 0 queued
    Then the line reads exactly "1 of 3 running"
    Given no limit and 4 tasks running
    Then the line is empty

  @USER-40 @manual
  Scenario: A task waiting for a slot wears the queued badge
    # ui/src/components/molecules/StatusBadges.tsx QueuedBadge, shown in ui/src/pages/TasksPage.tsx and ui/src/components/molecules/TaskCard.tsx
    Given "Most running at once" set to 1 and 2 tasks started
    When the Tasks page is shown
    Then exactly 1 task wears the badge "queued", in the list and on its card
    And the header line reads "Queued: 1 · 1 of 1 running"
    When the running task's turn ends and the queued one starts
    Then the badge is no longer shown on it

  @USER-41 @manual
  Scenario: A story started before the one it builds on waits, then starts once
    # a blocked story gets no worktree: internal/coordinator/loop.go line 133
    # the epic wakes its stories without a worktree: internal/coordinator/loop.go lines 509 to 518
    Given epic-7 with task-8 not done and task-9 that builds on task-8
    When the user clicks Start on task-9 while task-8 is not done
    Then task-9 is shown on the Tasks board and gets no worktree
    And its card still reads "Starts when task-8 is done"
    When task-8 is done
    Then task-9 starts exactly once, from dev
    And its card no longer shows the waiting line

  @USER-42 @manual
  Scenario: A task's page: the header, the facts under it and its 2 buttons
    # ui/src/pages/TaskPage.tsx lines 282 to 296, the header; lines 360 to 391, the facts
    # README.md, Use it, "A task"
    Given task-5 is open, with a worktree and the engineer at work
    When the page is shown
    Then the header shows the title, the status badge, the id task-5, the repo and the worktree's branch
    And exactly 3 header buttons are shown: Workspace, Terminal and "Redact a text"
    And the facts read Agent, Tokens, "Turns since your answer", Actions, Machine and "Last active", each shown once
    Given a task with no worktree yet
    Then the Workspace button is hidden and the header reads "setting up the worktree"
    Given a task with no agent at work
    Then the Terminal button is hidden and the Agent fact reads "none"

  @USER-43 @manual
  Scenario: The tabs, exactly as rendered, and the one an epic adds
    # ui/src/pages/TaskPage.tsx lines 436 to 449
    Given task-5 is open with 2 shared files and 4 handoffs
    When the tabs are shown
    Then exactly 8 tabs read, in order: Coordinator, Activity, Terminal, Files, Handoffs, Actions, Events, Details
    And Files carries the count 2 and Handoffs the count 4
    And the address carries ?tab=<id>, so a link opens the same tab
    Given the coordinator has asked a question
    Then the first tab reads exactly "Coordinator · asks you"
    Given an epic with 3 stories
    Then a Stories tab with the count 3 is shown first, 9 tabs in all, and it opens by default

  @USER-44 @manual
  Scenario: The plan down the left follows the agent's plan live
    # ui/src/components/organisms/PlanRail.tsx lines 10 to 14 and 65 to 137; ui/src/lib/plan.ts readPlan
    # README.md, Use it, "A task": each agent writes its plan before it starts and ticks it off
    Given task-5 with no turn yet
    Then the rail reads "No plan yet. Each agent writes its plan before it starts, and it shows here as it works."
    Given the coordinator wrote a plan and the engineer is working through a 4 step checklist with 1 step ticked
    When the rail is shown
    Then the coordinator's plan is shown under the heading Task, and the engineer's under Agents, with the count "1/4"
    And the first unticked step wears the pulsing mark "in progress", exactly 1 step at a time
    When the engineer ticks a step in its plan file
    Then the count reads "2/4" without a reload
    When the engineer's turn ends with every step ticked
    Then the count reads "4/4" and the plan moves under Earlier, greyed

  @USER-45 @test
  Scenario: The path above the tabs: engineer › reviewer › tester, the coordinator left out
    # ui/src/components/molecules/HandoffPath.test.tsx "counts an agent's turns in a row in muted ink, not faint" and "hides its chevrons from screen readers"
    # ui/src/components/molecules/HandoffPath.tsx lines 31 to 37, the stages; lines 74 to 80, done
    # the 2 steps and the done step are proven through the chevron count alone: 2 chevrons for 3 steps, ui/src/components/molecules/HandoffPath.test.tsx line 14
    Given turns by the coordinator, the engineer twice in a row, then the reviewer
    When the path is shown
    Then the coordinator is not a step: exactly 2 steps are shown, engineer then reviewer
    And the engineer's step carries the count "×2", for its 2 turns in a row
    And the chevrons between steps are hidden from screen readers
    Given the task is done
    Then the path ends in exactly 1 more step, "done"

  @USER-46 @test
  Scenario: The tokens and turns under the header, with the pencil
    # ui/src/pages/TaskPage.test.tsx "can change its budget and max turns while it isn't done" and "shows its tokens and turns, and changes neither"
    # ui/src/components/organisms/LimitEditor.tsx lines 36 to 52; the lists are @USER-06's, from ui/src/lib/limits.ts lines 26 and 33
    Given a working task with a budget of 5000 tokens, 1200 used, and 8 max turns
    When the facts are shown
    Then the Tokens fact and the "Turns since your answer" fact are shown
    And a pencil "Change the budget" and a pencil "Change the max turns" are shown, 2 pencils
    Given the same task done and archived
    Then the 2 facts are still shown and no pencil is shown

  @USER-47 @test
  Scenario: A message to the coordinator stops the turn in progress, and its reply says what it changed
    # internal/web/chat_api_test.go TestAMessageStopsTheTurnForTheCoordinator and TestTheCoordinatorsReplyComesBeforeItsDecision
    # internal/web/chat_api.go lines 55 to 63
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

  @USER-48 @manual
  Scenario: The chat's field, its status line and the message it refuses
    # ui/src/components/organisms/CoordinatorChat.tsx lines 107 to 124, the status and the hint; line 37, the 3 suggestions
    # a blank message: internal/web/chat_api.go lines 27 to 30
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

  @USER-49 @test
  Scenario: Activity lists everything that happened, newest first, under bands that name the author
    # ui/src/components/organisms/ActivityFeed.test.tsx "names every band's author as copy.md says, newest first", "narrows to the turn from its band, and from its continued band" and "narrowed, the band is the turn's heading alone, with no Show only"
    # ui/src/components/molecules/AuthorBand.tsx lines 12 to 27
    Given a task with turns by the coordinator, the engineer on another machine and the reviewer, with messages from the user between them
    When the Activity tab is shown
    Then the bands read newest first: the reviewer's running turn is the 1 at the top and the user's first message the 1 at the bottom
    And each turn's band names the agent, its team, the turn and its status, such as "engineer, Software team, turn-2, on <machine>, ended"
    And a run of the user's rows is banded exactly "you", and Agentry's own rows exactly "Agentry"
    When the user clicks a turn's band
    Then only that turn's rows are shown, exactly 1 band, with no "Show only this turn" on it

  @USER-50 @test
  Scenario: Activity that cannot be read offers Try again
    # ui/src/components/organisms/ActivityFeed.retry.test.tsx "a failed read, then Try again, then the feed, with focus on it and no other read" and "a retry that fails again shows the alert again; two quick clicks read once"
    # ui/src/components/organisms/ActivityFeed.tsx lines 546 to 565
    Given the task's events answer 502 Bad Gateway
    When the Activity tab is shown
    Then an alert "Couldn't load the activity" with the server's words and a "Try again" button is shown
    When the user clicks "Try again" twice at once
    Then exactly 1 read is made, the alert is hidden and "Loading activity…" is shown
    And the focus is kept on the feed, not dropped on the page
    When the read answers
    Then the rows are shown and the feed is no longer busy

  @USER-51 @test
  Scenario: Events are read a page at a time, and Load earlier reads the page before
    # internal/web/events_api_test.go TestAnEventPageIsBoundedAndSaysWhetherThereIsMore
    # ui/src/components/organisms/ActivityFeed.test.tsx "puts the way to read earlier events under the oldest row, in the scroller"
    # ui/src/live/event-pages.test.ts "load earlier reads the page before the oldest held, until there is none"; the button is hidden without earlier: ui/src/components/molecules/LoadEarlier.tsx line 12
    # 500 events in a page, 2000 at most: internal/web/events_api.go lines 18 and 19
    Given a task with 7 events, a refused proposal among them
    When a page of 3 is read
    Then exactly 3 events come back and the page says more: yes
    When the page before the newest is read with limit 3
    Then the last 3 events come back, newest included, and more: yes
    When every event is read
    Then all 7 come back, the refusal among them, and more: no
    And on the page the "Load earlier" button is shown under the oldest row, in the scroller, only while more lie before it

  @USER-52 @test
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

  @USER-53 @test
  Scenario: Shared files: add one, download it, take it away
    # internal/web/api_test.go TestYouShareFilesWithATask
    # internal/web/shared_api.go lines 40 to 91
    Given task-1 with no shared files
    When the user shares "data/sales.csv" holding 8 bytes
    Then the list shows exactly 1 file, "data/sales.csv", size 8, by "you"
    And the task's detail lists it under shared, 1 file
    When the user downloads it
    Then the body is exactly the 8 bytes shared
    When a file named "../escape" is shared
    Then it is refused with 400
    When the user takes "data/sales.csv" away
    Then the list is empty

  @USER-54 @manual
  Scenario: The Files tab drops files in, up to 2 GB each, and a story also has its epic's files
    # ui/src/components/organisms/SharedFiles.tsx lines 105 to 147
    # 2 GB: internal/sharedfiles/blobs.go lines 22 and 25
    # a story's epic's files, in epic/: internal/sharedfiles/folder.go line 28, internal/coordinator/files.go line 138
    Given task-5's Files tab is open with no files
    Then "No shared files" is shown with "Drop files here to share them with the task's agents. What they leave in .agentry/files shows here too."
    When the user drops 2 files on the tab
    Then a toast "Shared 2 with task-5" with "The next agent finds them in .agentry/files." is shown once
    And each row shows the file, its size, "you" and a Download link and a "Stop sharing it" button
    When a file over 2 GB is shared
    Then it is refused with "a shared file is at most 2 GB" and the row is not added
    Given a story of epic-7, and epic-7 has 1 shared file
    Then the story's worktree has that file under .agentry/files/epic, to read only

  @USER-55 @test
  Scenario: Handoffs shows each handoff as a card: who passed it to whom, and the turn it led to
    # ui/src/components/organisms/HandoffTimeline.test.tsx "draws each step as an article named by who passed it to whom, and when", "says Led to with the facts of an ended turn" and "folds a brief at 8 lines, and reports and your messages at 10"
    # 8 and 10 lines: ui/src/components/organisms/HandoffTimeline.tsx lines 35 and 36
    Given a task whose coordinator handed a brief to the engineer, whose turn-4 ended after 48 minutes with 14 actions and 210k tokens
    When the Handoffs tab is shown
    Then each step is an article named "<from> to <to>, <time>", newest first
    And the card's footer reads exactly "Led to turn-4 · engineer · 48m 00s · 14 actions · 210k tokens" and the button Activity, which opens that turn's activity
    And a brief folds at exactly 8 lines, a report or the user's message at 10, until Show all

  @USER-56 @manual
  Scenario: Actions and Events list the raw records, newest first, and the empty tabs say so
    # ui/src/pages/TaskPage.tsx lines 70 to 113, the columns; lines 455 to 536, the tabs
    Given task-5 with 12 actions, 1 of them waiting for approval
    When the Actions tab is shown
    Then exactly 6 columns read Time, Turn, Agent, Tool, Command, Decision, the newest action first
    And the Actions fact reads 12 with "1 waiting" beside it
    When the user clicks an action's row
    Then the Activity tab opens narrowed to its turn with that row picked out, exactly 1 row marked
    When the Events tab is shown
    Then exactly 5 columns read #, Time, Type, Actor, Data, the newest event first
    Given a task with no actions and no handoff yet
    Then "No actions yet" is shown with "Every tool call an agent makes shows here with the policy's decision."
    And "No handoffs yet" is shown with "When the coordinator hands the task to an agent, the brief shows here."

  @USER-57 @test
  Scenario: Details: the id, repo, epic, budget and turns, each with its control while the task is live
    # ui/src/pages/TaskPage.test.tsx "can change its repo and epic", "shows an epic as epic-N, in the header and its details" and "shows a story as task-N, with its epic as epic-M, linking to task-M"
    # ui/src/pages/TaskPage.tsx lines 540 to 575
    Given a live task's Details tab
    Then a Change button is shown on the Repo row and a list named Epic is shown on the Epic row
    Given a story of task-9
    Then the header reads "story of epic-9" and the link goes to exactly /tasks/task-9
    Given an epic
    Then its id is shown as epic-1 in the header and in Details, 2 times, and never as task-1
    And the erase dialog is titled "Erase epic-1 for good?"

  @USER-58 @manual
  Scenario: Change repo moves the task between turns, and the Epic list files it
    # ui/src/pages/TaskPage.tsx lines 620 to 665, RepoEditor; internal/web/api.go lines 62 to 72
    # ui/src/components/organisms/EpicPicker.tsx lines 12 to 14 and 27 to 35; internal/web/epics_api.go lines 67 and 73
    Given a live task's Details tab
    When the user clicks Change on the Repo row
    Then a field with the current repo is shown, and "Move the task" is disabled until the path differs
    When the user enters a path that is not a repository
    Then the move is refused with 400 and a toast "The task didn't move" is shown
    When the user enters another repository and clicks "Move the task"
    Then a toast "The task moved repos" with "Its worktree is made again there before the next turn." is shown once
    And while a turn is in progress the row reads exactly "moves to <repo> when this turn ends"
    When the user opens the Epic list
    Then it lists "No epic", the epics not done and the task's own epic, each as "epic-N · <title>"
    When the task is filed under a done epic through the route
    Then it is refused with 409 "is done; reopen it to add stories"

  @USER-59 @test
  Scenario: Erase this task asks first, and shows what is left afterwards
    # ui/src/pages/TaskPage.test.tsx "asks first, saying nothing undoes it and what goes, and erases nothing until you say so" and "once erased, shows the task erased with the time, what may be left, and the note on backups"
    # ui/src/components/organisms/EraseTask.tsx lines 59 to 83; internal/web/erase_api.go lines 46 to 102
    Given task-1's Details tab, with "Erase this task" at the bottom
    When the user clicks "Erase this task…"
    Then a dialog "Erase task-1 for good?" is shown reading "Nothing undoes this."
    And it lists what goes: every row of its history, its archive, "the shared files no other task names" and the copies "in its epic and in the task that asked for it"
    And nothing is erased yet: 0 calls
    When the user clicks Keep
    Then the dialog is hidden and nothing is erased
    When the user types "asked to" in "Why, for the log (optional)" and clicks "Erase for good"
    Then exactly 1 erasure is sent, with the reason "asked to"
    And the page reads "task-1 was erased at <time>", lists each warning, and says backups made before then still hold it

  @USER-60 @test
  Scenario: An epic with stories is not erased, and an erased task is gone from every route
    # internal/web/erase_test.go TestAnEpicWithStoriesIsntErased and TestTheRoutesOnAnErasedTaskSayItIsGone
    # ui/src/pages/TaskPage.test.tsx "read again, an erased task shows the server's 410 words, not that there is no such task"
    # internal/web/erase_api.go lines 110 to 137 and 355 to 361
    Given epic task-3 with the live story task-2
    When the user erases task-3
    Then it is refused with 409 "task-3 has stories (task-2); erase or move them out of the epic first"
    Given task-2 archived instead
    Then the refusal adds ", and reopen an archived one (task-2) to move it out"
    Given both archived, or task-2 moved out or erased
    Then task-3 is erased
    When any route of an erased task-2 is called: a move, a limit, its repo, its epic, its files, its labels, an answer, an approval
    Then each answers 410 "task-2 was erased on <date>", never 404
    And the page reads exactly those words, not "No task task-2"

  @USER-61 @test
  Scenario: Redact a text counts first, then redacts, and says what is left
    # ui/src/components/organisms/RedactText.test.tsx "shows the count first, redacts only once you say yes, then what it did and the note on backups"
    # ui/src/pages/TaskPage.test.tsx "asks about the text marked on the page, in %s"
    # internal/web/redact_api.go lines 37 to 68
    Given the user marked "sk-live-0123456789" on task-1's page
    When the user clicks "Redact a text" in the header
    Then a dialog "Redact a text in task-1" is shown and "The text to take out" holds exactly the marked text
    When the user gives the reason "pasted a key" and asks for the count
    Then "3 rows of task-1 hold this text." is shown with "Nothing undoes this.", and 0 rows are redacted yet
    When the user clicks "Redact 3 rows"
    Then exactly 3 rows are redacted, with the reason "pasted a key"
    And "Redacted 3 rows of task-1 at <time>." is shown, and backups made before then still hold the text

  @USER-62 @test
  Scenario: What redacting refuses, and a count gone stale
    # ui/src/components/organisms/RedactText.test.tsx "shows a refusal in the server's words, and redacts nothing" and "a row written since the count is refused 409, counted again and asked again with the new count"
    # internal/web/redact_api_test.go TestARedactionWithAStaleCountIsRefused and TestARefusedRedactionWritesNothing
    Given the text to redact is a number in a row's JSON
    When the user asks for the count
    Then "It can't be redacted: <the server's reason>" is shown and 0 rows are redacted
    Given 1 row held the text when it was counted, and a message holding it arrived since
    When the user clicks "Redact 1 row"
    Then the redaction is refused with 409 "2 rows hold this text now, not 1: count them again", and nothing is written
    And the page counts again and asks again with "Redact 2 rows"

  @USER-63 @test
  Scenario: Needs you: approve or deny an action the policy held
    # internal/web/api_test.go TestApproveOnlyWhatWasAsked
    # the panel "Needs you", reading "engineer asks to run Bash:" with the command, Approve and Deny: ui/src/components/organisms/NeedsYouPanel.tsx lines 77 to 99
    # the routes: internal/web/server.go lines 121 and 122
    Given the engineer asked to run "git push origin HEAD:dev" and the policy asked the user
    When the task is read
    Then its needsYou reads exactly "Waiting for you to approve the push to dev" and its approvals list exactly 1 action
    When the user clicks Approve
    Then the approval is recorded with 200
    When the same action is denied afterwards
    Then it is refused with 409 "already approved"

  @USER-64 @test
  Scenario: An approval nobody answered in time is held, and stays open for the user
    # internal/web/held_approval_test.go TestATimedOutApprovalStaysOpenUntilYouAnswer, and TestTheHookHearsItsRequestIsHeldBeforeItsDeadline for the agent being told no
    # the panel's line "no answer came in time, so engineer was told no and carried on": ui/src/components/organisms/NeedsYouPanel.tsx lines 83 to 88; its toast "Approved: back to the coordinator": line 67
    Given the engineer's hook gave up waiting for an answer on "git push origin HEAD:dev"
    When the wait runs out
    Then the action is held: the agent was told no and carried on, and the task still needs you
    And Questions lists exactly 1 approval, held, with the full command
    When the user approves it late
    Then the action is recorded approved and the coordinator is handed the command back, once

  @USER-65 @test
  Scenario: Questions in the dock gathers what waits for the user, oldest first
    # internal/web/api_test.go TestQuestionsGatherWhatWaitsForYou
    # Ctrl+J: ui/src/lib/shortcuts.ts line 16; the tab "Questions N": ui/src/components/organisms/TerminalDock.tsx line 57
    # oldest first: internal/web/questions_api.go line 54; the 5 kinds, labelled Approve, Question, Budget, "Max turns" and Waiting: ui/src/components/organisms/QuestionsPanel.tsx lines 20 to 26
    Given task-1's coordinator asked "Which layout?" with 2 options and a page, and task-2's policy asks about "git push"
    When the user presses Ctrl+J and opens Questions
    Then exactly 2 questions are listed, the older first
    And the first has kind question, the text "Which layout?", exactly 2 options and the page
    And the second has kind approval, its action, the command "git push" and the rule "every push asks"

  @USER-66 @manual
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

  @USER-67 @test
  Scenario: Raising the budget or the turn limit while stopped at it is the answer
    # internal/web/api_test.go TestRaisingTheBudgetLetsATaskGoOn and TestRaisingTheTurnLimitLetsATaskGoOn
    # internal/web/api.go lines 222 to 275; internal/tasks/task.go lines 145 to 163, StoppedAt
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

  @USER-68 @test
  Scenario: A limit answers only its own stop, and a done task's limits stay
    # internal/web/api_test.go TestALimitAnswersOnlyItsOwnStop, TestADoneTasksLimitsStay and TestALimitIsKeptThoughItsAnswerIsRefused
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

  @USER-69 @test
  Scenario: The stop's question names its limit and the numbers it stopped at
    # internal/web/api_test.go TestQuestionsShowWhichLimitStopped
    Given task-1 stopped at its 2 turns, task-2 at its budget of 200,000 with 210,000 used, and task-3 asked to copy a file
    When Questions is read
    Then task-1's question has kind turns, with exactly 2 of 2 turns
    And task-2's has kind budget, with exactly 210,000 tokens used of 200,000
    And task-3's has kind question and no limit, budget 0

  @USER-70 @test
  Scenario: An epic with every story done asks once, and waits quietly if left open
    # internal/coordinator/epics_test.go TestAnEpicStaysOpenWithEveryStoryDone
    # the question with the option "Close the epic": internal/coordinator/loop.go lines 352 to 356
    Given an epic whose 1 story is done
    When its coordinator finishes
    Then the epic asks the user exactly once whether to close it
    When the user answers "Leave it open, there may be more."
    Then the epic stays open and working, asked exactly 1 time, with no question waiting
    And nothing more runs: labelling it starts no turn
    When the user adds a story and it is done
    Then the epic hears it and asks again: asked exactly 2 times

  @USER-71 @test
  Scenario: Close the epic from its page, and one Visionaries session follows
    # internal/web/api_test.go TestYouCloseAnEpicYouLeftOpen and TestYouCloseAnEpic
    # internal/coordinator/followups_test.go TestClosingAnEpicStartsOneVisionariesSessionAboutIt
    # ui/src/components/organisms/OpenEpic.tsx lines 34 to 45; internal/web/api.go lines 417 to 426
    Given an epic left open with every story done, whose page shows the panel "All 1 stories done" with the button "Close the epic"
    Then the epic is not in Questions
    When the user clicks "Close the epic"
    Then the epic is done with its closing summary, and its closing is empty
    When Claude Code's desk sends "Close the epic" instead
    Then it is refused with 403 "that is approved on the page"
    Given a Visionaries team and the setting on
    When the user closes an epic
    Then exactly 1 Visionaries session starts, titled "Follow-ups after <epic title>", and a second pass starts no other

  @USER-72 @manual
  Scenario: A done task's page: what it still allows and what it no longer does
    # ui/src/pages/TaskPage.tsx lines 410 to 418, the Done banner; line 589, the worktree; lines 500 to 508, the terminal
    # ui/src/components/organisms/LimitEditor.tsx lines 39 and 56; ui/src/components/organisms/CoordinatorChat.tsx line 119; internal/web/chat_api.go lines 36 to 45
    # no Reopen on the page: ui/src/pages/TaskPage.tsx has none; the board's dialog: ui/src/components/organisms/TaskBoard.tsx line 283
    Given task-5 is done, moved there by the user
    When the page is shown
    Then the banner reads exactly "Done · Moved to Done by you" with the summary, or "Shipped" if an agent pushed its work
    And the path ends in "done", and the Terminal tab reads "No agent is working on this task"
    And the Tokens and Turns facts show with no pencil, and Details' Budget and Max turns show with no Change
    And the Worktree panel reads exactly "Removed when the task was done."
    And the page offers no Reopen button; only the board's drag off Done, titled "Reopen and send task-5 back for more work?", and the chat reopen it
    When the user sends a message in the chat, whose hint reads "Sending opens the task again."
    Then the task is reopened, exactly 1 task.reopened event

  @USER-73 @test
  Scenario: An archived task's page is read only
    # ui/src/pages/TaskPage.test.tsx "marks it archived and offers nothing that writes", "keeps its repo and epic as they are" and "shares no files and takes none away"
    # ui/src/pages/TaskPage.tsx lines 229 and 289; ui/src/components/organisms/CoordinatorChat.tsx lines 188 and 189
    Given task-1 archived on a date
    When the page is shown
    Then the badge "Archived <date>" is shown
    And "Add labels" is hidden, the field "Message tech-lead" is hidden, and the chat reads "The task is archived: it takes no messages."
    When the Details tab is shown
    Then no Change button and no Epic list are shown: the repo and the epic only show
    When the Files tab is shown
    Then "Share files" and "Stop sharing it" are hidden

  @USER-74 @test
  Scenario: An archived task takes no events until it is reopened
    # internal/web/archived_test.go TestMovingAnArchivedTaskIsRefused
    # internal/web/archives_test.go TestAnArchivedTaskReadBackStillTakesNoEvents
    Given task-1 done and archived
    When its rank, labels, an answer or a move to done is sent
    Then each is refused with 409 "task-1 is archived: an archived task takes no events"
    And the task is still archived, not unarchived
    When a move of a task that does not exist is sent
    Then it is refused with 404 "no such task task-9"

  @USER-75 @test
  Scenario: Reopening an archived task unarchives it first
    # internal/web/unarchive_test.go TestMovingAnArchivedTaskBackToWorkReopensIt and TestSayingSomethingToAnArchivedTaskReopensIt
    # README.md, Archiving, erasing and redacting, "Reopening it"
    Given task-2 archived, a story of task-3
    When the user moves it to In progress or To do on the board
    Then task-2 is whole again, not done, with its title and its epic kept
    And its log holds exactly 1 task.unarchived and 1 task.reopened
    When the user sends "  " to its coordinator
    Then it is refused with 400 and nothing is written: the task stays archived
    When the user sends "One more thing."
    Then its last 2 events are task.unarchived then task.reopened, and the task is reopened

  @USER-76 @manual
  Scenario: The Workspace page: the git view of the task's worktree
    # ui/src/pages/WorkspacePage.tsx lines 23 to 29 and 130 to 196; ui/src/components/organisms/SourceControl.tsx lines 8 to 13
    # files over 2 MB: internal/workspace/workspace.go lines 163 and 185
    Given task-5 has a worktree with 2 changed files, 1 added file and 3 commits on its branch
    When the user clicks Workspace on the task's page
    Then a new tab opens with the id, title, status badge and branch, and the header counts read "M 2" and "A 1"
    And the sidebar has Changes with the count 3, Commits with the count 3, and Explorer open with every file
    When the user opens a changed file
    Then it is shown next to its last committed version, with the views Code and Changes
    When the user opens a file in a commit
    Then it is shown next to the version before that commit, the header reading "in <7 characters> <subject>"
    When the user opens a file over 2 MB
    Then "Too big to show" is shown; a file that is not text reads "Not a text file"
    Given a worktree with nothing changed
    Then the header reads exactly "nothing to commit" and Changes reads "Nothing waiting to be committed."

  @USER-77 @manual
  Scenario: Markdown and SVG previews in the Workspace
    # ui/src/components/organisms/WorkspacePreview.tsx line 13, previewable; line 81, the outline; lines 169 to 272, the SVG
    # zoom from 5% to 1600%: line 183; 3 backdrops: line 231
    Given a Markdown file with 4 headings and an SVG in the worktree
    When the user opens the Markdown file
    Then it is shown as a document, Preview by default, with the images it names read from the workspace
    And an Outline of its headings is shown beside it, as it has more than 2
    And a link to another file of the worktree opens that file in the same tab; a link to the web opens in a new tab
    When the user clicks "Open Preview to the Side"
    Then the code and the preview are shown side by side, 2 panes
    When the user opens the SVG
    Then the drawing is shown fit to the window, with "Zoom in", "Zoom out", "Fit to the window", "Actual size" and exactly 3 backdrops: dark, light, checker
    And the zoom is kept between 5% and 1600%

  @USER-78 @manual
  Scenario: The Workspace dock: the coordinator, Claude, the terminal and the agents who worked there
    # ui/src/pages/WorkspacePage.tsx lines 72 to 89; ui/src/components/organisms/WorkspaceDock.tsx lines 33 to 49 and 89 to 109
    # the dock's height is kept in this browser, 280 px at first, never under 120: WorkspacePage.tsx lines 75 to 78
    Given the Workspace of task-5 while the engineer works
    When the user presses Ctrl+J
    Then the dock opens on Terminal, with exactly 4 tabs: Coordinator, Claude, Terminal and "Agents N", N the turns so far
    And the Questions tab is shown only while the coordinator asks the user
    When the user opens Claude
    Then a Claude Code session in the worktree is shown and the user is in control at once
    When the user opens Agents
    Then each turn is listed, newest first, as its agent, time, tokens and 1 of 4 states: "working now", "waiting for the usage limit", "ended" or "issue"
    When the user presses Ctrl+J again
    Then the dock is hidden, and its height is kept for the next time

  @USER-79 @test
  Scenario: A link in the chat opens a file in its Workspace preview
    # ui/src/components/organisms/EntryText.test.tsx "sends a repo path to the Workspace preview", "links a task id the board knows and a SHA of the task's commits, and nothing else" and "draws a javascript: link as its label"
    # ui/src/lib/links.ts lines 128 to 138, linkInto; ui/src/components/organisms/CoordinatorChat.tsx line 427
    # README.md, Use it, "Showing work"
    Given an agent wrote "[the design](docs/design.md)" in a report on task-75
    When the text is shown
    Then the link goes to exactly /tasks/task-75/workspace?file=docs%2Fdesign.md&view=preview, in a new tab
    Given the text names task-21, which the board knows, and 7 characters of one of the task's commits
    Then task-21 links to /tasks/task-21 and the commit to the Workspace at that commit, exactly 2 links
    And a word that names no task and no commit, and a javascript: link, are shown as plain text

  @USER-80 @manual
  Scenario: The Workspace button and the task's Stories tab for an epic
    # ui/src/pages/TaskPage.tsx line 293, the button; lines 356 to 358, 437 and 477, Stories
    # the count "1/3", titled "1 of 3 stories done": ui/src/components/molecules/StoryProgress.tsx lines 7 and 12
    # README.md, Use it, "Epics and stories": file tasks under an epic from the epic's Stories tab
    Given an epic with 3 stories, 1 done
    When its page is shown
    Then the Stories fact reads exactly "1/3", titled "1 of 3 stories done", and the Stories tab, count 3, is open
    And the Workspace button is hidden while the epic has no worktree
    Given task-5 with a worktree
    When the user clicks Workspace
    Then exactly 1 new browser tab opens at /tasks/task-5/workspace

  @USER-81 @manual
  Scenario: Ideas is in the sidebar under Work, with the count of open ideas
    # the Work section, Tasks then Plan, then each feature's item with its badge: ui/src/components/organisms/Sidebar.tsx lines 14 to 22 and 65 to 77
    # the Ideas item: ui/src/features/ideas/index.tsx line 10; its badge counts the open ideas: internal/ideas/routes_test.go TestIdeasBadgeCountsOpenIdeas
    # the page header: ui/src/features/ideas/IdeasPage.tsx lines 64 to 66
    Given a Visionaries team and 2 open ideas
    When the sidebar is shown
    Then Work lists exactly 3 items in order: Tasks, Plan, Ideas
    And the Ideas item carries the count 2, and Tasks the count of tasks that need you
    When the user opens Ideas
    Then the page is titled "Ideas" and exactly this line is shown under it: "Features the Visionaries suggest, from sessions on a theme or a question. Accept one to make it a task in your Backlog, or accept and start it."
    And the header shows exactly 1 button, "Ask the visionaries"

  @USER-82 @test
  Scenario: Open ideas are grouped by the session that suggested them, newest session first
    # internal/ideas/routes_test.go TestIdeasListNewestSessionFirst
    # the groups on the page: ui/src/features/ideas/IdeasPage.tsx lines 106 to 124 and 304 to 312, sessionsOf
    Given task-1, an older session in one repo, and task-2, a newer session in another
    And task-2 suggested an idea, then task-1 one, then task-2 another
    When the ideas are read
    Then they come in exactly the order task-2's second, task-2's first, task-1's
    And each is open, with an empty backedBy and no decidedAt
    When one repo's ideas are read with ?repo=
    Then exactly 1 idea comes back, task-1's
    When a repo with no ideas is read
    Then the list is empty

  @USER-83 @manual
  Scenario: An idea's card: the problem, the proposal, why now, the size, the risks and who backed or doubted it
    # ui/src/features/ideas/IdeaCard.tsx lines 25 and 26, the badges; lines 31 to 37, the parts; line 39, Not auto-accepted; lines 41 to 55, the buttons and links; line 97, "no one"
    # the sizes S, M and L read small, medium and large: ui/src/features/ideas/IdeaCard.tsx line 13
    # README.md, Ideas
    Given an open idea of size M, backed by product-strategist and technologist, doubted by skeptic, that follows up epic-7 and was suggested by task-12
    When its card is shown
    Then the title is shown with the badge "medium", and the badge "epic" only when the idea suggests an epic
    And exactly 4 parts are shown in order, "Problem", "Proposal", "Why now" and "Risks", a part with no text hidden
    And "Backed by" lists exactly 2 agents and "Doubted by" exactly 1; with nobody it reads "no one"
    And the line "Not auto-accepted: doubted by skeptic" is shown
    And the foot shows 2 buttons, Accept and Dismiss, then "follows up epic-7" and "from task-12", in which only epic-7 and task-12 are links, each to that task

  @USER-84 @test
  Scenario: Accept makes a task or an epic in the idea's repo, in your Backlog, with your note
    # internal/ideas/ideas_test.go TestAcceptMakesABacklogTaskInTheIdeasRepo and TestAnIdeaIsAcceptedOrDismissedOnce; internal/ideas/routes_test.go TestYouAcceptAnIdeaOnce
    # README.md, Ideas
    Given an open idea "Search the board" in a repo, suggested as a task
    When the user accepts it as an epic with the note "Yes, soon"
    Then exactly 1 task is made, kind epic, parked in backlog, in the idea's repo, for the coordinator tech-lead
    And its description names each of the idea's id, its title "Search the board", its proposal and the note "Yes, soon"
    And the idea is accepted, became that task, with the note "Yes, soon" kept
    And the reply is exactly task-2, column backlog, kind epic, accepted true
    When the user accepts it again
    Then it is refused with 409 "accepted already"
    When the user dismisses it instead
    Then it is refused with 409

  @USER-85 @manual
  Scenario: The Accept dialog: task or epic, the note, the token budget and where the task went
    # ui/src/features/ideas/IdeaDialogs.tsx line 63, the title; lines 95 to 109, Make it; line 110, Note; lines 113 to 127, Token budget; lines 130 to 139, the buttons
    # what was made: lines 64 to 75 and 176 to 191, placeOf; the toast: line 53
    # the budget preset to the epic's for an idea that follows one up, else the server's: lines 160 to 171, untouchedBudget
    Given an open idea "Dark mode" that follows up epic-7, whose budget is 2,000,000 tokens
    When the user clicks Accept on its card
    Then a dialog "Accept “Dark mode”" is shown with "Make it" offering exactly 2 choices, "A task" and "An epic", the suggested one pressed
    And the hint shown for a task is exactly "The session suggests one task: one change, on one branch."
    And "Note" is optional, "Kept with the idea, in its history."
    And "Token budget" is preset to exactly 2,000,000, the hint starting "From epic-7."
    And exactly 3 buttons are shown: "Accept as a task", "Accept and start" and Cancel
    When the user clicks "Accept as a task"
    Then the dialog reads "Made task-N from it. In your Backlog until you start it." with only a Close button, the task linked
    And a toast "Made task-N from the idea" is shown once
    Given the task was made but the idea not marked accepted
    Then the banner "The idea wasn't marked accepted" is shown with the server's reason

  @USER-86 @test
  Scenario: Accept and start makes the task In progress, with the budget you give
    # internal/ideas/routes_test.go TestYouAcceptAndStartAnIdea and TestATaskFromAnIdeaYouAcceptTakesTheServersLimits
    # internal/ideas/ideas_test.go TestAcceptAndStartMakesATaskOutOfTheBacklog and TestAgentryCantAcceptAndStart
    # 5,000,000 tokens and 12 turns are the test server's limits: internal/ideas/routes_test.go line 256
    Given a server whose defaults are 5,000,000 tokens and 12 turns, and open ideas to accept
    When the user accepts 1 with start and a budget of 700
    Then the task is made In progress, not parked, with a budget of exactly 700 and the server's 12 turns
    When the user accepts 1 with start and no budget
    Then its budget is exactly 5,000,000
    When the user accepts 1 with start and a budget of -1
    Then its budget is 0, which is no limit
    When the user accepts 1 without start and a budget of 0
    Then the task is parked in backlog with the budget 5,000,000
    When an accept sends start as "yes" or 1, or a budget of -2 or "lots"
    Then each is refused with 400 and no task is made
    And every accepted idea's becameColumn is the column its task is in: in-progress or backlog
    Given Agentry accepting an agreed idea for you
    Then it never starts the task: an accept with start is refused and nothing is made

  @USER-87 @test
  Scenario: An idea that looks like an open task makes none until you say anyway
    # internal/ideas/routes_test.go TestAnIdeaLikeAnOpenTaskIsRefusedUntilYouSayAnyway
    # ui/src/features/ideas/IdeaDialogs.test.tsx "shows the open task an idea looks like, and makes another only when you say anyway" and "Cancel leaves the idea open, sending nothing more"
    # the titles share more than half their words, past the filler words: internal/ideas/duplicate.go line 121, SameTitleAbove 0.5
    # the banner and the buttons: ui/src/features/ideas/IdeaDialogs.tsx lines 76 to 92
    Given the idea "Make a ship the user chose actually push, and count it shipped only after the push" accepted as task-2
    And the open idea "Make a ship the user chose push, and count it shipped only when it pushed" in the same repo
    When the user accepts the second
    Then it is refused with 409 "looks like the open task task-2", the body naming task-2, its title and its column backlog
    And nothing is written to the log
    And the dialog shows the banner "It looks like an open task" with a link to task-2 and reads "Nothing was made. Make another task from the idea anyway, or cancel to leave the idea open."
    When the user clicks "Make another anyway"
    Then exactly 1 more request is sent, with anyway true, and task-3 is made in the Backlog
    When the user clicks Cancel instead
    Then the dialog closes and nothing more is sent: the idea stays open

  @USER-88 @test
  Scenario: Dismiss puts the idea in the history with your reason
    # internal/ideas/routes_test.go TestYouDismissAnIdea and TestAnUnknownIdeaIsNotFound; internal/ideas/ideas_test.go TestDismissKeepsYourNote
    # the dialog "Dismiss “<title>”" with Reason, Dismiss and Keep, and the toast "Dismissed “<title>”" with "It is in the history.": ui/src/features/ideas/IdeaDialogs.tsx lines 205 to 229
    Given an open idea "Dark mode"
    When the user dismisses it with the note " The OS does it "
    Then the idea is dismissed with exactly the note "The OS does it" and a decidedAt
    When the user accepts it afterwards
    Then it is refused with 409
    When an idea that does not exist, or a task's id, is accepted or dismissed
    Then each is refused with 404 "no such idea"

  @USER-89 @manual
  Scenario: The History panel, and the page with every idea decided
    # ui/src/features/ideas/IdeasPage.tsx lines 86 to 104, the empty states; lines 137 to 145, History; lines 274 to 301, a row
    # README.md, Ideas: the history, "Auto-accepted" with its note on hover
    Given 3 ideas: 1 accepted as task-9, now In progress, 1 Agentry auto-accepted and 1 dismissed with a note
    When the Ideas page is shown
    Then "No open ideas: you have decided on every one. Ask the visionaries for more." is shown above the history
    And the panel "History" reads "3 decided" and lists each idea with its badge, accepted or dismissed
    And the accepted row reads "became task-9 (In progress)", "from task-N" and the date; the dismissed row shows its note under it
    And the auto-accepted row wears the badge "Auto-accepted", its note shown on hover
    Given no ideas at all and a Visionaries team
    Then "No ideas yet" is shown with the button "Ask the visionaries"

  @USER-90 @test
  Scenario: Auto-accept is off until you turn it on, and starting them counts only with it
    # internal/web/settings_api_test.go TestYouTurnAutoAcceptingIdeasOnAndOff and TestYouChooseTheAutoAcceptRule
    # internal/ideas/agreed_test.go TestAgentryAcceptsOnlyWhatTheSettingAndTheVisionariesAllow and TestAgentryStartsOnlyWhileTheSettingsSay
    # the defaults: internal/settings/settings.go line 46
    Given settings never changed
    When they are read
    Then visionariesOnEpicClose is yes, autoAcceptIdeas and autoStartIdeas are no, and autoAcceptRule is everyoneBacked
    When the user turns autoAcceptIdeas and autoStartIdeas on
    Then both are saved and the reply holds every setting
    When the user turns autoStartIdeas off
    Then autoAcceptIdeas is kept on
    When autoAcceptIdeas is sent as "yes", or autoAcceptRule as mostBacked
    Then each is refused with 400, the rule's refusal naming everyoneBacked and nobodyDoubted, and the settings are kept
    Given auto-accept off and an idea every viewpoint backed
    Then Agentry accepts nothing, and the rules refuse an accept in its name
    Given auto-accept on
    Then an idea 1 viewpoint doubted, or 1 did not back, stays open for you; your own accept of it is allowed
    And with autoStartIdeas or autoAcceptIdeas off, a start in Agentry's name is refused

  @USER-91 @test
  Scenario: Which ideas count as agreed: Everyone backed it, or Nobody doubted it
    # internal/ideas/agreed_test.go TestAnIdeaIsAgreedWhenEveryViewpointBacksIt and TestAnIdeaNobodyDoubtedIsAgreedWithTwoBackers; internal/ideas/routes_test.go TestAnOpenIdeaSaysWhyItWasntAutoAccepted
    # the 4 viewpoints of the Visionaries template, product-strategist, user-advocate, technologist and skeptic, under visionary-lead: internal/agents/teams.go lines 71 to 79
    # the page's line: ui/src/features/ideas/IdeaCard.tsx line 39
    Given the Visionaries team with its 4 viewpoints
    When an idea is judged under "Everyone backed it"
    Then it is agreed only when every 1 of the 4 backed it and none doubted it
    And 1 doubter gives "doubted by skeptic", 1 missing backer "not backed by skeptic", and nobody "not backed by product-strategist, user-advocate, technologist, skeptic"
    And a team of only its coordinator agrees on nothing: "the Visionaries have no viewpoints"
    When an idea is judged under "Nobody doubted it"
    Then 2 backers and no doubter is agreed; 1 backer is "only one backer"; none is "no backers"
    And a backer outside the team, or named twice, does not count
    Given auto-accept off
    When the ideas are read
    Then each open idea says why under the rule set, such as "doubted by skeptic", and an agreed one says nothing

  @USER-92 @test
  Scenario: Agentry never accepts a duplicate, and says which idea it repeats
    # internal/ideas/agreed_test.go TestAgentryAcceptsNoIdeaYouHadBefore and TestTheRulesRefuseAgentryADuplicate; internal/ideas/duplicate_test.go TestAnIdeaLooksLikeAnOpenTaskWithACloseTitle
    # the same title once case, punctuation and filler words are dropped, or more than 35% shared words in the problems and proposals, each at least 10 words: internal/ideas/duplicate.go lines 66 to 72, 144 and 148
    # README.md, Ideas: "Under either rule Agentry never accepts a duplicate"
    Given an open idea "Search the board" every viewpoint backed
    When another idea " search the Board" in the same repo was dismissed earlier
    Then the open idea is not auto-accepted: "duplicate of idea-1"
    When the earlier idea was accepted and became task-64
    Then the reason is exactly "duplicate of idea-1 (task-64)"
    When the same title was suggested later and is open or dismissed
    Then this idea is the original: nothing stops it; a later one counts only once it was accepted
    When the same title is in another repo, or the Visionaries named the idea it repeats
    Then another repo's never counts, and a named one gives "duplicate of <that id>"
    And the rules refuse Agentry's accept of a duplicate even when every viewpoint agreed

  @USER-93 @manual
  Scenario: The settings on the Ideas page: follow-ups on an epic close, auto-accept, the rule and starting them
    # ui/src/features/ideas/IdeasPage.tsx lines 226 to 262, the controls; lines 180 to 184, the 2 rules and their hints; lines 264 to 267, Saving and the banners
    # README.md, Ideas: the ticks at the top of the Ideas page; the page has them, not Settings
    Given the Ideas page with a Visionaries team and settings never changed
    When the controls are shown
    Then "Ask the visionaries for follow-ups when you close an epic" is ticked and "Auto-accept ideas the Visionaries agree on" is not
    And "Which ideas qualify" offers exactly 2 rules, "Everyone backed it" and "Nobody doubted it", the first picked, both greyed out
    And "and start them" is greyed out and not ticked
    When the user ticks "Auto-accept ideas the Visionaries agree on"
    Then "Saving…" is shown once, and the rules and "and start them" can be changed
    When a change is refused
    Then the banner "The setting wasn't saved" is shown with the server's words, and the control shows what the server kept

  @USER-94 @test
  Scenario: Ideas shows while a Visionaries team or an idea is there, and the page says what it is otherwise
    # internal/ideas/routes_test.go TestIdeasHiddenWithNoTeamAndNoIdeas, TestIdeasOnWithTheVisionariesTeam, TestIdeasKeptForTheirIdeasWhenTheTeamIsRemoved and TestIdeasBadgeCountsOpenIdeas
    # ui/src/features/ideas/view.test.ts "is off with no ideas and no team, and on with the team", "is on with ideas and no team" and "settles when no template turns Ideas on, on with ideas"
    # the explainer and the offer: ui/src/features/ideas/IdeasPage.tsx lines 155 to 167; ui/src/features/ideas/AskVisionaries.tsx lines 10 to 24
    Given no Visionaries team and no ideas
    When the features are read
    Then ideas is not enabled and not visible, its why "add a team from the Visionaries template, or one of yours based on it, to get ideas"
    And the Ideas page is in its off view, which shows what Ideas is and only the button "Ask the visionaries"
    When the Visionaries team is added
    Then ideas is enabled and shown: "the Visionaries team suggests ideas"
    When the team is removed while 2 ideas are left
    Then ideas is visible but not enabled: "kept while 2 ideas remain; no team from the Visionaries template or one based on it now"
    Given 4 ideas, 1 accepted and 1 dismissed
    Then the badge is exactly 2
    Given no template that turns Ideas on
    Then the page reads "This Agentry has no team template that turns Ideas on, so there is no team to ask." and offers no button

  @USER-95 @manual
  Scenario: Ask the visionaries opens New task with the team picked, adding the team first when you have none
    # ui/src/features/ideas/visionaries.ts lines 49 to 75, ask; ui/src/features/ideas/IdeasPage.tsx lines 58 and 72, the button's label
    # ui/src/lib/new-task.ts: ?new=task&newTeam=<team>; README.md, Ideas, "Ask the visionaries"
    Given the Ideas page and a team made from the Visionaries template
    When the user clicks "Ask the visionaries"
    Then the New task dialog opens with that team picked and the address carries exactly ?new=task&newTeam=Visionaries
    Given no such team yet
    Then the button shown reads exactly "Add the Visionaries team"
    When the user clicks it
    Then the team is added from its template once, the button reads "Adding the team…" meanwhile, and New task opens when the team shows
    When the policy lacks the profile the team needs
    Then the banner "The Visionaries team needs the read-mostly profile, which your policy file doesn't have" is shown with "Add the profile and the team"
    When the team was added but does not show up
    Then the banner "The Visionaries team isn't there" is shown, reading "The team was added but didn't show up; try again."

  @USER-96 @manual
  Scenario: The Agents page: the teams as cards, an org chart or a list, each team with its 3 actions
    # ui/src/pages/AgentsPage.tsx lines 126 to 157, the header and the views; lines 276 to 298, a team's heading; line 96, Remove team only with more than 1 team
    # the list's 7 columns: ui/src/pages/AgentsPage.tsx lines 45 to 69; a card: lines 304 to 340; the org chart: lines 343 to 401
    # README.md, Use it, "Teams"
    Given the Software and Writing teams, 9 agents in all
    When the Agents page is shown
    Then the header has exactly 2 buttons, "Add a team" and "New agent", and the line reads "2 teams · 9 agents"
    And the views are Teams, "Org chart" and List, the address carrying ?view=org or ?view=list for the last 2
    And each team's heading shows its name, "5 agents" or "4 agents", "· <description>" from its template, then "Edit team", "+ Add agent" and "Remove team"
    And each card shows the agent, the badge "coordinator" on the lead, a Review or Shipping badge where its work shows, its description, engine · model, profile and where it runs
    When the user picks List
    Then exactly 7 columns read Name, Team, Role, Description, Engine, Profile, "Runs on"
    When the user picks "Org chart"
    Then each team shows its coordinator, badged "lead", over its agents; a team with none reads "This team has no coordinator yet."
    Given only 1 team
    Then "Remove team" is hidden on its heading

  @USER-97 @test
  Scenario: Add a team from a template: Build your own, then your templates newest first, then the built-in ones
    # ui/src/components/organisms/AddTeam.test.tsx "yours come between Build your own and Built in, newest first, each saying when and where it was saved", "only yours can be deleted or exported", "every card's actions are named with it, and its coordinator chip uses the brand ink" and "is added by its id"
    # ui/src/pages/AgentsPage.test.tsx "while ${request} runs, Esc and a click outside leave it open; once it ends, both close it", run for an add, a delete and an import
    # a built-in template is added by its team name, yours by its id: ui/src/components/organisms/AddTeam.tsx lines 75 to 81
    Given 3 built-in templates and 3 templates of yours, Orders, Papers and Desk, saved on 3 dates
    When the user clicks "Add a team"
    Then the dialog "Add a team" shows exactly 3 headings in order: "Build your own", "Your templates", "Built in"
    And exactly 7 cards come in the order "A team of your own", Orders, Papers, Desk, Software, Writing, Visionaries: yours newest first
    And Papers reads "Saved 12 Oct from the Reading team."
    And only yours have "Delete template" and an Export link to /api/teams/templates/papers/export; a built-in one has neither
    And each card's buttons are named with it: "Add this team, Software", "Change it first, Papers", and "Add another" for a team you have
    When the user clicks "Add this team, Papers"
    Then exactly 1 request adds the template papers by its id, and a toast "Added the Papers team" with "Pick it on New task; its coordinator takes the task." is shown once
    And while the add runs, Esc and a click outside leave the dialog open; once it ends, both close it

  @USER-98 @test
  Scenario: A team whose profile the policy lacks is refused, then added with the profile in one click
    # internal/web/policy_api_test.go TestATeamNeedsItsProfilesInThePolicy
    # ui/src/components/organisms/AddTeam.test.tsx "refused for its profiles, offers them in its own card and adds it with them"
    # the offer's words: ui/src/components/molecules/MissingProfilesOffer.tsx lines 20 to 29
    Given a policy file without the read-mostly profile
    When the user adds the Visionaries team
    Then it is refused with 409, naming exactly 1 missing profile, read-mostly, and the Policy page, and nothing is proposed
    And the offer "The Visionaries team needs the read-mostly profile, which your policy file doesn't have" is shown in that card, with "Add the profile and the team"
    When the user clicks it
    Then exactly 1 more request is sent, with addMissingProfiles and the policy's hash the page saw
    And the policy is changed exactly once, "added profile read-mostly for team Visionaries", and the team is there with its 4 agents under visionary-lead
    When a template that does not exist is named, or the hash is stale
    Then the first is refused with 404 and the second with 409, each as JSON with an error line
    When an agent is added with a profile nobody wrote
    Then it is refused with 400 "no profile called dreaming", never naming the policy file's path

  @USER-99 @test
  Scenario: Build a team of your own: the New team panel, blank or from a copy of a template
    # ui/src/components/organisms/TeamPanel.test.tsx "Build a team in Add a team opens the panel with one coordinator" and "creates the team as typed, the coordinator on the default prompt"
    # internal/agents/custom_test.go TestACustomTeamGetsDefaultPrompts
    # the panel "New team" at ?team=new: ui/src/pages/AgentsPage.tsx lines 114 to 117; "Team name", "Start from" with Blank and "Copy a template", "Create team": ui/src/components/organisms/TeamPanel.tsx lines 63, 174, 202 and 289 to 297
    Given the Add a team dialog
    When the user clicks "Build a team"
    Then the dialog closes and the panel "New team" opens with the focus in "Team name", empty
    And "Start from" has Blank pressed, no Template list, and exactly 1 row, the coordinator, open and picked as "Coordinator, coordinator"
    And the foot reads "Creates the team and its coordinator." and the prompt's hint "Empty: it uses Agentry's coordinator prompt for a team of these agents."
    When the user types " Docs " and clicks "Create team"
    Then exactly 1 request is sent: the team "Docs" with 1 agent, coordinator, coordinates true, engine claude, colour blue, an empty prompt
    And the server gives an empty prompt its default: the coordinator's starts "You are the coordinator", an agent's names it as "one of the agents of the <team> team" and its first line ends "Your job: <its description>."
    And a prompt the user wrote is kept as written

  @USER-100 @test
  Scenario: What a team of your own is refused for, every problem at once
    # internal/agents/custom_test.go TestACustomTeamIsRefused
    # ui/src/components/organisms/TeamPanel.test.tsx "an empty team name is in the summary, which takes focus, and under the field" and "every problem at once: team, agents and each row's fields"
    # an agent's name: lowercase letters, digits and dashes, starting with a letter, at most 31 long: internal/agents/agent.go lines 18 to 22; the page's limit: ui/src/lib/custom-team.ts line 54
    Given the New team panel
    When the user clicks "Create team" with no name
    Then nothing is sent, the summary reads "Couldn't create the team: 1 thing to fix" and takes the focus, and "Give the team a name." is shown under the field
    When a team is sent with a name a team has, in any case, or a built-in template's team
    Then it is refused: "a team called Writing already", "a team called wRiTiNg already" or "NETWORKING is the name of the Networking template's team"
    When a team is sent with no agents, no coordinator or 2
    Then it is refused: "the Kitchen team needs agents", "exactly one coordinator; it has none", "exactly one coordinator; it has two: chef, taster"
    When an agent's name is blank, in upper case, 32 characters, repeated or in the roster
    Then each is refused: "every agent of the Kitchen team needs a name", "lowercase letters, digits and dashes, starting with a letter", "two agents called cook", "an agent called editor already exists"
    When an agent's column is serve or its colour mauve
    Then each is refused, the column being builds, review or ship only

  @USER-101 @test
  Scenario: Edit a team: saved whole in one request, its tasks following, with the profiles it needs
    # internal/web/team_edit_api_test.go TestATeamIsSavedWholeInOneRequest and TestATeamEditNeedsItsProfilesInThePolicy
    # the panel "Edit the <team> team" at ?team=<name>: ui/src/lib/custom-team.ts line 478; "No changes to save.": line 492; "Save changes": ui/src/components/organisms/EditTeamPanel.tsx line 268
    Given the Data team with its lead, analyst and charter, the analyst having learned "Know."
    When the user saves the team as " Numbers ", the analyst renamed statistician on codex, charter removed and plotter added
    Then exactly 1 team.edited event is written and the reply names the team Numbers with its new version
    And statistician keeps "Know." and is in Numbers, plotter is in Numbers with its prompt, and charter is no longer there
    And the team's version is no longer the one sent
    When an agent is saved with a profile nobody ships
    Then it is refused with 400 naming row 1, field profile: "analyst's profile dreaming isn't in policy.toml"
    When an agent is saved with a shipped profile the policy lacks
    Then it is refused with 409 naming missingProfiles read-mostly
    When the save is sent again with addMissingProfiles
    Then it is saved, the agent has read-mostly, and the policy changed exactly once

  @USER-102 @test
  Scenario: Rename a team, and remove a team or an agent once no unfinished task needs it
    # internal/web/api_test.go TestATeamIsRenamedWithItsTasks, TestATeamIsRemovedWithItsAgents and TestAgentsAreEditedFromThePage
    # ui/src/components/organisms/TeamPanel.test.tsx "Remove team asks in the footer, focus on Keep; Keep puts the footer back and focus on Remove team" and "Remove removes the team, closes the panel going back over its entry, says so and shows the page without it"
    # the refusals: internal/web/agents_api.go lines 78, 387 and 399; the page's dialog "Remove the <team> team?": ui/src/pages/AgentsPage.tsx lines 431 to 437
    Given the Data team with task-1 open
    When the user renames it " Analytics "
    Then the reply is exactly team Data, to Analytics; its coordinator and 2 agents are in Analytics and no team is called Data
    And task-1 stays with data-lead, and a new task for analytics goes to data-lead
    Given only the Software team
    When the user removes it
    Then it is refused with 409 "the only one"
    Given the Writing team with task-1 open
    When the user removes the team, or its editor
    Then each is refused with 409 naming task-1; an agent no task needs is removed
    When the user removes the only coordinator of a team
    Then it is refused with 409
    When the user clicks "Remove team" in the Edit team panel
    Then "Remove the Docs team and its 3 agents?" is shown with "Their prompts and what they learned go too.", the focus on Keep, nothing sent yet
    When the user clicks Remove
    Then exactly 1 request removes the team, a toast "Removed the Docs team" is shown once, and the panel closes

  @USER-103 @test
  Scenario: Save a team as a template, never over a name a template has
    # internal/web/templates_api_test.go TestATemplateIsSavedUnderItsName and TestATemplateNameOrIDTakenIsRefused
    # ui/src/components/organisms/TeamPanel.test.tsx "the footer becomes a form, named, with the team's name and its coordinator's description, focus on Template name" and "saved, the form closes, says so and focus goes back to Save as a template" and "with unsaved changes it is off and says why beside it, as on a phone"
    # "Save your changes first." while the panel has unsaved changes: ui/src/components/organisms/SaveAsTemplate.tsx lines 134 to 145
    Given the Edit team panel of the Planning team, with no unsaved changes
    When the user clicks "Save as a template"
    Then a form "Save as a template" is shown in the footer's place, "Template name" preset to Planning and "What it's for" to the coordinator's description, the focus in the name
    And the hint reads "Keeps each agent's prompt and settings and which one coordinates; not knowledge or where they run."
    When the user saves it with no name
    Then the template is saved as exactly id planning, team Planning, from Planning, yours, with the time it was saved
    When the user saves it as "  Reading Room  "
    Then its id is reading-room and its team "Reading Room"; built-in templates carry no saved time and no from
    When the user saves a name of yours in another case, or padded, or a built-in template's, or an id of yours
    Then each is refused with 409: "You have a template called My Planning already. Pick another name." or "Software is a built-in template. Pick another name.", and nothing is written
    When the save goes through on the page
    Then a toast "Saved Docs kit to your templates" with "Add it again from Add a team." is shown once and the form closes
    Given unsaved changes in the panel
    Then "Save as a template" is disabled, "Save your changes first." shown beside it

  @USER-104 @test
  Scenario: A team from your template comes with its prompts, a second copy is numbered, and Update my template saves it back
    # internal/agents/user_templates_test.go TestATeamIsAddedFromAUserTemplateWithItsPrompts, TestAUserTemplatesDescriptionFillsAnEmptyCoordinators and TestAUserTemplateAddedAgainGetsNumbers
    # internal/web/templates_api_test.go TestATemplateIsUpdatedFromItsTeam; ui/src/components/organisms/TeamPanel.test.tsx "it updates the template with the team as last saved, and says so" and "with unsaved changes it is off and says why beside it, as on a phone"
    Given your template Papers, described "Reads papers.", with lead and reader
    When the user adds a team from it
    Then lead coordinates Papers with the prompt "Hand out the reading." and reader is in Papers with "Read the papers.", each linked to the template papers
    And lead's empty description is filled with "Reads papers."; one it has is kept, and reader's stays empty
    When the user adds it again
    Then the second team is "Papers 2" with lead-2 and reader-2, the same prompts
    Given the Dreamers team saved as your template Dreams, "Big ideas.", and its lead's prompt changed since
    When the user clicks "Update my template"
    Then exactly 1 request is sent, replacing true, and the template keeps its id dreams and name Dreams with the prompt as last saved
    And a description sent is trimmed and kept, none keeps "Big ideas."; you still have exactly 1 template
    And a toast "Updated Docs kit in your templates" with "It has the Docs team as last saved." is shown once
    Given unsaved changes
    Then "Update my template" is disabled, "Save your changes first." shown beside it, in the footer and in the body

  @USER-105 @test
  Scenario: Export, import and delete one of your templates; a built-in one is neither exported nor removed
    # internal/web/templates_api_test.go TestAnExportedTemplateImportsAsItWas and TestATemplateIsRemoved
    # ui/src/components/organisms/AddTeam.test.tsx "Import a template opens a picker for .json files", "an import is sent as it is, said in a toast, and the list read again", "a refused import says why under the button, a name you have in the server's words" and "Delete deletes it, reads the list again and moves focus to the next card's Add this team"
    # a built-in template: internal/web/templates_api.go lines 188 and 264
    Given your template my-planning
    When the user clicks Export on its card
    Then the file my-planning.json is downloaded as an attachment, 200
    When the user deletes the template and imports the file
    Then the template is back exactly as it was, yours, saved anew
    When the user clicks "Import a template"
    Then a file picker opens for .json files only
    And an import that goes through shows a toast "Imported Papers to your templates" once and the list is read again
    When the import names a template you have
    Then the card "A team of your own" reads "Couldn't import the template" and "You have a template called Papers already. Pick another name.", and the button is not disabled
    When the user clicks "Delete template, Papers" then Delete
    Then the template is deleted, a toast "Deleted the Papers template" is shown once, and the focus moves to the next card's "Add this team"
    When a built-in template is exported or removed
    Then the export is refused with 400 and the removal with 409

  @USER-106 @test
  Scenario: What a team does: its coordinator's description, read by other coordinators or taken from its template
    # ui/src/components/organisms/TeamPanel.test.tsx "it shows the coordinator's description and the line other coordinators read, following the team's name", "a team made from a built-in template is read by the template's description, even with a new coordinator" and "on a team with no template, the coordinator's Description says other coordinators read it"
    # internal/web/templates_api_test.go TestYourTemplatesDescriptionDescribesItsCoordinator; internal/agents/user_templates_test.go TestAUserTemplatesDescriptionFillsAnEmptyCoordinators
    # the field "What this team does" and its hint: ui/src/components/organisms/TeamPanelParts.tsx lines 169 to 176; the heading's "· <description>": ui/src/pages/AgentsPage.tsx lines 169 and 285
    Given the Edit team panel of a team with no template
    When "What this team does" is shown
    Then it holds exactly the coordinator's description, with the hint "One line. Other coordinators read it to choose what to send this team."
    And the line other coordinators read is shown following the team's name as it is typed
    Given a team made from a built-in template
    Then only the template's description is read, and the coordinator's Description says so
    Given a team added from your template
    Then its coordinator is described by the template's description only when it has none of its own
    And the Agents page heading shows "· <description>" only for a team whose template has one

  @USER-107 @manual
  Scenario: An agent's page and its edit page: the prompt, what it knows, its engine, model, profile and where it runs
    # ui/src/pages/AgentPage.tsx lines 42 and 43, the buttons; line 49, Prompt; line 52, Knowledge; lines 56 to 67, Settings; line 33, no such agent
    # ui/src/pages/AgentEditPage.tsx lines 25 and 26; ui/src/components/organisms/AgentForm.tsx lines 118 to 199, the Agent panel; lines 201 to 246, Engine; lines 248 to 277, Runs on; line 280, the button
    # the "Starts each turn" row of an agent with no resume setting of its own, not a reviewer: ui/src/lib/resume.ts line 32
    # the model names claude knows: ui/src/components/organisms/AgentForm.tsx line 28; the knowledge history shows at most 8 changes: ui/src/components/organisms/KnowledgeEditor.tsx line 63
    Given the engineer, on claude with no model, under the developer profile
    When its page is shown
    Then the header shows its name, role badge and description, with exactly 2 buttons, "Redact a text" and Edit
    And the panel "Prompt" reads "Its standing instructions, given at the start of every turn"
    And the panel "Knowledge" has Save, disabled until the text changes, and lists at most 8 past changes, newest first, each by an agent or "you"
    And "Settings" lists exactly 6 rows: Engine claude, Model "the engine's default", "Policy profile" developer, Role "Works on the task", "Starts each turn" "Going on with its session", "Runs on"
    When the user clicks Edit
    Then the form shows exactly 7 fields in the Agent panel: Name, Team, Description, Colour, "Its work counts as" with Building, Review and Shipping, "Starts each turn", and Coordinates
    And Engine offers exactly claude and codex, Model lists opus, sonnet and haiku for claude, and "Policy profile" starts "default (developer)"
    And Save is disabled while the name or the prompt is empty
    When the user opens /agents/new
    Then the page is titled "New agent" and exactly this line is shown: "The coordinator can hand work to it from its next turn."
    Given a name no agent has
    Then the page reads "No agent named <name>"

  @USER-108 @test
  Scenario: An agent is added, changed, renamed and removed from the page, and what it knows can be edited
    # internal/web/api_test.go TestAgentsAreEditedFromThePage, TestRenamingAnAgentKeepsItsTasksGoing, TestYouEditWhatAnAgentKnows and TestTheKnowledgeHistoryIsTheSameFromPatches
    # the toasts "Added <name>" and "Saved <name>" with "The coordinator sees it from its next turn.": ui/src/components/organisms/AgentForm.tsx line 83; "Saved what <name> knows": ui/src/components/organisms/KnowledgeEditor.tsx line 39
    # the fixture starts with the Software team's 5 agents; the test asserts 5 again once designer is removed, not the count in between
    Given the Software team's 5 agents
    When the user adds designer and changes the engineer to codex with the model gpt-5
    Then the engineer is on codex, gpt-5, with an empty list of workers, not null
    When an agent is saved with a worker that does not exist, or the only coordinator is removed
    Then each is refused with 409
    When the user removes designer
    Then it is removed: exactly 5 agents again
    When the user renames the coordinator to lead-dev
    Then it keeps its prompt, its knowledge and coordinates; its open task goes on with lead-dev, and the task's first step is kept naming the old name
    When the user renames it to a name in use
    Then it is refused with 409
    When the user saves the engineer's knowledge as "  Prefer table tests.  "
    Then it is kept trimmed, its history has a change by "you", and a later change to the engineer's settings leaves it
    And the history lists exactly the same versions whether the log kept each as a copy or as a patch, across a rename

  @USER-109 @test
  Scenario: The Policy page: the 6 default profiles, who uses which, and what is in force
    # internal/web/policy_api_test.go TestThePolicyShowsItsDefaultsUsersAndWhatIsInForce
    # the 6 profiles Agentry ships, developer, read-only, read-mostly, network-diagnosis, autonomous and allow-everything: internal/policy/default.toml lines 82, 198, 333, 473, 658 and 683; use_for_everyone as shipped: line 79
    # the Software team's profiles, tech-lead and reviewer read-only, the rest developer: README.md, The policy, "Profiles"
    Given a policy file without read-mostly, with use_for_everyone set to allow-everything
    When the policy is read
    Then exactly 6 default profiles are listed, read-mostly missing and read-only current, the missing one with the text it would add
    And read-only is used by exactly 2 agents, reviewer and tech-lead
    And the reviewer's profile in force reads "read-only, overridden by allow-everything", and the agents list shows each agent's profile in force the same way

  @USER-110 @test
  Scenario: A missing default profile is added from the page, logged with its diff and a backup
    # internal/web/policy_api_test.go TestAMissingProfileIsAddedFromThePageAndLogged; internal/policy/live_test.go TestAgentryKeepsItsNewestBackups
    # the 20 newest backups are kept: internal/policy/live.go line 57; the row "missing" with "Add this profile": ui/src/components/organisms/DefaultProfiles.tsx lines 82 to 89 and 124
    Given a policy file without read-mostly, whose row reads "Your file doesn't have it. Adding it appends the <version> default, under a comment naming the version."
    When "Add this profile" is sent without the file's hash, or with a stale one
    Then it is refused with 409 "changed since the page loaded it: try again"
    When it is sent with the hash the page saw
    Then the file gains exactly 1 profile, read-mostly, under "# Added by Agentry <version>, the default profile of that version."
    And exactly 1 policy event is logged, by you, "added profile read-mostly", with the diff adding [profiles.read-mostly] and the file's text
    And a backup beside the file holds exactly the file as it was
    When it is added twice, or a profile Agentry does not ship is added
    Then the first is refused with 409 "already has" and the second with 404, and no refused change is logged
    Given 25 changes made by Agentry
    Then exactly 20 backups are kept, the newest ones

  @USER-111 @test
  Scenario: An outdated default is updated; one you changed is never overwritten, only replaced by choice
    # internal/web/policy_api_test.go TestAnOutdatedProfileIsUpdatedAndAnEditedOneOnlyByChoice
    # the rows "outdated" and "yours" with "Update to the <version> default", "Keep mine" and "Use the default": ui/src/components/organisms/DefaultProfiles.tsx lines 91 to 113 and 121 to 131
    Given a policy file from a build before v0.1.0, whose read-only is outdated
    When the user clicks "Update to the <version> default" on read-only
    Then the file holds this version's read-only and exactly 1 event, yours, "updated read-only to the <version> default", with its diff
    Given read-only edited by hand, its row marked "yours": "You changed it, so Agentry won't overwrite it. Yours against the default:"
    When an update is sent for it
    Then it is refused with 409 "won't overwrite it" and the file is kept as edited
    When the user clicks "Use the default"
    Then the file is exactly as Agentry ships it, and the event reads "replaced your read-only with the <version> default" with the diff

  @USER-112 @test
  Scenario: use_for_everyone: the notice on the Policy and Agents pages, and turning it off
    # internal/web/policy_api_test.go TestTurningOffUseForEveryoneGivesEachAgentItsOwnProfile and TestThePolicyShowsItsDefaultsUsersAndWhatIsInForce
    # ui/src/components/organisms/UseForEveryoneNotice.tsx lines 21 to 47; shown on the Agents page too: ui/src/pages/AgentsPage.tsx lines 139 to 143
    # README.md, The policy, "Allow everything, as shipped"
    Given the policy as shipped, use_for_everyone set to allow-everything
    When the policy is read
    Then the reviewer's profile in force reads exactly "read-only, overridden by allow-everything"
    When the user clicks "Turn it off"
    Then use_for_everyone is no longer set and the reviewer's profile in force is exactly read-only, not overridden
    When it is turned off again
    Then it is refused with 409 "isn't set"

  @USER-113 @test
  Scenario: Try a call against a profile: the rule that decides, the verdict and who decides today
    # internal/web/policy_api_test.go TestACallIsTriedAsAProfileWouldJudgeIt, TestATriedCallSaysWhichRuleDecidedByItsPlace, TestATriedCallSaysWhoDecidesOnlyWhenAnotherProfileDoes, TestTryingACallChangesNothing and TestATriedCallIsRefusedWhenItCantBeJudged
    # the form "Try a call against <profile>" with Tool, 7 suggested tools, "Command or path" and Try: ui/src/components/organisms/TryPolicyCall.tsx lines 10 and 55 to 80; the lines under it: lines 93 to 104
    # a verdict no profile rule made has Order 0, no rule place: internal/policy/verdict.go line 28; none of the named tests asserts it
    Given the policy as shipped and the developer profile's panel
    When the user tries Bash "go test ./..."
    Then the verdict shown is allow by the rule "stays inside the workspace", with its why, "as if run in <workspace>"
    And the verdict names the rule's place among the profile's rules, exactly 1 row for the page to mark
    When the user tries Bash "git push origin feature", Bash "sudo ls", or Read of a private key under .ssh
    Then the verdicts are ask "every push leaves this machine: you approve it", deny "never sudo" and deny "never touch secrets"
    And under read-only, Edit of main.go is deny "this agent changes nothing"
    And each adds "But today allow-everything decides for every agent:" with allow, while use_for_everyone is set; with it off, no such line
    When a call is tried
    Then nothing is logged and the policy's hash is the same
    When the profile does not exist, the tool is empty, or the workspace is a relative path
    Then they are refused with 404, 400 and 400; a tool with no input is judged

  @USER-114 @test
  Scenario: History lists every version, newest first, and Revert restores one as a new version after showing its diff
    # internal/web/policy_api_test.go TestEveryVersionShowsInTheHistoryAndCanBeRestored, TestAVersionShowsWhatRevertingToItWouldChange and TestARevertMayNotTakeAProfileFromItsAgents
    # the panel "History", who made each version, "refused", "in force", Diff and Revert: ui/src/components/organisms/PolicyHistory.tsx lines 26 to 91 and 135 to 138; the dialog: lines 96 to 129
    Given 3 versions: one loaded without its text, the first loaded, and use_for_everyone turned off on the page
    When the history is read
    Then exactly 3 versions come newest first: "turned off use_for_everyone", "first loaded", "loaded"
    And only the 2 with their text can be reverted to, and none of them is marked on disk
    When the user reads what reverting to the first would change
    Then the diff runs from the version in force to it, turning use_for_everyone back on, and the version names allow-everything where the one in force names no profile
    And the version in force shows an empty diff
    When the user reverts to the first
    Then the file is the first version again and exactly 1 new version is logged, yours, "reverted to the version of <date>"
    When a version without its text, position 999, or the version in force is reverted to
    Then they are refused with 409 "can't be restored", 404, and 409 "in force"
    When a revert would take read-mostly from the Visionaries
    Then it is refused with 409 naming the profile and visionary-lead, and the file is kept

  @USER-115 @manual
  Scenario: The Policy page's panels and banners: each profile, the override, the file, a mistake in it and the reload within 2 seconds
    # ui/src/pages/PolicyPage.tsx lines 47 to 60, the path and the hash; lines 63 to 73, the banners; lines 89 to 93, the file; lines 99 to 161, a profile's panel
    # the notice: ui/src/components/organisms/UseForEveryoneNotice.tsx lines 21 to 47, on the Agents page too: ui/src/pages/AgentsPage.tsx lines 139 to 143
    # the file is watched every 2 seconds: cmd/agentry/serve.go line 280; "Profiles agents need": ui/src/components/organisms/MissingProfiles.tsx lines 26 to 48
    # README.md, The policy
    Given the Policy page
    When it is shown
    Then the header shows the file's path and "file <hash>", the first 8 characters of the hash
    And the banner "Edit the file to change the policy" is shown, reading "Rules are read top to bottom and the first that matches decides. Save the file and it takes effect within two seconds."
    And the panel policy.toml, "The file as loaded", shows every line of the file
    And each profile is a panel marked "used by every agent", "default" or "not in use", with "otherwise <decision>" and a table of its rules: #, Decision, Rule, Why
    And a profile no agent names reads "No agent has this profile."; under the override the users read "Agents' own profile, overridden for now:"
    And under use_for_everyone the notice "Every agent uses allow-everything; their own profiles are ignored" is shown with "Turn it off", on the Agents page too
    And with an agent whose own profile the file lacks, the notice reads "Turning this off denies every call by <agent> until <profile> is back." with "Add it" for a profile Agentry ships
    When the user saves the file with a mistake
    Then within 2 seconds the banner "The file has a mistake, so the version before it is still in force" is shown with the mistake, and the version before stays in force
    Given an agent whose profile the file lacks and Agentry does not ship
    Then the panel "Profiles agents need" lists it, marked missing, with each agent that uses it

  @USER-116 @test
  Scenario: The Settings page lists the features that are off, with the way to turn each on, and what the log holds that this build skipped
    # ui/src/pages/SettingsPage.test.tsx "on a fresh install, lists Ideas with its why and Ask the visionaries", "doesn't list a visible feature, and says nothing is off", "says nothing until the manifest is read", "shows one tally line per prefix and per type", "ignores a feature only the server knows" and "shows a feature with no button by its label and why alone"
    # the page header and the Privacy panel: ui/src/pages/SettingsPage.tsx lines 33 and 53 to 67
    # the why is the server's sentence, internal/ideas/feature.go line 66; the page test's fixture carries a shorter one, ui/src/pages/SettingsPage.test.tsx lines 37 and 52
    Given a fresh install, with no Visionaries team
    When the Settings page is shown
    Then "Features that are off" lists exactly 1 feature, Ideas, with "add a team from the Visionaries template, or one of yours based on it, to get ideas" and the button "Ask the visionaries"
    And "Every feature is on." is not shown
    Given Ideas on
    Then Ideas is not listed, no button is shown, and "Every feature is on." is shown
    Given the features not read yet
    Then nothing is said either way
    Given events the server skipped, 31 from the prefix idea- and 4 of the type git.pr_opened
    Then exactly 2 lines read "31 events from a feature this build doesn't have: idea-" and "4 events of a type this build doesn't know: git.pr_opened"
    Given a feature only the server lists, or one with no button
    Then the first is not listed at all, and the second shows its label and why alone

  @USER-117 @manual
  Scenario: Redact a text everywhere, from the Settings page
    # ui/src/pages/SettingsPage.tsx lines 53 to 69; ui/src/components/organisms/RedactLog.tsx lines 39 and 44
    # README.md, Archiving, erasing and redacting
    Given the Settings page
    When the user clicks "Redact a text everywhere" in the Privacy panel
    Then the address carries ?redact=everywhere and the dialog "Redact a text everywhere" is shown
    And its field's hint reads "Paste it. It is matched exactly, spaces and all, in every task, archived ones too, in the agents and what they learned, and in Agentry's other records, such as the policy, the workers and the ideas."
    When the user closes the dialog
    Then ?redact is no longer in the address and nothing was redacted

  @USER-118 @test
  Scenario: A one-time code signs another device in once, within 5 minutes
    # internal/web/signin_test.go TestASigninCodeSignsInOnce, TestASigninCodeExpires and TestIssuingPastTheCapDropsTheOldestCode
    # 8 characters from 31 letters and digits, no 0, O, 1, I or L, shown as XXXX-XXXX, 5 minutes, at most 20 live: internal/web/signin.go lines 25 to 28 and 74
    # the cookie lasts 365 days: internal/web/server.go line 325
    # README.md, Signing in the installed app, or another device
    Given a device that is signed in asked for a code
    When the other device sends the code in lower case with spaces around its dash
    Then it is signed in: the reply holds the token and a cookie, HttpOnly, SameSite Lax, path /, lasting at least 300 days, not Secure over plain http
    And the cookie alone opens the API, with no token sent
    When the same code is sent again
    Then it is refused with 401 and no cookie
    Given a code 5 minutes old
    Then it is refused with 401; one a second short of 5 minutes signs in
    Given 21 codes issued
    Then the oldest no longer works, while the second and the 21st do

  @USER-119 @test
  Scenario: Wrong codes are limited: 5 a minute from one address, 30 a minute from everyone
    # internal/web/signin_test.go TestFailedSigninsAreLimited, TestFailedSigninsSentAtOnceAreLimited, TestSigningInDoesNotCountAsAFailure and TestThePastedTokenSignsIn
    # 5 failures a minute per client, 30 failed codes a minute in all: internal/web/signin.go lines 30 to 35; the refusals' words: lines 37 and 38
    Given a live code
    When one address sends 5 wrong codes within a minute
    Then its next try, with a good code or the token, is refused with 429 and Retry-After 60
    And another address signs in with the code; a minute later the first address is allowed again
    When 30 wrong codes come from 30 addresses within a minute
    Then every code is refused with 429 for a minute, while the token still signs in
    When 50 wrong codes are sent at once from one address
    Then exactly 5 are refused with 401 and 45 with 429
    And a sign-in that succeeds never counts as a failure
    When a wrong token is sent, or neither a code nor a token
    Then they are refused with 401 "wrong token: paste the link `agentry serve` printed" and 400 "send a code or the token"
    And behind an https proxy the cookie is Secure, yes

  @USER-120 @manual
  Scenario: The person menu, Sign in another device, and the sign-in page
    # ui/src/components/organisms/AccountMenu.tsx lines 29 and 40 to 45; ui/src/components/organisms/SignInDeviceDialog.tsx lines 50, 74 to 96, 97 to 110 and 111 to 118
    # ui/src/pages/SignInPage.tsx lines 52 to 78; what the field takes: ui/src/lib/signin.ts lines 12 to 24, a code being at most 8 characters, line 6
    # README.md, Signing in the installed app, or another device
    Given a device that is signed in
    When the user clicks "Signing in" in the top bar
    Then a menu lists exactly 3 items: "Sign in another device", "Notifications on this device" and "Sign out"
    When the user picks "Sign in another device"
    Then the dialog shows a code like ABCD-EFGH, a QR code of <address>/?signin=<code>, and "Expires in 5:00" counting down
    And it reads "The code works once, for five minutes. A new code doesn't stop this one working." with the buttons "New code" and Done
    And a list of addresses is shown only when the server has more than 1, each with its label
    When the code runs out
    Then "Expired: make a new code" is shown and the QR code is faded
    Given the other device, which opens the sign-in page
    Then the page reads "Sign in" with exactly 1 field, "Code, token or link", and the hint on where to get a code or paste the link
    When the user pastes the printed link, its bare token, a code with or without its dash, or scans the QR code
    Then each signs in once; a link with no token or code in it is refused on the page
    When a wrong code is sent
    Then the banner "Couldn't sign in" is shown with the server's words

  @USER-121 @test
  Scenario: Sign out forgets the cookie on the server and the token on the device
    # internal/web/signin_test.go TestSigningOutClearsTheCookie
    # the page forgets its token and says "Couldn't sign out" when the server refuses: ui/src/components/organisms/AccountMenu.tsx lines 44 and 45
    Given a device that is signed in
    When the user picks "Sign out"
    Then the reply is 200 with the cookie emptied, path /, expiring at once

  @USER-122 @test
  Scenario: Install app shows only where the browser can install the page
    # ui/src/lib/install.test.ts "is refused on a secure page whose service worker the browser refused", "is unavailable on a secure page with a service worker and no offer yet" and "is insecure on a plain http page, whatever the service worker did"
    # the 5 states and the button hidden when installed or unavailable, or refused with no CA: ui/src/lib/install.ts line 17 and 52 to 57; ui/src/components/molecules/InstallAppButton.tsx lines 35 and 36
    # the manifest: ui/public/manifest.webmanifest, display standalone, 3 icons
    # README.md, HTTPS and installing the app on other devices
    Given a secure page whose service worker the browser refused to register
    Then the install state is exactly "refused"
    Given a secure page with a service worker and no offer from the browser yet
    Then the install state is exactly "unavailable"
    Given a page over plain http at a network address
    Then the install state is exactly "insecure", whatever the service worker did

  @USER-123 @manual
  Scenario: The Install app button's note, where the page cannot be installed
    # ui/src/components/molecules/InstallAppButton.tsx lines 47 to 81
    # README.md, HTTPS and installing the app on other devices
    Given Agentry open over plain http at a network address, without -https
    When the user clicks "Install app"
    Then a note is shown: browsers install apps only from secure pages; open Agentry at the localhost address with the port on the computer it runs on, or over https, and install it there
    Given the same page while the server serves -https with its own CA
    Then the note names the https address and its trust page, exactly 2 links, and ends saying the localhost address with the port works too on the computer Agentry runs on
    Given a secure page where the browser refused the service worker, with a trust page
    Then the note shown reads "The browser won't install the app here. If it warned about the certificate, trust Agentry's certificate on the trust page, then reload this page."
    When the browser offers to install
    Then the button installs with exactly 1 prompt, and the button is hidden once the app is installed

  @USER-124 @test
  Scenario: Trusting a device once: the trust page per address, with the CA's fingerprint
    # internal/web/trust_test.go TestThePageIsToldWhereToTrustTheServersOwnCA; ui/src/lib/trust.test.ts "keeps the first eight pairs of a SHA-256"
    # the dialog's section "First time on this device? Trust Agentry's certificate", with its QR code and the fingerprint: ui/src/components/organisms/SignInDeviceDialog.tsx lines 53 to 73
    # README.md, HTTPS and installing the app on other devices, "Trust a device once"
    Given the server serving -https with its own CA, reachable at 2 network addresses and localhost
    When a sign-in code is issued
    Then each address carries exactly 1 trust page, at /trust/ over plain http at that address, with the CA's fingerprint
    And the page cuts the fingerprint to its first 8 pairs, the rest behind an ellipsis
    Given a public address behind a proxy doing TLS
    Then that address carries no trust page, and the others still do
    Given a server with no CA of its own
    Then no reply speaks of trust
    And no reply ever holds the token or a private key

  @USER-125 @test
  Scenario: Notifications on this device need an https public address and a browser that takes pushes
    # internal/web/push_api_test.go TestPushNeedsAnHTTPSPublicURL
    # ui/src/components/organisms/NotifyDeviceDialog.test.tsx "with push off at the server, its reason shows and there is no button", "with push on, the button shows" and "with push on but a browser without it, the browser's reason shows and there is no button"
    # the reasons: internal/webpush/webpush.go lines 234 to 236 and ui/src/lib/push.ts lines 14 and 17
    Given a server started without -public-url
    When the user opens "Notifications on this device"
    Then push is not available: the note reads "Push needs an https address. Start Agentry with -public-url https://… so phones can reach it." and no button is shown
    Given -public-url over plain http
    Then the reason names that address, and subscribing is refused with 409 and the reason
    Given -public-url over https
    Then push is available with the server's public key, and the button "Notify me on this device" is shown
    Given a browser with no push, or notifications blocked for the site
    Then the browser's reason is shown instead and no button

  @USER-126 @test
  Scenario: Turning notifications on and off for this device
    # internal/web/push_api_test.go TestSubscribingTwiceAndUnsubscribing and TestAMalformedPushSubscriptionIsABadRequest
    # ui/src/components/organisms/NotifyDeviceDialog.test.tsx "a subscription made with the server's key is sent again and shows as notified" and "a subscription made with another key is dropped in the browser and the button shows"
    # "Notifications weren't allowed." when the browser refuses permission, in the banner "Couldn't change notifications": ui/src/lib/push.ts line 43; ui/src/components/organisms/NotifyDeviceDialog.tsx line 45
    Given push available and this browser not subscribed
    When the user clicks "Notify me on this device" and allows notifications
    Then the browser's subscription is sent once and the dialog reads "This device is notified when something waits for you." with "Stop notifying this device"
    When the same endpoint is sent twice
    Then exactly 1 subscription is kept
    When the user clicks "Stop notifying this device"
    Then the subscription is removed, 0 subscribed, and removing it again is 404
    When a subscription with no endpoint, an http endpoint or no keys is sent
    Then each is refused with 400 and none is kept
    Given a subscription this browser made with another server's key
    Then it is dropped in the browser, nothing is sent, and the button is shown again

  @USER-127 @test
  Scenario: What arrives: 1 push per device for each thing that newly waits for you, and tapping it opens the task
    # internal/webpush/sink_test.go TestWhatWaitsAtStartIsNotPushed, TestANewEntryIsPushedOnceToEachDevice and TestTwoNewEntriesAreTwoPushes
    # ui/src/lib/sw.test.ts "shows its title and body, tagged, with the page to open", "opens its page when no window of the app is open", "shows its page in the app's open window" and "opens only Agentry, whatever address the push names"
    # a push carries at most 500 characters of the line: internal/webpush/sink.go line 15
    Given 2 devices subscribed and an approval already waiting when the server starts
    When the server starts
    Then nothing is pushed for what already waited
    When a new approval waits on task-4
    Then each device gets exactly 1 push: title "Agentry", body "task-4: Waiting for you to approve the push to dev", opening /tasks/task-4
    And checking again pushes nothing more
    When an approval and 3 open ideas newly wait at once
    Then exactly 2 pushes go out, the ideas one opening /
    When a push arrives in the browser
    Then a notification is shown with its title, body and tag; one that cannot be read shows "Agentry" with an empty body
    When the user taps it
    Then its page opens in the app's open window, or in a new one; a push naming another site opens only Agentry's start page

  @USER-128 @manual
  Scenario: The ? overlay shows exactly the 14 shortcuts, in 4 groups
    # the list: ui/src/lib/shortcuts.ts lines 14 to 29; the modifier the page shows, modKey: line 12
    # ui/src/components/organisms/ShortcutOverlay.tsx lines 8, 16, 19 and 37
    Given any page, with no field focused
    When the user presses ?
    Then a dialog "Keyboard shortcuts" is shown with exactly 14 rows in 4 groups: Anywhere, "Go to", Tasks, Lists
    And Anywhere lists 6: modKey K "Command palette", modKey J "Show or hide the terminal", ? "Show these shortcuts", n "New task", / "Search this page", Esc "Close a palette or overlay"
    And "Go to" lists 3: g then t Tasks, g then a Agents, g then p Policy, each chord drawn as 2 keys with "then" between
    And Tasks lists 2: 1 "Tasks as a list", 2 "Tasks as a board"; Lists lists 3: j "Next row", k "Previous row", Enter "Open the row"
    And modKey is drawn as the page renders it for this device, exactly 1 of ⌘ and Ctrl
    When the user presses Esc or clicks outside
    Then the dialog is hidden

  @USER-129 @test
  Scenario: The command palette finds tasks by their shown id, and an epic by task-N too
    # ui/src/components/organisms/CommandPalette.test.tsx "show an epic as epic-N and a task on its own as task-N", "find an epic by epic-N" and "still find an epic by task-N"
    # the field "Search tasks, agents and commands": ui/src/components/organisms/CommandPalette.tsx line 84
    Given the palette open with the epic task-21 "Ship v2" and the task task-30 "Fix the login", both working
    When nothing is typed
    Then exactly 2 rows read "Ship v2 epic-21 · working" and "Fix the login task-30 · working"
    When the user types epic-21
    Then exactly 1 row, the epic, is shown
    When the user types task-21
    Then the same 1 row, the epic, is shown

  @USER-130 @manual
  Scenario: The keys: the palette and the terminal from anywhere, g chords, j, k and Enter, and nothing while typing
    # modKey K and modKey J are caught before a field or the terminal sees them: ui/src/components/templates/AdminShell.tsx lines 97 to 113; the other keys: lines 116 to 123
    # keys do nothing while typing, a field, a textarea, a select or the terminal: ui/src/lib/shortcuts.ts lines 33 to 37 and 49; Enter only with nothing focused: line 50; a chord's second key within 1200 ms: line 59
    # the palette's keys, groups and "Nothing matches": ui/src/components/organisms/CommandPalette.tsx lines 59 to 65 and 90; j, k and Enter on the Tasks and Agents lists: ui/src/pages/TasksPage.tsx line 242, ui/src/pages/AgentsPage.tsx line 99
    Given any page
    When the user presses modKey K, even while typing in a field or the terminal
    Then the palette opens, the focus in "Search tasks, agents and commands", and the same keys close it
    And its rows are grouped "Go to", Commands, Tasks and Agents; ArrowDown and ArrowUp move, Enter runs the row, Esc closes; "Nothing matches "<text>"." is shown for no match
    When the user presses modKey J
    Then the dock is shown or hidden, from a field too
    When the user presses g then t, g then a, or g then p within 1.2 seconds
    Then the page goes to Tasks, Agents or Policy; a second key later than that is not a chord
    When the user presses j, k and Enter on the Tasks list or the Agents list
    Then the cursor moves down, up, never past the ends, and Enter opens the row under it
    When the focus is in a field, a textarea, a select, the terminal or on a button
    Then n, /, ?, g, j, k, 1 and 2 are text or the control's own, not shortcuts, and Enter is the control's
    And a key with Ctrl, Alt or the command key held is never one of these

  @USER-131 @test
  Scenario: A task done for the archive age is archived, in a round at start and then every hour
    # internal/archive/archive_test.go TestATaskIsArchivedOnceDoneForTheAge and TestTheArchiverRunsAtStartAndThenHourly
    # the age: 30 days unless -archive-after says otherwise, and 0 never archives: cmd/agentry/serve.go lines 70 to 72 and 105; the test's age, 30 days: internal/archive/archive_test.go line 22
    # a round every hour: internal/archive/archive.go line 20, roundEvery
    # the task list still shows an archived task as done, with its summary; its page is marked Archived and read only: @USER-73 and @USER-74; README.md, Archiving, erasing and redacting
    Given the default archive age, 30 days, and 3 tasks done, task-1 first, task-3 reopened since
    When a round runs 30 days after task-1 was done
    Then exactly 1 task is archived, task-1; task-2 and task-3 are not
    When a round runs 30 days after that
    Then exactly 1 more is archived, task-2; the reopened task-3 is not
    Given the archiver started with task-1 done for the age
    Then its first round runs at once: task-1 is archived, and it waits exactly 1 hour for the next
    When that hour passes, task-2 done for the age by then
    Then the round an hour on archives task-2, exactly 1 more
