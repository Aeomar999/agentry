Feature: Signing in
  Signing devices in with one-time codes, trusting Agentry's CA, installing the app, and the desktop app.

  # ---- Sign-in codes ---------------------------------------------------------

  @SIGNIN-01 @user @test
  Scenario: A one-time code signs another device in once, within 5 minutes
    # internal/web/signin_test.go TestASigninCodeSignsInOnce, TestASigninCodeExpires and TestIssuingPastTheCapDropsTheOldestCode
    # 8 characters from 31 letters and digits, no 0, O, 1, I or L, shown as XXXX-XXXX, 5 minutes, at most 20 live: internal/web/signin.go lines 25 to 28 and 74
    # the cookie lasts 365 days: internal/web/server.go line 325
    # README.md, Signing in the installed app, or another device
    Given a device that is signed in asked for a code
    When the other device sends the code in lower case with spaces around its dash
    Then it is signed in: the reply holds the token and a cookie, HttpOnly, SameSite Lax, path /, lasting at least 300 days, not Secure over plain http
    And the cookie alone opens the API, with no token sent
    When the same code is sent again
    Then it is refused with 401 and no cookie
    Given a code 5 minutes old
    Then it is refused with 401; one a second short of 5 minutes signs in
    Given 21 codes issued
    Then the oldest no longer works, while the second and the 21st do

  @SIGNIN-02 @user @test
  Scenario: Wrong codes are limited: 5 a minute from one address, 30 a minute from everyone
    # internal/web/signin_test.go TestFailedSigninsAreLimited, TestFailedSigninsSentAtOnceAreLimited, TestSigningInDoesNotCountAsAFailure and TestThePastedTokenSignsIn
    # 5 failures a minute per client, 30 failed codes a minute in all: internal/web/signin.go lines 30 to 35; the refusals' words: lines 37 and 38, and "send a code or the token": line 226; the tests assert the codes, not the words
    Given a live code
    When one address sends 5 wrong codes within a minute
    Then its next try, with a good code or the token, is refused with 429 and Retry-After 60
    And another address signs in with the code; a minute later the first address is allowed again
    When 30 wrong codes come from 30 addresses within a minute
    Then every code is refused with 429 for a minute, while the token still signs in
    When 50 wrong codes are sent at once from one address
    Then exactly 5 are refused with 401 and 45 with 429
    And a sign-in that succeeds never counts as a failure
    When a wrong token is sent, or neither a code nor a token
    Then they are refused with 401 and 400
    And behind an https proxy the cookie is Secure, yes

  # ---- Devices and the person menu -------------------------------------------

  @SIGNIN-03 @user @manual
  Scenario: The person menu, Sign in another device, and the sign-in page
    # ui/src/components/organisms/AccountMenu.tsx lines 29 and 40 to 45; ui/src/components/organisms/SignInDeviceDialog.tsx lines 50, 74 to 96, 97 to 110 and 111 to 118
    # ui/src/pages/SignInPage.tsx lines 52 to 78; what the field takes: ui/src/lib/signin.ts lines 12 to 24, a code being at most 8 characters, line 6
    # README.md, Signing in the installed app, or another device
    Given a device that is signed in
    When the user clicks "Signing in" in the top bar
    Then a menu lists exactly 3 items: "Sign in another device", "Notifications on this device" and "Sign out"
    When the user picks "Sign in another device"
    Then the dialog shows a code like ABCD-EFGH, a QR code of <address>/?signin=<code>, and "Expires in 5:00" counting down
    And it reads "The code works once, for five minutes. A new code doesn't stop this one working." with the buttons "New code" and Done
    And a list of addresses is shown only when the server has more than 1, each with its label
    When the code runs out
    Then "Expired: make a new code" is shown and the QR code is faded
    Given the other device, which opens the sign-in page
    Then the page reads "Sign in" with exactly 1 field, "Code, token or link", and the hint on where to get a code or paste the link
    When the user pastes the printed link, its bare token, a code with or without its dash, or scans the QR code
    Then each signs in once; a link with no token or code in it is refused on the page
    When a wrong code is sent
    Then the banner "Couldn't sign in" is shown with the server's words

  @SIGNIN-04 @user @test
  Scenario: Sign out forgets the cookie on the server and the token on the device
    # internal/web/signin_test.go TestSigningOutClearsTheCookie
    # the page forgets its token and says "Couldn't sign out" when the server refuses: ui/src/components/organisms/AccountMenu.tsx lines 44 and 45
    Given a device that is signed in
    When the user picks "Sign out"
    Then the reply is 200 with the cookie emptied, path /, expiring at once

  @SIGNIN-05 @user @test
  Scenario: Install app shows only where the browser can install the page
    # ui/src/lib/install.test.ts "is refused on a secure page whose service worker the browser refused", "is unavailable on a secure page with a service worker and no offer yet" and "is insecure on a plain http page, whatever the service worker did"
    # the 5 states and the button hidden when installed or unavailable, or refused with no CA: ui/src/lib/install.ts line 17 and 52 to 57; ui/src/components/molecules/InstallAppButton.tsx lines 35 and 36
    # the manifest: ui/public/manifest.webmanifest, display standalone, 3 icons
    # README.md, HTTPS and installing the app on other devices
    Given a secure page whose service worker the browser refused to register
    Then the install state is exactly "refused"
    Given a secure page with a service worker and no offer from the browser yet
    Then the install state is exactly "unavailable"
    Given a page over plain http at a network address
    Then the install state is exactly "insecure", whatever the service worker did

  @SIGNIN-06 @user @manual
  Scenario: The Install app button's note, where the page cannot be installed
    # ui/src/components/molecules/InstallAppButton.tsx lines 47 to 81
    # README.md, HTTPS and installing the app on other devices
    Given Agentry open over plain http at a network address, without -https
    When the user clicks "Install app"
    Then a note is shown: browsers install apps only from secure pages; open Agentry at the localhost address with the port on the computer it runs on, or over https, and install it there
    Given the same page while the server serves -https with its own CA
    Then the note names the https address and its trust page, exactly 2 links, and ends saying the localhost address with the port works too on the computer Agentry runs on
    Given a secure page where the browser refused the service worker, with a trust page
    Then the note shown reads "The browser won't install the app here. If it warned about the certificate, trust Agentry's certificate on the trust page, then reload this page."
    When the browser offers to install
    Then the button installs with exactly 1 prompt, and the button is hidden once the app is installed

  @SIGNIN-07 @user @test
  Scenario: Trusting a device once: the trust page per address, with the CA's fingerprint
    # internal/web/trust_test.go TestThePageIsToldWhereToTrustTheServersOwnCA; ui/src/lib/trust.test.ts "keeps the first eight pairs of a SHA-256"
    # the dialog's section "First time on this device? Trust Agentry's certificate", with its QR code and the fingerprint: ui/src/components/organisms/SignInDeviceDialog.tsx lines 53 to 73
    # README.md, HTTPS and installing the app on other devices, "Trust a device once"
    Given the server serving -https with its own CA, reachable at 2 network addresses and localhost
    When a sign-in code is issued
    Then each address carries a trust page at /trust/ with the CA's fingerprint, over plain http where the address has a port and over https where it has none
    And the page cuts the fingerprint to its first 8 pairs, the rest behind an ellipsis
    Given a public address behind a proxy doing TLS
    Then that address carries no trust page, and the others still do
    Given a server with no CA of its own
    Then no reply speaks of trust
    And no reply ever holds the token or a private key

  # ---- The desktop app -------------------------------------------------------

  @SIGNIN-08 @user @test
  Scenario: The desktop app starts Agentry when none is running
    # internal/launch/launch_test.go TestEnsureStartsAServerAndStopsOnlyThatOne and TestEnsureListensOnThisComputerByDefault
    # the data folder, default repo, binary, host and port: cmd/agentry-desktop/desktop_test.go TestLaunchOptions and TestTheAgentryBinaryIsNextToTheApp
    # started only when the start page asks: TestTheAppStartsAgentryWhenTheStartPageAsks, TestStopBeforeStartStartsNothing, TestStartReplySaysTheAppIsQuitting, TestStartPageAsksTheAppToStartAgentry and TestStartPageGoesWhereTheAppSays
    # docs/decisions.md, 2026-10-26
    Given no Agentry answering from the data folder
    When the user opens the desktop app
    Then it starts the agentry binary beside it as agentry serve, exactly once, on 127.0.0.1 from port 7800
    And the server uses the same data folder and default repo as Start Agentry, and the default repo is made if it is missing: yes
    And the server starts only once the window's start page asks for it, and the page then goes on to Agentry's page signed in: yes
    Given the app quits before its start page asks
    Then no server is started, then or later, and the start page says the app is quitting

  @SIGNIN-09 @user @test
  Scenario: The desktop app uses an Agentry that is already running
    # internal/launch/launch_test.go TestEnsureReusesARunningServerAndLeavesItRunning and TestEnsureUsesAServerAnotherLaunchStarted
    # docs/decisions.md, 2026-10-26
    Given Agentry already answering from the data folder, started by Start Agentry
    When the user opens the desktop app
    Then no second server is started, and the app shows the running one
    Given two launches at the same time, where the other launch's server answers first
    Then the app uses that server as one it found running, and its own child is stopped: yes

  @SIGNIN-10 @user @test
  Scenario: Quitting the desktop app stops only a server it started
    # internal/launch/launch_test.go TestEnsureStartsAServerAndStopsOnlyThatOne, TestEnsureReusesARunningServerAndLeavesItRunning and TestStopDoesNotWaitForAProcessHoldingTheOutput
    # docs/decisions.md, 2026-10-26
    Given the desktop app started its own server
    When the user quits the app
    Then that server is stopped, and it no longer answers
    Given the desktop app found a server already running
    When the user quits the app
    Then that server is kept running, and it still answers
    Given a process the server started still holds the server's output
    Then quitting still ends within 8 seconds: yes

  @SIGNIN-11 @user @test
  Scenario: Links that leave Agentry open in the system browser, not the window
    # cmd/agentry-desktop/desktop_test.go TestPageScriptSendsOutsideLinksToTheApp, TestLinksFollowTheRoute, TestOpensItselfWhereWailsRefuses and TestOpensItselfOpensTheLinkItChecked; internal/launch/launch_test.go TestRoute
    # docs/decisions.md, 2026-10-26
    Given Agentry's page in the desktop app's window
    When the user clicks a link to another site, such as GitHub or the docs, or a mail link, with the left or middle button, or the page opens one with window.open
    Then the window does not follow it, and the system browser opens it: yes
    And the browser opens it even when it holds characters such as parentheses, ! or ~: yes
    When the user right-clicks a link to another site
    Then the browser does not open it, and nothing is sent to the app
    And a link to the same host on another port counts as another site: yes
    When the user clicks a link within Agentry, or one that opens a new window on Agentry's own address
    Then it opens in the same window, and no browser opens
    When a link names a local file or a script
    Then it is not opened, in the window or the browser

  @SIGNIN-12 @user @manual
  Scenario: The desktop app shows the board signed in, with no browser
    # cmd/agentry-desktop/main.go, the start page in cmd/agentry-desktop/desktop.go startPage and startReply
    # docs/decisions.md, 2026-10-26
    Given the desktop app built for this computer, with the agentry binary beside it
    When the user opens it
    Then the window shows at once, reading "Starting Agentry…" until Agentry answers: yes
    And a window titled exactly "Agentry" shows the Tasks board, signed in, and no browser opens
    And the window and the app carry the Agentry mark: yes
    Given Agentry did not start, such as the agentry binary missing beside the app
    Then the window says why Agentry did not start, with the token hidden

  @SIGNIN-13 @user @manual
  Scenario: The desktop app's menu
    # cmd/agentry-desktop/main.go menu
    # docs/decisions.md, 2026-10-26
    Given Agentry's page in the desktop app's window
    When the user picks Reload, Zoom in, Zoom out, Actual size, Open in browser and Quit, or presses Ctrl (Cmd on a Mac) with R, =, -, 0, B and Q
    Then each does what it says, and Open in browser opens the same page signed in in the default browser: yes
    And the zoom is kept after Reload: yes
    When the user right-clicks in the window
    Then no webview menu is shown, and copy and paste still work from the keyboard: yes

  @SIGNIN-14 @user @manual
  Scenario: Closing the desktop app's window quits it, and a second launch shows the first
    # cmd/agentry-desktop/main.go options, SingleInstanceLock, OnShutdown, quitOnSignal, exitCode and loaded; cmd/agentry-desktop/desktop.go agentry.stop and markQuit
    # a quit before or during a start: internal/launch/launch_test.go TestEnsureStartsNothingAfterQuit and TestQuitStopsTheServerEnsureWaitsFor; cmd/agentry-desktop/desktop_test.go TestAQuitClosesLaunchQuitOnce and TestStartReplyAfterAQuitSaysTheAppIsQuitting
    # docs/decisions.md, 2026-10-26
    Given the desktop app open, on Linux, Windows or a Mac
    When the user closes its window
    Then the app quits, and a server it started stops with it: yes
    When the app is ended with SIGINT or SIGTERM instead, at any moment from launch on, even before its window opens
    Then the app quits, and a server it started stops with it: yes
    And if the signal comes before the start page asks, no server is started: yes
    And if it comes while a start is under way, the server being started is stopped at once: yes
    And a second SIGINT or SIGTERM while it quits ends the app at once, with exit status 130 or 143 unless the first quit has just finished, even when the first came before its window opened: yes
    And a server it started has been told to stop by then, unless that server was still starting within moments of the first signal: yes
    Given the desktop app open
    When the user opens it again, even after `agentry stop`, or opens it twice at once
    Then no second window or app is started, and the first window is shown
    And the second launch starts no server, and none is left running without an app: yes

  @SIGNIN-15 @user @test
  Scenario: The server the Mac desktop app starts finds claude where the user's shell does
    # cmd/agentry-desktop/desktop.go serverPath, shellPath and agentry.start, set in cmd/agentry-desktop/main.go run; internal/procs KillGroupOnCancel; internal/launch Options.Path, Start and DefaultRepo
    # cmd/agentry-desktop/desktop_test.go TestServerPathOnAMacIsTheShells, TestServerPathElsewhereIsTheAppsOwn, TestServerPathKeepsTheAppsOwnWhenTheShellGivesNone, TestShellPathGivesUpOnAShellThatHangs and TestServerPathIsAskedOnlyWhenTheAppStartsAServer; internal/launch/launch_test.go TestEnsureGivesTheServerAndGitThePath and TestLookPathInSkipsEntriesThatArentAbsolute; internal/procs/procs_unix_test.go TestKillGroupOnCancelStartsANewSession
    Given the desktop app opened from Finder or the Dock on a Mac, with launchd's PATH /usr/bin:/bin:/usr/sbin:/sbin, and claude in ~/.local/bin or /opt/homebrew/bin, which the user's shell files add to PATH
    When the app starts its own server
    Then the server and the git the app runs get the PATH the user's login shell sets up, so agents find claude: yes
    And it is given to them as their own PATH; the app's own is left as it was: yes
    And the PATH is asked of $SHELL (zsh when unset) once, just before the app starts its own server, and not when a server is already running: yes
    And what the shell prints before or after the PATH is left out of it: yes
    And the shell is given up on within 5 seconds, and the programs it started are stopped with it: yes
    And the shell runs in a session of its own, so it answers the same when the app is run from a terminal, and leaves that terminal alone: yes
    And if the shell fails, hangs, or prints no PATH, an empty one or one on more than one line, the app's own PATH is kept: yes
    And on Linux and Windows the app's own PATH is kept, and no shell is run: yes

  @SIGNIN-16 @user @test
  Scenario: install.sh puts the Linux desktop app in the applications menu
    # packaging/files/install-linux.sh, shipped as Agentry/install.sh by packaging/desktop.go packLinuxDesktop
    # packaging/desktop_install_test.go TestInstallShPutsTheAppInTheMenuByFullPaths, TestInstallShReplacesTheAppWhileItRuns, TestInstallShQuotesExecAndFollowsXDGDataHome and TestTheScriptsWorkWithCDPATHSet
    # README.md, Desktop app
    Given the Linux desktop .tar.gz unpacked, and a home folder whose path holds a space
    When the user runs ./install.sh, from any folder
    Then agentry-desktop, agentry and uninstall.sh are copied side by side to ~/.local/share/agentry, all 3 executable: yes
    And agentry.png is copied to ~/.local/share/icons/hicolor/512x512/apps, and agentry.desktop to ~/.local/share/applications: yes
    And the menu entry names the app and the icon by their full paths, so nothing needs to be on the PATH: yes
    And it prints each file it installed and the one command that uninstalls, ~/.local/share/agentry/uninstall.sh: yes
    When the user runs ./install.sh again, even while the app runs
    Then the same files are replaced, each copied to a new name and then renamed, and no temporary copy is left: yes
    Given XDG_DATA_HOME set to a folder whose path holds a space, a quote, a dollar sign, a backslash and a percent sign
    Then the files go under XDG_DATA_HOME instead, and the entry's Exec quotes that path as the Desktop Entry spec says: yes
    Given CDPATH set, and the script run by a relative path
    Then it installs the same files: yes

  @SIGNIN-17 @user @test
  Scenario: uninstall.sh takes the Linux desktop app out of the menu and keeps the data
    # packaging/files/uninstall-linux.sh, shipped as Agentry/uninstall.sh by packaging/desktop.go packLinuxDesktop
    # packaging/desktop_install_test.go TestUninstallShRemovesWhatInstallShWroteAndKeepsTheRest, TestUninstallShRemovesTheHandCopiesOnlyWhenTheyAreAgentrys, TestTheInstalledUninstallShWorksWithoutTheDownload and TestTheScriptsWorkWithCDPATHSet
    # README.md, Desktop app, Uninstall
    Given the desktop app installed by install.sh, data in ~/.agentry, and an agentry in ~/.local/bin, as Agentry's worker install puts it
    When the user runs ./uninstall.sh in the unpacked folder
    Then every file install.sh wrote is removed, and the folder ~/.local/share/agentry with them: yes
    And ~/.agentry and ~/.local/bin/agentry are kept, and it says the data in ~/.agentry stays and why ~/.local/bin/agentry stays: yes
    Given the unpacked folder deleted, and XDG_DATA_HOME no longer what install.sh had
    When the user runs ~/.local/share/agentry/uninstall.sh, the copy install.sh put there
    Then it removes the same files from its own folder's install, itself last: yes
    Given the copies the README of v0.20.0 had the user make by hand: agentry-desktop and agentry in ~/.local/bin, the icon and the menu entry
    When the user runs either uninstall.sh
    Then agentry-desktop, the icon and the menu entry are each removed when they are Agentry's, and agentry is kept: yes
    Given nothing installed
    When the user runs ./uninstall.sh
    Then it removes nothing, says so, and exits 0

  @SIGNIN-18 @user @test
  Scenario: uninstall.sh keeps every file that may not be only the desktop app's
    # packaging/files/uninstall-linux.sh, the comments on which copies are Agentry's
    # packaging/desktop_install_test.go TestUninstallShRemovesTheHandCopiesOnlyWhenTheyAreAgentrys, TestUninstallShPipedInRemovesTheInstallAndNotTheFolderItRunsIn, TestUninstallShWithoutInstallShBesideItKeepsTheDownload and TestUninstallShKeepsAFolderWhoseAppIsNotAgentrys
    Given ~/.local/bin/agentry-desktop whose build info does not name Agentry's desktop app
    When the user runs ./uninstall.sh
    Then it is kept: yes
    Given ~/.local/bin/agentry, even the same, byte for byte, as this release's, beside Agentry's agentry-desktop
    Then that agentry is never removed, since Agentry's Claude Code and worker installs put the same program there, and it says so: yes
    Given a menu entry named agentry.desktop that is not the app's window, with no StartupWMClass=Agentry line
    Then that entry is kept: yes
    Given uninstall.sh piped into sh from a folder holding a file named agentry, or run from the unpacked folder with its install.sh deleted
    Then that folder is not taken for the install: it removes the install under XDG_DATA_HOME, or ~/.local/share, and keeps every file in that folder: yes
    Given a folder named agentry, holding uninstall.sh and no install.sh, whose agentry-desktop is not Agentry's
    When the user runs uninstall.sh there
    Then every file in that folder is kept, since a folder's files are removed only when its agentry-desktop is Agentry's: yes
