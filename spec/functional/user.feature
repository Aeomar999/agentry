Feature: The user at the page

  The user is the person at the Agentry page: they describe work in their own words,
  watch it move across the board, answer what the agents ask, and shape epics and
  their stories in Plan. Every scenario here is behaviour that exists on dev today.

  # @USER-01 is the foundation story's stub and is merged in from dev

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
    # 72 characters: internal/web/api.go line 351, titleFrom
    Given a server with no tasks
    When the user adds a task with the description "Fix the login redirect" followed by a second line, and no title
    Then the task is created with exactly the title "Fix the login redirect"
    And a first line longer than 72 characters is cut at a word and ends in "..."
    When the user adds a task whose description is only spaces
    Then the task is refused with 400 "describe the task"

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
    Given a task that is done
    When a move to done is sent for it
    Then the move is refused with 409 "the task is done already"
    When a move to a column called sideways is sent
    Then the move is refused with 400 "no column called sideways"

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
    # what the search matches: ui/src/pages/TasksPage.tsx line 210
    Given the list with the epic task-21, its story task-22 and the task task-30
    Then the ids shown are exactly epic-21, task-22 and task-30
    When the user types "epic-21" in "Filter tasks"
    Then exactly 1 row is shown, epic-21
    When the user types "task-21"
    Then the same 1 row, epic-21, is shown

  @USER-32 @manual
  Scenario: Filtering by a label or an epic, and the empty result
    # ui/src/pages/TasksPage.tsx pickLabel, pickEpic and EmptyState
    # the 7 filters: ui/src/pages/TasksPage.tsx line 46, FILTERS
    Given the list with 1 task labelled bug and 1 without, and the key / to reach the search
    Then the chips are shown in the order All, Needs you, Working, Review, Shipping, Held, Done, each with its count
    When the user clicks the bug chip on a row
    Then only the labelled task is shown, the address carries ?label=bug, and a chip "label: bug ×" is shown
    When the user clicks that chip
    Then the filter is removed and both tasks are shown
    When the user clicks an epic's name on a Backlog card
    Then only that epic and its stories are shown, with a chip "epic: <title> ×"
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
    # a blocked story gets no turn: internal/coordinator/loop.go line 133
    Given a story that builds on a task not yet done
    When the loop looks at it
    Then the loop counts the story as blocked, so it does not get a turn
    Given a story that builds on a task that is done and archived
    When the loop looks at it
    Then the loop does not count it as blocked, as an archived task counts as done

  @USER-38 @manual
  Scenario: A waiting story says what it waits for
    # ui/src/lib/epics.ts waitingLine
    Given epic-7 with task-8 not started and task-9 that builds on task-8
    When the Plan page is shown
    Then task-9's card reads "Starts when task-8 is done"
    And the epic's header counts only task-8: "Start the ready ones · 1"
    When task-8 is done
    Then task-9 starts and its card no longer shows the waiting line

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
