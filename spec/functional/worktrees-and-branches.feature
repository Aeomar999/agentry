Feature: Worktrees and branches
  Each task works in its own worktree on its own branch, shown on the Workspace page.
  # Sources: internal/platform/worktrees, internal/platform/workspace, internal/domain/tasks/tasksapi/workspace_api.go,
  # ui/src/pages/WorkspacePage.tsx and their tests.

  # ---- Worktrees and branches ------------------------------------------------

  @TREE-01 @coordinator @test
  Scenario: Each task works in its own worktree on branch agentry/<task>
    # internal/platform/worktrees/worktree_test.go TestCreateSetsTheWorktreeUp
    Given a repo and a task task-1
    When Agentry prepares the task's worktree
    Then the worktree is on branch agentry/task-1, with its base commit kept and the port it was given, 7901
    # not under test, checked by hand: the base is the repo's current commit; the test checks it is a full commit id
    And the repo's untracked .env files are copied in, node_modules is linked, and Agentry's own files under .agentry/ never show as changes
    And a second task's worktree adds the ignore lines exactly once

  @TREE-02 @coordinator @test
  Scenario: Each worktree leases a port between 7900 and 7999
    # internal/platform/worktrees/worktree_test.go TestLeasePortSkipsTakenAndBusyPorts; PORT: internal/runtime/turns/turns_test.go TestStartRunsTheAgentOnAOneLinePrompt
    Given port 7900 is held by another unfinished task and 7901 has something listening on it
    When a new task's worktree is prepared
    Then its port is exactly 7902
    And a turn starts with the worktree's port in its PORT environment variable: yes
    # FirstPort = 7900, LastPort = 7999: internal/platform/worktrees/ports.go; PORT: internal/runtime/turns/turns.go environment

  @TREE-03 @coordinator @test
  Scenario: A finished task's worktree is removed only once its branch is merged into dev
    # internal/platform/worktrees/worktree_test.go TestRemoveOnlyOnceMerged
    Given a done task whose branch has commits not yet in dev
    When Agentry removes its worktree
    Then the worktree and the branch are kept, so no work is lost
    And once the branch is in a local dev with no remote, the worktree and the branch are no longer there
    # not under test, checked by hand: the branch in origin/dev counts as merged too: internal/platform/worktrees/worktree.go lines 209 to 212

  @TREE-04 @coordinator @test
  Scenario: The task's work follows its branch to the next machine
    # internal/platform/worktrees/handoff_test.go TestWorkMovesThroughTheHubAndBack
    Given a task's worktree on one machine with 1 commit and 1 file left uncommitted
    When the next turn's agent runs on another machine
    Then what was left uncommitted is committed first, the branch goes to the repo Agentry runs from, and the other machine's worktree made from that branch is clean, with 0 changes showing
    And the worktree it left is no longer on the first machine
    # not under test, checked by hand: the leftovers are exactly 1 commit
    # the coordinator's choice of machine: internal/runtime/coordinator/machines_test.go TestTheCoordinatorChoosesTheMachine

  @TREE-05 @worker @test
  Scenario: A task's branch follows the work through the hub and back
    Given a task's worktree on one machine, with 1 commit and 1 file left uncommitted
    When the work leaves that machine
    Then what was uncommitted is committed, the hub gets the branch, and the worktree there is gone: yes
    # not under test, checked by hand: as 1 commit (internal/platform/worktrees/handoff.go line 19). In internal/platform/worktrees/handoff_test.go TestWorkMovesThroughTheHubAndBack the hub is the worktree's own repo;
    # a worker's hand-off to the hub is asserted in internal/web/workers_test.go TestAWorkerJoinsAndTakesWork
    When the next machine makes its worktree from the hub's branch
    Then the uncommitted file is there, the worktree is clean, and its origin is the repo's own remote: yes
    When that machine's agent changes the file and the work goes back
    Then the hub's branch has the change, the history has the first commit, and the base moved: no
    And a branch with 0 new commits sends 0 bundles
    # internal/platform/worktrees/handoff_test.go TestWorkMovesThroughTheHubAndBack, TestABranchWithNothingNewBundlesNothing

  @TREE-06 @worker @test
  Scenario: Work moves only to another machine, between turns
    Given a task with 0 worktrees yet
    When a move of its work is recorded
    Then it is refused: yes
    Given a task with its worktree on the machine Agentry runs on
    When a move to that same machine is recorded
    Then it is refused: yes
    When a move to a worker is recorded
    Then it is accepted: yes
    When a move is recorded while a turn runs
    Then it is refused: yes
    # internal/core/rules/worker_rules_test.go TestWorkMovesOnlyToAnotherMachine

  # ---- The Workspace ---------------------------------------------------------

  @TREE-07 @user @manual
  Scenario: The Workspace page: the git view of the task's worktree
    # ui/src/pages/WorkspacePage.tsx lines 23 to 29 and 130 to 196; ui/src/components/organisms/SourceControl.tsx lines 8 to 13
    # files over 2 MB: internal/platform/workspace/workspace.go lines 163 and 185
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

  @TREE-08 @user @manual
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

  @TREE-09 @user @manual
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

  @TREE-10 @user @test
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

  @TREE-11 @user @manual
  Scenario: The Workspace button and the task's Stories tab for an epic
    # ui/src/pages/TaskPage.tsx line 300, the button; lines 363 to 365, 444 and 484, Stories
    # the count "1/3", titled "1 of 3 stories done": ui/src/components/molecules/StoryProgress.tsx lines 7 and 12
    # README.md, Use it, "Epics and stories": file tasks under an epic from the epic's Stories tab
    Given an epic with 3 stories, 1 done
    When its page is shown
    Then the Stories fact reads exactly "1/3", titled "1 of 3 stories done", and the Stories tab, count 3, is open
    And the Workspace button is hidden while the epic has no worktree
    Given task-5 with a worktree
    When the user clicks Workspace
    Then exactly 1 new browser tab opens at /tasks/task-5/workspace

  @TREE-12 @user @test
  Scenario: Claude in the dock starts in the page's theme, runs agentry task without asking, and edits nothing
    # internal/runtime/desk/desk_test.go TestTheDeskStartsClaudeWithWhatItNeeds
    Given the page in the light theme, and the desk not running
    When the desk starts
    Then Claude starts in its repo with the desk's prompt, the server's address and the desk token, and the light theme in its settings: yes
    And it runs in the default permission mode, with Bash(agentry task:*) allowed without asking, and Edit, Write, MultiEdit and NotebookEdit each disallowed
    And starting it again while it runs restarts Claude: no
    When the desk is started again in the dark theme
    Then Claude starts again in the dark theme, resuming its own conversation: yes
    # an agent's session starting in the page's theme, and a terminal on the page following a theme change: USE-29
    # README.md, Use it, The dock
