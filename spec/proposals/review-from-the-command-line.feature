Feature: Review from the command line
  The user reads a task's work from a terminal with agentry review, and
  approves it, sends it back or discards it there, without opening the page.
  Today the work is read on the Workspace page and decided by moving the
  task's card on the board: Shipping ships it now, In progress with a note
  sends it back, and Done finishes it. agentry review uses the same reads
  and the same moves, so a decision from the terminal is the one a drag on
  the board makes, logged the same way.

  # ---- Reading the work ------------------------------------------------------

  @PROP-12 @user @test @to-worktrees-and-branches
  Scenario: agentry review shows the diff of a task's worktree against its base
    # the Workspace's git view reads the same changes and commits (@TREE-07)
    # test: cmd/agentry/review_test.go (to write)
    Given task-5 has a worktree with 2 changed files, 1 added file and 3 commits on its branch agentry/task-5
    When the user runs agentry review task-5
    Then the first line names task-5, its title, its status, its branch and its base
    And the next lines list the 3 commits, newest first, each with its 7 characters and subject
    And then comes the diff from the base to the worktree as it stands, committed and not, with exactly 3 files in it
    And the command exits 0

  @PROP-13 @user @test @to-worktrees-and-branches
  Scenario: A task whose worktree is on a worker is shown the same
    # a turn runs on the machine its agent may run on, and the Workspace reads it there (@MACHINE-30)
    # test: cmd/agentry/review_test.go (to write)
    Given task-7's worktree is on a joined worker, which is online
    When the user runs agentry review task-7 on the machine Agentry runs on
    Then the diff is read from the worker's worktree, the same as for a local one: yes
    Given the worker is offline
    Then the command says the worker is offline, naming it, and exits 1

  @PROP-14 @user @test @to-worktrees-and-branches
  Scenario: Nothing to review, a removed worktree and an unknown task each say so
    # a finished task's worktree is removed once its branch is merged into dev (@TREE-03)
    # test: cmd/agentry/review_test.go (to write)
    Given a task whose worktree has nothing changed and no commits since its base
    When the user runs agentry review on it
    Then it prints exactly "nothing to review: the worktree is the same as its base" and exits 0
    Given a task whose worktree was removed after its branch was merged
    Then it says the worktree is gone because the branch was merged, and exits 1
    Given no task called task-99
    Then it says "no task called task-99" and exits 1

  # ---- Deciding --------------------------------------------------------------

  @PROP-15 @user @test @to-tasks-and-board
  Scenario: agentry review approve ships the task, as Shipping on the board does
    # the move to Shipping (@TASK-20, internal/web/board_api.go)
    # test: cmd/agentry/review_test.go and internal/web/api_test.go (to write)
    Given task-5 of the Software team in Review
    When the user runs agentry review approve task-5
    Then the task's column is shipping, and the coordinator's next direction starts "Ship this task now: hand it to deployer": yes
    And exactly 1 event is written, marked as moved there by the user, the same as a drag to Shipping
    Given a task of the Research team, which has no agent that ships work
    When the user approves it
    Then it is refused with "Research has no agent that ships work", the command exits 1, and no event is written

  @PROP-16 @user @test @to-tasks-and-board
  Scenario: agentry review send-back sends the task back with the user's note, as In progress on the board does
    # a move back to In progress from Review with a note (@TASK-19, @TASK-23, internal/web/board_api.go sentBack)
    # test: cmd/agentry/review_test.go (to write)
    Given task-5 in Review
    When the user runs agentry review send-back task-5 "The redirect breaks sign-out."
    Then the coordinator's next direction reads exactly "This needs more work: hand it back to an agent that builds it. The redirect breaks sign-out."
    And the task's column is in-progress, marked as sent back by the user: yes
    When send-back is run with no note
    Then it is refused with "say what needs to change", the command exits 2, and nothing is sent

  @PROP-17 @user @test @to-tasks-and-board
  Scenario: agentry review discard finishes the task after asking once, and keeps its branch
    # Done finishes the task at once with one last turn for the coordinator (@TASK-17); a branch not merged keeps its worktree (@TREE-03)
    # test: cmd/agentry/review_test.go (to write)
    Given task-5 in Review with a turn in progress
    When the user runs agentry review discard task-5
    Then it asks exactly once "Discard task-5? It is marked done and its branch is not merged. [y/N]"
    When the user answers anything but y
    Then nothing is sent and the command exits 1
    When the user answers y, or runs it with --yes
    Then the task is done, marked as moved there by the user, the turn in progress is stopped and the coordinator is given its wrap-up turn: yes
    And its branch and worktree are kept, not merged: yes

  # ---- Who may decide ----------------------------------------------------------

  @PROP-18 @test @to-security
  Scenario: Only the user's own credential reviews; the desk token cannot
    # the desk token opens tasks and nothing more (@SEC-47, @SEC-48); the dock's Claude runs agentry task without asking (@TREE-12)
    # test: internal/web/api_test.go (to write)
    Given the desk's Claude session, which holds the desk token and runs agentry task without asking
    When it runs agentry review, approve, send-back or discard with the desk token
    Then the server answers HTTP 401 every time, and no event is written
    And agentry review is not a subcommand of agentry task, so Bash(agentry task:*) does not allow it: yes
    When the same commands run with the page token, or a Claude Code connection's credential
    Then each is allowed

  @PROP-19 @test @to-security
  Scenario: No agent reviews its own work from the command line
    # an agent can never approve its own request (@SEC-46); Agentry's own files are off limits for every profile (@POLICY-38)
    # the command rule for agentry review is new: today's rule for Agentry's own files matches paths, and agentry review names none
    # test: internal/policy/default_test.go (to write)
    Given an agent under any of the 6 profiles, allow-everything included
    When it runs agentry review approve task-5, agentry review send-back task-5 "x" or agentry review discard task-5 --yes
    Then each is denied by a new built-in command rule for agentry review, beside the rule for Agentry's own files, and nothing is sent
    And agentry review task-5 is denied the same way, as it reads with the user's credential too: yes

  # ---- Failures and limits -------------------------------------------------------

  @PROP-20 @test @to-reliability
  Scenario: A server that cannot be reached, or a move it refuses, changes nothing
    # a move the route refuses (@TASK-22)
    # test: cmd/agentry/review_test.go (to write)
    Given Agentry is not running
    When the user runs agentry review approve task-5
    Then it says it cannot reach Agentry, naming the address it tried, and exits 1
    Given a task that is done
    When the user discards it
    Then the server refuses the move with 409, the command prints the reason and exits 1, and no event is written

  @PROP-21 @test @to-limits
  Scenario: A file over 2 MB is named in the diff, not printed
    # the Workspace shows "Too big to show" for a file over 2 MB (@TREE-07, internal/workspace/workspace.go)
    # test: cmd/agentry/review_test.go (to write)
    Given a worktree with a changed text file of 3 MB and a changed image
    When the user runs agentry review on the task
    Then the 3 MB file is listed with "too big to show" and none of its lines are printed
    And the image is listed with "not a text file": yes

  @PROP-22 @manual @to-usability
  Scenario: agentry review explains itself
    # agentry with no command lists its commands (cmd/agentry/main.go printUsage)
    Given a terminal on the machine Agentry runs on
    When the user runs agentry with no command, and then agentry review with no task
    Then the first lists review among the commands, with exactly 1 line saying what it does
    And the second prints the 4 forms, agentry review task-N, approve, send-back with its note and discard with --yes, and exits 2

# Open questions
# Which credential agentry review uses: the page token on the machine Agentry runs on and a Claude Code connection's credential elsewhere (recommended), or a new review token.
# Whether approve ships the task (recommended, as a drag to Shipping) or only marks it reviewed and lets the coordinator decide what comes next.
# Whether discard only finishes the task and keeps its branch (recommended, as Done does) or also deletes the branch and the worktree.
# Whether send-back needs a note (recommended) or may go with none, as a drag to In progress may.
# Whether the diff includes changes not yet committed (recommended, as the Workspace shows them) or only the branch's commits.
# Whether an agent is stopped by a new built-in command rule for agentry review (recommended), by a rule in the shipped profiles that the user could change, or by giving agentry review no credential an agent can reach, so it fails with 401.
