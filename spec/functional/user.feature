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
    # a blocked story gets no turn: internal/coordinator/loop.go line 133
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
    # the 2 steps and the done step are proven through the chevron count alone: 2 chevrons for 3 steps, HandoffPath.test.tsx line 14
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
