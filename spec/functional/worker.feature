Feature: Workers
  A machine joined to Agentry so that agents can run on it.
  # Sources: cmd/agentry/worker.go, cmd/agentry/worker_manage.go,
  # internal/workers, internal/workerlink, internal/workerservice,
  # internal/machines, internal/turns, internal/worktrees/handoff.go,
  # internal/sharedfiles, internal/pinned, internal/web/workers_api.go and
  # their tests. WORKER-01 is the foundation's stub; the scenarios here start
  # at WORKER-02.

  @WORKER-01 @test
  Scenario: A machine joins once, with a code, and gets its own credential
    Given an install command made on the Workers page, with its join code
    When the machine runs the command and joins
    Then the machine gets a credential of its own, and the log keeps only its hash
    And the same code is refused for a second machine
    # README.md, Add machines; PLAN.md T-022; internal/workers/workers_test.go

  # ---- Joining ---------------------------------------------------------------

  @WORKER-02 @test
  Scenario: Add worker makes a one-time install command for a machine
    Given the user is signed in on the Workers page
    When they ask to add a worker
    # internal/workers/workers.go InviteLifetime
    Then the page gives 1 code that works 1 time, for 1 hour
    And 1 install command per address the server has, each fetching the install script with that code
    And the install script fetched with the code carries the code 1 time
    And the script fetched with a wrong code is refused with status 401
    And the agentry program for the machine's system is served with the code: yes
    # internal/web/workers_test.go

  @WORKER-03 @test
  Scenario: A machine joins with the code and is named after its host
    Given an install command whose code has not been used
    When the machine sends the code with what it knows about itself
    Then it gets 1 credential of its own, and its name is its host's name made lowercase letters, digits and dashes
    # internal/web/workers_api.go workerName: at most 28 characters, starting with a letter; freeName adds -2, -3 and so on to a name taken
    And the same code sent again is refused with status 401, saying the code is unknown, used or expired
    And the Workers page lists the worker, online once it connects: yes
    # internal/web/workers_test.go

  @WORKER-04 @test
  Scenario: A join code works once, for an hour, and only hashes are kept
    Given a join code made now
    When a machine joins with it
    Then the code lets a 2nd machine join: no
    And a wrong code lets a machine join: no
    # internal/workers/workers.go InviteLifetime: the code expires 1 hour after it was made
    And the code 1 hour and 1 second after it was made lets a machine join: no
    And the log holds the code or the credential as given: no, only their hashes
    And the credential's hash, as the log keeps it, lets a worker in: no
    When the worker is removed
    Then its credential lets it in: no, and the roster has 0 workers
    # internal/workers/workers_test.go

  @WORKER-05 @test
  Scenario: The rules on who may join and where agents run
    Given a joined worker and an agent whose Runs on names it and the machine Agentry runs on
    When a machine joins with a code nobody made
    Then it is refused: yes
    When a machine joins under the name local, the name of the machine Agentry runs on
    Then it is refused: yes
    When an agent is given a worker that never joined under Runs on
    Then it is refused, naming the workers that have joined, 1 line
    When the user removes a worker an agent runs on
    Then it is refused, naming the agent, until the worker is taken off its Runs on: yes
    When the agent's Runs on is cleared and the worker removed
    Then it is removed: yes
    # internal/rules/worker_rules_test.go

  @WORKER-06 @test
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
    # cmd/agentry/worker_test.go

  @WORKER-07 @test
  Scenario: The install command run again on a joined machine spends no code
    Given a machine that has joined the server, whose credential the server still takes
    When the install command is run on it again
    Then the join is sent 0 times, and the output says the machine has joined already
    Given a machine that joined over plain http, and a server now served over https
    When the install command with the CA's fingerprint is run on it
    Then the worker moves to the https address and keeps its credential, with 0 joins sent
    Given a worker whose server regenerated its CA
    When the install command is run without the new fingerprint
    Then it is refused, saying the CA was regenerated and to join again with -ca-fingerprint, with 0 joins sent
    When it is run with the new fingerprint
    Then the worker keeps the new CA and its credential, with 0 joins sent
    # cmd/agentry/worker_test.go

  @WORKER-08 @manual
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

  @WORKER-09 @manual
  Scenario: On Linux the worker is a systemd user service
    Given a Linux machine with a systemd user session
    When the user runs `agentry worker start`
    # internal/workerservice/service.go ForThisMachine; systemd.go
    Then the service is a systemd user service called agentry-worker.service, 1 unit file in the user's systemd folder
    And it runs `agentry worker run` with the PATH of the shell that started it, its output appended to 1 log file, worker.log
    # internal/workerservice/systemd.go: Restart=always, RestartSec=5
    And it is restarted 5 seconds after it stops, every time
    And lingering is turned on for the user where the machine allows, so it runs logged out: yes
    # 1. On Linux with a systemd user session, run `agentry worker start`.
    # 2. Check ~/.config/systemd/user/agentry-worker.service exists, and `systemctl --user is-active agentry-worker.service` says active.
    # 3. Kill the worker process: systemd starts it again within 5 seconds.
    # 4. Log out and in: it runs, as `loginctl enable-linger` was run for the user where allowed.
    # 5. Run `agentry worker leave`: the unit is disabled and the file removed.

  @WORKER-10 @manual
  Scenario: On macOS the worker is a launchd agent, or a background process over SSH
    Given a Mac
    When the user runs `agentry worker start` in Terminal on the Mac
    # internal/workerservice/launchd.go
    Then the service is a launchd agent labelled dev.agentry.worker, 1 plist in the user's LaunchAgents, with RunAtLoad and KeepAlive
    When the user runs it over SSH, where launchd takes no agent
    Then it runs as a background process instead, and the output says to run `agentry worker start` in Terminal on the Mac 1 time to have it start at login
    # 1. On a Mac, in Terminal, run `agentry worker start`.
    # 2. Check ~/Library/LaunchAgents/dev.agentry.worker.plist exists and `launchctl print gui/<uid>/dev.agentry.worker` says running.
    # 3. Kill the worker process: launchd starts it again (KeepAlive).
    # 4. Over SSH on the same Mac, run `agentry worker start`: it says macOS sets up login services only from the Mac itself, and runs as a background process.

  @WORKER-11 @test
  Scenario: Where the first service cannot start, the second one runs
    Given a machine whose first service cannot start, with a background process as the second
    When the worker is started
    Then the background process runs, and the service's kind names it: yes
    When the first service can start, and the worker is started again
    Then the first runs and the background process is stopped, 1 service running
    # internal/workerservice/fallback_test.go

  @WORKER-12 @manual
  Scenario: On Windows the worker is a background process started at logon by Task Scheduler
    Given a Windows machine
    When the user runs `agentry worker start`
    # internal/workerservice/logintask.go
    Then the worker runs as a background process, its process id in worker.pid, 1 file
    And 1 Task Scheduler task called Agentry worker runs `agentry worker start` at logon, with limited rights
    And when the task cannot be made, the worker still runs now: yes
    # 1. On Windows, run `agentry worker start` in PowerShell.
    # 2. Check `schtasks /Query /TN "Agentry worker"` lists a task that runs at logon.
    # 3. Check the worker.pid file in the worker's folder names a running process.
    # 4. Sign out and in: the task runs `agentry worker start` again.
    # 5. Run `agentry worker leave`: the task is deleted and the process stopped.

  @WORKER-13 @manual
  Scenario: With neither systemd nor launchd the worker is a plain background process
    Given a machine with neither systemd nor launchd
    When the user runs `agentry worker start`
    # internal/workerservice/background.go
    Then the worker runs detached from the shell, its process id in worker.pid, and is started again after a reboot: no
    # internal/workerservice/background.go Stop: 50 checks 100 milliseconds apart
    When the user runs `agentry worker stop`
    Then it is told to stop and waited for up to 5 seconds, after which the stop fails saying so
    # 1. In a container with neither systemd nor launchd, run `agentry worker start`.
    # 2. Check `agentry worker status` names the service "background process (not started again after a reboot)".
    # 3. Run `agentry worker stop` and time it: it returns once the process is gone, or says it didn't stop within 5 s.

  # ---- The worker commands ----------------------------------------------------

  @WORKER-14 @manual
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

  @WORKER-15 @manual
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

  @WORKER-16 @manual
  Scenario: agentry worker logs shows the worker's log
    Given a worker with a log
    When the user runs `agentry worker logs`
    # cmd/agentry/worker_manage.go runLogs: tail -n 100
    Then the last 100 lines of worker.log in the worker's folder are printed
    When the user runs it with -f
    Then the log is followed as it grows: yes
    # 1. Run `agentry worker logs` and count the lines: at most 100, from ~/.agentry-worker/worker.log.
    # 2. Run `agentry worker logs -f` and restart the worker: new lines appear as they are written.

  @WORKER-17 @manual
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

  @WORKER-18 @test
  Scenario: The worker updates itself to the server's agentry
    Given a worker running an older agentry than the server has for its system
    When it asks the server for agentry for its system, saying the hash of its own
    Then the server sends the newer one with its hash, and the worker writes it beside itself, checks the hash, and swaps it in, 1 file replaced
    And the new program can be run: yes
    Given a worker running the same agentry as the server
    When it asks again
    Then the server answers 304 Not Modified and 0 files change
    # internal/workerlink/worker.go Run: with SelfUpdate the check runs before every dial, and an update restarts the process
    # cmd/agentry/worker_manage.go runUpdate: `agentry worker update` does the same, with 5 minutes to fetch, and restarts the service if it runs
    # internal/workerlink/update.go swap: on Windows the old program is moved aside as .old and removed by the next update
    # internal/workerlink/update_test.go

  @WORKER-19 @manual
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

  @WORKER-20 @manual
  Scenario: agentry worker run works in the foreground
    Given a machine that has joined, with its service stopped
    When the user runs `agentry worker run`
    Then the worker runs in the foreground until stopped, which is what the service runs, 1 process
    And `agentry worker` with no command does the same: yes
    # 1. On a joined machine with the service stopped, run `agentry worker run` in a terminal.
    # 2. Check the log line "<name>, working for <server>" and that the Workers page shows it online.
    # 3. Press Ctrl-C: it stops.
    # 4. Run plain `agentry worker`: it does the same as run.

  # ---- The link ---------------------------------------------------------------

  @WORKER-21 @test
  Scenario: A worker keeps dialing when its link drops, and says once when the CA changed
    Given a worker whose server shows a certificate its kept CA did not sign
    When the worker runs for 3.5 seconds
    # internal/workerlink/worker.go Run: the wait starts at 1 second, doubles each time, and is at most 30 seconds
    Then it dials at 0, 1 and 3 seconds, logging link down each time, at least 2 times
    And it says how to join again with a new fingerprint 1 time, not on every dial
    # internal/workerlink/worker.go Run: a link that was up resets the wait to 1 second; a credential the server no longer takes ends the run
    # internal/workerlink/worker_test.go

  @WORKER-22 @test
  Scenario: A connected worker says hello and what it runs, and the page reads its details
    Given a joined worker that connects with its credential
    When it says hello with its version, engines and missing tools
    Then the server records it online with those, and the Workers page lists it online: yes
    # internal/workerlink/server.go Connect: a link with no hello within 10 seconds is closed
    # internal/workerlink/server.go StatusEvery, silentFor: the worker says what it runs every 2 seconds, and a link silent for 20 seconds is hung up
    And the worker's details page shows what the worker reads about itself over the link, within 15 seconds
    # internal/web/workers_api.go showWorker: 15 seconds for the details, and the last 20 joined, online and offline entries
    When the worker's link drops
    Then the server records it offline, unless a newer link replaced it: yes
    # internal/web/workers_test.go

  @WORKER-23 @test
  Scenario: Over https a worker trusts only Agentry's own CA
    Given a server served over https with Agentry's own CA, and a worker that kept that CA
    When the worker reaches the server
    Then the connection is made: yes
    When another CA's certificate is shown, even one the system trusts
    Then the connection is refused: yes
    When the kept CA cannot be read
    Then every server is refused, and the system's store is used instead: no
    And a worker with 0 CA kept uses the plain defaults, as for a server over plain http
    And a certificate issued again by the same CA keeps working: yes
    # internal/pinned/pinned_test.go

  # ---- Where a turn runs -------------------------------------------------------

  @WORKER-24 @test
  Scenario: A turn runs where the work is, or on the first online machine the agent may use
    Given 2 workers online, 1 with the agent's engine and 1 without, and 1 worker offline
    When an agent with 0 workers under Runs on is placed
    Then it runs on the machine Agentry runs on, 1 machine always there
    When an agent may run on the offline worker, the online one with its engine, and the machine Agentry runs on, in that order
    Then it runs on the online worker with its engine, the first of the 3 that can
    When the task's work is already on a machine the agent may use
    Then it stays there, so the work moves 0 times
    When the agent may run only on a worker without its engine, or only on an offline one
    Then it is placed: no, and the reason names each machine, such as offline or has no engine
    # internal/machines/fleet_test.go

  @WORKER-25 @test
  Scenario: A coordinator can send an agent to a named machine
    Given a joined worker, offline, that the tester may run on, and the machine Agentry runs on
    When the coordinator's brief is made
    Then it has 1 Machines section listing each machine with its host, system and arch, online or offline, and who may run there
    And it names the 1 machine the task's work is on now
    When the coordinator assigns an agent to a machine it may not run on
    Then the turn starts: no, and the coordinator hears the agent may not run there and where it runs
    When the coordinator names a machine that does not exist
    Then the coordinator hears there is no machine by that name, with the machines listed
    When the coordinator names a machine that is offline, by its host's name
    # internal/machines/fleet.go WaitOn refuses at once; the 30 second OfflineGrace wait applies only to a worker that went offline within the last 30 seconds or is expected back after a restart, covered by internal/machines/fleet_test.go TestTheFleetWaitsForAWorkerComingBack
    Then the host's name is taken as that machine, the turn starts: no, and the coordinator hears at once that it couldn't start where it chose, as the machine is offline
    When the coordinator names a machine the agent may use, in any letter case
    Then the turn runs there, 1 turn, and the trail names that machine
    # internal/coordinator/machines_test.go

  @WORKER-26 @test
  Scenario: An agent whose machines are all offline asks the user
    Given an agent whose Runs on names 1 worker, which is offline
    When the coordinator assigns the agent
    Then the task asks the user: the agent can't run anywhere now, naming the offline worker, after 1 turn
    And the question says to bring 1 of its workers online, or change where it runs, and answer to carry on
    # internal/coordinator/loop_test.go

  @WORKER-27 @test
  Scenario: After a restart, a worker that may be running agents is waited for
    Given Agentry just started, with a worker the log had online before
    When an agent that runs only on that worker is placed
    # internal/machines/fleet.go OfflineGrace, WaitFor: checked every 250 milliseconds for 30 seconds
    Then the placement waits until the worker dials back in, within 30 seconds, and runs there, 1 machine
    And a worker never seen is waited for: no
    # internal/machines/fleet_test.go

  # ---- Work following the turn ------------------------------------------------

  @WORKER-28 @test
  Scenario: A task's branch follows the work through the hub and back
    Given a task's worktree on one machine, with 1 commit and 1 file left uncommitted
    When the work leaves that machine
    Then what was uncommitted is committed as 1 commit, the hub gets the branch, and the worktree there is gone
    When the next machine makes its worktree from the hub's branch
    Then the uncommitted file is there, the worktree is clean, and its origin is the repo's own remote: yes
    When that machine's agent changes the file and the work goes back
    Then the hub's branch has the change, the history has the first commit, and the base moved: no
    And a branch with 0 new commits sends 0 bundles
    # internal/worktrees/handoff_test.go

  @WORKER-29 @test
  Scenario: A turn runs on the worker, resuming its session, and its terminal shows on the server
    Given a task whose work started on the machine Agentry runs on, and a worker connected
    When the work moves to the worker over git
    Then the page lists the task's files and commits on the worker, including the 1 file committed when the work left
    When a turn starts there going on with the agent's last session
    Then the worker reports the task running within seconds: yes, and the terminal on the server shows the session resuming, 1 session
    When the worker hands the work back
    Then a worktree made from the hub has what the agent left: yes
    # internal/web/workers_test.go

  @WORKER-30 @test
  Scenario: Work moves only to another machine, between turns
    Given a task with 0 worktrees yet
    When a move of its work is recorded
    Then it is refused: yes
    Given a task with its worktree on the machine Agentry runs on
    When a move to that same machine is recorded
    Then it is refused: yes
    When a move to a worker is recorded
    Then it is accepted, 1 move
    When a move is recorded while a turn runs
    Then it is refused: yes
    # internal/rules/worker_rules_test.go

  @WORKER-31 @test
  Scenario: A task's shared files follow the work to the next machine
    Given a task's worktree with files in .agentry/files
    When the turn ends and the work moves
    Then the files are sent to the server as blobs by their hash, and the next worktree receives the same files: yes
    # internal/coordinator/loop.go startOn: the last turn's files are read back where it ran before the work moves
    # internal/sharedfiles/blobs.go MaxSize: a shared file is at most 2 GB
    # internal/sharedfiles/blobs_test.go: the same bytes make the same blob, so a file kept by 2 tasks is stored 1 time
    And a shared file's name may leave .agentry/files: no
    # internal/sharedfiles/sharedfiles_test.go

  @WORKER-32 @test
  Scenario: A worker fetches a file into the shared files only from inside its worktrees, unless the user allows
    Given a worker whose worktrees live in 1 folder, and a file outside it
    When a coordinator's fetch names the file outside
    Then it is refused as outside the worker's folder, and 0 files are sent
    When the fetch names a folder inside the worktrees
    Then every file in it is sent under the name given, 1 file for 1 file
    When the user has allowed it, by answering Copy it
    Then the file outside is sent: yes
    # internal/sharedfiles/sharedfiles_test.go

  # ---- A machine going offline mid-task --------------------------------------

  @WORKER-33 @test
  Scenario: A worker that goes offline is taken to still run its turn for a while
    Given a worker running a task's turn
    When its link drops
    # internal/machines/fleet.go OfflineGrace
    Then for 30 seconds the task counts as running there, so the watchdog takes the agent as crashed: no
    And after 30 seconds, nothing runs there that Agentry can see
    And a worker never seen runs anything: no
    And a stale link's end removes a newer link: no
    # internal/machines/fleet_test.go

  @WORKER-34 @test
  Scenario: The user hears that a machine went offline with work on it
    Given a worker online with 1 task on it
    When it goes offline
    Then the news for the user has 1 line: the machine went offline, and the task waits until it is back
    And a worker offline with 0 tasks on it is news: no
    # internal/coordinator/loop.go moveWork: when the work's machine is offline and the next agent runs elsewhere, the task asks the user to bring it online and answer
    # internal/mcp/waiting_test.go

  @WORKER-35 @manual
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

  # ---- What the machine advertises --------------------------------------------

  @WORKER-36 @manual
  Scenario: A machine advertises its system, its tools and its disk
    Given a worker connected
    When its details are read
    # internal/machines/about.go AboutHere
    Then they give its host, system as its maker names it, kernel, arch, CPU, CPUs, memory in all and free, 3 load averages, boot time, time and zone
    And the disk in all and free where its worktrees are, 2 numbers
    # internal/machines/about.go toolsKept
    And each engine and git, with version and path, read again only after 5 minutes
    # internal/machines/about.go output: each program asked for its version gets 3 seconds
    And agentry's version, its path, its folder, the PATH agents get, the tasks running, the worktrees and the copies of repos there, 7 more facts
    # 1. Open a worker on the Workers page and read its details.
    # 2. Check the system, kernel, arch, CPU, memory, load, boot time, disk and time with the machine itself.
    # 3. Install or remove a tool on the worker and read again: the tools change only after 5 minutes.

  @WORKER-37 @test
  Scenario: A worker says which engines it has, and agents are placed by them
    Given a worker that connected saying it has claude at a version
    When an agent on claude is checked against it
    Then the worker has claude: yes
    When an agent on another engine is checked against it
    Then the worker has that engine: no
    # internal/workers/workers_test.go

  # ---- What a worker cannot do -----------------------------------------------

  @WORKER-38 @test
  Scenario: Without a credential the server takes, a worker reaches nothing
    Given a worker the user removed on the Workers page
    When it dials the link with its old credential
    Then it is refused with status 401, saying the worker isn't joined
    And its open link is dropped at once: yes
    # internal/web/workers_api.go requireWorker: the link, the git hub, the bundle, the update, leaving and the blobs all need the credential
    # internal/web/workers_test.go

  @WORKER-39 @test
  Scenario: An agent's hook call needs the hook token, and only the working agent passes the gate
    Given a task with the engineer at work
    When a hook call arrives with a wrong hook token
    Then it is refused with status 401
    When it arrives with the right token
    Then it is answered with status 200
    When a gate call names an agent that is not the one working
    Then the action is denied: yes
    # internal/hooks/handler_test.go

  @WORKER-40 @test
  Scenario: The gate fails closed when the server cannot answer
    Given an agent session on a worker whose server is unreachable, slow or broken
    When the engine's gate hook calls `agentry hook`
    Then the hook exits 2 within its own deadline, so the action is blocked, 1 time each
    # internal/turns/turns.go environment: the deadline is 1790 seconds, 10 under the engine's gate timeout of 1800 (internal/engines/engine.go GateTimeoutSeconds)
    And the server's reply, when it comes, is printed as it is and its exit code kept: yes
    # cmd/agentry/hook_test.go

  @WORKER-41 @test
  Scenario: An agent on any machine never pushes to main, never force pushes, and every push asks the user
    Given an agent with the developer profile
    # internal/policy/default.toml profiles.developer.rules
    When it runs git push to main or master, by name or by refspec, from any folder
    Then the policy denies it: yes
    When it runs git push with a force flag, in any spelling
    Then the policy denies it: yes
    # internal/policy/default.toml "every push leaves this machine: you approve it": any other git push asks the user first
    # internal/policy/default_test.go
