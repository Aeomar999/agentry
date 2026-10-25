Feature: The agents
  The engineer, reviewer, tester, deployer and the other teams' agents, each in its own worktree and session, every action through the gate.

  @AGENT-01 @test
  Scenario: A push asks the user first
    Given an agent under the developer profile, in its task's worktree
    When the agent runs git push origin HEAD:dev
    Then the user is asked before the push runs
    And a push to main, or a force push, is denied
    # PLAN.md T-013, T-033; README.md, The policy; internal/policy/default_test.go

  # --- Every agent's turn

  @AGENT-02 @test
  Scenario: An agent's turn starts from its own prompt and the coordinator's brief
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the coordinator assigned the engineer with the instructions "Remove every TODO in src/."
    When the engineer's turn starts
    Then its brief opens with its own prompt, "You are the engineer", and shows exactly those instructions
    And the prompt is the engineer's own, not the coordinator's
    # each prompt is embedded from defaults/<name>.md, or defaults/<team folder>/<name>.md for the other teams: internal/agents/defaults.go, teams.go Prompted

  @AGENT-03 @test
  Scenario: An agent's turn ends with a report, and every report goes back to the coordinator
    # internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator
    Given the engineer's brief, whose "When you finish" section asks for 1 short report for the coordinator
    When the engineer ends its turn with the report "Removed 3 TODOs and committed."
    Then the next turn is the coordinator's, and no other agent's turn starts first
    And the report is shown in the coordinator's brief as exactly 1 step, "engineer → tech-lead: Removed 3 TODOs and committed."
    And the brief told the agent not to stop to ask questions, and to put them in the report
    # finish: internal/coordinator/brief.go

  @AGENT-04 @test
  Scenario: An agent works only in the task's worktree, on branch agentry/<task>
    # internal/coordinator/carried_brief_test.go TestABriefFromASiblingsSessionSaysItIsANewTask
    Given a task task-3 whose worktree is ready on branch agentry/task-3, with port 7903
    When any agent's turn starts on it
    Then the brief's "Where the work is" names exactly that worktree and branch, and says never to create or change repos or folders outside it
    And the same port, 7903, is in the PORT environment variable of the turn
    # whereText: internal/coordinator/brief.go; PORT: internal/turns/turns.go environment; the branch: internal/worktrees/worktree_test.go TestCreateSetsTheWorktreeUp

  @AGENT-05 @test
  Scenario: An agent writes the plan file its brief asks for, and the plan follows the file
    # internal/hooks/handler_test.go TestThePlanFollowsTheFile
    Given a brief whose "Your plan" section asks for a Markdown checklist in .agentry/turn-N-plan.md before anything else
    When the agent writes 2 unticked steps, then ticks both before its turn ends
    Then the plan kept on the turn is the same as the file's last content, with both steps ticked
    And an unchanged plan is recorded 0 more times
    # PlanFile: internal/machines/local.go; the section's words: internal/coordinator/brief.go plan

  @AGENT-06 @test
  Scenario: An agent keeps its notes across tasks in .agentry/knowledge.md
    # internal/hooks/handler_test.go TestWhatTheAgentLearnedIsKept
    Given the brief's "What you know from earlier tasks" shows the agent's notes and asks it to update .agentry/knowledge.md before it finishes
    When the engineer's turn ends with "This repo formats with gofmt -s." in that file
    Then the engineer's knowledge is exactly that text, recorded once, by the engineer
    And its next brief shows the same notes
    # KnowledgeFile: internal/machines/local.go; knowledge: internal/coordinator/brief.go

  @AGENT-07 @test
  Scenario: Each default agent runs under the profile the code gives it
    # internal/agents/agents_test.go TestEachDefaultAgentHasAProfile
    Given a new install's Software team
    When the roster is read
    Then it has exactly 5 agents: tech-lead under read-only, engineer under developer, reviewer under read-only, tester under developer, deployer under developer
    And exactly 2 of them check work, the reviewer and the tester; the deployer ships it, and the engineer builds it
    # internal/agents/defaults.go; the columns: internal/agents/agent.go Builds, Reviews, Ships

  @AGENT-08 @test
  Scenario: The other teams' agents run under the profiles their templates give them
    # internal/agents/agents_test.go TestTheVisionariesTeam
    Given the Visionaries and the Networking teams added from their templates
    When the roster is read
    Then all 5 Visionaries are under read-mostly, and only the skeptic's work counts as review
    And network-lead is under read-only, network-engineer under developer, network-verifier and network-security under network-diagnosis, the last 2 counting as review
    And every profile a template names is 1 of the 6 in the shipped policy
    # TestTheNetworkingTeam in the same file; the profiles: internal/agents/teams.go; internal/agents/prompts_policy_test.go TestEveryTemplateProfileIsInTheShippedPolicy

  @AGENT-56 @manual
  Scenario: In Writing, Research and Design the coordinator is read-only and the others are developers
    Given the Writing, Research and Design teams added from their templates
    When each agent's profile and column are read on the Agents page
    Then in each of the 3 teams exactly 1 agent coordinates, under read-only, and the 3 others are under developer
    And exactly 1 agent in each team counts as review: the fact-checker, the critic and the design-critic
    # the templates: internal/agents/teams.go, writing, research and design
    # Steps: on the Teams page add Writing, Research and Design from their templates; open each team's agents and
    # read the profile and the column of each of its 4 agents; compare with teams.go.

  @AGENT-09 @test
  Scenario: Only the agent working on the task passes the gate
    # internal/hooks/handler_test.go TestOnlyTheWorkingAgentPassesTheGate
    Given the engineer's turn is open on the task
    When a gate call arrives from the reviewer
    Then it is denied
    And the engineer's calls are judged with exactly its own profile, "developer"
    # TestTheGateUsesTheWorkingAgentsProfile in the same file

  @AGENT-10 @test
  Scenario: As shipped, every agent allows everything until the user turns that line off
    # internal/policy/default_test.go TestAllowEverything
    Given the shipped policy, with use_for_everyone = "allow-everything"
    When an agent under developer, read-only or autonomous runs git push origin HEAD:dev or npm install left-pad, or writes a file outside the workspace
    Then each is allowed, judged by the profile "allow-everything"
    And an edit of Agentry's own policy file is still denied
    And with that line commented out, each agent is judged by its own profile, as every scenario below assumes
    # use_for_everyone: internal/policy/default.toml line 79; the tests load the policy with it commented out: default_test.go loadDefault

  # --- The engineer

  @AGENT-11 @manual
  Scenario: The engineer changes the code with tests and commits on the branch, and never pushes
    Given the engineer's brief asks for a change
    When its turn runs
    Then its prompt says to write 1 failing test first for a bug and tests for new behaviour, and to run the project's tests, linters and type checks
    And it says to commit in small working steps with conventional messages, and never to push: the deployer does
    And its report says what changed and why, each commit, how it was tested with commands and results, and anything left or risky
    # the prompt: internal/agents/defaults/engineer.md
    # Steps: start a task that needs a small code change, read the engineer's report and its commits with git log on the task branch;
    # check that no push happened (the task's Actions list no git push from the engineer).

  @AGENT-12 @test
  Scenario: The developer profile allows what stays inside the workspace
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given the engineer under developer
    When it runs ls -la, npm test, git add -A && git commit -m 'remove TODO', echo hi > notes.md, mkdir -p src/refunds, rm notes.md, or git fetch origin && git merge origin/dev
    Then each is allowed by the rule "stays inside the workspace"
    And Edit on src/billing.ts in the workspace is allowed by "edit inside the workspace, the diff is reviewed", and Read is allowed anywhere
    # the rules: internal/policy/default.toml lines 151 to 195

  @AGENT-13 @test
  Scenario: What may reach outside the workspace asks the user
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given the engineer under developer
    When it runs sed -i '555d' src/billing.ts, echo hi > /tmp/notes.md, cp src/a.ts /tmp/a.ts, cd /etc && ls, npm install left-pad, python deploy.py, echo $(whoami), or WebFetch
    Then each is asked, and none runs before the user answers
    And a tool the policy does not name, and a command it cannot read, are asked too
    # otherwise = "ask" and unreadable = "ask": internal/policy/default.toml lines 84 to 86; WebFetch: line 146

  @AGENT-14 @test
  Scenario: A setting or an upper-case variable before a command asks, a plain toolchain setting does not
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given the engineer under developer
    When it runs git -c k=v commit -m x, GIT_EXTERNAL_DIFF=./x.sh git diff, GOFLAGS=-toolexec=./x.sh go test ./..., or export PAGER=./x.sh; git log
    Then each is asked, since the setting can make the program run a command
    And GOTOOLCHAIN=local GOPROXY=off go test ./... and git -C . commit -m x are allowed
    # variables: internal/policy/default.toml lines 51 to 57; git's own options are left out before matching: lines 47 to 50

  # --- The reviewer

  @AGENT-15 @manual
  Scenario: The reviewer reads every change and reports what it finds, and changes nothing
    Given the reviewer's brief names the task branch's changes
    When its turn runs
    Then its prompt says it changes nothing: no edits, no commits
    And it reads every change with git log and git diff from the base commit, and labels each finding must fix, should fix or nit, with the file and line
    And its report ends with the must-fix list, then exactly 1 verdict line last
    # the prompt: internal/agents/defaults/reviewer.md
    # Steps: on a task with a committed change, let the reviewer take a turn; check its report's last line is a verdict and
    # that git status in the worktree shows 0 changes from it.

  @AGENT-16 @test
  Scenario: The verdict is read from the last 5 lines of a checker's report
    # internal/coordinator/checks_test.go TestVerdictOf
    Given a checking agent's report
    When Agentry reads its verdict
    Then "Verdict: approved" and "verdict - approved, with two nits" are approved, and "**Verdict: Changes needed**" and "Verdict: not approved" are changes needed
    And a report with no verdict in its last 5 lines, such as "I approved of the naming.", has no verdict, and the coordinator is told to ask that agent again for one
    # verdictOf: internal/coordinator/checks.go; the message for no verdict: unchecked in the same file
    # verdictOf also reads "Verdict: rejected" as changes needed (checks.go, the rejected prefix); TestVerdictOf has no such case

  @AGENT-17 @test
  Scenario: Every checking agent's brief ends with how to give its verdict
    # internal/coordinator/resumed_brief_test.go TestAResumedReviewerBriefStillEndsWithTheVerdict
    Given an agent whose work counts as review, such as the reviewer
    When its brief is written, fresh or for a resumed session
    Then the brief ends with exactly the 2 lines it may use, "Verdict: approved" and "Verdict: changes needed", and says to approve only work it would ship as it is
    And an agent whose work counts as building gets no verdict section
    # VerdictFormat: internal/coordinator/checks.go; finish: internal/coordinator/brief.go

  @AGENT-18 @test
  Scenario: The read-only profile lets the reviewer read and test, keep its notes, and change nothing else
    # internal/policy/default_test.go TestTheReadOnlyProfile
    Given the reviewer under read-only
    When it calls Edit on main.go, or runs git commit -am 'x' or git push origin HEAD:dev
    Then each is denied by "this agent changes nothing" or "never commits or changes branches"
    And Edit on .agentry/knowledge.md in the workspace is allowed by "keeps its own notes", and the same file in another folder is denied
    And Read on main.go, git diff c1169f1..HEAD and go test -v ./... && gofmt -l . are allowed
    # [profiles.read-only]: internal/policy/default.toml line 198; "keeps its own notes" line 211; "this agent changes nothing" line 219; "never commits or changes branches" line 322

  # --- The tester

  @AGENT-19 @manual
  Scenario: The tester runs the tests, adds missing ones and tries the change for real
    Given the tester's brief gives the coordinator's acceptance criteria
    When its turn runs
    Then its prompt says to run every test in the suite, the linters and the build, and record the exact commands and results
    And to exercise the change as a user would, on the port in PORT, leaving no server running when it finishes
    And to fill the gaps with tests committed on the task branch, with 0 changes to the code under test, and never to push
    And its report ends with the failures, each with steps to reproduce, then exactly 1 verdict line last
    # the prompt: internal/agents/defaults/tester.md
    # Steps: on a task with a reviewed change, let the tester take a turn; check its commits touch only tests, and its report's
    # last line is a verdict.

  @AGENT-20 @test
  Scenario: The tester runs under developer, so it can commit tests, and its work counts as review
    # internal/agents/agents_test.go TestEachDefaultAgentHasAProfile
    Given the Software team as shipped
    When the tester's profile and column are read
    Then its profile is exactly "developer", the same as the engineer's
    And its work counts as review, so its verdict is 1 of the checks a done decision waits for
    # Column: Reviews on the tester: internal/agents/defaults.go; checks by column: internal/coordinator/checks.go recordOf

  # --- The deployer

  @AGENT-21 @manual
  Scenario: The deployer merges the finished branch into dev and pushes it with one exact command
    Given the deployer's brief says the work was reviewed and tested
    When its turn runs
    Then its prompt says to stop and report when the working tree is not clean or the instructions do not say the work was reviewed and tested
    And to run git fetch origin, then git merge origin/dev into the task branch, and on a conflict git merge --abort and report it, resolving 0 conflicts itself
    And to run the tests and build once more after the merge, and push only when they pass
    And the push is exactly git push origin HEAD:dev, as a command of its own, with nothing before it and nothing chained
    And never to force push, rewrite history, or push to any branch but dev
    # the prompt: internal/agents/defaults/deployer.md
    # Steps: ship a reviewed task; check the deployer's Actions show git fetch origin, git merge origin/dev, the tests, then
    # git push origin HEAD:dev on its own; then move an unreviewed task to the deployer and check its report stops.

  @AGENT-22 @test
  Scenario: Agentry counts a turn as pushed only by an approved git push that finished
    # internal/tasks/pushed_test.go TestOnlyAFinishedApprovedPushIsPushed
    Given a turn's actions
    When Agentry asks whether the turn pushed the work
    Then yes only for an action whose command starts with git push, approved, with a result
    And a denied push, a push with no result yet, and git status each count as no
    And a push wrapped in timeout or chained after cd is not seen, which is why the prompt gives the exact command
    # Pushed: internal/tasks/task.go; a done task shows Shipped only then: internal/tasks/summary_test.go TestOnlyATaskWithAFinishedApprovedPushIsPushed

  @AGENT-23 @test
  Scenario: Every push asks the user
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given the deployer under developer
    When it runs git push origin HEAD:dev, or a bare git push
    Then each is asked, by the rule "every push leaves this machine: you approve it"
    And the push runs only once the user approves, and the agent is told "you approved"
    And the deployer's prompt names no push that the policy would allow without asking
    # the rule: internal/policy/default.toml line 134; the wait: internal/hooks/handler_test.go TestAnAskedActionWaitsForTheUser;
    # the prompt check: internal/agents/prompts_policy_test.go TestTheDefaultPromptsOnlyAskForCommandsThePolicyPermits

  @AGENT-24 @test
  Scenario: A push the user does not answer is held for them, and the deployer told to carry on
    # internal/hooks/handler_test.go TestAnAskThatTimesOutIsHeldForTheUser
    Given the deployer asked to push and the user gave no answer before the hook's deadline
    When the wait runs out
    Then the agent is told no, with "the command is held for the user's approval. Don't retry it now"
    And the action is held and undecided, the task is in Needs you, and 0 hook calls still wait on it
    And the task's waiting line reads exactly "Waiting for you to approve the push to dev"
    # the hook's deadline is 1790 seconds, GateTimeoutSeconds 1800 less 10: internal/turns/turns.go environment, internal/engines/engine.go;
    # the server answers 5 seconds before it: internal/hooks/handler.go replyMargin; the line: internal/tasks/waiting_test.go TestAHeldPushNamesItsBranch

  @AGENT-25 @test
  Scenario: A late approval lets the same command through once, in the agent's next turn
    # internal/hooks/handler_test.go TestALateApprovalLetsTheSameCommandThroughOnce
    Given a held push the user approved after the turn moved on
    When the coordinator is handed the command back and hands the deployer its next turn
    Then exactly that command, from that agent, in that turn, runs once without asking, and git push origin HEAD:main does not use the pass
    And the same push asked for again after that is asked about again
    And a policy denial still denies it, another agent's identical request is asked, and a pass not used in that turn is gone
    # TestAPassDoesntOverrideADenial, TestAPassIsOnlyForTheAgentThatAsked, TestAPassEndsWithTheAgentsNextTurn, TestALateDenialGivesNoPass in the same file;
    # PassFor: internal/tasks/task.go

  @AGENT-26 @test
  Scenario: A ship the user chose needs no earlier review, and counts only once the push went through
    # internal/coordinator/next_test.go TestYourShipTakesTheWorkAsItIs
    Given the user moved the task to Shipping and the coordinator handed it to the deployer
    When the deployer's turn starts
    Then its brief adds "The user chose to ship this as it is, so no earlier review or test is needed", and that failing checks after the merge mean no push
    And once an approved push finished in that turn, done is allowed with 0 reviews
    And after a push that was denied, hung, or stopped by failed tests, done is refused with "the work isn't checked yet", and a change after the ship needs checking again
    # YouChoseToShip: internal/coordinator/brief.go; TestYourShipTellsTheShipperToShipItAsItIs in the same file

  @AGENT-27 @test
  Scenario: The deployer reads the pushed commit's pipeline without asking
    # internal/policy/default_test.go TestTheDeveloperReadsAPipeline
    Given the deployer under developer, after its push
    When it runs glab ci list --sha <hash> -F json, or gh run list --commit <hash> --json status,conclusion,url
    Then each is allowed
    And gh run view 123 -w, glab ci get -p 123 --with-variables, gh run rerun 123 and gh run watch 123 are asked
    And the prompt says to report the pipeline once, with 0 waiting or polling
    # the rules: internal/policy/default.toml lines 164 to 168 and 192 to 193; the prompt: internal/agents/defaults/deployer.md

  # --- The other teams

  @AGENT-28 @manual
  Scenario: The Writing team researches, drafts, fact-checks and edits a piece
    Given a task of the Writing team
    When its agents take their turns
    Then the editor writes brief.md and does not write the piece itself
    And the researcher records every fact with its source in research/notes.md, invents 0 sources, and does not write the piece
    And the writer drafts draft.md with every fact sourced or marked [NEEDS SOURCE: what], and commits it
    And the fact-checker writes fact-check.md with a verdict per claim, does not rewrite the draft, and ends with exactly 1 verdict line
    # the prompts: internal/agents/defaults/writing/editor.md, researcher.md, writer.md, fact-checker.md
    # Steps: give the Writing team a short article; read brief.md, research/notes.md, draft.md and fact-check.md on the branch,
    # and check the fact-checker's report ends with a verdict.

  @AGENT-29 @test
  Scenario: The Writing team has one coordinator and three agents, the fact-checker checking the work
    # internal/agents/agents_test.go TestTeamsComeFromTemplatesWithTheirOwnCoordinator
    Given the Writing team added from its template
    When the roster is read
    Then its coordinator is the editor, and it has exactly 3 other agents, the fact-checker last
    And a task that names no team still goes to the first team's coordinator, tech-lead
    And added again, the team is "Writing 2" with the coordinator editor-2, and nothing the user has is renamed
    # internal/agents/teams.go AddTeam, MadeFor

  @AGENT-30 @manual
  Scenario: The Research team frames a question, gathers evidence, analyses it reproducibly and challenges it
    Given a task of the Research team
    When its agents take their turns
    Then the research-lead writes question.md and does not do the research itself
    And the investigator records sources in sources.md and data in data/ as CSV with a README, and never changes raw data after recording it
    And the analyst writes scripts in analysis/ that anyone can rerun, with uncertainty on every estimate, and SVG charts in analysis/charts/
    And the critic reruns the analysis, writes review.md, changes nothing else, and ends with exactly 1 verdict line
    # the prompts: internal/agents/defaults/research/research-lead.md, investigator.md, analyst.md, critic.md
    # Steps: give the Research team a question answerable from a CSV in the repo; read question.md, sources.md, analysis/ and
    # review.md on the branch, rerun the scripts, and check the critic's report ends with a verdict.

  @AGENT-31 @manual
  Scenario: The Design team frames the problem, draws every state, writes the words and critiques it
    Given a task of the Design team
    When its agents take their turns
    Then the product-lead writes spec.md with the problem in 1 sentence and a measure of success, and does not design it itself
    And the product-designer writes flows.md and SVG wireframes in designs/ for every state, with text contrast at least 4.5:1 and targets at least 44 by 44 pixels
    And the ux-writer writes the copy deck copy.md, naming every button with a verb and an object
    And the design-critic reviews against the spec and WCAG 2.2 AA, writes critique.md with a severity per finding, changes nothing else, and ends with exactly 1 verdict line
    # the prompts: internal/agents/defaults/design/product-lead.md, product-designer.md, ux-writer.md, design-critic.md
    # Steps: give the Design team a small screen to design; read spec.md, flows.md, designs/, copy.md and critique.md on the
    # branch, and check the critic's report ends with a verdict.

  @AGENT-32 @test
  Scenario: The Visionaries read the board and change no code
    # internal/agents/prompts_policy_test.go TestTheVisionariesReadTheBoardAndChangeNothing
    Given each of the 5 Visionaries under read-mostly
    When it calls the board tools list_tasks or show_task, or runs git log --oneline -30
    Then each is allowed
    And create_task is asked, and Edit on main.go, Write on ideas.md and git commit -am 'ideas' are denied
    And every prompt of the team says it changes no code and commits nothing: its report is its work
    # the prompts: internal/agents/defaults/visionaries/visionary-lead.md, product-strategist.md, user-advocate.md, technologist.md, skeptic.md

  @AGENT-33 @test
  Scenario: The Visionaries' ideas land in Ideas from the lead's done decision, for the user to decide
    # internal/ideas/ideas_test.go TestASuggestionComesFromATask
    Given the visionary-lead's done decision carries "ideas", each with a title, the problem, the proposal and a size of S, M or L
    When the loop applies the decision
    Then each idea is kept once, from that task and about its repo
    And an idea with an empty title, a size "XL", a kind "story", from a task that does not exist, or from an agent not working on the task is refused
    And only the user accepts or dismisses an idea, never an agent
    # IdeasFormat and suggest: internal/ideas/coordinator.go; the user decides: TestOnlyYouAcceptOrDismiss in the same file;
    # the skeptic's verdict closes the session: internal/agents/defaults/visionaries/skeptic.md

  @AGENT-34 @test
  Scenario: The Networking checkers diagnose without asking, and any live change asks whoever runs it
    # internal/agents/prompts_policy_test.go TestTheNetworkingTeamDiagnosesAndAsksBeforeChanging
    Given network-verifier and network-security under network-diagnosis, and network-engineer under developer
    When a checker runs ping -c 3 -W 2 <host>, dig +short <name>, ss -tlnp, ip route show, docker network inspect <network>, ufw status, nft list ruleset, iptables -S or wg show
    Then each is allowed, with 0 asked
    And ufw allow 443, an ip route add and systemctl restart nginx are asked for every agent of the team
    # "diagnoses the network": internal/policy/default.toml line 544; "changes live network state": line 515; the engineer's are asked by developer's otherwise = "ask", line 84

  @AGENT-35 @test
  Scenario: The network-diagnosis profile prints no private key and takes no extra option
    # internal/policy/default_test.go TestTheNetworkDiagnosisProfile
    Given a checker under network-diagnosis
    When it runs wg show wg0 dump, sudo wg show wg0 dump, wg show wg0 private-key, wg genkey, or reads a WireGuard config, acme.json or a privkey.pem
    Then each is denied, by "never prints a private key" or "never touch secrets"
    And ping -c 1 -W 1 --extra <host> is asked by "network commands take no other options", and ss -tlnp --extra is asked by the profile's otherwise = "ask", since ss is not in that rule
    # the rules: internal/policy/default.toml lines 480 to 486, 509 to 512 and 532 to 541; otherwise = "ask": line 475

  @AGENT-36 @manual
  Scenario: Networking touches nothing live without the user's go-ahead
    Given a task of the Networking team
    When its agents take their turns
    Then the lead's prompt says nothing live changes without the user's explicit go-ahead in this task, which the lead asks for only once the change has the exact commands, the effect, a check, a tested rollback and a way back in
    And the engineer changes config in the repo and never a live machine's firewall, routes, VPN, DNS records or proxy
    And the verifier and the security reviewer never change live state and cannot write files, so their evidence is their report, ending with exactly 1 verdict line
    And the lead's prompt says the shipped policy allows everything, so these rules are the only guard until the user turns that line off
    # the prompts: internal/agents/defaults/networking/network-lead.md, network-engineer.md, network-verifier.md, network-security.md
    # Steps: give the Networking team a config change in a repo; check the engineer's commits touch only the repo, that its report
    # proposes the live change with commands, check and rollback, and that the lead asks the user before any of it runs.

  # --- Under the read-only and read-mostly profiles

  @AGENT-37 @test
  Scenario: The judging profiles write no file from the shell
    # internal/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs echo hi > notes.md, echo hi >> notes.md, cat > notes.md <<'EOF', go test ./... &> out.txt, or echo hi > /tmp/notes.md
    Then each is denied by "writes no files from the shell"
    And go test ./... > /dev/null and a redirect into Claude's scratch folder are allowed
    # the rule: internal/policy/default.toml lines 225, 360 and 503; the allowed forms: TestTheJudgingProfilesStillReadAndTest

  @AGENT-38 @test
  Scenario: The judging profiles run nothing that changes files
    # internal/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs sed -i 's/a/b/' main.go, sed --in-place 's/a/b/' main.go, perl -pi -e 's/a/b/' main.go, rm main.go, mv a.go b.go, cp a.go b.go, touch new.go, mkdir -p docs, chmod +x test.sh, ln -s a b, sort -o out.txt in.txt, find . -name '*.log' -delete, git log --output=log.txt, echo hi | tee notes.md, go test ./... | tee out.txt, or gofmt -w .
    Then each is denied by "runs nothing that changes files"
    And cat, head, sed -n '1,5p' main.go, grep -r foo . and find . -name x are allowed
    # the rule, with tee and gofmt -w in it: internal/policy/default.toml lines 231, 366 and 557; gofmt -w: TestTheJudgingProfilesCloseWhatTheReviewFound

  @AGENT-39 @test
  Scenario: The judging profiles never commit or change branches
    # internal/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs git commit -m x, git add -A && git commit -m x, git -c user.name=x commit -m x, git -C . commit -m x, git push origin HEAD:dev, git add -A, git reset --hard HEAD~1, git checkout -- main.go, git stash, git merge origin/dev, git rebase dev, git tag v1, git worktree add ../x, git config user.name x, git branch -D old, or git branch new
    Then each is denied by "never commits or changes branches"
    And git branch, git branch -a, git branch --show-current, git tag and git config --get user.name are allowed, as listing forms
    # the rule: internal/policy/default.toml lines 322, 463 and 648; the listing forms: lines 311 to 314

  @AGENT-40 @test
  Scenario: Under the judging profiles, go and the test and lint tools write no files
    # internal/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs go build ./..., go install ./cmd/agentry, go test -c ./internal/policy, go test -coverprofile=c.out ./..., go test -mod=mod ./..., or go vet -vettool=./x ./...
    Then each is denied by "go writes no files"
    And npx eslint --fix ., npx vitest run -u, npx jest --coverage, pytest --junitxml=report.xml and cargo clippy --fix are denied by "test and lint tools write no files"
    And go test ./..., go test -count=1 -cover -run TestX -v ./internal/policy, go vet ./..., gofmt -l ., npx eslint ., npx jest --ci and pytest are allowed
    # the rules: internal/policy/default.toml lines 243 and 254; the test and lint flags: TestTheJudgingProfilesCloseWhatTheReviewFound
    # the allowed forms: TestTheJudgingProfilesStillReadAndTest (go test, go vet, gofmt -l, npx eslint); npx jest --ci and pytest: TestTheJudgingProfilesStillRunTheirTests and TestTheJudgingProfilesDenyEverySpellingOfAWritingFlag

  @AGENT-41 @test
  Scenario: Every spelling of a writing flag is denied
    # internal/policy/default_test.go TestTheJudgingProfilesDenyEverySpellingOfAWritingFlag
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs pytest -o cache_dir=src, pytest -ocache_dir=src, pytest -vo cache_dir=src, cargo -Zunstable-options test, npx jest -iu, npx vitest run -ru, npx eslint -fo out.txt ., or npx jest --config='{"collectCoverage":true}'
    Then each is denied, the flag found first, joined or in a short option cluster
    And python3 -m pytest -q -k 'not slow' tests, npx jest --ci and npx vitest run --reporter=verbose are allowed
    # clusters matched one by one: internal/policy/default.toml lines 58 to 60

  @AGENT-42 @test
  Scenario: The judging profiles run no tests from outside the workspace
    # internal/policy/default_test.go TestTheJudgingProfilesRunNoTestsFromOutside
    Given an agent under read-only, read-mostly or network-diagnosis, and a folder in Claude's scratch place
    When it runs go test <scratch>/..., cd <scratch> && npm test, pytest <scratch>, npx jest --reporters <scratch>/r.js, npx vitest run --config <scratch>/v.ts, or cargo test --manifest-path <scratch>/Cargo.toml
    Then each is asked, by "runs no tests from outside the workspace"
    And npx jest --config <scratch>/j.js and npx vitest run --execArgv=<node options> are denied
    And go test ./internal/policy/, npx jest --reporters=default,./tools/r.js and npx vitest run --config vitest.config.ts are allowed from the workspace, and the developer runs these from scratch too
    # the rule: internal/policy/default.toml lines 292, 433 and 618; TestTheJudgingProfilesStillRunTheirTests, TestOnlyTheJudgingProfilesAskAboutRunnersOutside

  @AGENT-43 @test
  Scenario: The gate follows cd, pushd and popd when it judges a test runner
    # internal/policy/default_test.go TestTheJudgingProfilesFollowCd
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs (cd <scratch> && npm test), pushd <scratch> && go test ./..., popd; go test ./..., cd internal && cd - && go test ./..., or cd .. && go test ./...
    Then each is asked
    And cd ~ && go test ./... and cd internal; cd ../..; go test ./... are denied, since those folders hold Agentry's own files
    And cd internal/policy && go test . and (cd <scratch>); go test ./... are allowed
    # a relative path is counted from every folder a cd may have reached: TestARelativePathIsCountedFromWhereCdWent

  @AGENT-44 @test
  Scenario: A flag that names a command is denied, and sed -n beyond its plain form asks
    # internal/policy/default_test.go TestTheJudgingProfilesCloseWhatTheReviewFound
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs git grep -O'touch x' foo, rg --pre ./x.sh foo, sort --compress-program=./x.sh in.txt, or npx vitest run --execArgv=<node options>
    Then each is denied by "runs no command a flag names"
    And sed -n 1p f -e 'w out' and npx tsc --noEmit -p . --init are asked by "beyond their plain forms, these ask"
    And sed -n 'w out' f, sed -n '1e touch x' f, uniq in.txt out.txt and npx tsc match no rule, so each is asked by the profile's otherwise = "ask"
    And sed -n '1,5p' main.go, sed -n 12p main.go and npx tsc --noEmit are allowed
    # the rules: internal/policy/default.toml lines 274 and 286; the plain sed -n forms: line 307; otherwise = "ask": line 200
    # npx vitest run --execArgv: TestTheJudgingProfilesRunNoTestsFromOutside; the rest: TestTheJudgingProfilesCloseWhatTheReviewFound

  @AGENT-45 @test
  Scenario: Every value a variable may hold is judged
    # internal/policy/default_test.go TestEveryValueOfAVariableIsJudged
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs for c in status 'commit -qam x'; do git $c; done, or c=log; c=commit; git $c -m x
    Then each is denied, the commit found among the values
    And c=status; printf -v c commit; git $c -m x and c=$(echo commit); git $c are asked, since the value cannot be known
    And for f in a b; do cat $f; done is allowed, and under developer for c in status 'push -f origin main'; do git $c; done is denied
    # a variable with several values is matched with each: internal/policy/default.toml lines 55 to 57; too many values: TestAVariableWithTooManyValuesIsUnknown

  @AGENT-46 @test
  Scenario: Read-mostly also reads the task board
    # internal/policy/default_test.go TestTheReadMostlyProfile
    Given an agent under read-mostly
    When it calls the board tools status, list_tasks or show_task
    Then each is allowed by "reads the task board"
    And create_task, answer_question and start are asked, since they write to the board
    And Edit on main.go, Write on ideas.md, git commit -am 'x' and cat .env are denied, and Edit on .agentry/knowledge.md is allowed
    # the rule: internal/policy/default.toml line 421

  @AGENT-47 @test
  Scenario: Agentry's own files are off limits for every profile, and no policy file can change that
    # internal/policy/default_test.go TestAgentrysOwnFilesAreAlwaysOffLimits
    Given an agent under any of the 6 profiles, allow-everything included
    When it runs sqlite3 ~/.agentry/agentry.db 'delete from events', cat ~/.agentry/tls/ca.key, tar c ~/.agentry, cp -a ~ /tmp/x, grep -r BEGIN ~/.agentry, ls -R ~/.agentry, bash -c 'cat ~/.agentry/tls/ca.key', cat ~/.agentry/*/ca.key, or cat /proc/self/cwd/../../tls/ca.key, or calls Edit on ~/.agentry/policy.toml or Read on ~/.agentry/ui-token
    Then each is denied by the built-in rule for Agentry's own files
    And cat ~/.agentry/worktrees/task-1/*.go, ls ~, grep -rn TODO . and go test ./... are not refused as Agentry's own files
    # built in and not in the policy file: internal/policy/default.toml lines 14 to 15; the protected files: the log, the policy, the sign-in token and the TLS folder

  # --- Rules every agent meets

  @AGENT-48 @test
  Scenario: rm -rf is refused
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs rm -rf /tmp/x, or ls && rm -rf build
    Then each is denied by "never rm -rf", in every spelling the rule lists: -rf, -fr, -Rf, -fR, -r -f, -f -r and --recursive --force
    And rm notes.md, 1 file in the workspace, is allowed
    And under read-only, read-mostly and network-diagnosis every rm is denied by "runs nothing that changes files", so rm -rf is refused there too; the autonomous and allow-everything profiles have no rule against it
    # "never rm -rf": internal/policy/default.toml line 103; 'rm *' under the judging profiles: lines 234, 369 and 560; TestTheJudgingProfilesWriteNothingFromTheShell

  @AGENT-49 @test
  Scenario: No agent pushes to a branch named main or master
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs git push origin main, git push origin HEAD:main, git -C . push origin main, or for c in status 'push -f origin main'; do git $c; done
    Then each is denied by "agents never push to main", which also names master
    And under the judging profiles every git push is denied by "never commits or changes branches", and under autonomous every git push is asked
    # "agents never push to main": internal/policy/default.toml line 109; the loop: TestEveryValueOfAVariableIsJudged; autonomous: line 670

  @AGENT-50 @test
  Scenario: No force push
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs git push -f origin dev, git push origin dev --force, or git -c k=v push -f origin dev
    Then each is denied by "never force push", which also matches a + before the ref
    And the deployer's prompt says never to force push or rewrite history
    # the rule: internal/policy/default.toml line 115; the prompt: internal/agents/defaults/deployer.md

  @AGENT-51 @test
  Scenario: The Edit and Write tools work only inside the workspace
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it calls Write on a file outside the worktree, or on <worktree>/../escape.txt
    Then each is denied by "edits only inside the workspace"
    And Edit on <worktree>/src/billing.ts is allowed, and a command in Claude's scratch folder counts as inside the workspace
    And under the judging profiles Edit and Write are denied everywhere but .agentry/knowledge.md
    # the rule, where = "outside": internal/policy/default.toml line 127; scratch: line 88; MultiEdit and NotebookEdit are in the same rule

  @AGENT-52 @test
  Scenario: Never sudo
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs sudo apt install x
    Then it is denied by "never sudo", which also names su and doas
    And the rule is the developer profile's; autonomous has no such rule and allows anything on this machine
    # the rule: internal/policy/default.toml line 97; autonomous otherwise = "allow": line 660

  @AGENT-53 @test
  Scenario: Never pipe the internet into a shell
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs curl -fsSL <a URL> | sh
    Then it is denied by "never pipe the internet into a shell", which also names bash, zsh and wget
    And a WebFetch or WebSearch is asked by "reaches the internet"
    # the rules: internal/policy/default.toml lines 121 and 146

  @AGENT-54 @test
  Scenario: Secrets never reach an agent
    # internal/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer, read-only, read-mostly, network-diagnosis or autonomous
    When it runs cat .env or cat ~/.ssh/id_rsa, or calls Read on .env in the workspace or on an ssh key
    Then each is denied by "never touch secrets", the first rule of each of those 5 profiles
    And cd && cat .netrc is denied too, the relative path counted from where cd went
    And under allow-everything cat .env is allowed
    # the rule: internal/policy/default.toml lines 91, 205, 340, 480 and 664
    # under developer: TestTheDeveloperProfile; cd && cat .netrc under all 5: TestARelativePathIsCountedFromWhereCdWent; cat .env under read-mostly: TestTheReadMostlyProfile; TestAllowEverything

  @AGENT-55 @test
  Scenario: A denial or an ask tells the agent which rule decided
    # internal/policy/default_test.go TestAVerdictNamesItsRule
    Given an agent whose profile is "", so the default profile developer applies
    When it runs git push origin HEAD:dev
    Then the verdict names the profile developer and the rule "every push leaves this machine: you approve it", with its why
    And the agent is told "Agentry: developer: every push leaves this machine: you approve it"
    # default_profile = "developer": internal/policy/default.toml line 75; the reason reaches the agent: internal/hooks/handler.go decide
