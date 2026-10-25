Feature: Compatibility
  The systems, engines and browsers Agentry runs on and with.

  @COMPAT-01 @manual
  Scenario: The worker builds for six kinds of machine
    Given a checkout of the repo on a machine with Go
    When the user runs make workers
    Then 6 binaries are built in bin/, for darwin, linux and windows, each on arm64 and amd64
    # Makefile, WORKER_PLATFORMS; PLAN.md T-062; README.md, Add machines

  # --- Operating systems: the zips, Start Agentry and the workers ---
  # Agentry is one Go program with no C in it, so every system's build is made on any one machine.
  # No test runs the Makefile, the packaging program or the start scripts, so these are checked by hand.

  @COMPAT-02 @manual
  Scenario: The zips for six kinds of computer are made on any one machine
    Given a checkout of the repo on one machine with Go and git, and the page built with make ui
    When the user runs make package
    Then 6 zips are made in release/, one each for mac-apple-silicon, mac-intel, linux-x64, linux-arm64, windows-x64 and windows-arm64
    And each is named agentry-<version>-<label>.zip, with the version from git describe, or dev when git gives none
    And every build is made with CGO_ENABLED=0, so no C compiler and no cross toolchain is needed, whatever the machine is
    And without the built page the command is refused with "run it from the repo's root, with the page built (make ui)"
    # README.md, From the source (make package); PLAN.md T-062; packaging/main.go lines 25-32, 55-57, 66-67, 77; Makefile lines 17 (WORKER_PLATFORMS) and 20-22; no test

  @COMPAT-03 @manual
  Scenario: Each zip holds this system's agentry, the five others' and Start and Stop Agentry
    Given a zip from make package, for any of the 6 kinds
    When the user unzips it
    Then the folder Agentry holds agentry (agentry.exe in a Windows zip) and the 5 other builds as agentry-<os>-<arch>, each executable
    And the 2 scripts are Start Agentry.command and Stop Agentry.command on a Mac, Start Agentry.cmd and Stop Agentry.cmd on Windows, start-agentry.sh and stop-agentry.sh on Linux
    And README.txt names only that system's start, stop and first-run steps, with Windows line ends in a Windows zip
    And the other builds stay next to agentry: they are what the server hands every machine that joins as a worker
    # README.md, What Start Agentry does (The other programs in the folder); PLAN.md T-062; packaging/main.go lines 98-142; packaging/files/README.txt; no test

  @COMPAT-04 @manual
  Scenario: Start Agentry and Stop Agentry do the same on every system
    Given the Agentry folder from a zip on a Mac, on Windows and on Linux, with git and Claude Code installed
    When the user double-clicks Start Agentry.command or Start Agentry.cmd, or runs ./start-agentry.sh
    Then each runs agentry open from its own folder, and the page opens in the browser signed in
    And on a Mac the quarantine mark is removed from the whole folder first, so every program inside may run
    And on Windows the window waits for a key after the links are printed; on a Mac and on Linux it does not
    And since Agentry is not signed yet, the first start on a Mac needs right-click, Open, then Open again, and on Windows More info, then Run anyway; Linux needs no step beyond chmod +x when the unzip lost permissions
    And Stop Agentry runs agentry stop the same way on each system, and on Windows waits 5 seconds before the window closes
    # README.md, Mac, Windows, Linux, What Start Agentry does; PLAN.md T-062; packaging/files/start-unix.sh, start-windows.cmd, stop-unix.sh, stop-windows.cmd; no test

  @COMPAT-05 @manual
  Scenario: Start Agentry says when git or Claude Code is missing
    Given a machine where git, claude or both are not on the PATH
    When the user runs Start Agentry, or agentry open
    Then for each missing one a line is printed: "Note: <tool> isn't installed here, and agents need it." with where to get it
    And Agentry still starts and opens the page: nothing else is needed, no Docker, no Node, no tmux
    # README.md, What you need; cmd/agentry/open.go lines 47-50, 274-282; no test

  @COMPAT-06 @test
  Scenario: The install command runs in sh on macOS and Linux and in PowerShell on Windows
    Given a server with its own CA, and a join code from the Workers page
    When the sh command is run in sh with curl, and the PowerShell command in pwsh with curl.exe
    Then the last 2 requests the sh command sends carrying the code are the script at /worker/install.sh, then the agentry download
    And the 2 requests the PowerShell command sends carrying the code are the script at /worker/install.ps1, then the build for windows
    And with another CA's fingerprint in the command, 0 requests carrying the code are sent by either
    # not under test, checked by hand: the scripts ask for the machine's own os and arch (internal/web/install.sh line 35, internal/web/install.ps1 line 42); the PowerShell test skips where pwsh or curl.exe is missing
    # README.md, Add machines; PLAN.md T-022; internal/web/install_https_test.go TestThePinnedInstallCommandsRunInSh and TestThePinnedInstallCommandRunsInPowerShell

  @COMPAT-07 @manual
  Scenario: Where the worker's agentry goes on each system
    Given the Workers page open, and a machine to join
    When the user picks macOS / Linux or Windows above the command and runs it on the machine
    Then the picker starts on Windows only when the browser says it runs on Windows, else on macOS / Linux, and the note under the command reads "In a terminal." or "In PowerShell."
    And on macOS and Linux agentry is installed in ~/.local/bin, or AGENTRY_BIN, and that folder is added to the PATH in .zshrc, .bash_profile on a Mac, .bashrc or .profile, once
    And on Windows agentry.exe is installed in %LOCALAPPDATA%\Agentry\bin, or AGENTRY_BIN, and that folder is added to the user's Path once; an agentry.exe already there is moved to agentry.exe.old first, since Windows does not replace a program that runs
    And on every system the script notes git and claude when they are not found, joins with the code, and starts the worker
    # README.md, Add machines; PLAN.md T-022; internal/web/install.sh lines 16, 43-68; internal/web/install.ps1 lines 17, 50-74; ui/src/components/molecules/InstallCommand.tsx lines 16, 26-29, 48-49; no test

  @COMPAT-08 @test
  Scenario: A machine of the server's own kind gets the server's own binary
    Given a server whose binary has no agentry-<os>-<arch> builds beside it
    When the install script asks for /worker/agentry with a working code and the server's own os and arch
    Then the answer is 200 with the server's own binary
    And with an unknown code the install script itself is refused with 401
    # not under test, checked by hand: a used or expired code is refused alike, and the build too needs a code that works (internal/web/workers_api.go lines 262 and 272)
    # README.md, Add machines; PLAN.md T-022; internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  @COMPAT-09 @manual
  Scenario: A machine of another kind gets the build beside the server, or none
    Given a server started from the Agentry folder of a zip, with the 5 other builds beside agentry
    When a machine of another os and arch runs the install command, and then one whose build was removed from the folder
    Then the first gets agentry-<os>-<arch> from beside the server's binary, 200
    And the second gets 404, "this server has no agentry for <os>/<arch>", and the script stops with exit 1 saying so, with nothing installed
    And os and arch are taken only as lower-case letters and digits; anything else gets the same 404
    # README.md, What Start Agentry does (The other programs in the folder); internal/web/workers_api.go lines 284-326; internal/web/install.sh lines 35-39; internal/web/install.ps1 lines 42-47; no test

  @COMPAT-10 @manual
  Scenario: The worker is kept running the way its system does
    Given a machine that has joined, on Linux with systemd, on Linux without it or in a container, on a Mac, and on Windows
    When the install script, or agentry worker start, starts the worker
    Then on Linux with a user systemd it is the systemd user service agentry-worker.service under ~/.config/systemd/user, restarted always, 5 seconds after it stops
    And on a Mac it is the launchd agent dev.agentry.worker under ~/Library/LaunchAgents; from a session with no login, as over SSH, it is a plain background process and the command says to run agentry worker start in Terminal on the Mac once
    And on Windows it is a background process, with exactly 1 Task Scheduler task, "Agentry worker", that starts it again at login
    And where there is neither systemd nor launchd it is a plain background process with a pid file, and agentry worker status names the kind in every case
    # README.md, Add machines; PLAN.md T-022; internal/workerservice/service.go lines 38-53; systemd.go lines 15-36; launchd.go lines 16-22; logintask.go lines 12-22; cmd/agentry/worker_manage.go lines 75-78; no test of the pick per system

  @COMPAT-11 @manual
  Scenario: A worker machine needs git and Claude Code and no open port
    Given a machine that has joined over a network where only outbound connections are allowed
    When the user runs agentry worker status there, and opens the Workers page on the server
    Then the worker connects, since it dials out to the server: no port is opened on the machine
    And status shows the engines found with their versions, as claude <version> or codex <version>, and lists git under missing with "(agents need it)" when it is not on the PATH
    And the Workers page shows the machine's os/arch and the same engines, or "no engines found"
    # README.md, Add machines; internal/machines/describe.go lines 18-33; cmd/agentry/worker_manage.go lines 104-131; ui/src/pages/WorkersPage.tsx lines 39, 48; no test of the output

  # --- Engines: Claude Code and Codex ---

  @COMPAT-12 @test
  Scenario: The rules know the engines there are adapters for, Claude Code and Codex
    Given the engine adapters compiled into agentry and the rules that check an agent's engine
    When the adapters' names and the rules' known engines are compared, and an agent names another engine
    Then the adapters' names and the rules' known engines are the same list, and adapters named claude and codex are found: yes
    And an agent with another engine is refused by the rules: yes
    # not under test, checked by hand: the list is claude and codex alone (internal/rules/agent_rules.go line 15), and the refusal says "unknown engine" (line 92)
    # README.md, What you need (Codex works too); PLAN.md T-008; internal/engines/engines_test.go TestTheRulesKnowExactlyTheEnginesThereAreAdaptersFor; internal/rules/rules_test.go TestAgentRules

  @COMPAT-13 @test
  Scenario: Claude Code is started in never-ask mode with Agentry's own settings file
    Given an agent whose engine is claude, with a model and a prompt, fresh or resuming a session
    When the claude adapter prepares and launches the turn
    Then a resumed session's command is exactly claude --model <model> --resume <session> --settings <worktree>/.agentry/claude-settings.json --permission-mode bypassPermissions <prompt>
    And a fresh session's command holds --model <model> and --permission-mode bypassPermissions, resumes nothing and ends with the prompt: yes
    And the settings file is valid JSON holding the hooks, tui "default", spinnerTipsEnabled false and the env CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY=1
    # README.md, What you need; PLAN.md T-008; internal/engines/engines_test.go TestLaunchIsNeverAskWithThePrompt, TestLaunchResumesTheSession, TestPrepareWritesHooksThatFailClosed and TestClaudeSettingsKeepItUnattended

  @COMPAT-14 @test
  Scenario: Codex is started in never-ask mode with its hooks in the worktree
    Given an agent whose engine is codex, with a model and a prompt, fresh or resuming a session
    When the codex adapter prepares and launches the turn
    Then a resumed session's command is exactly codex resume --model <model> -s workspace-write -a never --dangerously-bypass-hook-trust <session> <prompt>
    And a fresh session's command holds --model <model> and -a never, resumes nothing and ends with the prompt: yes
    And the hooks are written to <worktree>/.codex/hooks.json as valid JSON, with the gate hook ending in "|| exit 2" and the report hook in "|| true"
    And a Codex turn's tokens are not counted, with CodexTokensNotRead as the reason: yes
    # not under test, checked by hand: the reason reads "Codex's token use isn't read" (internal/engines/codex.go line 69); the test compares with the constant itself
    # README.md, What you need (Codex works too); PLAN.md T-008, T-028 (registering hook trust is not built); internal/engines/engines_test.go TestLaunchIsNeverAskWithThePrompt, TestLaunchResumesTheSession and TestPrepareWritesHooksThatFailClosed; internal/engines/tokens_test.go TestCodexTokensStayUnread

  @COMPAT-15 @test
  Scenario: Both engines give Agentry's signals from their hooks
    Given 12 hook payloads recorded from Claude Code 2.1.301 and Codex 0.163.0: 7 from Claude Code (session start, prompt, 2 tool calls, tool done, notification and stop) and 5 from Codex (session start, prompt, tool call, permission request and stop)
    When each is parsed by its engine's adapter
    Then each payload gives exactly the signal expected of it: 6 kinds from Claude Code (session started, prompt received, action requested, action finished, waiting, turn ended) and 4 from Codex (session started, prompt received, action requested, turn ended)
    And in both engines' configurations PreToolUse is a gate whose command ends in "|| exit 2", and Stop a report whose command ends in "|| true"
    # PLAN.md T-008, proof 07; internal/engines/engines_test.go TestParseHookGivesTheSameSignalsOnBothEngines and TestPrepareWritesHooksThatFailClosed

  @COMPAT-16 @test
  Scenario: Claude Code's trust question is answered before the session, in both path forms on Windows
    Given Claude Code's config file in CLAUDE_CONFIG_DIR, with another project and a setting in it
    When the claude adapter prepares a turn in a new worktree
    Then the worktree is marked hasTrustDialogAccepted in the config: yes
    And the other project and the setting are kept, and the file's mode stays 0600
    And on Windows a folder has 2 keys, the second with forward slashes, since Claude Code writes either; elsewhere exactly 1
    # not under test, checked by hand: with no CLAUDE_CONFIG_DIR the file is ~/.claude.json (internal/engines/claude_trust.go line 23); the folder is marked under each key (line 48), so the session never stops at "Do you trust this folder?"; CI runs the Unix half of the key test only
    # PLAN.md T-008; internal/engines/engines_test.go TestPrepareTrustsTheWorktree; internal/engines/claude_test.go TestAWindowsFolderIsTrustedBothWays

  @COMPAT-17 @manual
  Scenario: Engine versions are read and shown, and no minimum is required
    Given a worker machine with claude and codex on the PATH, or only one of them
    When agentry worker status runs, or the machine describes itself to the server
    Then each engine found is listed as its name and the first word of its --version output that starts with a digit, asked with a 10 second limit
    And an engine that is not on the PATH is left out; one that is there but prints no version is listed by name only
    And no version is compared against a minimum: Agentry has no lowest supported version of Claude Code or Codex, and starts any it finds
    # README.md, Add machines (agentry worker status); internal/machines/describe.go lines 18-61; no test

  @COMPAT-18 @test
  Scenario: A session started from inside Claude Code or Codex drops only the parent's markers
    Given agentry open run from a Claude Code or Codex session, so its environment carries that session's markers
    When Agentry starts and drops them
    Then the markers that tie a process to a running session are removed: CLAUDECODE, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_MESSAGING_TOKEN and CODEX_SANDBOX: yes
    And the user's own engine configuration is kept: CLAUDE_CONFIG_DIR, CLAUDE_CODE_USE_BEDROCK, CODEX_HOME and ANTHROPIC_API_KEY: yes
    And a second pass removes 0 more
    # not under test, checked by hand: the other listed names and every CLAUDE_CODE_MESSAGING_ name go alike (internal/engines/inherited.go line 45), and an agent's session is started without them (internal/sessions/host.go line 335)
    # README.md, From the source (agentry open); internal/engines/inherited_test.go TestOnlyParentSessionMarkersAreDropped; cmd/agentry/serve_test.go TestStartingDropsAParentSessionAndSaysWhat

  @COMPAT-19 @manual
  Scenario: On Windows an engine installed as a .cmd launcher runs through cmd.exe
    Given a Windows worker where claude is on the PATH as claude.cmd, as npm installs it
    When an agent's turn starts there
    Then the engine is found by its full path on the session's PATH, never by name alone, and a .cmd or .bat launcher is started as cmd.exe /c <launcher> with the engine's arguments after it
    And on every other system the program found is started as it is, with no shell in between
    # internal/sessions/host.go lines 295-314; no test

  # --- Browsers that install the app ---
  # The code names no browser it supports. It names two that refuse a service worker on any page, and the README's trust steps name Safari on iPhone and iPad, Chrome on Linux and Firefox.

  @COMPAT-20 @test
  Scenario: The app installs only from a secure page
    Given a page that is not a secure context, as over plain http at a network address, and one that is, as at http://localhost or https with a certificate the device trusts
    When the page registers its service worker and waits for the browser's install offer
    Then on the plain http page the install state is "insecure", whatever the service worker did: yes
    And on the secure page with the worker registered and no offer yet the state is "unavailable"
    # not under test, checked by hand: the insecure page is not offered the install, and the states are installed, ready, insecure, refused and unavailable (ui/src/lib/install.ts line 17)
    # README.md, What Start Agentry does (As an app); packaging/files/README.txt lines 16-19; ui/src/lib/install.test.ts "is insecure on a plain http page, whatever the service worker did" and "is unavailable on a secure page with a service worker and no offer yet"

  @COMPAT-21 @test
  Scenario: A browser that refuses the service worker gets no install offer
    Given a secure page whose service worker registration the browser rejects, as with a certificate the device does not trust, or in Firefox's private window or Brave, which refuse one on any page
    When the registration fails
    Then the install state is "refused", not "unavailable", so the page can say why there is no offer
    # README.md, HTTPS and installing the app on other devices; ui/src/lib/install.ts lines 5-9, 29-34; ui/src/lib/install.test.ts "is refused on a secure page whose service worker the browser refused"

  @COMPAT-22 @manual
  Scenario: The Install app button offers, explains or hides itself by the state
    Given the page open in a browser, in each of the 5 install states
    When the user looks at the top bar and presses Install app where it shows
    Then with an offer ready the button shows the browser's own install prompt, once per offer; the browser makes a new one if it is dismissed
    And on an insecure page the button opens a note that browsers install apps only from secure pages: open Agentry at http://localhost:<port> on the computer it runs on, or over https; with a server CA the note gives the https address and its trust page too
    And when the service worker was refused, the note points to the trust page when the server has one, and the button is hidden when it has none
    And once installed, or with no offer and no refusal, the button is hidden; the installed app opens standalone, in its own window, and shows the sign-in page when it has no token yet
    # README.md, What Start Agentry does (As an app), Signing in the installed app, or another device; ui/src/lib/install.ts lines 19-21, 41-49, 66-73; ui/src/components/molecules/InstallAppButton.tsx lines 35-81; ui/public/manifest.webmanifest (display standalone); no test

  # --- Go and Node ---

  @COMPAT-23 @manual
  Scenario: Building and running the server needs Go 1.27 and git, and no Node
    Given a checkout of dev with the built page committed in internal/web/dist
    When the user reads go.mod, the Makefile, .gitlab-ci.yml and the Dockerfile, and runs go build -o bin/agentry ./cmd/agentry
    Then go.mod says go 1.27.2 and the README says Go 1.27 and git; the build needs no Node, since the page is in the binary
    And CI checks in golang:1.27-bookworm with go vet ./... and go test ./..., and is refused before the tests when internal/web/dist/index.html is missing, with "The UI is not built. Run make ui and commit internal/web/dist."
    And the image is built in golang:1.27-bookworm with CGO_ENABLED=0 and runs on node:24-bookworm-slim only because Claude Code 2.1.301 is installed in it with npm, its auto-updater off
    And Node is needed only for make ui, which runs npm ci and npm run build in ui
    # README.md, From the source, Deploy it; PLAN.md T-060; go.mod line 3; Makefile lines 1-3, 10; .gitlab-ci.yml lines 21, 29-31; Dockerfile lines 5, 11, 13, 16-17; no test

  @COMPAT-24 @benchmark
  Scenario: The UI's toolchain is pinned to exact versions, with no Node version pinned
    Given ui/package.json on the branch
    When the user reads its dependencies and looks for an engines field, a .nvmrc or a .tool-versions at the root and in ui
    Then every one of the 26 dependencies is an exact version: react 19.3.1, react-dom 19.3.1, typescript 5.9.3, vite 8.3.4, vitest 5.0.5, tailwindcss 4.3.3, @xterm/xterm 5.5.0 and monaco-editor 0.52.2 among them
    And 0 of them carries a range marker such as ^ or ~
    And there is no engines field and no .nvmrc or .tool-versions: the Node version is not pinned for the UI build
    And npm run lint is exactly tsc --noEmit and prettier --check src, and make test runs it after go vet and go test
    # README.md, Work on it; measured 2026-10-25 at commit 5ecec34a with grep -c of a version starting in ^ or ~ in ui/package.json (0 of 26 version lines), grep "engines" in it (none) and ls -a of the root and ui (no .nvmrc, no .tool-versions); ui/package.json; Makefile lines 25-26

  # --- HTTPS across devices ---
  # The certificate's lifetimes, names and the key's modes are security.feature's; here only what each system installs and checks.

  @COMPAT-25 @test
  Scenario: The CA is handed out in the form each kind of device installs
    Given agentry serve runs with -https
    When any device, over plain http or https and with no token, asks for /trust/ca.crt, /trust/ca.pem and /trust/agentry.mobileconfig
    Then ca.crt is the certificate in DER, and ca.pem the same certificate in PEM, both 200
    And agentry.mobileconfig is an iOS configuration profile, content type application/x-apple-aspen-config, with exactly 1 payload holding the CA, PayloadType com.apple.security.root, the same bytes when downloaded again, and identifiers another CA's profile never shares
    # not under test, checked by hand: the README's Android, macOS and Windows steps install ca.crt and its Linux step ca.pem, and it gives the trust steps per device: Android, iPhone and iPad in Safari, macOS, Windows, Linux, and Firefox on every system (README.md, Trust a device once)
    # README.md, Trust a device once; cmd/agentry/https_test.go TestTheCACertificateIsServedToAnyone; cmd/agentry/trust_test.go TestTheIOSProfile

  @COMPAT-26 @test
  Scenario: The CA key's permissions are checked on macOS and Linux, and a linked key is refused
    Given the CA folder tls in Agentry's data folder
    When Agentry opens the CA on macOS or Linux
    Then a key readable by others is refused with "chmod 600", and a folder open to others is set back to 0700
    And a key that is a symbolic link is refused: yes
    # not under test, checked by hand: the link is found with O_NOFOLLOW on macOS and Linux (internal/tlsca/key_unix.go line 14) and with Lstat on Windows, which has no O_NOFOLLOW (internal/tlsca/key_windows.go line 13); on Windows, which keeps no Unix permissions, the code makes no permission check (internal/tlsca/ca.go lines 120 and 332) and the 2 permission tests skip themselves
    # README.md, What the CA can sign (0600 on macOS and Linux); internal/tlsca/ca_test.go TestAKeyOthersMayReadIsRefused, TestAFolderOpenToOthersIsClosed and TestAKeyThatIsALinkIsRefused

  @COMPAT-27 @manual
  Scenario: A worker on Windows is updated by moving the old agentry aside
    Given a Windows worker whose agentry.exe is older than the server's, with the worker running
    When agentry worker update runs, or the install command is run again
    Then the server's build for windows and the machine's arch is downloaded whole, checked against its SHA-256 ETag, and refused with "the new agentry didn't arrive whole" when the bytes do not match
    And since Windows does not replace a program that runs, the old agentry.exe is renamed agentry.exe.old first and removed at the next update; on macOS and Linux the new file is renamed over the old one in 1 step
    # README.md, Add machines (run again, it updates it); internal/workerlink/update.go lines 57, 85-93, 104-120; internal/web/install.ps1 lines 48-55; no test of the Windows branch
