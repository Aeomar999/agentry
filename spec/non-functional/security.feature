Feature: Security
  Signing in, credentials, the gate and what no agent may do.

  @SEC-01 @test
  Scenario: Wrong sign-in codes are counted and limited
    Given the sign-in page and a one-time code
    When 5 wrong codes come from one address within a minute
    Then the next sign-in from that address is refused with 429 and a Retry-After of 60 seconds
    And a sign-in from another address is allowed
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-02 @test
  Scenario: Every tool call passes the gate before it runs
    Given an agent's turn is running under Claude Code with the PreToolUse hook installed as the gate for every tool
    When the agent asks to run a tool
    Then the hook posts the call to the server exactly once
    And the agent gets the server's reply, allow or deny, as the hook's only output, before the tool runs
    # README.md, What happens to one action; PLAN.md T-009; cmd/agentry/hook_test.go

  @SEC-03 @test
  Scenario: The gate fails closed when the server cannot be reached
    Given a gate call with a deadline of 300 ms, and a server that is unreachable, answers after the deadline, or answers with anything but HTTP 200 and a JSON reply
    When the hook is called for PreToolUse
    Then the hook exits 2
    And its stderr says "Agentry blocked this (fail closed)" and the reason, every time
    And no tool call runs, whatever the tool
    And the hook gives up within 1 second
    # README.md, What happens to one action; PLAN.md T-009; cmd/agentry/hook_test.go

  @SEC-04 @test
  Scenario: A gate hook blocks on any failure and a report hook never blocks
    Given Agentry prepares an engine's hook settings for a task
    When the settings are written
    Then the PreToolUse gate's command ends with "|| exit 2", so the hook blocks the call on every failure
    And the Stop report's command ends with "|| true", so a report never blocks
    # PLAN.md T-008; internal/engines/engines_test.go

  @SEC-05 @test
  Scenario: The hook gives up before the engine would kill it
    Given the engine lets a gate hook run for 1800 seconds
    When a turn's environment is prepared
    Then the hook's own deadline is more than 0 and less than 1800 seconds, so the hook fails closed and never open
    # PLAN.md T-009; internal/turns/turns_test.go

  @SEC-06 @test
  Scenario: The server answers before the hook's deadline
    Given a gate call that the policy says to ask about, with no answer from the user
    When the call carries the hook's deadline
    Then with a deadline of 20 seconds the server waits at most 15 seconds, 5 seconds less
    And with a deadline of 1 second the server waits at most 750 ms, a quarter less
    And with a deadline of 1790 seconds, as a real turn sends, the server waits at most 1785 seconds
    # PLAN.md T-009; internal/hooks/handler_test.go

  @SEC-07 @test
  Scenario: An ask that gets no answer in time is held for the user
    Given a gate call that the policy says to ask about, with no answer from the user before the server's wait ends
    When the wait runs out
    Then an action.held event is recorded exactly once
    And the agent is told no, with the words that the command is held for the user's approval
    And the action is kept in Needs you for the user to approve or deny
    # README.md, What happens to one action; PLAN.md T-009; internal/hooks/handler_test.go

  @SEC-08 @test
  Scenario: Only the agent whose turn is running passes the gate
    Given the engineer's turn is running on a task
    When a gate call arrives naming the reviewer as its agent
    Then the call is denied
    And the engineer's own calls are judged with the engineer's profile, not the reviewer's
    # PLAN.md T-041; internal/hooks/handler_test.go

  @SEC-09 @test
  Scenario: The gate needs the hook token
    Given the server takes hook calls at /hooks and nothing else with the hook token
    When a call arrives with the wrong hook token
    Then the answer is HTTP 401
    And the compare takes the same time whatever the token, so no token is guessed byte by byte
    # PLAN.md T-009; internal/hooks/handler_test.go

  @SEC-10 @test
  Scenario: Rules are read top to bottom and a tool with no rule gets the profile's otherwise
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When the agent calls a tool Agentry has no rule for
    Then the call is asked, by the profile's otherwise
    And every verdict names its profile, its rule and the rule's place, rules counted from 1, 0 for otherwise and Agentry's own files
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go; internal/policy/effective_test.go

  @SEC-11 @test
  Scenario: A shell command Agentry cannot parse gets the profile's unreadable
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When the agent runs a shell command with an unterminated quote
    Then the call is asked, by the profile's unreadable
    # README.md, The policy; PLAN.md T-039; internal/policy/default_test.go

  @SEC-12 @test
  Scenario: As shipped, every agent allows everything but Agentry's own files
    Given the shipped policy as written, with use_for_everyone set to allow-everything
    When any agent pushes to main with force, pushes to dev, reads .env, installs a package or writes a file outside the worktree
    Then the call is allowed
    And every verdict's profile is allow-everything, whatever the agent's own
    # README.md, The policy; PLAN.md T-069; internal/policy/default_test.go

  @SEC-13 @test
  Scenario: Agentry's own files are off limits under all six profiles
    Given the shipped policy, and the protected files: the log, policy.toml, features.toml, the four token files and the TLS folder with its CA key
    When an agent under any of the six profiles, allow-everything included, reads, edits, copies, archives or greps a protected file, by its path, by a pattern, through a folder that holds it, through a shell string, or through a path that loops back in
    Then the call is denied
    And the rule is "Agentry's own files are off limits", the same under every profile
    And a command whose text names the hook token's environment variable is denied too
    # README.md, The policy; PLAN.md T-040; internal/policy/default_test.go; cmd/agentry/jobs_test.go for features.toml, protected by cmd/agentry/policy.go with the hook, desk and helper token files

  @SEC-14 @test
  Scenario: Under developer, shell work inside the workspace is allowed
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it runs ls, npm test, go test, a local git add and commit, a redirect into the worktree, rm of one file in the worktree, go test with GOTOOLCHAIN and GOPROXY set, or a glab or gh command that lists a pipeline
    Then every call is allowed
    And each is allowed by the rule "stays inside the workspace"
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-15 @test
  Scenario: Under developer, reading anywhere and editing inside the worktree are allowed
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it reads a file outside the worktree that is not a secret, and edits a file inside the worktree
    Then the read is allowed by the rule "reading is always allowed"
    And the edit is allowed by the rule "edit inside the workspace, the diff is reviewed"
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-16 @test
  Scenario: Under developer, what may reach outside the machine asks the user
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it pushes to any branch but main or master, redirects out of the worktree, runs a substitution, sets an upper-case variable before a command, runs find -exec, fetches a web page, opens a pipeline in a browser, shows a pipeline's variables, or runs a command no rule names such as sed -i
    Then every call is asked
    And the push is asked by the rule "every push leaves this machine: you approve it"
    And find -exec is asked by the rule "commands that run other commands"
    And the web page is asked by the rule "reaches the internet"
    And the pipeline is asked by the rule "pipeline commands that open a browser or show CI variables"
    And the rest are asked by the profile's otherwise
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-17 @test
  Scenario: Under developer, rm -rf is denied in every spelling
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it runs rm -rf alone, or hidden in a chain after another command
    Then the call is denied
    And the rule is "never rm -rf", whose 7 spellings are listed in the shipped policy's default.toml
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go asserts the 2 forms; internal/policy/default.toml lists the 7 spellings

  @SEC-18 @test
  Scenario: Under developer, a push to main or master is denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it pushes to main or master by branch name, by refspec, through git -C, or hidden in a shell loop's values
    Then the call is denied
    And the rule is "agents never push to main"
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-19 @test
  Scenario: Under developer, force pushes, sudo and piping the internet into a shell are denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it force pushes to any branch, runs sudo, or pipes a download into sh
    Then every call is denied
    And the force push is denied by the rule "never force push"
    And sudo is denied by the rule "never sudo"
    And the download is denied by the rule "never pipe the internet into a shell"
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-20 @test
  Scenario: Under developer, secrets are denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it reads .env with cat or the Read tool, or a key under ~/.ssh
    Then every call is denied
    And the rule is "never touch secrets"
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-21 @test
  Scenario: Under developer, a write outside the worktree is denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the developer profile
    When it writes a file under /etc, or a file reached by a .. path that leaves the worktree
    Then every call is denied
    And the rule is "edits only inside the workspace"
    # README.md, The policy; PLAN.md T-013; internal/policy/default_test.go

  @SEC-22 @test
  Scenario: Under read-only, the agent reads, runs the tests and keeps its own notes
    Given the shipped policy with use_for_everyone turned off, and an agent with the read-only profile, as the reviewer and every coordinator but the Visionaries' have
    When it reads a file, runs go test and gofmt -l, or edits its own knowledge file in the worktree
    Then every call is allowed
    And the tests are allowed by the rule "reads and runs the tests"
    And the knowledge file is allowed by the rule "keeps its own notes"
    # README.md, The policy; PLAN.md T-041; internal/policy/default_test.go

  @SEC-23 @test
  Scenario: Under read-only, every change is denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the read-only profile
    When it edits any other file, commits, pushes, redirects into a file, pipes into tee, runs sort -o, find -delete, gofmt -w, eslint --fix, or jest --coverage
    Then every call is denied
    And the edit is denied by the rule "this agent changes nothing"
    And the commit and the push are denied by the rule "never commits or changes branches"
    And the redirect is denied by the rule "writes no files from the shell"
    And tee, sort -o, find -delete and gofmt -w are denied by the rule "runs nothing that changes files"
    And eslint --fix and jest --coverage are denied by the rule "test and lint tools write no files"
    # README.md, The policy; PLAN.md T-041; internal/policy/default_test.go

  @SEC-24 @test
  Scenario: Under read-only, read-mostly and network-diagnosis, a flag that names a program to run is denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the read-only, read-mostly or network-diagnosis profile
    When it runs git grep -O, rg --pre or sort --compress-program
    Then every call is denied
    And the rule is "runs no command a flag names"
    # README.md, The policy; PLAN.md T-086; internal/policy/default_test.go

  @SEC-25 @test
  Scenario: Under read-mostly, the Visionaries read the task board
    Given the shipped policy with use_for_everyone turned off, and an agent with the read-mostly profile, as the Visionaries have
    When it calls the board's status, list_tasks or show_task tool
    Then every call is allowed
    And each is allowed by the rule "reads the task board"
    # README.md, Ideas; PLAN.md T-041; internal/policy/default_test.go

  @SEC-26 @test
  Scenario: Under read-mostly, changing the board asks the user
    Given the shipped policy with use_for_everyone turned off, and an agent with the read-mostly profile
    When it calls create_task, answer_question or start
    Then every call is asked
    And each is asked by the profile's otherwise, which is ask
    # README.md, Ideas; PLAN.md T-041; internal/policy/default_test.go

  @SEC-27 @test
  Scenario: Under read-mostly, files and secrets are denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the read-mostly profile
    When it edits a file, commits, redirects into a file, or reads .env
    Then every call is denied
    # README.md, Ideas; PLAN.md T-041; internal/policy/default_test.go

  @SEC-28 @test
  Scenario: Under network-diagnosis, the fixed read forms run
    Given the shipped policy with use_for_everyone turned off, and an agent with the network-diagnosis profile
    When it runs ping -c with -W, dig +short, ss -tlnp, ip route show, docker network inspect, iptables -L -n -v, or wg show
    Then every call is allowed
    And each is allowed by the rule "diagnoses the network"
    # README.md, The policy; internal/policy/default_test.go

  @SEC-29 @test
  Scenario: Under network-diagnosis, any other option and any live change asks the user
    Given the shipped policy with use_for_everyone turned off, and an agent with the network-diagnosis profile
    When it runs one of the fixed read forms with any other option, opens a firewall port, adds a route or restarts a service
    Then every call is asked
    And the firewall, the route and the service are asked by the rule "changes live network state"
    # README.md, The policy; internal/policy/default_test.go

  @SEC-30 @test
  Scenario: Under network-diagnosis, keys and tunnel dumps are denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the network-diagnosis profile
    When it dumps a tunnel with wg show dump, makes a key with wg genkey, or reads a WireGuard config, an ACME store or a certificate's private key
    Then every call is denied
    And the dump and the key are denied by the rule "never prints a private key"
    And the config, the store and the certificate's key are denied by the rule "never touch secrets"
    # README.md, The policy; internal/policy/default_test.go

  @SEC-31 @test
  Scenario: Under autonomous, anything on this machine runs
    Given the shipped policy with use_for_everyone turned off, and an agent with the autonomous profile
    When it installs a package or runs a script
    Then every call is allowed
    And each is allowed by the profile's otherwise, which is allow
    # README.md, The policy; internal/policy/default_test.go

  @SEC-32 @test
  Scenario: Under autonomous, acting outside this machine asks the user
    Given the shipped policy with use_for_everyone turned off, and an agent with the autonomous profile
    When it pushes to a branch, creates a repo on a forge, or runs curl
    Then every call is asked
    And each is asked by the rule "acts outside this machine: you approve it"
    # README.md, The policy; internal/policy/default_test.go

  @SEC-33 @test
  Scenario: Under autonomous, secrets are denied
    Given the shipped policy with use_for_everyone turned off, and an agent with the autonomous profile
    When it reads .env
    Then the call is denied
    And the rule is "never touch secrets"
    # README.md, The policy; internal/policy/default_test.go

  @SEC-34 @test
  Scenario: An asked action puts the task in Needs you
    Given the policy says ask for the agent's command
    When the gate judges it
    Then exactly one action.asked event is recorded, with the verdict's profile, rule and why
    And the task is shown in Needs you
    And the command does not run until the user answers
    # README.md, What happens to one action; PLAN.md T-013; internal/hooks/handler_test.go

  @SEC-35 @test
  Scenario: The user's approval lets the asked command run
    Given an asked action waiting in Needs you while the hook waits
    When the user approves it
    Then the agent is told "Agentry: you approved: " followed by the user's reason, exactly once
    And the command is allowed
    # README.md, What happens to one action; PLAN.md T-013; internal/hooks/handler_test.go

  @SEC-36 @manual
  Scenario: The user's denial stops the asked command
    Given a task whose agent runs under the developer profile, and the agent runs a command the policy asks about, such as a git push, so the action waits in Needs you while the hook waits
    When the user opens Needs you, writes a reason and denies the action
    Then the agent's terminal shows "Agentry: you denied: " followed by the user's reason, exactly once
    And the command does not run, so the branch is not pushed
    # README.md, What happens to one action; PLAN.md T-013; the text is internal/hooks/handler.go's reply on action.denied; no test asserts it while the hook waits

  @SEC-37 @test
  Scenario: A late approval lets the same command through once, for the agent that asked
    Given an asked action that was held because no answer came in time
    When the user approves it after the hold
    Then the same command from the agent that asked passes the gate exactly once
    And the same command from another agent is not passed
    And the pass ends with the agent's next turn, so a later call is judged again, not passed
    And a pass never overrides a denial
    # README.md, What happens to one action; internal/hooks/handler_test.go

  @SEC-38 @test
  Scenario: A broken policy file never replaces a working one
    Given a working policy.toml in force
    When the file is saved with a decision word Agentry does not know
    Then the file is refused with a message that names the unknown word
    And the policy before it stays in force, with no rule changed
    And the same broken file is reported exactly once, not on every watch
    # README.md, The policy; PLAN.md T-042; internal/policy/file_test.go

  @SEC-39 @test
  Scenario: A fixed policy file takes over from a refused one
    Given a refused policy.toml and the policy before it still in force
    When the file is saved with the mistake fixed
    Then the fixed policy is in force, with every rule read from the file
    And no refusal is shown any more
    # README.md, The policy; PLAN.md T-042; internal/policy/file_test.go

  @SEC-40 @test
  Scenario: A broken policy file at start stops the server
    Given a policy.toml that does not load
    When agentry serve starts
    Then the server does not start
    And its error names the file, the line and the newest backup to copy back, exactly one
    # README.md, The policy; PLAN.md T-075; internal/policy/file_test.go

  @SEC-41 @manual
  Scenario: A saved policy change takes effect within two seconds
    Given Agentry is running with the shipped policy and the Policy page is open
    When the user saves a valid change to policy.toml and waits 2 seconds
    Then the Policy page shows the new version within 2 seconds
    And the next gate call is judged by the new version, not the old
    # README.md, The policy; PLAN.md T-042; the watch interval is set to 2 s in cmd/agentry/serve.go

  @SEC-42 @test
  Scenario: Agentry backs the policy up before it changes it, and keeps the newest 20
    Given the user changes the policy from the page
    When Agentry writes the file
    Then a copy of the file as it was is kept beside it, named by its time
    And at most 20 backups are kept, the newest
    # README.md, The policy; PLAN.md T-069; internal/policy/live_test.go

  @SEC-43 @test
  Scenario: Every policy version is in the log and can be restored
    Given a policy that has been loaded, changed on the page and edited on disk
    When the user opens the policy's history
    Then every version loaded is listed with who made it and a diff
    And every file refused is listed with its reason
    And only a version whose text the log holds can be reverted
    # README.md, The policy; PLAN.md T-069; internal/policy/log_test.go

  @SEC-44 @manual
  Scenario: Four tokens are made once and kept across restarts
    Given the user starts agentry serve on a fresh data dir
    When the user lists the data dir
    Then it holds exactly 4 token files: ui-token, hook-token, desk-token and helper-token
    And each holds 32 hex characters from 16 random bytes
    And each file's mode is 0600
    And after a restart the same 4 tokens are in force, so the printed link and running sessions keep working
    And no token expires or rotates
    # README.md, What Start Agentry does; the tokens are made in cmd/agentry/serve.go

  @SEC-45 @test
  Scenario: The page token opens the API three ways
    Given the server has a page token
    When a request carries it as a Bearer header, as ?token= in the address, or in the cookie the printed link set
    Then the API answers HTTP 200 each way
    # README.md, Signing in the installed app, or another device; PLAN.md T-006; internal/web/api_test.go

  @SEC-46 @test
  Scenario: The hook token does not open the API
    Given the server has a page token and a hook token
    When a request carries the hook token as a Bearer header, or no token at all
    Then the API answers HTTP 401
    And an agent can never approve its own request
    # README.md, What happens to one action; PLAN.md T-009; internal/web/api_test.go

  @SEC-47 @test
  Scenario: The desk token opens tasks
    Given the desk's Claude session holds the desk token
    When it lists, creates or shows a task, reads a task's events, answers a question or files a task under an epic
    Then every call is allowed
    # README.md, Use it; PLAN.md T-046; internal/web/api_test.go

  @SEC-48 @test
  Scenario: The desk token opens nothing more
    Given the desk's Claude session holds the desk token
    When it approves or denies an action, closes an epic, or reads the policy or the changes
    Then the answer is HTTP 401 every time
    # README.md, Use it; PLAN.md T-046; internal/web/api_test.go

  @SEC-49 @test
  Scenario: The helper token sends one task's messages
    Given the Claude session in a task's Workspace holds the helper token
    When it posts a message to its own task's claude/message route
    Then the message is allowed
    # README.md, Use it; internal/web/claude_api_test.go

  @SEC-50 @test
  Scenario: The helper token opens no other route
    Given the Claude session in a task's Workspace holds the helper token
    When it calls any other route, such as answer or the tasks list
    Then the answer is HTTP 401
    # README.md, Use it; internal/web/api_test.go

  @SEC-51 @test
  Scenario: Only the real page token issues sign-in codes
    Given a Claude Code connection's credential, which counts as the page token on the rest of the API
    When that credential asks for a sign-in code
    Then the answer is HTTP 401
    And the real page token asking the same gets exactly 1 code
    # README.md, Use it from Claude Code; PLAN.md T-063; internal/web/signin_test.go

  @SEC-52 @test
  Scenario: The sign-in cookie lasts a year and is HttpOnly
    Given a device opens the printed link or signs in with a code
    When the server sets the cookie
    Then its name is agentry_ui, its path is /, and it lasts at least 300 days, 365 as the code sets it
    And it is HttpOnly with SameSite Lax, so no script on the page reads it
    And it is Secure only when the request came over https
    And the app's manifest, service worker and icons are served with no token
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go asserts at least 300 days, internal/web/server.go setUICookie sets 365; cmd/agentry/https_test.go; internal/web/api_test.go

  @SEC-53 @test
  Scenario: A sign-in code is 8 characters and signs another device in
    Given a signed-in device asks for a code
    When the code is shown and another device types it, in any case, with or without spaces and dashes
    Then the code has exactly 8 characters from a 31-character alphabet with no 0, O, 1, I or L, shown as 4-4
    And the other device gets the page token and the cookie, so every API call from it is allowed
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-54 @test
  Scenario: A sign-in code works once and for 5 minutes
    Given a sign-in code that was issued 300 seconds ago, or one already used
    When a device types it
    Then the answer is HTTP 401
    And the code is kept in memory only, never in the log
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-55 @test
  Scenario: At most 20 codes are live at once
    Given 20 codes have been issued and none used
    When a 21st code is issued
    Then the oldest code no longer works
    And the 20 newest codes still work
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-56 @test
  Scenario: Wrong codes from every address together are limited too
    Given 30 wrong codes have come from every address together within a minute
    When any address sends another code
    Then the answer is HTTP 429
    And wrong tokens do not count toward that cap of 30, so the printed link still signs in
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-57 @test
  Scenario: Guesses sent at once do not all get checked
    Given one address sends 50 wrong codes at the same moment
    When the server counts them
    Then no more than 5 are checked, because each attempt counts as failed before it is checked
    And the lock-out ends 60 seconds after the oldest counted failure, when the next try is checked, not refused
    And a successful sign-in is taken back out of the count, so a right code after 4 wrong ones leaves 4
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-58 @test
  Scenario: Signing out clears this device's cookie
    Given a signed-in device
    When the user signs out
    Then the cookie is cleared with a MaxAge of -1
    And the page keeps no stored token
    And the server's page token is not rotated, so other devices stay signed in
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go

  @SEC-59 @test
  Scenario: A feature's secret reaches its job and never the log
    Given features.toml beside the policy holds a feature's table with a secret under its secrets table
    When the feature's job runs and reads the secret
    Then the job gets the secret's value exactly as written
    And the secret is not in the server's output
    And the secret is not in agentry.db, its WAL or the snapshot
    And the page and the API never show it
    And no agent may read features.toml, whatever the policy says
    # README.md, Features' settings and secrets; cmd/agentry/jobs_test.go; internal/policy/default_test.go

  @SEC-60 @test
  Scenario: A broken settings file is refused without quoting it
    Given a working features.toml in force
    When the file is saved with a mistake on one line
    Then the file is refused and the version before stays in force, with no setting changed
    And the error names the file and the line, never what the line holds
    And a table header that is not a feature name is not named in the error
    And each feature reads only its own table
    # README.md, Features' settings and secrets; internal/settingsfile/settingsfile_test.go

  @SEC-61 @test
  Scenario: A secret that reached the log can be redacted without being kept
    Given an agent's knowledge in the log holds a secret
    When the user redacts it
    Then every row holding it is rewritten to "[redacted]"
    And the log records which rows changed, never the secret nor its hash
    And a turn that was running keeps the secret out of what it writes back, from memory only
    And the held secrets are gone when the server restarts, kept nowhere on disk
    # README.md, Archiving, erasing and redacting; internal/store/redact_test.go; internal/hooks/redactions_test.go

  @SEC-62 @test
  Scenario: Agentry's own CA signs a server certificate that is renewed in time
    Given agentry serve runs with -https
    When the CA and the server certificate are made
    Then the CA lasts exactly 10 years and is name-constrained to this machine's names, loopback and the private ranges
    And the server certificate lasts 397 days and never past the CA's end
    And the server certificate is kept only in memory
    And it is renewed 30 days before its end, or when the machine's addresses change
    # README.md, What the CA can sign; internal/tlsca/ca_test.go; internal/tlsca/issue_test.go

  @SEC-63 @test
  Scenario: The CA's key never leaves the server
    Given agentry serve runs with -https
    When any device asks for the trust page or the CA certificate, over http or https, with no token, and then for any path that could reach the CA's key
    Then the trust page and the certificate are shown
    And the key is never served
    And the key's file mode is 0600 in a 0700 folder
    # README.md, Trust a device once; cmd/agentry/https_test.go; internal/tlsca/ca_test.go

  @SEC-64 @test
  Scenario: Plain HTTP from another device is sent to https
    Given agentry serve runs with -https
    When a device other than this machine opens plain http, and this machine opens plain http on loopback
    Then the other device gets a 308 redirect to https
    And this machine gets the page as it is, with no redirect
    # README.md, HTTPS and installing the app on other devices; cmd/agentry/https_test.go

  @SEC-65 @manual
  Scenario: The gate hook's timeout is 1800 seconds in the engine's settings
    Given Agentry prepares an engine's hook settings for a task
    When the user opens the written settings file and finds the PreToolUse hook
    Then its timeout is exactly 1800 seconds
    # PLAN.md T-008; internal/engines/engine.go GateTimeoutSeconds, written by internal/engines/hook_config.go gateHook; no test asserts the number

  @SEC-66 @manual
  Scenario: The hook's deadline is 10 seconds shorter than the engine's timeout
    Given a turn's environment prepared for an agent
    When the user reads the hook deadline variable in that environment
    Then its value is "29m50s", exactly 1790 seconds
    # PLAN.md T-009; internal/turns/turns.go environment, GateTimeoutSeconds minus 10; no test asserts the number
