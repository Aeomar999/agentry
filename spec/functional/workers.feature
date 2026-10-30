Feature: Workers
  A machine joined to Agentry so that agents can run on it.
  # Sources: cmd/agentry/worker.go, cmd/agentry/worker_manage.go,
  # internal/domain/workers, internal/domain/workers/workerlink, internal/domain/workers/workerservice,
  # internal/runtime/machines, internal/runtime/turns, internal/platform/pinned, internal/web/workers_api.go
  # and their tests.

  # ---- Joining ---------------------------------------------------------------

  @MACHINE-01 @worker @test
  Scenario: A machine joins once, with a code, and gets its own credential
    Given an install command made on the Workers page, with its join code
    When the machine runs the command and joins
    Then the machine gets a credential of its own, and the log holds it as given: no
    # not under test, checked by hand: the log keeps the credential's hash (internal/domain/workers/workers.go line 180)
    And the same code is refused for a second machine
    # README.md, Add machines; internal/domain/workers/workers_test.go TestAMachineJoinsOnceWithItsCode

  @MACHINE-02 @worker @test
  Scenario: Add worker makes a one-time install command for a machine
    Given the user is signed in on the Workers page
    When they ask to add a worker
    Then the page gives a code, and an install command that fetches the install script with that code: yes
    # not under test, checked by hand: the code works for 1 hour (internal/domain/workers/workers.go line 28 InviteLifetime; the tests use the constant)
    And 1 install command per address the server has, each fetching the install script with that code
    And the install script fetched with the code carries the code: yes
    And the script fetched with a wrong code is refused with status 401
    And the agentry program for the machine's system is served with the code: yes
    # internal/web/workers_test.go TestAWorkerJoinsAndTakesWork; the commands per address: internal/web/addresses_test.go TestInstallCommands

  @MACHINE-03 @worker @test
  Scenario: A machine joins with the code and is named after its host
    Given an install command whose code has not been used
    When the machine sends the code with what it knows about itself
    Then it gets a credential, and its name is its host's name before the first dot, in lowercase: yes
    # not under test, checked by hand: other characters become dashes (internal/web/workers_api.go line 198 workerName)
    # internal/web/workers_api.go workerName: at most 28 characters, starting with a letter; freeName adds -2, -3 and so on to a name taken
    And the same code sent again is refused, saying the code is unknown, used or expired: yes
    # not under test, checked by hand: the refusal's status is 401 (internal/web/workers_api.go line 182)
    And the Workers page lists the worker, online once it connects: yes
    # internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  @MACHINE-04 @worker @test
  Scenario: A join code works once, for its lifetime, and its join keeps no secret as given
    Given a join code made now
    When a machine joins with it
    Then the code lets a 2nd machine join: no
    And a wrong code lets a machine join: no
    # internal/domain/workers/workers.go InviteLifetime: the code expires 1 hour after it was made
    And the code 1 second after its lifetime lets a machine join: no
    # not under test, checked by hand: the lifetime is 1 hour (internal/domain/workers/workers.go line 28; the test uses the constant)
    And the join's entry in the log holds the code or the credential as given: no
    # not under test, checked by hand: the log keeps their hashes (internal/domain/workers/workers.go line 180); the invite's entry keeps only the code's hash (internal/domain/workers/workers.go line 162)
    And the credential's hash lets a worker in: no
    When the worker is removed
    Then its credential lets it in: no, and the roster has 0 workers
    # internal/domain/workers/workers_test.go TestAMachineJoinsOnceWithItsCode

  @MACHINE-05 @worker @test
  Scenario: The rules on who may join and where agents run
    Given a joined worker and an agent whose Runs on names it and the machine Agentry runs on
    When a machine joins with a code nobody made
    Then it is refused: yes
    When a machine joins under the name local, the name of the machine Agentry runs on
    Then it is refused: yes
    When an agent is given a worker that never joined under Runs on
    Then it is refused: yes
    # not under test, checked by hand: the refusal names the workers that have joined (internal/core/rules/worker_rules.go line 53)
    When the user removes a worker an agent runs on
    Then it is refused: yes
    # not under test, checked by hand: the refusal names the agent and says to take the worker off its workers first (internal/core/rules/worker_rules.go line 102)
    When the agent's Runs on is cleared and the worker removed
    Then it is removed: yes
    # internal/core/rules/worker_rules_test.go TestWorkerRules

  @MACHINE-06 @worker @test
  Scenario: Joining over https checks the server's CA before the code is sent
    Given a server served over https with Agentry's own CA
    When the machine runs `agentry worker join` with the CA's fingerprint, the server and a code
    Then the join is sent 1 time, and the worker keeps the server's https address, its CA and its credential
    And a fingerprint written in lower case without colons is taken: yes
    When the machine runs it with another CA's fingerprint
    Then the join is sent 0 times, nothing is saved, and the error says the server isn't trusted
    When the machine runs it over https with no fingerprint
    Then the join is sent 0 times, and the error names the fingerprint to give, the -ca-fingerprint flag and `agentry ca`
    Given a server over plain http
    When the machine joins with a fingerprint
    Then the join is sent 0 times, as there is no Agentry CA to check
    When the machine joins without one
    Then it joins with 0 CA kept: yes
    # cmd/agentry/worker_test.go TestJoiningOverHTTPSWithTheRightFingerprintKeepsTheCA, TestAWrongFingerprintIsRefusedBeforeTheCodeIsSent,
    # TestHTTPSWithoutAFingerprintIsRefusedWithIt, TestAServerOverPlainHTTP

  @MACHINE-07 @worker @test
  Scenario: The install command run again on a joined machine spends no code
    Given a machine that has joined the server, whose credential the server still takes
    When the install command is run on it again
    Then the join is sent 0 times, and the output says the machine has joined already
    Given a machine that joined over plain http, and a server now served over https
    When the install command with the CA's fingerprint is run on it
    Then the worker moves to the https address and keeps the CA, with 0 joins sent
    Given a worker whose server regenerated its CA
    When the install command is run without the new fingerprint
    Then it is refused, saying the CA was regenerated and to join again with -ca-fingerprint, with 0 joins sent
    When it is run with the new fingerprint
    Then the worker keeps the new CA, with 0 joins sent
    # not under test, checked by hand: in both moves the worker keeps its credential (cmd/agentry/worker.go line 153); the tests check the saved server and CA only
    # cmd/agentry/worker_test.go TestJoiningAgainWithTheFingerprintMovesAWorkerToHTTPS, TestJoiningAgainAfterTheCAWasRegenerated

  @MACHINE-08 @worker @manual
  Scenario: The install script installs agentry, joins and starts the worker
    Given a machine that reaches the server
    When the user runs the install command there
    Then agentry is downloaded with the code for the machine's system, 1 program, and made executable
    # internal/web/install.sh: AGENTRY_BIN or ~/.local/bin; internal/web/install.ps1: AGENTRY_BIN or %LOCALAPPDATA%\Agentry\bin
    And it is put in the user's bin folder and added to the PATH of new shells: yes
    And a note names each of git and claude that is missing, up to 2 notes
    And the machine joins with the code, 1 join, then the worker is started in the background
    And a worker started by hand earlier is stopped first: yes
    # 1. On a machine with git and Claude Code signed in, run the sh command from the Workers page (PowerShell on Windows).
    # 2. Check agentry is in ~/.local/bin (or AGENTRY_BIN; %LOCALAPPDATA%\Agentry\bin on Windows) and on the PATH of a new shell.
    # 3. Check the output says the machine joined and the service started, and the Workers page shows it online.
    # 4. Remove git from the PATH and run the command on a fresh machine: a note says agents need it.
    # 5. Run the same command a second time on the machine: it updates agentry and restarts, spending no code.

  # ---- The background service ------------------------------------------------

  @MACHINE-09 @worker @manual
  Scenario: On Linux the worker is a systemd user service
    Given a Linux machine with a systemd user session
    When the user runs `agentry worker start`
    # internal/domain/workers/workerservice/service.go ForThisMachine; systemd.go
    Then the service is a systemd user service called agentry-worker.service, 1 unit file in the user's systemd folder
    And it runs `agentry worker run` with the PATH of the shell that started it, its output appended to 1 log file, worker.log
    # internal/domain/workers/workerservice/systemd.go: Restart=always, RestartSec=5
    And it is restarted 5 seconds after it stops, every time
    And lingering is turned on for the user where the machine allows, so it runs logged out: yes
    # 1. On Linux with a systemd user session, run `agentry worker start`.
    # 2. Check ~/.config/systemd/user/agentry-worker.service exists, and `systemctl --user is-active agentry-worker.service` says active.
    # 3. Kill the worker process: systemd starts it again within 5 seconds.
    # 4. Log out and in: it runs, as `loginctl enable-linger` was run for the user where allowed.
    # 5. Run `agentry worker leave`: the unit is disabled and the file removed.

  @MACHINE-10 @worker @manual
  Scenario: On macOS the worker is a launchd agent, or a background process over SSH
    Given a Mac
    When the user runs `agentry worker start` in Terminal on the Mac
    # internal/domain/workers/workerservice/launchd.go
    Then the service is a launchd agent labelled dev.agentry.worker, 1 plist in the user's LaunchAgents, with RunAtLoad and KeepAlive
    When the user runs it over SSH, where launchd takes no agent
    Then it runs as a background process instead, and the output says to run `agentry worker start` in Terminal on the Mac 1 time to have it start at login
    # 1. On a Mac, in Terminal, run `agentry worker start`.
    # 2. Check ~/Library/LaunchAgents/dev.agentry.worker.plist exists and `launchctl print gui/<uid>/dev.agentry.worker` says running.
    # 3. Kill the worker process: launchd starts it again (KeepAlive).
    # 4. Over SSH on the same Mac, run `agentry worker start`: it says macOS sets up login services only from the Mac itself, and runs as a background process.

  @MACHINE-11 @worker @test
  Scenario: Where the first service cannot start, the second one runs
    Given a machine whose first service cannot start, with a background process as the second
    When the worker is started
    Then the background process runs, and the service's kind names it: yes
    When the first service can start, and the worker is started again
    Then the first runs and the background process is stopped, 1 service running
    # internal/domain/workers/workerservice/fallback_test.go TestFallbackStartsTheSecondWhenTheFirstCant

  @MACHINE-12 @worker @manual
  Scenario: On Windows the worker is a background process started at logon by Task Scheduler
    Given a Windows machine
    When the user runs `agentry worker start`
    # internal/domain/workers/workerservice/logintask.go
    Then the worker runs as a background process, its process id in worker.pid, 1 file
    And 1 Task Scheduler task called Agentry worker runs `agentry worker start` at logon, with limited rights
    And when the task cannot be made, the worker still runs now: yes
    # 1. On Windows, run `agentry worker start` in PowerShell.
    # 2. Check `schtasks /Query /TN "Agentry worker"` lists a task that runs at logon.
    # 3. Check the worker.pid file in the worker's folder names a running process.
    # 4. Sign out and in: the task runs `agentry worker start` again.
    # 5. Run `agentry worker leave`: the task is deleted and the process stopped.

  @MACHINE-13 @worker @manual
  Scenario: With neither systemd nor launchd the worker is a plain background process
    Given a machine with neither systemd nor launchd
    When the user runs `agentry worker start`
    # internal/domain/workers/workerservice/background.go
    Then the worker runs detached from the shell, its process id in worker.pid, and is started again after a reboot: no
    # internal/domain/workers/workerservice/background.go Stop: 50 checks 100 milliseconds apart
    When the user runs `agentry worker stop`
    Then it is told to stop and waited for up to 5 seconds, after which the stop fails saying so
    # 1. In a container with neither systemd nor launchd, run `agentry worker start`.
    # 2. Check `agentry worker status` names the service "background process (not started again after a reboot)".
    # 3. Run `agentry worker stop` and time it: it returns once the process is gone, or says it didn't stop within 5 s.

  # ---- The worker commands ---------------------------------------------------

  @MACHINE-14 @worker @manual
  Scenario: agentry worker start starts the service and waits for the link
    Given a machine that has joined
    When the user runs `agentry worker start`
    Then the service is written again and started, or restarted if it ran, 1 service
    # cmd/agentry/worker_manage.go runStart: 15 seconds, checked every 300 milliseconds
    And within 15 seconds the output says it connected to the server, or why it can't reach it yet
    And after 15 seconds with no word from the worker, it says so and points at `agentry worker logs`
    # cmd/agentry/worker.go runWorker: -dir defaults to ~/.agentry-worker
    And every worker command takes 1 flag -dir, the worker's folder, which is ~/.agentry-worker by default
    # 1. On a joined machine, run `agentry worker start`.
    # 2. Check the output: "Started <name> as a <service kind>." then "Connected to <server>. Agents can run here now."
    # 3. Stop the server and run `agentry worker start` again: within 15 seconds it says it can't reach the server yet and points at the logs.
    # 4. Run `agentry worker start -dir <other folder>` on a machine that never joined: it says to run the command from the Workers page.

  @MACHINE-15 @worker @manual
  Scenario: agentry worker status says everything about the worker here
    Given a running worker connected to its server
    When the user runs `agentry worker status`
    Then the first line names the worker and its server, and 6 or 7 rows follow
    And the service row names the service's kind and says running: yes or no, and the link row says connected since when, or why not
    # cmd/agentry/worker_manage.go updateNote: the server is asked within 5 seconds
    And the agentry row gives its version and whether the server has a newer one, asked within 5 seconds
    And the engines row lists the engines found, the missing row the tools agents need that are absent, and the running row the tasks with a live agent here, none when 0
    # 1. On a joined machine, run `agentry worker status`.
    # 2. Check the rows: service, link, agentry, engines, missing (only when a tool is missing), running, folder.
    # 3. Put a newer agentry on the server: the agentry row says the server has a newer one, within 5 seconds.

  @MACHINE-16 @worker @manual
  Scenario: agentry worker logs shows the worker's log
    Given a worker with a log
    When the user runs `agentry worker logs`
    # cmd/agentry/worker_manage.go runLogs: tail -n 100
    Then the last 100 lines of worker.log in the worker's folder are printed
    When the user runs it with -f
    Then the log is followed as it grows: yes
    # 1. Run `agentry worker logs` and count the lines: at most 100, from ~/.agentry-worker/worker.log.
    # 2. Run `agentry worker logs -f` and restart the worker: new lines appear as they are written.

  @MACHINE-17 @worker @manual
  Scenario: agentry worker stop and restart
    Given a running worker with 1 agent turn in progress
    When the user runs `agentry worker stop`
    Then the service stops, the link is recorded as stopped, and the agent's turn is killed: no
    And the output says agents running here keep going until their turn ends: yes
    When the user runs `agentry worker stop` again
    Then it says it isn't running, and exits 0
    When the user runs `agentry worker restart`
    Then the service is stopped and started, 1 restart
    # 1. With an agent's turn running on the worker, run `agentry worker stop`.
    # 2. Check the output says agents running here keep going until their turn ends, and the turn's process is still alive.
    # 3. Run `agentry worker stop` again: "It isn't running."
    # 4. Run `agentry worker restart`: the service stops and starts, and the link comes back.

  @MACHINE-18 @worker @test
  Scenario: The worker updates itself to the server's agentry
    Given a worker running an older agentry than the server has for its system
    When it asks the server for agentry for its system, saying the hash of its own
    Then the worker's program is replaced by the server's: yes
    # not under test, checked by hand: the worker writes it beside itself as .new, checks its hash against the server's ETag and swaps it in (internal/domain/workers/workerlink/update.go lines 82 to 98); the test's server is a stand-in
    And the new program can be run: yes
    Given a worker running the same agentry as the server
    When it asks again
    Then it is replaced: no
    # not under test, checked by hand: the server answers 304 Not Modified (internal/domain/workers/workerlink/update.go line 75)
    # internal/domain/workers/workerlink/worker.go Run: with SelfUpdate the check runs before every dial, and an update restarts the process
    # cmd/agentry/worker_manage.go runUpdate: `agentry worker update` does the same, with 5 minutes to fetch, and restarts the service if it runs
    # internal/domain/workers/workerlink/update.go swap: on Windows the old program is moved aside as .old and removed by the next update
    # internal/domain/workers/workerlink/update_test.go TestAWorkerUpdatesToTheServersAgentry

  @MACHINE-19 @worker @manual
  Scenario: agentry worker leave takes the machine off the server
    Given a machine that has joined
    When the user runs `agentry worker leave`
    Then the server records the worker gone and its credential stops working, 1 worker fewer
    And the service is stopped and removed, and worker.json and link.json are deleted, 2 files
    And its repos and worktrees are deleted: no
    When the user runs it with -purge
    Then the repos and worktrees folders are deleted too, 2 folders
    And when the server already removed the worker, leaving still finishes here: yes
    # 1. On a joined machine, run `agentry worker leave`.
    # 2. Check the Workers page no longer lists it, the service is gone, and worker.json and link.json are deleted from the worker's folder.
    # 3. Check the repos and worktrees folders are still there; join again and run `agentry worker leave -purge`: they are deleted.
    # 4. Remove a worker on the page first, then run `agentry worker leave` on it: it still leaves, as the server answers 401.

  @MACHINE-20 @worker @manual
  Scenario: agentry worker run works in the foreground
    Given a machine that has joined, with its service stopped
    When the user runs `agentry worker run`
    Then the worker runs in the foreground until stopped, which is what the service runs, 1 process
    And `agentry worker` with no command does the same: yes
    # 1. On a joined machine with the service stopped, run `agentry worker run` in a terminal.
    # 2. Check the log line "<name>, working for <server>" and that the Workers page shows it online.
    # 3. Press Ctrl-C: it stops.
    # 4. Run plain `agentry worker`: it does the same as run.

  # ---- The link --------------------------------------------------------------

  @MACHINE-21 @worker @test
  Scenario: A worker keeps dialing when its link drops, and says once when the CA changed
    Given a worker whose server shows a certificate its kept CA did not sign
    When the worker runs for 3.5 seconds
    # internal/domain/workers/workerlink/worker.go Run: the wait starts at 1 second, doubles each time, and is at most 30 seconds
    Then it logs link down at least 2 times
    # not under test, checked by hand: it dials at 0, 1 and 3 seconds
    And it says how to join again with a new fingerprint 1 time, not on every dial
    # internal/domain/workers/workerlink/worker.go Run: a link that was up resets the wait to 1 second; a credential the server no longer takes ends the run
    # internal/domain/workers/workerlink/worker_test.go TestAWorkerWhoseCAChangedSaysOnceHowToJoinAgain

  @MACHINE-22 @worker @test
  Scenario: A connected worker says hello and what it runs, and the page reads its details
    Given a joined worker that connects with its credential
    When it says hello with its version, engines and missing tools
    Then the Workers page lists it online: yes
    # not under test, checked by hand: the server records its version, engines and missing tools with it (internal/domain/workers/workerlink/server.go line 80)
    # internal/domain/workers/workerlink/server.go Connect: a link with no hello within 10 seconds is closed
    # internal/domain/workers/workerlink/server.go StatusEvery, silentFor: the worker says what it runs every 2 seconds, and a link silent for 20 seconds is hung up
    And the worker's details page shows what the worker reads about itself over the link: yes
    # internal/web/workers_api.go showWorker: 15 seconds for the details, and the last 20 joined, online and offline entries
    When the worker's link drops
    Then the fleet takes it as gone, unless a newer link replaced it: yes
    # not under test, checked by hand: the server records it offline (internal/domain/workers/workerlink/server.go line 88)
    # internal/web/workers_test.go TestAWorkerJoinsAndTakesWork; the link dropping: internal/runtime/machines/fleet_test.go TestTheFleetPicksWhereAnAgentWorks

  @MACHINE-23 @worker @test
  Scenario: Over https a worker trusts only Agentry's own CA
    Given a server served over https with Agentry's own CA, and a worker that kept that CA
    When the worker reaches the server
    Then the connection is made: yes
    When another CA's certificate is shown, even one the system trusts
    Then the connection is refused: yes
    When the kept CA cannot be read
    Then even a server that CA vouched for is refused: yes
    # not under test, checked by hand: the system's store is used instead: no (internal/platform/pinned/pinned.go line 23); the test's server is one the system's store refuses too
    And a worker with 0 CA kept uses the plain defaults, as for a server over plain http
    And a certificate issued again by the same CA keeps working: yes
    # internal/platform/pinned/pinned_test.go TestAPinnedClientReachesTheServerItsCAVouchesFor, TestAPinnedClientRefusesAnotherCAEvenOneTheSystemTrusts,
    # TestAPinnedClientRefusesEverythingWhenItsCAIsUnreadable, TestWithNoCAItIsThePlainDefault, TestTheClientKeepsWorkingWhenTheLeafIsIssuedAgain

  @MACHINE-24 @worker @manual
  Scenario: A machine advertises its system, its tools and its disk
    Given a worker connected
    When its details are read
    # internal/runtime/machines/about.go AboutHere
    Then they give its host, system as its maker names it, kernel, arch, CPU, CPUs, memory in all and free, 3 load averages, boot time, time and zone
    And the disk in all and free where its worktrees are, 2 numbers
    # internal/runtime/machines/about.go toolsKept
    And each engine and git, with version and path, read again only after 5 minutes
    # internal/runtime/machines/about.go output: each program asked for its version gets 3 seconds
    And agentry's version, its path, its folder, the PATH agents get, the tasks running, the worktrees and the copies of repos there, 7 more facts
    # 1. Open a worker on the Workers page and read its details.
    # 2. Check the system, kernel, arch, CPU, memory, load, boot time, disk and time with the machine itself.
    # 3. Install or remove a tool on the worker and read again: the tools change only after 5 minutes.

  @MACHINE-25 @worker @test
  Scenario: Without a credential the server takes, a worker reaches nothing
    Given a worker the user removed on the Workers page
    When it dials the link with its old credential
    Then it is refused with status 401: yes
    # not under test, checked by hand: the refusal says the worker isn't joined (internal/web/workers_api.go line 230), and its open link is dropped at once (line 149)
    # internal/web/workers_api.go requireWorker: the link, the git hub, the bundle, the update, leaving and the blobs all need the credential
    # internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  # ---- Where a turn runs -----------------------------------------------------

  @MACHINE-26 @worker @test
  Scenario: A turn runs where the work is, or on the first online machine the agent may use
    Given 2 workers online, 1 with the agent's engine and 1 without, and 1 worker offline
    When an agent with 0 workers under Runs on is placed
    Then it runs on the machine Agentry runs on: yes
    When an agent may run on the offline worker, the online one with its engine, and the machine Agentry runs on, in that order
    Then it runs on the online worker with its engine, the first of the 3 that can
    When the task's work is already on a machine the agent may use
    Then it stays there: yes
    When the agent may run only on a worker without its engine, or only on an offline one
    Then it is placed: no, and the reason names each machine, such as offline or has no engine
    # internal/runtime/machines/fleet_test.go TestTheFleetPicksWhereAnAgentWorks

  @MACHINE-27 @worker @test
  Scenario: A coordinator can send an agent to a named machine
    Given a joined worker, offline, that the tester may run on, and the machine Agentry runs on
    When the coordinator's brief is made
    Then it has a Machines section listing the worker with its host, system and arch, online or offline, and who may run there: yes
    # not under test, checked by hand: the section lists each machine, the one Agentry runs on too
    And it names the machine the task's work is on now: yes
    When the coordinator assigns an agent to a machine it may not run on
    Then the turn starts: no, and the coordinator hears the agent may not run there and where it runs
    When the coordinator names a machine that does not exist
    Then the coordinator hears there is no machine by that name, with the machines listed
    When the coordinator names a machine that is offline, by its host's name
    # internal/runtime/machines/fleet.go WaitOn refuses at once; the 30 second OfflineGrace wait applies only to a worker that went offline within the last 30 seconds or is expected back after a restart, covered by internal/runtime/machines/fleet_test.go TestTheFleetWaitsForAWorkerComingBack
    Then the host's name is taken as that machine, the turn starts: no, and the coordinator hears that it couldn't start where it chose, as the machine is offline
    When the coordinator names a machine the agent may use, in any letter case
    Then the turn runs there, and the trail names that machine: yes
    # internal/runtime/coordinator/machines_test.go TestTheCoordinatorChoosesTheMachine

  @MACHINE-28 @worker @test
  Scenario: An agent whose machines are all offline asks the user
    Given an agent whose Runs on names 1 worker, which is offline
    When the coordinator assigns the agent
    Then the task asks the user: the agent can't run anywhere now, naming the offline worker, after 1 turn
    # not under test, checked by hand: the question says to bring one of its workers online, or change where it runs, and answer to carry on (internal/runtime/coordinator/loop.go line 219)
    # internal/runtime/coordinator/loop_test.go TestAnAgentWhoseWorkersAreOfflineAsksYou

  @MACHINE-29 @worker @test
  Scenario: After a restart, a worker that may be running agents is waited for
    Given Agentry just started, with a worker the log had online before
    When an agent that runs only on that worker is placed
    # internal/runtime/machines/fleet.go OfflineGrace, WaitFor: checked every 250 milliseconds for 30 seconds
    Then the placement waits until the worker dials back in, and runs there: yes
    And a worker never seen is found: no
    # not under test, checked by hand: the wait is at most 30 seconds, and a worker never seen is not waited for (internal/runtime/machines/fleet.go line 147)
    # internal/runtime/machines/fleet_test.go TestTheFleetWaitsForAWorkerComingBack

  @MACHINE-30 @worker @test
  Scenario: A turn runs on the worker, resuming its session, and its terminal shows on the server
    Given a task whose work started on the machine Agentry runs on, and a worker connected
    When the work moves to the worker over git
    Then the page lists the task's files and commits on the worker, including the file committed when the work left: yes
    When a turn starts there going on with the agent's last session
    Then the worker reports the task running within seconds: yes, and the terminal on the server shows the session resuming: yes
    When the worker hands the work back
    Then a worktree made from the hub has what the agent left: yes
    # internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  @MACHINE-31 @worker @test
  Scenario: A worker says which engines it has, and agents are placed by them
    Given a worker that connected saying it has claude at a version
    When an agent on claude is checked against it
    Then the worker has claude: yes
    When an agent on another engine is checked against it
    Then the worker has that engine: no
    # internal/domain/workers/workers_test.go TestAMachineJoinsOnceWithItsCode

  # ---- Going offline ---------------------------------------------------------

  @MACHINE-32 @worker @test
  Scenario: A worker that goes offline is taken to still run its turn for a while
    Given a worker running a task's turn
    When its link drops
    # internal/runtime/machines/fleet.go OfflineGrace
    Then just after, the task counts as running there: yes
    And once its grace has passed, nothing runs there that Agentry can see
    # not under test, checked by hand: the grace is 30 seconds (internal/runtime/machines/fleet.go line 28; the test uses the constant), so the watchdog takes the agent as crashed: no
    And a worker never seen runs anything: no
    And a stale link's end removes a newer link: no
    # internal/runtime/machines/fleet_test.go TestTheFleetPicksWhereAnAgentWorks

  @MACHINE-33 @worker @test
  Scenario: The user hears that a machine went offline with work on it
    Given a worker online with 1 task on it
    When it goes offline
    Then the news for the user says the machine went offline, and the task waits until it is back: yes
    # not under test, checked by hand: it is 1 line, and a worker offline with 0 tasks on it is news: no (internal/mcp/waiting.go line 67)
    # internal/runtime/coordinator/loop.go moveWork: when the work's machine is offline and the next agent runs elsewhere, the task asks the user to bring it online and answer
    # internal/mcp/waiting_test.go TestTheNewsIsWhatIsNew

  @MACHINE-34 @worker @manual
  Scenario: A worker offline longer than its grace stops the task until it is back
    Given an agent's turn running on a worker
    When the worker is offline for more than 30 seconds
    Then its turn is seen as running: no, and the next placement asks the user, 1 question
    And the Workers page shows it offline since when, and the page's history keeps the last 20 joined, online and offline entries
    # 1. Give an agent 1 worker under Runs on and start a task that reaches that agent.
    # 2. While its turn runs on the worker, pull the worker's network.
    # 3. Within 30 seconds the Workers page shows the worker offline; the turn is not marked crashed yet.
    # 4. Plug it back in within 30 seconds: the worker dials back and the turn goes on.
    # 5. Repeat and leave it offline past 30 seconds: the task asks you, and Claude Code and the page say the machine went offline.
