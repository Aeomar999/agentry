Feature: Policy
  The policy profiles each agent runs under, the gate that judges every action,
  and the Policy page.
  # Sources: cmd/agentry/hook.go, cmd/agentry/policy.go, internal/domain/policy,
  # internal/domain/policy/hooks, internal/domain/agents/agent.go, internal/web/policy_api.go,
  # ui/src/pages/PolicyPage.tsx, ui/src/components/organisms/TryPolicyCall.tsx,
  # ui/src/components/organisms/DefaultProfiles.tsx,
  # ui/src/components/organisms/PolicyHistory.tsx and their tests.

  # ---- The Policy page -------------------------------------------------------

  @POLICY-01 @user @test
  Scenario: The Policy page: the 6 default profiles, who uses which, and what is in force
    # internal/web/policy_api_test.go TestThePolicyShowsItsDefaultsUsersAndWhatIsInForce
    # the 6 profiles Agentry ships, developer, read-only, read-mostly, network-diagnosis, autonomous and allow-everything: internal/domain/policy/default.toml lines 82, 198, 333, 473, 658 and 683; use_for_everyone as shipped: line 79
    # the Software team's profiles, tech-lead and reviewer read-only, the rest developer: README.md, The policy, "Profiles"
    Given a policy file without read-mostly, with use_for_everyone set to allow-everything
    When the policy is read
    Then exactly 6 default profiles are listed, read-mostly missing and read-only current, the missing one with the text it would add
    And read-only is used by exactly 2 agents, reviewer and tech-lead
    And the reviewer's profile in force reads "read-only, overridden by allow-everything", and the agents list shows each agent's profile in force the same way

  @POLICY-02 @user @test
  Scenario: A missing default profile is added from the page, logged with its diff and a backup
    # internal/web/policy_api_test.go TestAMissingProfileIsAddedFromThePageAndLogged; internal/domain/policy/live_test.go TestAgentryKeepsItsNewestBackups
    # the 20 newest backups are kept: internal/domain/policy/live.go line 57; the row "missing" with "Add this profile": ui/src/components/organisms/DefaultProfiles.tsx lines 82 to 89 and 124
    Given a policy file without read-mostly, whose row reads "Your file doesn't have it. Adding it appends the <version> default, under a comment naming the version."
    When "Add this profile" is sent without the file's hash, or with a stale one
    Then it is refused with 409 "changed since the page loaded it: try again"
    When it is sent with the hash the page saw
    Then the file no longer lacks read-mostly: it comes under "# Added by Agentry <version>, the default profile of that version."
    And exactly 1 policy event is logged, by you, "added profile read-mostly", with the diff adding [profiles.read-mostly] and the file's text
    And a backup beside the file holds exactly the file as it was
    When it is added twice, or a profile Agentry does not ship is added
    Then the first is refused with 409 "already has" and the second with 404, and no refused change is logged
    Given 25 changes made by Agentry
    Then exactly 20 backups are kept, the newest ones

  @POLICY-03 @user @test
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

  @POLICY-04 @user @test
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

  @POLICY-05 @user @test
  Scenario: Try a call against a profile: the rule that decides, the verdict and who decides today
    # internal/web/policy_api_test.go TestACallIsTriedAsAProfileWouldJudgeIt, TestATriedCallSaysWhichRuleDecidedByItsPlace, TestATriedCallSaysWhoDecidesOnlyWhenAnotherProfileDoes, TestTryingACallChangesNothing and TestATriedCallIsRefusedWhenItCantBeJudged
    # the form "Try a call against <profile>" with Tool, 7 suggested tools, "Command or path" and Try: ui/src/components/organisms/TryPolicyCall.tsx lines 10 and 55 to 80; the lines under it: lines 93 to 104
    # a verdict no profile rule made has Order 0, no rule place: internal/domain/policy/verdict.go line 28; none of the named tests asserts it
    # the page's words "as if run in <workspace>" and "But today allow-everything decides for every agent:" are TryPolicyCall.tsx's; the tests assert the reply they are drawn from
    Given the policy as shipped and the developer profile's panel
    When the user tries Bash "go test ./..."
    Then the verdict shown is allow by the rule "stays inside the workspace", with its why
    And the verdict names the rule's place among the profile's rules, exactly 1 row for the page to mark
    When the user tries Bash "git push origin feature", Bash "sudo ls", or Read of a private key under .ssh
    Then the verdicts are ask "every push leaves this machine: you approve it", deny "never sudo" and deny "never touch secrets"
    And under read-only, Edit of main.go is deny "this agent changes nothing"
    And each adds that today allow-everything decides, with allow, while use_for_everyone is set; with it off, nothing is added
    When a call is tried
    Then nothing is logged and the policy's hash is the same
    When the profile does not exist, the tool is empty, or the workspace is a relative path
    Then they are refused with 404, 400 and 400; a tool with no input is judged

  @POLICY-06 @user @test
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
    Then the file is the same as the first version, and the newest version logged is yours, "reverted to the version of <date>"
    When a version without its text, position 999, or the version in force is reverted to
    Then they are refused with 409 "can't be restored", 404, and 409 "in force"
    When a revert would take read-mostly from the Visionaries
    Then it is refused with 409 naming the profile and visionary-lead, and the file is kept

  @POLICY-07 @user @manual
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

  # ---- Which profile each agent runs under -----------------------------------

  @POLICY-08 @agent @test
  Scenario: Each default agent runs under the profile the code gives it
    # internal/domain/agents/agents_test.go TestEachDefaultAgentHasAProfile; the 5: TestTheDefaultsAreFiveAgentsWithPrompts in the same file
    Given a new install's Software team
    When the roster is read
    Then it has exactly 5 agents: tech-lead under read-only, engineer under developer, reviewer under read-only, tester under developer, deployer under developer
    # not under test, checked by hand: exactly 2 of them check work, the reviewer and the tester; the deployer ships it, and the engineer builds it
    # internal/domain/agents/defaults.go; the columns: internal/domain/agents/agent.go Builds, Reviews, Ships

  @POLICY-09 @agent @test
  Scenario: The other teams' agents run under the profiles their templates give them
    # internal/domain/agents/agents_test.go TestTheVisionariesTeam
    Given the Visionaries and the Networking teams added from their templates
    When the roster is read
    Then all 5 Visionaries are under read-mostly, and only the skeptic's work counts as review
    And network-lead is under read-only, network-engineer under developer, network-verifier and network-security under network-diagnosis, the last 2 counting as review
    And every profile a template names is in the shipped policy
    # not under test, checked by hand: the shipped policy has 6 profiles
    # internal/domain/agents/agents_test.go TestTheNetworkingTeam; the profiles: internal/domain/agents/teams.go; internal/domain/agents/prompts_policy_test.go TestEveryTemplateProfileIsInTheShippedPolicy

  @POLICY-10 @agent @test
  Scenario: As shipped, every agent allows everything until the user turns that line off
    # internal/domain/policy/default_test.go TestAllowEverything
    Given the shipped policy, with use_for_everyone = "allow-everything"
    When an agent under developer, read-only or autonomous runs git push origin HEAD:dev or npm install left-pad, or writes a file outside the workspace
    Then each is allowed, judged by the profile "allow-everything"
    And an edit of Agentry's own policy file is still denied
    And with that line commented out, each agent is judged by its own profile, as every scenario below assumes
    # use_for_everyone: internal/domain/policy/default.toml line 79; the tests load the policy with it commented out: internal/domain/policy/default_test.go loadDefault

  @POLICY-11 @agent @test
  Scenario: The tester runs under developer, so it can commit tests
    # internal/domain/agents/agents_test.go TestEachDefaultAgentHasAProfile
    Given the Software team as shipped
    When the tester's profile and column are read
    Then its profile is exactly "developer", the same as the engineer's
    # not under test, checked by hand: its work counts as review, so its verdict is 1 of the checks a done decision waits for
    # Column: Reviews on the tester: internal/domain/agents/defaults.go; checks by column: internal/coordinator/checks.go recordOf

  @POLICY-12 @agent @manual
  Scenario: In Writing, Research and Design the coordinator is read-only and the others are developers
    Given the Writing, Research and Design teams added from their templates
    When each agent's profile and column are read on the Agents page
    Then in each of the 3 teams exactly 1 agent coordinates, under read-only, and the 3 others are under developer
    And exactly 1 agent in each team counts as review: the fact-checker, the critic and the design-critic
    # the templates: internal/domain/agents/teams.go, writing, research and design
    # Steps: on the Teams page add Writing, Research and Design from their templates; open each team's agents and
    # read the profile and the column of each of its 4 agents; compare with teams.go.

  # ---- The gate --------------------------------------------------------------

  @POLICY-13 @coordinator @test
  Scenario: The gate judges each action by the working agent's own profile
    # internal/domain/policy/hooks/handler_test.go TestTheGateUsesTheWorkingAgentsProfile
    Given the engineer, whose profile is developer, is the agent working on the task
    When it asks to run a command
    Then the policy is asked with exactly the profile "developer", not the coordinator's

  @POLICY-14 @agent @test
  Scenario: Only the agent working on the task passes the gate
    # internal/domain/policy/hooks/handler_test.go TestOnlyTheWorkingAgentPassesTheGate
    Given the engineer's turn is open on the task
    When a gate call arrives from the reviewer
    Then it is denied
    And the engineer's calls are judged with exactly its own profile, "developer"
    # TestTheGateUsesTheWorkingAgentsProfile in the same file

  @POLICY-15 @agent @test
  Scenario: A denial or an ask tells the agent which rule decided
    # internal/domain/policy/default_test.go TestAVerdictNamesItsRule
    Given an agent whose profile is "", so the default profile developer applies
    When it runs git push origin HEAD:dev
    Then the verdict names the profile developer and the rule "every push leaves this machine: you approve it", with its why
    # not under test, checked by hand: the agent is told "Agentry: developer: every push leaves this machine: you approve it" (internal/domain/policy/hooks/handler.go line 367)
    # default_profile = "developer": internal/domain/policy/default.toml line 75; the reason reaches the agent: internal/domain/policy/hooks/handler.go decide

  @POLICY-16 @worker @test
  Scenario: An agent's hook call needs the hook token, and only the working agent passes the gate
    Given a task with the engineer at work
    When a hook call arrives with a wrong hook token
    Then it is refused with status 401
    When it arrives with the right token
    Then it is answered with status 200
    When a gate call names an agent that is not the one working
    Then the action is denied: yes
    # internal/domain/policy/hooks/handler_test.go TestTheHTTPEndpointNeedsTheHookToken, TestOnlyTheWorkingAgentPassesTheGate

  @POLICY-17 @worker @test
  Scenario: The gate fails closed when the server cannot answer
    Given an agent session on a worker whose server is unreachable, slow or broken
    When the engine's gate hook calls `agentry hook`
    Then the hook exits 2 within its own deadline, so the action is blocked: yes
    # internal/turns/turns.go environment: the deadline is 1790 seconds, 10 under the engine's gate timeout of 1800 (internal/platform/engines/engine.go GateTimeoutSeconds)
    And the server's reply, when it comes, is printed as it is: yes
    # not under test, checked by hand: its exit code is kept (cmd/agentry/hook.go line 50); the test's reply has exit code 0, the default
    # cmd/agentry/hook_test.go TestTheHookFailsClosed, TestTheHookPassesTheServersReplyThrough

  # ---- The developer profile -------------------------------------------------

  @POLICY-18 @agent @test
  Scenario: The developer profile allows what stays inside the workspace
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given the engineer under developer
    When it runs ls -la, npm test, git add -A && git commit -m 'remove TODO', echo hi > notes.md, mkdir -p src/refunds && touch src/refunds/index.ts, rm notes.md, or git fetch origin && git merge origin/dev
    Then each is allowed
    And Edit on src/billing.ts in the workspace is allowed by "edit inside the workspace, the diff is reviewed", and Read is allowed anywhere
    # the rule name, for Edit on main.go in the workspace: internal/web/policy_api_test.go TestATriedCallRunsInAStandInWorktreeThatIsntAgentrys
    And go test ./... is allowed by "stays inside the workspace"
    # the rule name, for go test ./...: internal/web/policy_api_test.go TestACallIsTriedAsAProfileWouldJudgeIt; the rules: internal/domain/policy/default.toml line 171 and line 158
    # the rules: internal/domain/policy/default.toml lines 151 to 195

  @POLICY-19 @agent @test
  Scenario: What may reach outside the workspace asks the user
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given the engineer under developer
    When it runs sed -i '555d' src/billing.ts, echo hi > /tmp/notes.md, cp src/a.ts /tmp/a.ts, cd /etc && ls, npm install left-pad, python deploy.py, echo $(whoami), or WebFetch
    Then each is asked, and none runs before the user answers
    And a tool the policy does not name, and a command it cannot read, are asked too
    # otherwise = "ask" and unreadable = "ask": internal/domain/policy/default.toml lines 84 to 86; WebFetch: line 146
    # none runs before the answer: internal/domain/policy/hooks/handler_test.go TestAnAskedActionWaitsForTheUser

  @POLICY-20 @agent @test
  Scenario: A setting or an upper-case variable before a command asks, a plain toolchain setting does not
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given the engineer under developer
    When it runs git -c k=v commit -m x, GIT_EXTERNAL_DIFF=./x.sh git diff, GOFLAGS=-toolexec=./x.sh go test ./..., or export PAGER=./x.sh; git log
    Then each is asked, since the setting can make the program run a command
    And GOTOOLCHAIN=local GOPROXY=off go test ./... and git -C . commit -m x are allowed
    # variables: internal/domain/policy/default.toml lines 51 to 57; git's own options are left out before matching: lines 47 to 50

  @POLICY-21 @agent @test
  Scenario: The deployer reads the pushed commit's pipeline without asking
    # internal/domain/policy/default_test.go TestTheDeveloperReadsAPipeline
    Given the deployer under developer, after its push
    When it runs glab ci list --sha <hash> -F json, or gh run list --commit <hash> --json status,conclusion,url
    Then each is allowed
    And gh run view 123 -w, glab ci get -p 123 --with-variables, gh run rerun 123 and gh run watch 123 are asked
    # not under test, checked by hand: the prompt says to report the pipeline once, with 0 waiting or polling: internal/domain/agents/defaults/deployer.md line 9
    # the rules: internal/domain/policy/default.toml lines 164 to 168 and 192 to 193; the prompt: internal/domain/agents/defaults/deployer.md

  # ---- The judging profiles --------------------------------------------------

  @POLICY-22 @agent @test
  Scenario: The read-only profile lets the reviewer read and test, keep its notes, and change nothing else
    # internal/domain/policy/default_test.go TestTheReadOnlyProfile
    Given the reviewer under read-only
    When it calls Edit on main.go, or runs git commit -am 'x' or git push origin HEAD:dev
    Then each is denied, and Edit on main.go is denied by "this agent changes nothing"
    # the rule name: internal/web/policy_api_test.go TestACallIsTriedAsAProfileWouldJudgeIt
    And Edit on .agentry/knowledge.md in the workspace is allowed, and Write on the same file in another folder is denied
    # not under test, checked by hand: the rules "never commits or changes branches" and "keeps its own notes", at the lines below
    And Read on main.go, git diff c1169f1..HEAD and go test -v ./... && gofmt -l . are allowed
    # [profiles.read-only]: internal/domain/policy/default.toml line 198; "keeps its own notes" line 211; "this agent changes nothing" line 219; "never commits or changes branches" line 322

  @POLICY-23 @agent @test
  Scenario: The Visionaries read the board and change no code
    # internal/domain/agents/prompts_policy_test.go TestTheVisionariesReadTheBoardAndChangeNothing
    Given each of the 5 Visionaries under read-mostly
    When it calls the board tools list_tasks or show_task, or runs git log --oneline -30
    Then each is allowed
    And create_task is asked, and Edit on main.go, Write on ideas.md and git commit -am 'ideas' are denied
    # not under test, checked by hand: every prompt of the team says it changes no code and commits nothing, its report is its work: the first line of each prompt below
    # the prompts: internal/domain/agents/defaults/visionaries/visionary-lead.md, product-strategist.md, user-advocate.md, technologist.md, skeptic.md

  @POLICY-24 @agent @test
  Scenario: The Networking checkers diagnose without asking, and any live change asks whoever runs it
    # internal/domain/agents/prompts_policy_test.go TestTheNetworkingTeamDiagnosesAndAsksBeforeChanging
    Given network-verifier and network-security under network-diagnosis, and network-engineer under developer
    When a checker runs ping -c 3 -W 2 <host>, dig +short <name>, ss -tlnp, ip route show, docker network inspect <network>, ufw status, nft list ruleset, iptables -S or wg show
    Then each is allowed, with 0 asked
    And ufw allow 443, an ip route add and systemctl restart nginx are asked for every agent of the team
    # "diagnoses the network": internal/domain/policy/default.toml line 544; "changes live network state": line 515; the engineer's are asked by developer's otherwise = "ask", line 84

  @POLICY-25 @agent @test
  Scenario: The network-diagnosis profile prints no private key and takes no extra option
    # internal/domain/policy/default_test.go TestTheNetworkDiagnosisProfile
    Given a checker under network-diagnosis
    When it runs wg show wg0 dump, sudo wg show wg0 dump, wg show wg0 private-key, wg genkey, or reads a WireGuard config, acme.json or a privkey.pem
    Then each is denied
    And ping -c 1 -W 1 --extra <host> and ss -tlnp --extra are asked
    # not under test, checked by hand: the rules, "never prints a private key" or "never touch secrets" for the denials, "network commands take no other options" for ping, and the profile's otherwise = "ask" for ss, since ss is not in that rule
    # the rules: internal/domain/policy/default.toml lines 480 to 486, 509 to 512 and 532 to 541; otherwise = "ask": line 475

  @POLICY-26 @agent @test
  Scenario: The judging profiles write no file from the shell
    # internal/domain/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs echo hi > notes.md, echo hi >> notes.md, cat > notes.md <<'EOF', go test ./... &> out.txt, or echo hi > /tmp/notes.md
    Then each is denied
    # not under test, checked by hand: the rule that denies them, "writes no files from the shell"
    And go test ./... > /dev/null and a redirect into Claude's scratch folder are allowed
    # the rule: internal/domain/policy/default.toml lines 225, 360 and 503; the allowed forms: TestTheJudgingProfilesStillReadAndTest

  @POLICY-27 @agent @test
  Scenario: The judging profiles run nothing that changes files
    # internal/domain/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs sed -i 's/a/b/' main.go, sed --in-place 's/a/b/' main.go, perl -pi -e 's/a/b/' main.go, rm main.go, mv a.go b.go, cp a.go b.go, touch new.go, mkdir -p docs, chmod +x test.sh, ln -s a b, sort -o out.txt in.txt, find . -name '*.log' -delete, git log --output=log.txt, echo hi | tee notes.md, go test ./... | tee out.txt, or gofmt -w .
    Then each is denied
    # not under test, checked by hand: the rule that denies them, "runs nothing that changes files"
    And cat, head, sed -n '1,5p' main.go, grep -r foo . and find . -name x are allowed
    # the rule, with tee and gofmt -w in it: internal/domain/policy/default.toml lines 231, 366 and 557; gofmt -w: TestTheJudgingProfilesCloseWhatTheReviewFound; the allowed forms: TestTheJudgingProfilesStillReadAndTest; cat: TestTheJudgingProfilesFollowCd (cd <scratch> && cat out.txt)

  @POLICY-28 @agent @test
  Scenario: The judging profiles never commit or change branches
    # internal/domain/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs git commit -m x, git add -A && git commit -m x, git -c user.name=x commit -m x, git -C . commit -m x, git push origin HEAD:dev, git add -A, git reset --hard HEAD~1, git checkout -- main.go, git stash, git merge origin/dev, git rebase dev, git tag v1, git worktree add ../x, git config user.name x, git branch -D old, or git branch new
    Then each is denied
    # not under test, checked by hand: the rule that denies them, "never commits or changes branches"
    And git branch, git branch -a, git branch --show-current, git tag and git config --get user.name are allowed, as listing forms
    # the rule: internal/domain/policy/default.toml lines 322, 463 and 648; the listing forms: lines 311 to 314; they are allowed in TestTheJudgingProfilesStillReadAndTest

  @POLICY-29 @agent @test
  Scenario: Under the judging profiles, go and the test and lint tools write no files
    # internal/domain/policy/default_test.go TestTheJudgingProfilesWriteNothingFromTheShell
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs go build ./..., go install ./cmd/agentry, go test -c ./internal/domain/policy, go test -coverprofile=c.out ./..., go test -mod=mod ./..., or go vet -vettool=./x ./...
    Then each is denied
    And npx eslint --fix ., npx vitest run -u, npx jest --coverage, pytest --junitxml=report.xml and cargo clippy --fix are denied
    # not under test, checked by hand: the rules, "go writes no files" and "test and lint tools write no files"
    And go test ./..., go test -count=1 -cover -run TestX -v ./internal/domain/policy, go vet ./..., gofmt -l ., npx eslint ., npx jest --ci and pytest are allowed
    # the rules: internal/domain/policy/default.toml lines 243 and 254; the test and lint flags: TestTheJudgingProfilesCloseWhatTheReviewFound
    # the allowed forms: TestTheJudgingProfilesStillReadAndTest (go test, go vet, gofmt -l, npx eslint); npx jest --ci and pytest: TestTheJudgingProfilesStillRunTheirTests and TestTheJudgingProfilesDenyEverySpellingOfAWritingFlag

  @POLICY-30 @agent @test
  Scenario: Every spelling of a writing flag is denied
    # internal/domain/policy/default_test.go TestTheJudgingProfilesDenyEverySpellingOfAWritingFlag
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs pytest -o cache_dir=src, pytest -ocache_dir=src, pytest -vo cache_dir=src, cargo -Zunstable-options test, npx jest -iu, npx vitest run -ru, npx eslint -fo out.txt ., or npx jest --config='{"collectCoverage":true}'
    Then each is denied, the flag found first, joined or in a short option cluster
    And python3 -m pytest -q -k 'not slow' tests, npx jest --ci and npx vitest run --reporter=verbose are allowed
    # clusters matched one by one: internal/domain/policy/default.toml lines 58 to 60

  @POLICY-31 @agent @test
  Scenario: The judging profiles run no tests from outside the workspace
    # internal/domain/policy/default_test.go TestTheJudgingProfilesRunNoTestsFromOutside
    Given an agent under read-only, read-mostly or network-diagnosis, and a folder in Claude's scratch place
    When it runs go test <scratch>/..., cd <scratch> && npm test, pytest <scratch>, npx jest --reporters <scratch>/r.js, npx vitest run --config <scratch>/v.ts, or cargo test --manifest-path <scratch>/Cargo.toml
    Then each is asked
    # not under test, checked by hand: the rule that asks, "runs no tests from outside the workspace"
    And npx jest --config <scratch>/j.js and npx vitest run --execArgv=<node options> are denied
    And go test ./internal/domain/policy/, npx jest --reporters=default,./tools/r.js and npx vitest run --config vitest.config.ts are allowed from the workspace, and the developer runs these from scratch too
    # the rule: internal/domain/policy/default.toml lines 292, 433 and 618; TestTheJudgingProfilesStillRunTheirTests, TestOnlyTheJudgingProfilesAskAboutRunnersOutside

  @POLICY-32 @agent @test
  Scenario: The gate follows cd, pushd and popd when it judges a test runner
    # internal/domain/policy/default_test.go TestTheJudgingProfilesFollowCd
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs (cd <scratch> && npm test), pushd <scratch> && go test ./..., popd; go test ./..., cd internal && cd - && go test ./..., or cd .. && go test ./...
    Then each is asked
    And cd ~ && go test ./... and cd internal; cd ../..; go test ./... are denied, since those folders hold Agentry's own files
    And cd internal/domain/policy && go test . and (cd <scratch>); go test ./... are allowed
    # a relative path is counted from every folder a cd may have reached: TestARelativePathIsCountedFromWhereCdWent

  @POLICY-33 @agent @test
  Scenario: A flag that names a command is denied, and sed -n beyond its plain form asks
    # internal/domain/policy/default_test.go TestTheJudgingProfilesCloseWhatTheReviewFound
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs git grep -O'touch x' foo, rg --pre ./x.sh foo, sort --compress-program=./x.sh in.txt, or npx vitest run --execArgv=<node options>
    Then each is denied
    And sed -n 1p f -e 'w out', npx tsc --noEmit -p . --init, sed -n 'w out' f, sed -n '1e touch x' f, uniq in.txt out.txt and npx tsc are asked
    And sed -n '1,5p' main.go, sed -n 12p main.go and npx tsc --noEmit -p ui are allowed
    # not under test, checked by hand: the rules, "runs no command a flag names" for the denials, "beyond their plain forms, these ask" for the first 2 asks, and the profile's otherwise = "ask" for the other 4, which match no rule
    # the rules: internal/domain/policy/default.toml lines 274 and 286; the plain sed -n forms: line 307; otherwise = "ask": line 200
    # npx vitest run --execArgv: TestTheJudgingProfilesRunNoTestsFromOutside; the rest: TestTheJudgingProfilesCloseWhatTheReviewFound; the allowed forms: TestTheJudgingProfilesStillReadAndTest

  @POLICY-34 @agent @test
  Scenario: Every value a variable may hold is judged
    # internal/domain/policy/default_test.go TestEveryValueOfAVariableIsJudged
    Given an agent under read-only, read-mostly or network-diagnosis
    When it runs for c in status 'commit -qam x'; do git $c; done, or c=log; c=commit; git $c -m x
    Then each is denied, the commit found among the values
    And c=status; printf -v c commit; git $c -m x and c=$(echo commit); git $c are asked, since the value cannot be known
    And for f in a b; do cat $f; done is allowed, and under developer for c in status 'push -f origin main'; do git $c; done is denied
    # a variable with several values is matched with each: internal/domain/policy/default.toml lines 55 to 57; too many values: TestAVariableWithTooManyValuesIsUnknown

  @POLICY-35 @agent @test
  Scenario: Read-mostly also reads the task board
    # internal/domain/policy/default_test.go TestTheReadMostlyProfile
    Given an agent under read-mostly
    When it calls the board tools status, list_tasks or show_task
    Then each is allowed
    # not under test, checked by hand: the rule that allows them, "reads the task board"
    And create_task, answer_question and start are asked, since they write to the board
    And Edit on main.go, Write on ideas.md, git commit -am 'x' and cat .env are denied, and Edit on .agentry/knowledge.md is allowed
    # the rule: internal/domain/policy/default.toml line 421

  # ---- Rules every agent meets -----------------------------------------------

  @POLICY-36 @agent @test
  Scenario: A push asks the user first
    Given an agent under the developer profile, in its task's worktree
    When the agent runs git push origin HEAD:dev
    Then the user is asked before the push runs
    And a push to main, or a force push, is denied
    # README.md, The policy; internal/domain/policy/default_test.go

  @POLICY-37 @agent @test
  Scenario: Every push asks the user
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given the deployer under developer
    When it runs git push origin HEAD:dev, or a bare git push
    Then each is asked, and the verdict for git push origin HEAD:dev names the rule "every push leaves this machine: you approve it"
    # the rule named: internal/domain/policy/default_test.go TestAVerdictNamesItsRule; not under test, checked by hand: the same rule for the bare git push
    And the push runs only once the user approves, and the agent is told "you approved"
    And the deployer's prompt names no push that the policy would allow without asking
    # the rule: internal/domain/policy/default.toml line 134; the wait: internal/domain/policy/hooks/handler_test.go TestAnAskedActionWaitsForTheUser;
    # the prompt check: internal/domain/agents/prompts_policy_test.go TestTheDefaultPromptsOnlyAskForCommandsThePolicyPermits

  @POLICY-38 @agent @test
  Scenario: Agentry's own files are off limits for every profile, and no policy file can change that
    # internal/domain/policy/default_test.go TestAgentrysOwnFilesAreAlwaysOffLimits
    Given an agent under any of the 6 profiles, allow-everything included
    When it runs sqlite3 ~/.agentry/agentry.db 'delete from events', cat ~/.agentry/tls/ca.key, tar c ~/.agentry, cp -a ~ /tmp/x, grep -r BEGIN ~/.agentry, ls -R ~/.agentry, bash -c 'cat ~/.agentry/tls/ca.key', cat ~/.agentry/*/ca.key, or cat /proc/self/cwd/../../tls/ca.key, or calls Edit on ~/.agentry/policy.toml or Read on ~/.agentry/ui-token
    Then each is denied by the built-in rule for Agentry's own files
    And cat ~/.agentry/worktrees/task-1/*.go, ls ~, grep -rn TODO . and go test ./... are not refused as Agentry's own files
    # built in and not in the policy file: internal/domain/policy/default.toml lines 14 to 15; the protected files: the log, the policy, the sign-in token and the TLS folder

  @POLICY-39 @agent @test
  Scenario: rm -rf is refused
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs rm -rf /tmp/x, or ls && rm -rf build
    Then each is denied
    And rm notes.md, 1 file in the workspace, is allowed
    And under read-only, read-mostly and network-diagnosis rm main.go is denied too
    # not under test, checked by hand: the rule "never rm -rf" and every spelling it lists, -rf, -fr, -Rf, -fR, -r -f, -f -r and --recursive --force; under the judging profiles the rule is "runs nothing that changes files", so rm -rf is refused there too; the autonomous and allow-everything profiles have no rule against it
    # "never rm -rf": internal/domain/policy/default.toml line 103; 'rm *' under the judging profiles: lines 234, 369 and 560; TestTheJudgingProfilesWriteNothingFromTheShell

  @POLICY-40 @agent @test
  Scenario: No agent pushes to a branch named main
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs git push origin main, git push origin HEAD:main, git -C . push origin main, or for c in status 'push -f origin main'; do git $c; done
    Then each is denied
    And under the judging profiles git push origin HEAD:dev is denied, and under autonomous it is asked
    # not under test, checked by hand: the rule "agents never push to main", which also names master (internal/domain/policy/default.toml line 110); under the judging profiles every git push denied by "never commits or changes branches", and under autonomous every git push asked
    # "agents never push to main": internal/domain/policy/default.toml line 109; the loop: TestEveryValueOfAVariableIsJudged; autonomous: line 670

  @POLICY-41 @agent @test
  Scenario: No force push
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs git push -f origin dev, git push origin dev --force, or git -c k=v push -f origin dev
    Then each is denied
    # not under test, checked by hand: the rule "never force push", which also matches a + before the ref, and the deployer's prompt saying never to force push or rewrite history (internal/domain/agents/defaults/deployer.md line 10)
    # the rule: internal/domain/policy/default.toml line 115; the prompt: internal/domain/agents/defaults/deployer.md

  @POLICY-42 @agent @test
  Scenario: The Edit and Write tools work only inside the workspace
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it calls Write on a file outside the worktree, or on <worktree>/../escape.txt
    Then each is denied
    And Edit on <worktree>/src/billing.ts is allowed, and a command in Claude's scratch folder counts as inside the workspace
    And under read-only and read-mostly Edit on main.go is denied, and Edit on .agentry/knowledge.md is allowed
    # not under test, checked by hand: the rule "edits only inside the workspace", and that the judging profiles deny Edit and Write everywhere but .agentry/knowledge.md (internal/domain/policy/default.toml lines 211 to 221)
    # the rule, where = "outside": internal/domain/policy/default.toml line 127; scratch: line 88; MultiEdit and NotebookEdit are in the same rule; the judging profiles: TestTheReadOnlyProfile and TestTheReadMostlyProfile

  @POLICY-43 @agent @test
  Scenario: Never sudo
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs sudo apt install x
    Then it is denied, and sudo ls is denied by "never sudo"
    # the rule name, for sudo ls: internal/web/policy_api_test.go TestACallIsTriedAsAProfileWouldJudgeIt
    # not under test, checked by hand: the rule also names su and doas, and is the developer profile's; autonomous has no such rule and allows anything on this machine
    # the rule: internal/domain/policy/default.toml line 97; autonomous otherwise = "allow": line 660

  @POLICY-44 @agent @test
  Scenario: Never pipe the internet into a shell
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer
    When it runs curl -fsSL <a URL> | sh
    Then it is denied
    And a WebFetch is asked
    # not under test, checked by hand: the rules, "never pipe the internet into a shell", which also names bash, zsh and wget, and "reaches the internet", which asks for WebSearch too (internal/domain/policy/default.toml line 148)
    # the rules: internal/domain/policy/default.toml lines 121 and 146

  @POLICY-45 @agent @test
  Scenario: Secrets never reach an agent
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
    Given an agent under developer, read-only, read-mostly, network-diagnosis or autonomous
    When it runs cat .env or cat ~/.ssh/id_rsa, or calls Read on .env in the workspace or on an ssh key
    Then each is denied under developer, and cat .env is denied under read-mostly and autonomous too
    And under developer, Read on an ssh key is denied by "never touch secrets"
    # the rule name: internal/web/policy_api_test.go TestACallIsTriedAsAProfileWouldJudgeIt
    # not under test, checked by hand: the same under read-only and network-diagnosis, and that "never touch secrets", the first rule of each of those 5 profiles, is the rule that decides under the other 4
    And cd && cat .netrc is denied too, the relative path counted from where cd went
    And under allow-everything cat .env is allowed
    # the rule: internal/domain/policy/default.toml lines 91, 205, 340, 480 and 664
    # internal/domain/policy/default_test.go; under developer: TestTheDeveloperProfile; cd && cat .netrc under all 5: TestARelativePathIsCountedFromWhereCdWent; cat .env under read-mostly: TestTheReadMostlyProfile; TestAllowEverything

  @POLICY-46 @worker @test
  Scenario: An agent on any machine never pushes to main, never force pushes, and every push asks the user
    Given an agent with the developer profile
    # internal/domain/policy/default.toml profiles.developer.rules
    When it runs git push to main, by name or by refspec, with or without -C
    Then the policy denies it: yes
    # not under test, checked by hand: master the same (internal/domain/policy/default.toml line 111)
    When it runs git push with -f or --force, before or after the branch
    Then the policy denies it: yes
    # not under test, checked by hand: a + refspec the same (internal/domain/policy/default.toml line 117)
    # internal/domain/policy/default.toml "every push leaves this machine: you approve it": any other git push asks the user first
    # internal/domain/policy/default_test.go TestTheDeveloperProfile
