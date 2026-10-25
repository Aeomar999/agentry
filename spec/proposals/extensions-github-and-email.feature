Feature: Shipping through GitHub, and email
  Two features, each turned on in features.toml. The first, git, ships a
  task through a pull request: for a repo the feature is on for, the
  deployer pushes the task branch instead of dev, and a job in the server
  opens a pull request from it into dev, asks GitHub every two minutes what
  became of it and its pipeline, and writes what changed as git events in
  the task's stream. The user merges on GitHub. The task page gets a Merge
  request tab, Claude Code a show_pipeline tool, and a failed pipeline or a
  refusal waits for the user. The second, email, sends the user's address
  what already waits for them. Today the deployer merges the task branch
  into dev and pushes it, nothing records the pipeline that push starts
  beyond one line of its report, and what waits reaches the user only in
  Claude Code, on the page and as a push.

  # ---- Turning git on ----------------------------------------------------------

  @PROP-30 @user @test @to-settings
  Scenario: While git is off, the Settings page says how to turn it on, and never takes the token
    # docs/design/t-019-shipping.md sections 7 and 9; builds on @SET-01 (internal/features/features.go Manifest) and features.toml (internal/features/settings.go Settings)
    # test: ui/src/pages/SettingsPage.test.tsx and internal/githost/feature_test.go (to write)
    Given a features.toml with no git table
    When the Settings page is shown
    Then Git is listed among the features that are off, with its why "add a token to features.toml to open pull requests": yes
    And its way to turn it on shows the git table with host "github", project, base and poll, the token's scopes and the path of the file: yes
    And the page has no field for the token: yes
    Given the git table with host "github", a project "owner/repo" and a token under its secrets table
    Then git is enabled, and Git is not listed among the features that are off
    Given the same table with no token, or no project
    Then git is not enabled and the job makes 0 calls to GitHub

  @PROP-31 @test @to-security
  Scenario: The git token is used only by the server, and never reaches the log, the page or an agent
    # docs/design/t-019-shipping.md sections 9 and 10; builds on @SEC-59 (cmd/agentry/jobs_test.go TestASecretReachesTheJobAndNeitherLog) and features.toml being protected (cmd/agentry/policy.go)
    # test: internal/githost/secrets_test.go (to write), running the job and polling against a fake GitHub
    Given features.toml holding a known token in the git secrets table
    When the job opens a pull request, polls it and is refused once
    Then the token is in no event, no snapshot, no line of the server's output and no reply of the API: yes
    And no agent's environment holds it, so the deployer's gh uses the machine's own sign-in as today: yes
    And the job sends it only to the api address in the file, which no agent can change: yes

  # ---- Shipping through a pull request -----------------------------------------

  @PROP-32 @agent @manual @to-agents
  Scenario: For a repo git is on for, the deployer pushes the task branch, not dev
    # docs/design/t-019-shipping.md section 4, decision 1 in section 12; changes @TURN-13 for those repos only (internal/agents/defaults/deployer.md)
    # the brief's section comes from the feature's Coordinator.Brief (internal/features/features.go Coordinator)
    Given git on for the repo of task-121, and a reviewed and tested task handed to the deployer
    When the deployer's turn runs
    Then its brief says this repo ships through pull requests, and its push is exactly git push origin HEAD:agentry/task-121, as a command of its own: yes
    And the push asks the user first, as every push does: yes
    Given a task in a repo git is not on for
    Then the deployer pushes dev with git push origin HEAD:dev, the same as today

  @PROP-33 @user @test @to-new-pull-requests
  Scenario: The approved, finished push of the task branch opens exactly 1 pull request into dev
    # docs/design/t-019-shipping.md sections 2 and 3, steps 5 to 8; the push is read as Task.PushTo, over the parser of the waiting line (internal/tasks/waiting.go plainPush)
    # test: internal/githost/job_test.go (to write), with a fake GitHub
    Given git on for the repo, and the deployer's approved push of agentry/task-121 with its result in the log
    When the job's next pass reads the task
    Then it asks GitHub for the branch, finds it, and opens exactly 1 pull request from agentry/task-121 into dev
    And the request is titled with the task's title, and its body holds the task's link and its summary when it has one: yes
    And exactly 1 git.pr_opened is written in task-121's stream by System, with the host github, the number, its link, the branch, the base and the head commit

  @PROP-34 @coordinator @test @to-coordinator-and-handoffs
  Scenario: The task is done at the push, as today, and its pull request opens just after
    # docs/design/t-019-shipping.md section 4, decision 5 in section 12; the coordinator finishes once a ship turn pushed (internal/coordinator/checks.go recordOf, internal/tasks/task.go Pushed), @TURN-14
    # test: internal/coordinator/next_test.go (to write)
    Given a ship turn whose push of the task branch was approved and finished
    When the coordinator decides done
    Then the task is done: yes
    And no agent is started to open the pull request, which is the job's: yes
    When git.pr_opened and later git events arrive on the done task
    Then the rules let them in and 0 turns start for them

  @PROP-35 @user @test @to-new-pull-requests
  Scenario: A push that does not ship the task branch opens nothing
    # docs/design/t-019-shipping.md section 2: Task.PushTo, readPush skipping a dry run
    # test: internal/tasks/waiting_test.go and internal/githost/job_test.go (to write)
    Given git on for the repo
    When a task's ship turn pushed to dev, ran git push -n or git push --dry-run, or ran a push the parser cannot read
    Then 0 pull requests are opened and 0 git events are written for it
    And the waiting line for the dry-run push reads the same as before: yes
    Given a log from before git was on, every task shipped to dev
    Then turning git on opens nothing for those tasks

  @PROP-36 @user @test @to-new-pull-requests
  Scenario: A task never has two pull requests open at once
    # docs/design/t-019-shipping.md section 3, "Never two requests for one task" and step 4; the rule in rules.Check under the store's lock (section 1, The task slice)
    # test: internal/githost/rule_test.go and internal/githost/job_test.go (to write)
    Given task-121 with its pull request open
    When the deployer pushes the branch again
    Then the open request takes the new head and 0 new requests are opened
    And a second git.pr_opened while the request is open is refused by the rules: yes
    Given the request merged on GitHub since the last poll, and the task reopened and shipped again
    When the job's next pass reads the new push
    Then it writes exactly 1 git.pr_changed saying merged, then opens exactly 1 new pull request: yes

  @PROP-37 @user @test @to-new-pull-requests
  Scenario: A pull request closed on GitHub is left closed until the next push, which reopens it
    # docs/design/t-019-shipping.md section 3, steps 3 and 7: a polled git.pr_changed is never a mark
    # test: internal/githost/job_test.go (to write)
    Given task-121's request closed on GitHub without merging, and the poll has recorded it
    When the job's passes go on with no new push
    Then the request is not reopened: yes
    When the deployer pushes the branch again
    Then the job reopens that same request and writes exactly 1 git.pr_opened for it

  @PROP-38 @test @to-reliability
  Scenario: A restart between GitHub's reply and the write records the request, and opens no second
    # docs/design/t-019-shipping.md section 3, step 5
    # test: internal/githost/job_test.go (to write)
    Given the job opened a pull request on GitHub and the server stopped before git.pr_opened was written
    When the server starts and the job's next pass reads the push
    Then it finds the request on GitHub and writes exactly 1 git.pr_opened with found true, opening nothing
    And when the request was merged or closed meanwhile, exactly 1 git.pr_changed with that state follows: yes
    And the tab shows it as found open on GitHub: yes
    And GitHub's 422 "A pull request already exists" to an open is taken the same way, and 0 requests are opened: yes

  # ---- When GitHub says no -------------------------------------------------------

  @PROP-39 @user @test @to-new-pull-requests
  Scenario: A branch GitHub does not have, or a repo the token cannot see, is refused and waits for the user
    # docs/design/t-019-shipping.md section 3, step 6: GitHub answers 404 for both, so the job asks for the repo after a 404 on the branch
    # test: internal/githost/job_test.go (to write)
    Given the deployer's push of agentry/task-121 counted as finished, and GitHub has no such branch
    When the job's next pass reads the push
    Then exactly 1 git.refused is written with the status 404 and the reason "the branch agentry/task-121 isn't on GitHub", and 0 requests are opened
    Given a token that cannot see the repo
    Then the reason is "the token can't see owner/repo on GitHub", and 0 requests are opened
    And the task stays done, and no agent is started: yes

  @PROP-40 @user @test @to-new-pull-requests
  Scenario: A refusal is not tried again by itself, and Try again has the job try once more
    # docs/design/t-019-shipping.md sections 3 ("When the host says no"), 4 and 7; git.retry is refused while no refusal is outstanding
    # test: internal/githost/job_test.go and internal/githost/routes_test.go (to write)
    Given GitHub answers 401 to opening task-121's pull request
    When the job's passes go on
    Then exactly 1 git.refused is written, with the status and GitHub's message cut to 300 characters, and no more until the user acts
    And the Merge request tab shows the refusal with "Try again": yes
    When the user fixes the token and clicks "Try again"
    Then exactly 1 git.retry is written by the user, and the next pass opens the request
    When "Try again" is sent while no refusal is outstanding
    Then it is refused and nothing is written

  @PROP-41 @test @to-reliability
  Scenario: A network error, a timeout, 429 or a 5xx writes nothing and is tried again later
    # docs/design/t-019-shipping.md section 3: every call has its own 30 second timeout; backoff 1, 2, 4 up to 30 minutes, or Retry-After
    # test: internal/githost/github_test.go and internal/githost/job_test.go (to write), against a fake GitHub that never answers, answers 429 with Retry-After, and answers 502
    Given a fake GitHub that hangs, then answers 429 with Retry-After 120, then 502
    When the job tries to open task-121's request
    Then each call ends at its 30 second timeout, and 0 events are written for any of the 3 answers
    And the next tries come after 120 seconds for the 429, and after 1, 2 and 4 minutes, up to 30, for the others: yes
    And each logs 1 line naming the host, the task and the status, never the token or a header

  # ---- Asking GitHub every two minutes ------------------------------------------

  @PROP-42 @user @test @to-new-pull-requests
  Scenario: Polling brings back the merge and the pipeline, and writes only what changed
    # docs/design/t-019-shipping.md section 3, "Asking the host every two minutes"; for GitHub the pipelines are the workflow runs on the request's head (section 11, story 13)
    # test: internal/githost/poll_test.go (to write), with a fake GitHub
    Given task-121's request open, and its workflow run on the head failing on the job test
    When the job polls
    Then exactly 1 git.pipeline is written with the status failed and the job test failed
    When it polls again with nothing changed on GitHub
    Then nothing is written
    When the request is merged on GitHub and its last run has finished
    Then exactly 1 git.pr_changed saying merged is written, and the next poll makes 0 calls about that request
    And a push after that merge still opens a new request, as a polled change is never a mark: yes

  @PROP-43 @test @to-limits
  Scenario: The poll interval, the backoff on a bad token, and the size of what is kept
    # docs/design/t-019-shipping.md sections 1 and 3: poll 2m when unset, 30s at least; a 401 or 403 backs the whole poll off to 30 minutes; a URL cut to 2 KB, a job name to 200 characters, jobs to 100
    # test: internal/githost/poll_test.go (to write)
    Given the git table with no poll, then with poll "10s", then with poll "5m"
    Then the job polls every 2 minutes, then every 30 seconds, never less, then every 5 minutes from its next pass
    Given GitHub answers a poll with 401
    Then the whole poll backs off to 30 minutes, with 1 log line, and makes 0 calls for any request until then
    Given a run with a 3 KB link and 150 jobs, one named with 300 characters
    Then the event keeps 2 KB of the link, exactly 100 jobs and 200 characters of the name

  @PROP-44 @test @to-performance
  Scenario: Polling costs 2 calls a request, and 1 more for each running run
    # docs/design/t-019-shipping.md section 3: the request and its runs, and the jobs of each run that is new, changed, pending or running
    # test: internal/githost/poll_test.go (to write), counting the fake GitHub's calls
    Given 20 open requests, each with 1 running workflow run
    When the job polls once
    Then it makes exactly 60 calls to GitHub
    Given the same 20 requests once their runs have finished, unchanged since the last poll
    Then a poll makes exactly 40 calls

  # ---- What the user sees ------------------------------------------------------

  @PROP-45 @user @test @to-tasks-and-board
  Scenario: The Merge request tab shows the request, its latest pipeline and a refusal
    # docs/design/t-019-shipping.md section 7, the taskTabs slot; GET /api/tasks/{task} gains slices (section 1)
    # test: ui/src/features/git/MergeRequestTab.test.tsx and ui/src/pages/TaskPage.test.tsx (to write)
    Given task-121 with request 412 open into dev, and its latest run on the head failed on test and passed lint
    When the task page is shown
    Then a tab "Merge request" is shown after Details, with 412, its link, open and dev: yes
    And the pipeline shows failed with its link, short commit and when, its 2 jobs failed first: test, then lint
    Given a run waiting on a manual job
    Then it is shown as "waiting on a manual job" with the job's name, not as passed or skipped
    Given a task with no git events
    Then no Merge request tab is shown

  @PROP-46 @claude-code @test @to-claude-code-tools
  Scenario: show_pipeline gives a task's request and pipeline from the log, with no call to GitHub
    # docs/design/t-019-shipping.md section 5; the tool comes from the feature's Tools (internal/features/features.go), listed while the feature is enabled or visible
    # test: internal/githost/tools_test.go (to write), on a fixture log
    Given task-121 with request 412 open and its latest run failed on test
    When Claude Code calls show_pipeline with task "task-121"
    Then the reply names 412, open, with its link, then the run, failed, with its short commit, then the job test failed first: yes
    And it makes 0 calls to GitHub and needs no token in agentry mcp
    Given a task with no request, or git off
    Then the reply says there is no request yet and why, or that git is off and how to turn it on: yes

  @PROP-47 @user @test @to-new-pull-requests
  Scenario: A failed pipeline waits for the user, once per run, until a later one passes or the request closes
    # docs/design/t-019-shipping.md section 6: the key git/<task>/pipeline/<id>/<attempt>; builds on @TOOLS-44 and @NOTIFY-03 (internal/webpush/sink.go)
    # test: internal/githost/waiting_test.go (to write)
    Given task-121's latest run on its request's head failed on test and lint
    When the waiting list is read
    Then it holds exactly 1 entry: "task-121's pipeline failed on agentry/task-121: test, lint" with the run's link
    When the same failure is written twice
    Then the waiting list still holds exactly 1 entry for it
    When the run is retried and fails again
    Then it is a new entry, so it is pushed once more: yes
    When a later run on the head is running or passes, or the request is merged or closed, or the task archived
    Then the entry is no longer listed: yes
    And a failure on an older head, or on an older request, waits for no one: yes

  @PROP-48 @test @to-compatibility
  Scenario: A log from before git, and a build without it, read as before
    # docs/design/t-019-shipping.md section 1, "Old logs, and a build without the feature"; builds on the unknown tally of @SET-01
    # test: internal/current/slices_test.go and internal/githost/feature_test.go (to write), with a fixture log
    Given a log with no git events
    Then every task reads the same as without the feature: yes
    Given a build without git and a log holding 31 git.pipeline events
    When it starts
    Then every task reads the same, nothing fails, and the Settings page counts "31 events of a type this build doesn't know: git.pipeline"
    And a new git event proposed to it is refused as an unknown event type: yes
    When git is added back
    Then all 31 events replay into the task's git state: yes

  # ---- Email -----------------------------------------------------------------

  @PROP-49 @user @test @to-notifications
  Scenario: With email on, each thing that newly waits is sent once to the user's address
    # the old build plan, "notification and email extensions"; the same list and the same once-only rule as Web Push: @NOTIFY-03 (internal/webpush/sink.go Sink, which reads the server's waiting list)
    # test: internal/email/sink_test.go (to write), with a fake mail server
    Given email on, and an approval already waiting when the server starts
    When the server starts
    Then nothing is sent for what already waited
    When a new approval waits on task-4
    Then exactly 1 mail goes to the user's address, its subject "Agentry: task-4", its body "task-4: Waiting for you to approve the push to dev" and the link to the task's page
    And checking again sends nothing more
    When that approval is answered, and the same entry waits again later
    Then it is sent once more, as it is news again: yes

  @PROP-50 @user @test @to-settings
  Scenario: Email is off until features.toml names the user's address and the mail server
    # docs/design/extensions.md section 3, Notifications; features.toml keeps secrets in a table under the feature's (internal/features/settings.go Secret), @SET-01
    # test: internal/email/feature_test.go and ui/src/pages/SettingsPage.test.tsx (to write)
    Given a features.toml with no email table
    When the Settings page is shown
    Then Email is listed among the features that are off, with its way to turn it on: the email table with to, server and from, and the password under its secrets table: yes
    And the page has no field for the password or the address: yes
    Given the email table with to, server and from, and the password under its secrets
    Then email is enabled, and an edit to the table reaches the next mail without a restart: yes

  @PROP-51 @test @to-security
  Scenario: The mail password and the user's address never reach the log, and mail goes only to that address
    # @SEC-59 for the password; the user's address is kept with it in features.toml, as push.json keeps a device's address out of the log (internal/webpush/webpush.go)
    # test: internal/email/secrets_test.go (to write)
    Given features.toml holding a known password and the user's address in the email table
    When 3 mails are sent and 1 fails
    Then neither the password nor the address is in any event, snapshot, reply of the API, or line of the server's output: yes
    And every mail goes to that address only, and no agent, page or tool can name another: yes
    And the mail server is reached over TLS only, and a server that cannot start TLS gets 0 mails: yes

  @PROP-52 @test @to-reliability
  Scenario: A mail that cannot be sent is logged in the server's log, and stops nothing else
    # Web Push does the same: a push that fails goes to the server's log, never the event log, and does not stop the others (internal/webpush/sink.go push)
    # test: internal/email/sink_test.go (to write)
    Given email on and the mail server refusing the login
    When 2 entries newly wait
    Then 2 lines are logged in the server's log, each naming the entry's tag and the error, never the password: yes
    And 0 events are written for them, and each device subscribed to Web Push still gets its push: yes
    And the failed mails are not sent again on later checks: yes

# Open questions
# Host order: the design (section 12) builds GitLab first and GitHub as story 13; recommended: build in the design's order, with these scenarios holding for both hosts.
# GitHub's statuses: the design leaves GitHub's own table for story 13; recommended: map a workflow run's status and conclusion onto the same 7 statuses as GitLab's, with an unknown one writing nothing for that run, or keep GitHub's names as they are.
# The tab's label: "Merge request" for both hosts, as the design names it (recommended), or "Pull request" when the host is GitHub.
# The webhook: not built, as Agentry has no public address (decision 4); recommended: polling only, and the GitHub route of section 8 once there is a public https address.
# Merging: the user merges on GitHub (decision 3, recommended), or a later setting merge = "auto" once the pipeline passes.
# What an email sends: each new entry of the waiting list, as Web Push sends (recommended), or a digest of what waits, sent at most once an hour.
# How an email is sent: through the user's own mail server over SMTP with TLS, named in features.toml (recommended), or through a mail service's API with a key.
# The mail's credentials: the password in the email secrets table of features.toml, as the git token is kept (recommended), or read from the machine's keychain.
# The mail's link: the task's page at -public-url when one is set (recommended), and no link without one, or always the address the server prints.
