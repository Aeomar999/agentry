Feature: Tasks and the board
  Adding a task, the board and its columns, the Tasks page, a task's page,
  running limits, search, the keys and the archive.

  # ---- Adding a task ---------------------------------------------------------

  @TASK-01 @user @test
  Scenario: A new task goes to its team's coordinator
    Given Agentry runs with its Software team
    When the user adds a task from the page, with a description in their own words
    Then the task is shown with the team's coordinator as its coordinator
    And the path its work took shows exactly 1 handoff, from the user to the coordinator
    # README.md, First steps; internal/web/api_test.go TestCreateListAndShowATask

  @TASK-02 @user @manual
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

  @TASK-03 @user @test
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

  @TASK-04 @user @test
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

  @TASK-05 @user @manual
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

  @TASK-06 @user @manual
  Scenario: The team and kind fields on the form
    # the Team field shows only with more than 1 team: ui/src/components/organisms/NewTaskForm.tsx, teams.length > 1
    Given 2 teams, Software and Writing, and the New task dialog is open
    Then the Team field is shown with the chips "Let the coordinators decide", Software and Writing
    And "Let the coordinators decide" is the 1 chip pressed
    And the Kind list reads "Let the coordinator decide", with Story and Epic as the other 2 choices
    Given only the Software team
    Then the Team field is hidden

  @TASK-07 @user @manual
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

  # ---- Repos and labels ------------------------------------------------------

  @TASK-08 @user @test
  Scenario: Another repo named by a git address is cloned for the task
    # internal/web/api_test.go TestATaskInARepoNamedByItsAddressWorksInAClone
    Given a git repository reachable by an address
    When the user adds a task with "Another repo" set to that address
    Then the task's repo is a clone under the repos folder, not the address itself
    And the clone's origin is exactly the address given
    When the user adds a task whose address names no repository
    Then the task is refused with 400

  @TASK-09 @user @test
  Scenario: A new repo is made at the path the user gives
    # internal/web/api_test.go TestATaskCanStartANewRepo
    Given a path where no folder exists
    When the user adds a task titled "Design market data" with "A new repo" at that path
    Then the task's repo is exactly that path
    And the new repo has exactly 1 commit, "Start: Design market data"
    When the user adds another task with "A new repo" at the same path
    Then the task is refused with 400

  @TASK-10 @user @manual
  Scenario: The repo choice on the form asks for a full path
    # "Give the repo's full path, starting with /.": ui/src/components/organisms/NewTaskForm.tsx
    Given the New task dialog is open
    Then "Where the work goes" shows 3 choices: "The default repo", "Another repo" and "A new repo"
    And the repo field is hidden while "The default repo" is picked
    When the user picks "Another repo" and types "projects/app"
    And clicks Add task
    Then the task is not created and the banner reads "Give the repo's full path, starting with /."

  @TASK-11 @user @test
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

  @TASK-12 @user @manual
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

  @TASK-13 @user @test
  Scenario: A repo named by a link to a .tar.gz is downloaded and made a repo
    # internal/worktrees/repo_test.go TestARepoNamedByALinkToAnArchiveIsDownloaded
    Given a link to a .tar.gz, with a query after its name, whose files sit in one top folder app-1.2
    When Agentry fetches the repo the link names
    Then the link counts as an address, and the archive is downloaded into a folder whose name starts with app-1.2-, which is a git repo: yes
    And the files are at the top of the repo, out of the archive's folder, and main.go is committed: yes
    And a link that answers 404 is refused, saying 404
    # not under test, checked by hand: a task whose Another repo is such a link is fetched this way (internal/web/api.go line 100)
    # README.md, Use it (Add a task); PLAN.md T-044

  # ---- The board and its columns ---------------------------------------------

  @TASK-14 @user @manual
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

  @TASK-15 @user @test
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

  @TASK-16 @user @test
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

  @TASK-17 @user @test
  Scenario: Done finishes the task at once, with one last turn for the coordinator
    # internal/web/api_test.go TestYourMovesOnTheBoardStand
    Given a task with a turn in progress
    When the user moves its card to Done
    Then the task is done in the same request, marked as moved there by the user
    And the turn in progress is stopped, not finished
    And the coordinator is given its wrap-up turn: yes
    # not under test, checked by hand: nothing else runs

  @TASK-18 @user @test
  Scenario: To do and the Backlog park the task
    # internal/web/api_test.go TestYourMovesOnTheBoardStand and TestCardsMoveThroughTheStages
    Given a task with a turn in progress
    When the user moves its card to To do
    Then the task is held, parked in todo, in the same request
    And the turn in progress is stopped and the coordinator is given its wrap-up turn: yes
    Given a task just added, with no turn running
    When the user moves it to the Backlog
    Then the task is held, parked in backlog, and shown in the Backlog only
    When the user moves it to To do
    Then the task is parked in todo, and no turn runs until the user moves it on

  @TASK-19 @user @test
  Scenario: In progress starts a parked task, and Review has it checked next
    # internal/web/api_test.go TestCardsMoveThroughTheStages
    Given a task parked in To do
    When the user moves it to In progress
    Then the task is working, no longer parked, and no new step is written, as there is nothing to say
    When the user moves it to Review with the note "Check the redirect on Safari."
    Then the coordinator's next direction reads exactly "Have this task reviewed next. Check the redirect on Safari."

  @TASK-20 @user @test
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

  @TASK-21 @user @manual
  Scenario: A drag the board does not allow
    # "No agent in this team ships work": ui/src/components/organisms/TaskBoard.tsx line 153
    Given the board with a card of a team that has no agent that ships work
    When the user drags that card over Shipping
    Then the column does not accept the drop and its border is not marked as a target
    And the line "No agent in this team ships work" is shown under the column's heading
    Given a done card in Done
    When the user drags it over Done
    Then the drop is not allowed and no dialog opens

  @TASK-22 @user @test
  Scenario: A move the route refuses
    # internal/web/api_test.go TestCardsMoveThroughTheStages
    # the words: internal/web/board_api.go lines 92 and 112; the test asserts the codes only
    Given a task that is done
    When a move to done is sent for it
    Then the move is refused with 409
    When a move to a column called sideways is sent
    Then the move is refused with 400

  @TASK-23 @user @test
  Scenario: A done task dragged back to In progress is reopened for more work
    # internal/web/api_test.go TestCardsMoveThroughTheStages
    Given a task that is done
    When the user moves it to In progress with the note "The fix broke sign-out."
    Then the task is working again and has no worktree yet
    And the coordinator's next direction reads exactly "Reopen this task. This needs more work: hand it back to an agent that builds it. The fix broke sign-out."

  @TASK-24 @user @manual
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

  @TASK-25 @user @test
  Scenario: Dragging within a column or a list row sets the priority order
    # internal/web/api_test.go TestTasksListInPriorityOrder
    Given 3 tasks added in the order first, second, third
    Then the tasks list newest first, in exactly the order third, second, first
    When the user drops the first task between the third and the second
    Then the tasks list in exactly the order third, first, second

  @TASK-26 @user @manual
  Scenario: Done keeps the last 7 days on the board, the list keeps everything
    # 7 days: ui/src/lib/epics.ts line 7, DONE_DAYS
    Given 1 task done 2 days ago and 1 task done 8 days ago
    When the board is shown
    Then the Done column shows exactly 1 card
    And the line "1 more, done over 7 days ago or in a finished epic, are in the list" is shown under it
    When the list is shown with the Done filter
    Then both tasks are shown, 2 rows

  @TASK-27 @user @test
  Scenario: The Shipping filter chip is there only while the board has the column
    # ui/src/pages/TasksPage.test.tsx
    Given a board without Shipping, and the address ?status=shipping
    Then the Shipping chip is hidden and All is the 1 chip pressed
    Given a board without Shipping but a task in it
    Then the Shipping chip is kept and pressed

  # ---- Running limits, search and keys ---------------------------------------

  @TASK-28 @user @test
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

  @TASK-29 @user @test
  Scenario: Over the limit, the other tasks wait as queued
    # internal/coordinator/slots_test.go TestTheLoopRunsNoMoreTasksThanTheLimit and TestALimitUnderWhatRunsQueuesEveryOneWaiting
    Given a limit of 2 and 4 tasks started
    When the loop runs
    Then exactly 2 tasks have a turn at once
    And the next task starts by rank once a slot frees, never a third at the same time
    Given a limit of 1 while 2 tasks already run
    Then both keep running, 2 counted as running
    And the 1 task waiting behind them is marked queued

  @TASK-30 @user @test
  Scenario: Search finds a task by its id, title, repo, agent or label, and shows epics as epic-N
    # ui/src/pages/TasksPage.test.tsx
    # what the search matches: ui/src/pages/TasksPage.tsx line 211
    Given the list with the epic task-21, its story task-22 and the task task-30
    Then the ids shown are exactly epic-21, task-22 and task-30
    When the user types "epic-21" in "Filter tasks"
    Then exactly 1 row is shown, epic-21
    When the user types "task-21"
    Then the same 1 row, epic-21, is shown

  @TASK-31 @user @manual
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

  @TASK-32 @user @test
  Scenario: The running line counts the queued first
    # ui/src/lib/limits.test.ts
    Given a limit of 2, 2 tasks running and 3 queued
    Then the line reads exactly "Queued: 3 · 2 of 2 running"
    Given a limit of 3, 1 task running and 0 queued
    Then the line reads exactly "1 of 3 running"
    Given no limit and 4 tasks running
    Then the line is empty

  @TASK-33 @user @manual
  Scenario: A task waiting for a slot wears the queued badge
    # ui/src/components/molecules/StatusBadges.tsx QueuedBadge, shown in ui/src/pages/TasksPage.tsx and ui/src/components/molecules/TaskCard.tsx
    Given "Most running at once" set to 1 and 2 tasks started
    When the Tasks page is shown
    Then exactly 1 task wears the badge "queued", in the list and on its card
    And the header line reads "Queued: 1 · 1 of 1 running"
    When the running task's turn ends and the queued one starts
    Then the badge is no longer shown on it

  @TASK-34 @user @manual
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

  @TASK-35 @user @test
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

  @TASK-36 @user @manual
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

  # ---- A task's page ---------------------------------------------------------

  @TASK-37 @user @manual
  Scenario: A task's page: the header, the facts under it and its 2 buttons
    # ui/src/pages/TaskPage.tsx lines 289 to 303, the header; lines 367 to 398, the facts
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

  @TASK-38 @user @manual
  Scenario: The tabs, exactly as rendered, and the one an epic adds
    # ui/src/pages/TaskPage.tsx lines 443 to 456
    Given task-5 is open with 2 shared files and 4 handoffs
    When the tabs are shown
    Then exactly 8 tabs read, in order: Coordinator, Activity, Terminal, Files, Handoffs, Actions, Events, Details
    And Files carries the count 2 and Handoffs the count 4
    And the address carries ?tab=<id>, so a link opens the same tab
    Given the coordinator has asked a question
    Then the first tab reads exactly "Coordinator · asks you"
    Given an epic with 3 stories
    Then a Stories tab with the count 3 is shown first, 9 tabs in all, and it opens by default

  @TASK-39 @user @manual
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

  @TASK-40 @user @test
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

  @TASK-41 @user @test
  Scenario: The tokens and turns under the header, with the pencil
    # ui/src/pages/TaskPage.test.tsx "can change its budget and max turns while it isn't done" and "shows its tokens and turns, and changes neither"
    # ui/src/components/organisms/LimitEditor.tsx lines 36 to 52; the lists are @TASK-05's, from ui/src/lib/limits.ts lines 26 and 33
    Given a working task with a budget of 5000 tokens, 1200 used, and 8 max turns
    When the facts are shown
    Then the Tokens fact and the "Turns since your answer" fact are shown
    And a pencil "Change the budget" and a pencil "Change the max turns" are shown, 2 pencils
    Given the same task done and archived
    Then the 2 facts are still shown and no pencil is shown

  @TASK-42 @user @test
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

  @TASK-43 @user @test
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

  @TASK-44 @user @test
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

  @TASK-45 @user @test
  Scenario: Handoffs shows each handoff as a card: who passed it to whom, and the turn it led to
    # ui/src/components/organisms/HandoffTimeline.test.tsx "draws each step as an article named by who passed it to whom, and when", "says Led to with the facts of an ended turn" and "folds a brief at 8 lines, and reports and your messages at 10"
    # 8 and 10 lines: ui/src/components/organisms/HandoffTimeline.tsx lines 35 and 36
    Given a task whose coordinator handed a brief to the engineer, whose turn-4 ended after 48 minutes with 14 actions and 210k tokens
    When the Handoffs tab is shown
    Then each step is an article named "<from> to <to>, <time>", newest first
    And the card's footer reads exactly "Led to turn-4 · engineer · 48m 00s · 14 actions · 210k tokens" and the button Activity, which opens that turn's activity
    And a brief folds at exactly 8 lines, a report or the user's message at 10, until Show all

  @TASK-46 @user @manual
  Scenario: Actions and Events list the raw records, newest first, and the empty tabs say so
    # ui/src/pages/TaskPage.tsx lines 70 to 113, the columns; lines 462 to 543, the tabs
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

  @TASK-47 @user @test
  Scenario: Details: the id, repo, epic, budget and turns, each with its control while the task is live
    # ui/src/pages/TaskPage.test.tsx "can change its repo and epic", "shows an epic as epic-N, in the header and its details" and "shows a story as task-N, with its epic as epic-M, linking to task-M"
    # ui/src/pages/TaskPage.tsx lines 547 to 582; the words "story of": line 317, which the test does not assert
    Given a live task's Details tab
    Then a Change button is shown on the Repo row and a list named Epic is shown on the Epic row
    Given a story of task-9
    Then the header shows epic-9 and the link goes to exactly /tasks/task-9
    Given an epic
    Then its id is shown as epic-1 in the header and in Details, 2 times, and never as task-1
    And the erase dialog is titled "Erase epic-1 for good?"

  @TASK-48 @user @manual
  Scenario: Change repo moves the task between turns, and the Epic list files it
    # ui/src/pages/TaskPage.tsx lines 627 to 672, RepoEditor; internal/web/api.go lines 62 to 72
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

  @TASK-49 @user @test
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
    Then the erasure of task-1 is sent, with the reason "asked to"
    And the page reads "task-1 was erased at <time>", lists each warning, and says backups made before then still hold it

  @TASK-50 @user @test
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

  @TASK-51 @user @manual
  Scenario: A done task's page: what it still allows and what it no longer does
    # ui/src/pages/TaskPage.tsx lines 417 to 425, the Done banner; line 596, the worktree; lines 507 to 515, the terminal
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

  # ---- The archive -----------------------------------------------------------

  @TASK-52 @user @test
  Scenario: An archived task's page is read only
    # ui/src/pages/TaskPage.test.tsx "marks it archived and offers nothing that writes", "keeps its repo and epic as they are" and "shares no files and takes none away"
    # ui/src/pages/TaskPage.tsx lines 236 and 296; ui/src/components/organisms/CoordinatorChat.tsx lines 188 and 189
    Given task-1 archived on a date
    When the page is shown
    Then the badge "Archived <date>" is shown
    And "Add labels" is hidden, the field "Message tech-lead" is hidden, and the chat reads "The task is archived: it takes no messages."
    When the Details tab is shown
    Then no Change button and no Epic list are shown: the repo and the epic only show
    When the Files tab is shown
    Then "Share files" and "Stop sharing it" are hidden

  @TASK-53 @user @test
  Scenario: An archived task takes no events until it is reopened
    # internal/web/archived_test.go TestMovingAnArchivedTaskIsRefused
    # internal/web/archives_test.go TestAnArchivedTaskReadBackStillTakesNoEvents
    Given task-1 done and archived
    When its rank, labels, an answer or a move to done is sent
    Then each is refused with 409 "task-1 is archived: an archived task takes no events"
    And the task is still archived, not unarchived
    When a move of a task that does not exist is sent
    Then it is refused with 404 "no such task task-9"

  @TASK-54 @user @test
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

  @TASK-55 @user @test
  Scenario: A task done for the archive age is archived, in a round at start and then every hour
    # internal/archive/archive_test.go TestATaskIsArchivedOnceDoneForTheAge and TestTheArchiverRunsAtStartAndThenHourly
    # the age: 30 days unless -archive-after says otherwise, and 0 never archives: cmd/agentry/serve.go lines 70 to 72 and 105; the test's age, 30 days: internal/archive/archive_test.go line 22
    # a round every hour: internal/archive/archive.go line 20, roundEvery
    # the task list still shows an archived task as done, with its summary; its page is marked Archived and read only: @TASK-52 and @TASK-53; README.md, Archiving, erasing and redacting
    Given the default archive age, 30 days, and 3 tasks done, task-1 first, task-3 reopened since
    When a round runs 30 days after task-1 was done
    Then exactly 1 task is archived, task-1; task-2 and task-3 are not
    When a round runs 30 days after that
    Then exactly 1 more is archived, task-2; the reopened task-3 is not
    Given the archiver started with task-1 done for the age
    Then its first round runs at once: task-1 is archived, and it waits exactly 1 hour for the next
    When that hour passes, task-2 done for the age by then
    Then the round an hour on archives task-2
