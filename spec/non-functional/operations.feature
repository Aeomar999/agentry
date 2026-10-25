Feature: Operations: installing, running, securing, cleaning up and shipping Agentry

  # Every number below comes from the code; the comment on each names its
  # file and constant. Addresses are written as placeholders: the server's
  # own are printed by agentry serve, and the deploy target is named only
  # as that.

  # Installing on the three systems

  @OPS-02 @manual
  Scenario: Install on a Mac by downloading the zip, unzipping it and double-clicking Start Agentry
    # packaging/main.go platforms: mac-apple-silicon (darwin/arm64), mac-intel (darwin/amd64); launchers Start Agentry.command, Stop Agentry.command
    # packaging/files/start-unix.sh clears the quarantine attribute and runs ./agentry open
    # Steps:
    # 1. Download agentry-<version>-mac-apple-silicon.zip (M1 and later) or agentry-<version>-mac-intel.zip from the releases page.
    # 2. Unzip it: a folder named Agentry appears.
    # 3. Double-click Start Agentry.command in it. The first time, right-click it, choose Open, then Open again, since Agentry is not signed.
    # 4. Check the browser opened Agentry, signed in.
    Given a Mac with git and Claude Code installed and signed in
    When the zip for it is downloaded, unzipped and Start Agentry.command is run
    Then the folder holds 1 agentry program, 5 builds for other kinds of machine, Start Agentry.command, Stop Agentry.command and README.txt
    And the page opens in the browser, signed in: yes
    And Start Agentry says whether git or Claude Code is missing: yes

  @OPS-03 @manual
  Scenario: Install on Windows by downloading the zip, extracting it and double-clicking Start Agentry.cmd
    # packaging/main.go platforms: windows-x64 (windows/amd64), windows-arm64 (windows/arm64); launchers Start Agentry.cmd, Stop Agentry.cmd
    # packaging/files/start-windows.cmd runs agentry.exe open, then pause, so the links can be copied
    # Steps:
    # 1. Download agentry-<version>-windows-x64.zip, or agentry-<version>-windows-arm64.zip on an ARM PC.
    # 2. Right-click it, Extract All.
    # 3. Double-click Start Agentry.cmd in the Agentry folder. The first time, if Windows says it protected the PC, click More info, then Run anyway.
    # 4. Read the links in the window; it waits for a key.
    Given a Windows PC with git and Claude Code installed and signed in
    When the zip for it is downloaded, extracted and Start Agentry.cmd is run
    Then the folder holds agentry.exe, 5 builds for other kinds of machine, Start Agentry.cmd, Stop Agentry.cmd and README.txt
    And the window waits for a key after printing the links: yes
    And the page opens in the browser, signed in: yes

  @OPS-04 @manual
  Scenario: Install on Linux by downloading the zip, unzipping it and running start-agentry.sh
    # packaging/main.go platforms: linux-x64 (linux/amd64), linux-arm64 (linux/arm64); launchers start-agentry.sh, stop-agentry.sh, mode 0755 in the zip
    # Steps:
    # 1. Download agentry-<version>-linux-x64.zip or agentry-<version>-linux-arm64.zip.
    # 2. Unzip it with a tool that keeps permissions (unzip does).
    # 3. Run ./start-agentry.sh in the Agentry folder, or double-click it if the file manager runs scripts.
    # 4. If it will not run: chmod +x agentry start-agentry.sh stop-agentry.sh, and run it again.
    Given a Linux machine with git and Claude Code installed and signed in
    When the zip for it is downloaded, unzipped and ./start-agentry.sh is run
    Then the folder holds agentry, 5 builds for other kinds of machine, start-agentry.sh, stop-agentry.sh and README.txt
    And the page opens in the browser, signed in: yes

  # Start Agentry and Stop Agentry

  @OPS-05 @test
  # cmd/agentry/open_test.go
  Scenario: Start Agentry starts the server in the background and prints one signed-in link per address
    # packaging/files/start-unix.sh and start-windows.cmd run agentry open
    # cmd/agentry/open.go runOpen: a server already running is opened, not started again; startServer writes serve.log and serve.pid in the data folder
    # cmd/agentry/open.go runOpen waits up to 30 seconds, checking every 200 ms, for the new server to answer
    Given a server listening on every interface on port 7901, with a public address set
    When agentry open runs
    Then it prints 1 link for this computer, at localhost, carrying the page's token
    And it prints 1 link for the public address and 1 for each of this machine's network addresses, each carrying the token
    And it opens exactly 1 of them in the browser, this computer's
    And it says the window can be closed and Agentry keeps running: yes
    Given a server listening on this computer only
    When agentry open runs
    Then it prints only this computer's link: yes

  @OPS-06 @test
  # cmd/agentry/open_test.go
  Scenario: Start Agentry passes serve's flags on, and says when they wait for the next start
    # cmd/agentry/open.go passedOn: -https, -budget, -max-turns, -resume-sessions, -archive-after
    Given agentry open run with -https, -budget 0 and -max-turns=0 while no server runs
    When it starts the server
    Then the server is started with exactly those 3 flags, and none of the others: yes
    Given agentry open run with none of serve's flags
    When it starts the server
    Then the server is started with 0 extra flags, so serve's own defaults hold
    Given agentry open run with -https, -budget and -max-turns while a server already runs
    Then it opens the page and says Agentry is already running and the 3 flags apply only when it next starts: yes
    And it says to give -https on every start, since Start Agentry does not pass it: yes

  @OPS-07 @manual
  Scenario: Stop Agentry stops the server that Start Agentry started
    # packaging/files/stop-unix.sh and stop-windows.cmd run agentry stop; cmd/agentry/open.go runStopServer
    # cmd/agentry/open.go pidFile = serve.pid; the wait is 50 checks of 100 ms = 5 seconds
    # internal/procs/procs_unix.go Terminate sends SIGTERM; procs_windows.go Terminate kills the process
    # Steps:
    # 1. Start Agentry.
    # 2. Run Stop Agentry (Stop Agentry.command, Stop Agentry.cmd or ./stop-agentry.sh).
    # 3. Check serve.pid is gone from the data folder and the page no longer answers.
    # 4. Run Stop Agentry again.
    Given Agentry running from Start Agentry, so the data folder holds serve.pid
    When Stop Agentry is run
    Then the server process is asked to stop and has ended within 5 seconds: yes
    And serve.pid is removed: yes
    And it prints that Agentry stopped and agents' sessions that were running carry on
    When Stop Agentry is run again
    Then it prints that Agentry isn't running, and exits 0

  @OPS-08 @test
  # cmd/agentry/serve_test.go
  Scenario: Only one server runs on a data folder at a time
    # cmd/agentry/serve_lock.go lockFile = serve.lock; serve_lock_unix.go flock LOCK_EX|LOCK_NB; serve_lock_windows.go LockFileEx exclusive
    Given a server running on a data folder, with a snapshot write under way and a task's files in it
    When a second agentry serve starts on the same folder, on another port
    Then it fails, saying another agentry serve is using the folder, with the first one's pid: yes
    And the folder's files are exactly as before: yes
    And the lock is released when the first server ends, however it ends: yes

  # Where data lives

  @OPS-09 @manual
  Scenario: Agentry keeps everything in .agentry in the home folder on every system, and sessions in the system's temporary folder
    # cmd/agentry/serve.go -data default: filepath.Join(home, ".agentry"); cmd/agentry/open.go the same; the launchers pass no -data
    # cmd/agentry/open.go -repo default: filepath.Join(home, "Agentry"), made at the first start
    # cmd/agentry/serve.go: agentry.db, state.snapshot, files, repos, worktrees, ui-token, hook-token, desk-token, helper-token
    # cmd/agentry/policy.go policyFile = policy.toml, settingsFile = features.toml; internal/webpush/webpush.go File = push.json
    # internal/tlsca/ca.go Folder = tls; internal/client/client.go ServerFile = server.json
    # cmd/agentry/serve_lock.go serve.lock; cmd/agentry/open.go serve.log and serve.pid
    # internal/sessions/proc_unix.go sessionsDir: the temporary folder, agentry-<uid>; proc_windows.go: the user cache folder, agentry\sessions
    # cmd/agentry/worker.go -dir default: filepath.Join(home, ".agentry-worker")
    # Steps:
    # 1. Start Agentry and make one task.
    # 2. List .agentry in the home folder.
    # 3. On a worker, list .agentry-worker in the home folder.
    Given Agentry started with no -data and no -repo
    Then the data folder is .agentry in the home folder on Mac, Windows and Linux alike: yes
    And it holds the log agentry.db, the snapshot state.snapshot, and the folders files, repos and worktrees
    And it holds the 4 token files ui-token, hook-token, desk-token and helper-token, which stay the same across restarts
    And it holds policy.toml, features.toml, push.json, server.json, serve.lock, serve.log and serve.pid, and tls when -https is on
    And new tasks work in the git repo Agentry in the home folder unless they name another: yes
    And the sessions' sockets are in the temporary folder agentry-<uid> on Mac and Linux, and in the user cache folder under agentry\sessions on Windows
    And a worker keeps its credential, repos, worktrees and worker.log in .agentry-worker in its home folder

  # The address

  @OPS-10 @test
  # cmd/agentry/serve_test.go
  Scenario: The server listens on every interface on port 7800 unless -addr says otherwise
    # cmd/agentry/serve.go -addr default 0.0.0.0:7800; localURL, localhostURL, networkURLs
    Given a server started with the default -addr
    Then it listens on port 7800 on every interface: yes
    And its own link is at loopback on port 7800, and it prints 1 more link for each of this machine's network addresses
    Given a server started with -addr 127.0.0.1:7800
    Then other machines reach it: no
    And it prints links for network addresses: no, only this computer's

  @OPS-11 @manual
  Scenario: Start Agentry takes the first free port from 7800, trying 20 ports
    # cmd/agentry/open.go -port default 7800; freeAddress tries port to port+19 on 0.0.0.0
    # Steps:
    # 1. Occupy port 7800 with another program.
    # 2. Run agentry open with no server running, and read the port in the printed links.
    # 3. Occupy ports 7800 to 7819 and run agentry open again.
    Given port 7800 is taken by another program
    When agentry open starts a server
    Then the server listens on port 7801: yes
    Given ports 7800 to 7819 are all taken
    When agentry open starts a server
    Then it fails, saying there is no free port from 7800 to 7819, and exits 1

  @OPS-12 @test
  # internal/web/addresses_test.go
  Scenario: -public-url is the address people open, for the printed link and the install commands
    # cmd/agentry/serve.go -public-url; the printed link is it, or the listened address when it is empty; "From another machine" links print only without it
    Given a server started with -public-url set to an https address, listening on every interface
    When the page asks for the addresses other machines reach
    Then the list leads with the public address, then 1 entry per network address, then this computer's: yes
    And the address chosen for the install commands is the public address: yes
    Given a server started with no -public-url, listening on every interface
    When the page open at localhost asks for the addresses
    Then the list holds 1 entry per network address and this computer's, and the first network address is chosen: yes
    Given a server listening on this computer only
    Then the list holds exactly 1 address, this computer's

  # HTTPS with the local CA

  @OPS-13 @test
  # internal/tlsca/ca_test.go
  Scenario: The first start with -https makes a CA that signs only for this machine and private addresses
    # internal/tlsca/ca.go Folder = tls, caLifetime = 10 years, the key is 0600 on Mac and Linux, privateRanges
    Given a data folder with no tls folder
    When the server starts with -https
    Then tls/ca.key and the CA certificate are made once and reused at the next start: yes
    And the CA lasts 10 years
    And its name constraints are critical and permit localhost, this machine's hostname, hostname.local, loopback and the private ranges 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 100.64.0.0/10 and fd00::/8: yes
    And a leaf it signs for a name on the public web verifies: no
    And a key others may read, a key that is a link, or a half-made CA is refused: yes

  @OPS-14 @test
  # internal/tlsca/issuer_test.go
  Scenario: The server certificate is issued in memory and renewed by itself
    # internal/tlsca/issue.go leafLifetime = 397 days, always localhost, 127.0.0.1, ::1; hosts.go adds the hostname, hostname.local and every network address
    # internal/tlsca/issuer.go renewBefore = 30 days, hostsFresh = 30 seconds
    Given a running server with -https
    Then every handshake while nothing changes gets the same certificate: yes
    And the certificate lasts 397 days and never past the CA's end: yes
    When this machine's addresses change
    Then a new certificate naming them is issued with no restart: yes
    And the machine's addresses are listed at most once every 30 seconds
    When the certificate is within 30 days of its end
    Then a new one is issued: yes
    And a client that trusts the CA connects: yes

  @OPS-15 @test
  # cmd/agentry/https_test.go
  Scenario: One port serves https and plain http, and hands out the CA to any device that asks
    # cmd/agentry/https.go frontDoor, trustPrefix = /trust/ (internal/web/addresses.go TrustPath); ca.crt, ca.pem and agentry.mobileconfig under it
    Given a server with -https on one port
    Then a request over TLS at localhost or loopback is served over HTTP/2: yes
    And a plain http request from this machine is served: yes
    And a plain http request from another machine gets a 308 redirect to the same path over https: yes
    And a plain http request at a host the certificate does not cover gets the trust page with the addresses that work: yes
    And /trust/ and the CA certificate under it are served to anyone, with no token, over plain http and https alike: yes
    And the CA's key is served: no
    And the sign-in cookie is marked Secure over TLS: yes

  @OPS-16 @manual
  Scenario: A device trusts the CA once, then installs the app over https
    # cmd/agentry/trust.html: the trust page lists every step per system; cmd/agentry/ca.go prints the SHA-256 fingerprint
    # Steps:
    # 1. On the device, open http://<server address>:<port>/trust/, the address serve printed after "Trust it once on each device"; on a phone, scan the QR code under Sign in another device.
    # 2. Compare the fingerprint on the page with what agentry ca prints on the server.
    # 3. Download the CA and trust it as the page says for the device's system (Android: Install a certificate; iPhone: install the profile, then turn on full trust; macOS: Always Trust in the System keychain; Windows: certutil -addstore -f Root; Linux: the system's CA store, and Chrome's own; Firefox: its own list).
    # 4. Open the https address, sign in, and click Install app.
    Given a device that does not trust the CA yet and a server with -https
    When the trust page is opened over plain http
    Then it shows the CA's SHA-256 fingerprint, and it equals the one agentry ca prints: yes
    When the CA is downloaded and trusted as the page says, and the https address is opened
    Then the page is secure and Install app works: yes

  @OPS-17 @test
  # cmd/agentry/ca_test.go
  Scenario: Regenerating the CA means every device, worker and connected Claude Code trusts again
    # cmd/agentry/ca.go -regenerate; internal/tlsca/ca.go Regenerate
    Given a server running from a data folder
    When agentry ca -regenerate runs for that folder
    Then it refuses, saying to stop Agentry first, and exits 1
    Given the server stopped
    When agentry ca -regenerate runs
    Then the CA's fingerprint differs from before: yes
    And it says devices that trusted the old one must trust the new one from /trust/, and workers and Claude Code machines must join again with the new fingerprint: yes

  @OPS-18 @test
  # cmd/agentry/worker_test.go
  Scenario: A worker joins an https server by the CA's fingerprint, and is refused on a wrong one before the code is sent
    # cmd/agentry/server_ca.go trustServer; cmd/agentry/worker.go runJoin -ca-fingerprint
    Given a server with -https whose CA has fingerprint F
    When agentry worker join -ca-fingerprint F <server> <code> runs
    Then the worker keeps the CA and joins over https: yes
    When the fingerprint given is not F
    Then it is refused and the code is sent: no
    When the server is https and no fingerprint is given, and the machine does not trust the certificate
    Then it is refused, saying to use -ca-fingerprint: yes
    Given a worker that joined over plain http before -https was turned on
    When the join command runs again with the fingerprint
    Then the worker is moved to https and keeps its credential, and the code is used: no

  # Push notifications

  @OPS-19 @test
  # internal/web/push_api_test.go
  Scenario: Push notifications need an https public address and a subscription from the browser
    # internal/webpush/webpush.go Available: -public-url must be an https address; File = push.json beside the log
    # internal/web/push_api.go: subscribing is refused while push is off, 400 when malformed
    Given a server with no -public-url, or one that is not https
    Then GET /api/push says push is available: no, with the reason
    And POST /api/push/subscriptions is refused: yes
    Given a server with an https -public-url
    When a browser posts its push subscription
    Then it is answered 200 and kept in push.json, with the VAPID key pair, beside the log and never in it: yes
    And it is still there after a restart: yes
    When a malformed subscription, or one whose endpoint is not https, is posted
    Then it is answered 400 with a reason: yes
    When the same endpoint subscribes twice and then unsubscribes
    Then 1 subscription is kept, then 0

  @OPS-20 @test
  # internal/webpush/sink_test.go
  Scenario: What newly needs the user is pushed once to each subscribed device
    # cmd/agentry/push.go pushTimeout = 30 seconds, pushFallback = 30 seconds; internal/webpush/sink.go lineShown = 500
    Given 2 subscribed devices and an entry already waiting when the server started
    Then the entry already waiting is pushed: no
    When 1 new entry needs the user
    Then each device gets 1 push for it
    When the entry leaves Needs you and comes back
    Then it is pushed again: yes
    When a device's push service says the subscription is gone
    Then that device is dropped and the other still gets its pushes: yes
    And a push carries at most 500 characters of the entry's line

  # The workers' background services

  @OPS-21 @manual
  Scenario: The install command from the Workers page joins the machine and starts the worker in the background
    # internal/web/workers_api.go: the page gives one command per system; internal/web/install.sh runs agentry worker join [-ca-fingerprint F] [-name N] <server> <code>, then agentry worker start
    # cmd/agentry/worker.go workerCommands: join, start, stop, restart, status, logs [-f], update, leave [-purge], run
    # internal/workerservice/service.go ForThisMachine: systemd user service agentry-worker.service on Linux; launchd agent dev.agentry.worker on macOS, or a background process when started over SSH; on Windows a background process started at login by the Task Scheduler task "Agentry worker"
    # cmd/agentry/worker_manage.go runStart waits 15 seconds for the link; worker.log in the worker's folder
    # Steps:
    # 1. On the Workers page, make a join code and copy the command for the other machine's system.
    # 2. Run it there: in a terminal on Mac or Linux, in PowerShell on Windows.
    # 3. Run agentry worker status and agentry worker logs on the machine.
    # 4. Log out and in again, and check agentry worker status says it is running.
    Given a join code from the Workers page, valid for 1 hour
    When the install command runs on the other machine
    Then the machine joins with the code and the code is spent: yes
    And agentry worker start reports within 15 seconds whether it connected to the server
    And the service is a systemd user service on Linux, a launchd agent on a Mac, or a Task Scheduler login task on Windows: yes
    And the worker's output goes to worker.log in its folder: yes
    When the machine is logged out and in again
    Then the worker is running again: yes, except a plain background process, which is not started again
    When agentry worker stop runs
    Then it says agents running there keep going until their turn ends: yes

  @OPS-22 @test
  # internal/workerlink/update_test.go
  Scenario: A worker updates itself to the server's agentry and restarts
    # internal/workerlink/update.go: before each dial the worker compares its build with the server's by ETag and replaces itself
    # cmd/agentry/worker.go runAsWorker: Run returns ErrUpdated and the process restarts into the new program (not covered by the test)
    Given a worker whose agentry differs from the one the server has for its system
    When it checks for an update
    Then it replaces its own program with the server's: yes
    And the program keeps its executable mode: yes

  @OPS-23 @test
  # cmd/agentry/mcp_test.go
  Scenario: Claude Code on another machine names the server by AGENTRY_URL, AGENTRY_TOKEN and AGENTRY_CA_FINGERPRINT
    # cmd/agentry/mcp.go: the Integrations page's command runs agentry mcp connect [-ca-fingerprint F] <server> <code>; the hooks and agentry mcp read the three variables, or find the server as connect saved it
    # cmd/agentry/kept_ca.go: a CA checked by fingerprint is kept beside the saved connection, so later runs make 0 requests for it
    Given AGENTRY_URL at an https server, AGENTRY_TOKEN and AGENTRY_CA_FINGERPRINT set
    When agentry mcp reaches the server
    Then it trusts only the CA with that fingerprint, and the call succeeds: yes
    When the hooks reach the server with the same variables
    Then the CA is fetched and checked 1 time, and later runs fetch it 0 times, with the fingerprint accepted with or without colons and in either case
    When AGENTRY_CA_FINGERPRINT names another CA
    Then the kept CA is checked again and the server is refused: yes
    When agentry mcp connect is given a wrong fingerprint
    Then it refuses before the code is sent: yes
    When agentry mcp connect is given an https server and no fingerprint, and the machine does not trust the certificate
    Then it refuses, naming -ca-fingerprint and the fingerprint the server offered: yes

  # Erasing and redacting

  @OPS-24 @test
  # internal/web/erase_test.go
  Scenario: Erasing a task removes every row of it and what copied its words, for good
    # internal/web/erase_api.go erase: POST /api/tasks/{task}/erase, with an optional reason; the page only, never the desk
    # internal/store/erase.go Erase: the rows, the archive blobs its markers name, a task.erased marker with who, when and why
    Given a done task whose summary its epic and its asker copied, with shared files of its own and one shared with another task, and a planted secret in its rows
    When it is erased
    Then the answer names the task and a time within 1 minute of now
    And its rows, its archive files and the shared files only it names are gone from the disk: yes
    And the copies of its words in its epic's and its asker's rows read [redacted], and the rest of those rows is as it was: yes
    And the other tasks are still in the state: yes
    And the task reads as erased on that day, not as no such task: yes
    Given a live task with a session running
    When it is erased
    Then its session is stopped before its rows are read: yes
    Given an epic with a story still naming it, or a task already erased
    When it is erased
    Then it is refused with 409 and nothing is written: yes

  @OPS-25 @test
  # internal/web/redact_api_test.go
  Scenario: Redacting a text counts first, then rewrites only the rows that hold it
    # internal/web/server.go: POST /api/tasks/{task}/redact/preview and /redact, /api/agents/redact, /api/redact for the whole log
    # internal/store/redact.go Redacted = [redacted]; redact_archived.go: unarchive, redact, archive again in a new blob, delete the old
    Given a task whose description holds a secret, and a snapshot on disk that holds it too
    When the redaction is previewed
    Then the preview says how many rows hold the text
    When the redaction is confirmed with that count
    Then those rows read [redacted] in place of the secret, with every other byte as it was: yes
    And the secret is found on the disk, in the log, the WAL or the snapshot: no
    When the rows that hold the text are not the ones counted
    Then the redaction is refused with 409 and nothing is written: yes
    Given an archived task that holds the secret
    When it is redacted
    Then it is unarchived, redacted and archived again in a new file, and its old file is deleted: yes
    Given an agent whose knowledge holds the secret in a version
    When the agents are redacted
    Then each version that holds it is written again whole without it, and 0 versions are added

  @OPS-26 @test
  # internal/store/redact_test.go
  Scenario: A redaction that cannot leave the rest of a row as it was is refused
    # internal/store/redact.go: the refusals, each with its reason, before anything is written
    Given a row that holds the text in a key
    When a redaction of the text is asked for
    Then it is refused and 0 rows are written
    Given a row that holds the text in a number, or written otherwise than the log writes it
    Then it is refused: yes
    Given a row where the text would still show next to [redacted] once rewritten
    Then it is refused: yes
    Given a reason that holds the text itself
    Then it is refused: yes
    Given a stream that is erased or archived
    Then it is refused before its rows are read: yes

  # The Makefile and the CI check

  @OPS-27 @manual
  Scenario: The Makefile's targets build the UI, the binary, the worker builds, run the checks and package the zips
    # Makefile: all = ui build workers; ui = cd ui && npm ci && npm run build; build = go build -o bin/agentry ./cmd/agentry
    # Makefile: WORKER_PLATFORMS = darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64 windows/arm64, built with CGO_ENABLED=0 to bin/agentry-<os>-<arch>
    # Makefile: test = go vet ./... && go test ./... then cd ui && npm run lint; package = ui, then go run ./packaging
    # Steps:
    # 1. Run make -n all, make -n test and make -n package and read the commands.
    # 2. Run make build and check bin/agentry.
    # 3. Run make workers and list bin/.
    Given a checkout with Go 1.27, Node and git
    When make runs with no target
    Then it runs ui, build and workers in that order: yes
    When make workers runs
    Then bin holds 6 worker builds, one per platform, beside bin/agentry
    When make test runs
    Then it runs go vet, go test and the UI's lint, and fails if any fails: yes
    When make package runs
    Then it builds the UI first and then runs the packaging program: yes

  @OPS-28 @manual
  Scenario: The CI check fails when the UI is not built and committed, then vets and tests
    # .gitlab-ci.yml stages: check, build, deploy; job check: image golang:1.27-bookworm, timeout 15 minutes, on merge requests and the dev branch
    # .gitlab-ci.yml job check script: test -f internal/web/dist/index.html or fail saying to run make ui and commit internal/web/dist; go vet ./...; go test ./...
    # .gitlab-ci.yml job build_image: on dev only, needs check, timeout 20 minutes, builds and pushes the image; job deploy: on dev only, needs build_image, calls the deploy target's hook and fails unless it answers 200
    # Steps:
    # 1. Open a merge request against dev and watch the pipeline.
    # 2. On a branch, delete internal/web/dist/index.html, push, and read the check job's output.
    Given a commit on a merge request or on dev
    Then the pipeline runs the job check, and the jobs build_image and deploy only on dev: yes
    And the UI's lint runs in the pipeline: no, it runs in make test
    When internal/web/dist/index.html is missing from the commit
    Then the job check fails within its first step, saying the UI is not built and to run make ui and commit internal/web/dist
    When the UI is built and committed
    Then the job check runs go vet and go test, with a timeout of 15 minutes

  # How a release is packaged

  @OPS-29 @manual
  Scenario: make package makes one zip per kind of computer, each holding everything Agentry needs
    # packaging/main.go: 6 platforms; the zips go to release/agentry-<version>-<label>.zip; version from git describe --tags --always --dirty, or dev without git
    # packaging/main.go pack: the folder Agentry with this computer's agentry (agentry.exe on Windows), the 5 other builds as agentry-<os>-<arch>, the two launchers and README.txt with the version and the first-run note for the system
    # packaging/main.go run: refuses unless internal/web/dist/index.html is there
    # README.md Make a release: tag the commit on dev, push the tag, make package on the tagged commit in a clean checkout, publish the zips with gh release create
    # Steps:
    # 1. On a clean checkout of the tagged commit, run make package.
    # 2. List release/ and unzip -l one zip of each system.
    # 3. Run go run ./packaging in a tree without internal/web/dist/index.html.
    Given a clean checkout at tag v0.1.0 with the UI built
    When make package runs
    Then release holds 6 zips: agentry-v0.1.0-mac-apple-silicon.zip, mac-intel, linux-x64, linux-arm64, windows-x64 and windows-arm64
    And each zip holds 1 folder, Agentry, with 1 agentry program for its system and 5 builds for the others
    And the Mac zips hold Start Agentry.command and Stop Agentry.command, the Windows zips Start Agentry.cmd and Stop Agentry.cmd, and the Linux zips start-agentry.sh and stop-agentry.sh
    And each zip holds README.txt naming version v0.1.0 and the first-run note for its system
    Given a tree with changes
    When make package runs
    Then the version in the zip names ends in -dirty: yes
    Given a tree without internal/web/dist/index.html
    When go run ./packaging runs
    Then it refuses, saying to run it from the repo's root with the page built, and exits 1

  @OPS-30 @manual
  Scenario: The deploy target runs one image from dev, pinned to one node, with its home as a volume
    # Dockerfile: the server built with CGO_ENABLED=0, with git and Claude Code pinned at CLAUDE_CODE_VERSION 2.1.301, EXPOSE 7800
    # docker-stack.dev.yml: AGENTRY_PUBLIC_URL from DOMAIN over https, the home volume, healthcheck every 30 s with a 5 s timeout, 3 retries and a 20 s start period, stop_grace_period 30 s, placement on NODE, update order stop-first
    # Steps:
    # 1. Push to dev and watch the pipeline's deploy job.
    # 2. Read the server's log on the deploy target for the printed link.
    # 3. Open the dock's Claude tab once, after the first deploy, to sign Claude in.
    Given a commit on dev that passed the job check
    When the pipeline's build_image and deploy jobs run
    Then the deploy target runs the new image, stopping the old copy first: yes
    And the container is checked every 30 seconds at port 7800 and given 30 seconds to stop, so a hook call waiting for the user can record its denial: yes
    And Claude's sign-in, the repos and Agentry's log and policy survive the redeploy in the home volume: yes
