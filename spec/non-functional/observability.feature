Feature: Observability
  What Agentry shows about itself: tokens, turns, events, the status line.

  @OBS-01 @test
  Scenario: A task's tokens are the sum of its turns'
    Given a task whose 2 turns have ended, each with the tokens its engine counted from its transcript
    When the task is read from the log
    Then the task's tokens used are exactly the sum of the 2 turns' tokens
    # not under test, checked by hand: the sum is shown on the task's page (ui/src/pages/TaskPage.tsx line 373), in the list (ui/src/pages/TasksPage.tsx line 116) and on its card (ui/src/components/molecules/TaskCard.tsx line 78)
    # PLAN.md T-051; README.md, Use it, Budgets and turn limits; internal/tasks/split_test.go TestATurnWithOnlyATotalHasNoSplit

  @OBS-02 @test
  Scenario: The catalogue refuses a proposal of the wrong shape or from the wrong writer
    Given a catalogue that knows each type's stream kind, required fields and writers
    When a proposal arrives with the unknown type "task.exploded", one with a required field missing, one with no actor, a turn.started with no correlation, an agents event in a task stream, and one from a kind of actor the type does not allow
    Then the first 5 are refused with the check "shape", and the last with the check "who"
    And a task.created with its title, repo and coordinator from a human is not refused: yes
    # not under test, checked by hand: the store runs both checks on every proposal before it writes one (internal/store/store.go lines 341 and 344), and the shape check wants an actor's kind and id (internal/events/catalogue.go line 367), so every event in the log names who wrote it
    # README.md, How it works, How it fits together; PLAN.md T-002; internal/events/catalogue_test.go TestCheckShapeAndWriter

  @OBS-34 @test
  Scenario: A feature's stream prefix or event type that would collide with the core's or another feature's is refused
    # internal/events/catalogue_test.go TestNewCatalogueRefusesWhatWouldCollide and TestARegisteredPrefixIsNotATask
    Given the Ideas feature registered with its stream prefix idea-
    When another registration takes the prefix idea-, a prefix overlapping another's, a prefix a core stream starts with, a prefix task- starts with or one that starts with task-, a type both in its streams and in tasks', or stream types with no prefix
    Then each is refused, naming the prefix or the type as taken, the prefix as overlapping task-, or the types as needing a stream prefix
    And a task type a feature registered is allowed in a task stream and refused in an idea stream, and a catalogue without that registration refuses it: yes
    And a stream that starts with idea- is not a task stream: yes
    # PLAN.md T-088; README.md, Ideas (GET /api/features)

  @OBS-03 @test
  Scenario: A refused proposal is kept in the log as a rejection, not as the event
    Given a task whose task.created is in the log
    When a second task.created for it is proposed, and then a proposal of the unknown type "task.exploded"
    Then the second task.created is refused with the check "order" and the unknown type with the check "shape"
    And each refusal is written to the log as a proposal.rejected event, the first saying who sent it and the reason "task already exists": yes
    And the task's stream of accepted events still holds exactly 1
    # not under test, checked by hand: a rejection is in the task's stream, by system:validator, with the type and the check (internal/store/store.go lines 397 and 399)
    # README.md, How it works, What happens to one action; PLAN.md T-003; internal/store/store_test.go TestProposeAppendsOrRecordsTheRejection

  @OBS-04 @test
  Scenario: Every event type has a schema file its events validate against
    Given the catalogue with the Ideas feature, and the schema files under internal/events/schemas, one per type
    When an event with just what its type needs is checked against its file, and events with an empty actor id, a missing data.agent, a position of 1.5 and an "at" of "yesterday" are checked
    Then the event with just what its type needs passes for every one of the catalogue's types
    And each bad event is refused with a problem that names the field: "actor.id: want at least 1 characters", "data.agent: required field missing", "position: want an integer" and "at: want a date-time"
    And the unknown type "task.exploded" has no spec and no schema file
    # not under test, checked by hand: there are 81 files, one per type (ls internal/events/schemas); the store checks proposals with the catalogue (internal/store/store.go lines 341 and 344), and only tests call Validate (internal/events/validate.go line 20)
    # PLAN.md T-036; internal/events/validate_test.go TestAnEventValidatesAgainstItsSchema, TestAnEventValidatesAgainstSchemasOwnOutput and TestEveryCommittedSchemaChecksAnEvent

  @OBS-05 @test
  Scenario: A task's events API is its whole record, in order, rejections included
    Given a task with 600 label events and 1 refused proposal among them, and another task's events between
    When the page walks GET /api/tasks/{task}/events from ?after=0, each page after the last position of the one before, with the default page and with ?limit=7
    Then the pages add up to exactly the task's record, in position order, the rejection included and no other task's event
    And each page says with "more" whether events lie beyond it, yes or no
    # not under test, checked by hand: each entry carries its position, time, type, actor as kind:id, correlation and data (internal/web/views.go lines 122 to 129); the test builds the record it compares with through the same entryOf
    # README.md, Use it, A task; internal/web/events_api_test.go TestTheEventPagesAddUpToTheTasksRecord; the rejection and "more": TestAnEventPageIsBoundedAndSaysWhetherThereIsMore

  @OBS-06 @test
  Scenario: A task's event pages read the newest events first and earlier pages on request
    Given a task's event pages watched from log position 10, and a log with 7 events for it
    When they are watched, and then an earlier page is asked for until there is nothing earlier
    Then the first read is the page before position 11, and no second read follows while the log stays at 10
    And each earlier page read is the page before the oldest event held, and a second ask while one is read asks nothing more
    And once the task's first event is held, there is no earlier page to offer
    # not under test, checked by hand: the Activity and Events tabs read through these pages, and their Load earlier button asks for the earlier page (ui/src/live/use-events.ts line 20)
    # README.md, Use it, A task; ui/src/live/event-pages.test.ts "reads the newest page, before where the detail was read, and nothing more while the log stays" and "load earlier reads the page before the oldest held, until there is none"

  @OBS-07 @test
  Scenario: Activity is everything that happened, newest first, under bands that say who wrote it
    Given a task with 3 turns, 2 messages from you, a note from Agentry and a message from another team's coordinator
    When the Activity feed is shown
    Then the rows come newest first, in 8 runs, each under a band naming its author: an agent with its team and role, "you", or "Agentry"
    And a turn's band names the turn, the machine it ran on, whether it ended or runs, its tokens as "41k tokens" and its duration as "5m 00s"
    And clicking a turn's band asks for that turn alone, and narrowed to it the feed shows only that turn's band: yes
    But a task with no rows yet says "Nothing here yet"
    # not under test, checked by hand: the Activity tab shows this feed (ui/src/pages/TaskPage.tsx line 490)
    # README.md, Use it, A task; ui/src/components/organisms/ActivityFeed.test.tsx "names every band's author as copy.md says, newest first", "is one list of runs, one item each, with its band and its rows; no regions among them", "puts the turn's facts in the band: badge, tokens, and the duration for a phone", "narrows to the turn from its band, and from its continued band", "narrowed, the band is the turn's heading alone, with no Show only" and "a new task"

  @OBS-08 @test
  Scenario: A coordinator's handoff shows under its turn and offers the turn it led to
    Given a coordinator's turn that assigned the engineer on a named machine, and the engineer's turn after it
    When the Activity feed is shown
    Then the handoff is shown as the newest row under the coordinator's turn band, reading "coordinator to engineer on" the machine it named, with the instructions it gave
    And it carries a button "Show only turn-2": yes
    # not under test, checked by hand: the button narrows the feed to that turn (ui/src/components/organisms/ActivityFeed.tsx line 410); the Activity tab shows this feed (ui/src/pages/TaskPage.tsx line 490)
    # README.md, How it works, How a task moves; ui/src/components/organisms/ActivityFeed.test.tsx "keeps the coordinator's handoff under its turn's band, from → to with ' to ' said"

  @OBS-09 @test
  Scenario: When the events cannot be read, Activity says so and offers Try again
    Given a task whose events read fails with "502 Bad Gateway"
    When the Activity feed is shown, Try again is clicked twice at once, and later a retry fails again
    Then an alert is shown that says "Couldn't load the activity", quotes the server's answer and adds "If it keeps failing, check that Agentry is still running."
    And the two clicks make exactly 1 read, with focus kept on the feed through the loading
    And a retry that fails shows the alert again, and one that succeeds shows the rows with no alert
    # not under test, checked by hand: the Activity tab shows this feed (ui/src/pages/TaskPage.tsx line 490)
    # README.md, Use it, A task; ui/src/components/organisms/ActivityFeed.retry.test.tsx "a failed read, then Try again, then the feed, with focus on it and no other read" and "a retry that fails again shows the alert again; two quick clicks read once"; the words: ui/src/components/organisms/ActivityFeed.test.tsx "an error, with what the server said and Try again"

  @OBS-10 @test
  Scenario: The handoff path counts an agent's turns in a row as one step
    Given a task whose turns went coordinator, engineer, engineer, reviewer
    When its handoff path is shown
    Then the engineer's 2 turns in a row are one step marked "×2", in muted ink
    And the chevrons between the steps are hidden from screen readers, exactly 2 of them when the task is done
    # not under test, checked by hand: the task page shows the path above its tabs (ui/src/pages/TaskPage.tsx line 438)
    # README.md, Use it, A task; ui/src/components/molecules/HandoffPath.test.tsx "counts an agent's turns in a row in muted ink, not faint" and "hides its chevrons from screen readers"

  @OBS-11 @test
  Scenario: The Handoffs timeline is every step of the trail, newest first, each offering the turn it led to
    Given a task whose trail has steps from you to the coordinator, and from the coordinator to the engineer and to the reviewer, with the engineer's turn ended and the reviewer's running
    When the handoff timeline is shown
    Then each step is an article named "<from> to <to>, <time>", newest first
    And the footer of the step to the engineer reads "Led to turn-4·engineer·48m 00s·14 actions·210k tokens", is named "Led to turn-4, engineer. Open its activity" for screen readers, and a click asks to open turn-4: yes
    And the step to the reviewer is shown as running in its footer, or as usage limit while its turn waits for one
    And a step with no turn, such as the user's to the coordinator, has no button
    # README.md, Use it, A task; ui/src/components/organisms/HandoffTimeline.test.tsx "draws each step as an article named by who passed it to whom, and when", "says Led to with the facts of an ended turn", "opens the turn a footer names", "puts the running badge in a running turn's footer, and in its name", "says a turn waiting for the usage limit is waiting, not running" and "makes the footer the card's only button, and no button wraps the card"

  @OBS-12 @test
  Scenario: The coordinator's brief carries the trail, numbered, as who handed what to whom
    Given a task "Remove the TODOs" the coordinator assigned to the engineer, whose turn ended with the report "Removed 3 TODOs and committed."
    When the coordinator's briefs are written
    Then the first holds "1. you → tech-lead: Remove the TODOs"
    And the one after the engineer's turn holds "engineer → tech-lead: Removed 3 TODOs and committed."
    # not under test, checked by hand: the trail is under the section "What has happened so far", every step numbered as "N. from → to: text" (internal/coordinator/brief.go lines 50 and 124)
    # README.md, How it works, How a task moves; PLAN.md T-032; internal/coordinator/loop_test.go TestTheLoopTakesATaskRoundTheCoordinator

  @OBS-13 @test
  Scenario: A viewer of the live terminal gets the screen so far, then the live output
    Given a task whose agent's session printed "agent-ready" before anyone watched
    When 2 viewers connect to the task's terminal
    Then each gets the screen so far, holding "agent-ready": yes
    And both get the session's state saying 2 viewers: yes
    And what the session prints after that reaches both: yes
    But a task with no running session answers HTTP 404, with no terminal to watch
    # not under test, checked by hand: the state, with who is in control and which viewer each is, comes first, then the screen, then the live output (internal/terminal/hub.go lines 129 and 130); the page's route is GET /api/tasks/{task}/terminal (internal/web/server.go line 181)
    # README.md, Use it, A task; PLAN.md T-018; internal/terminal/terminal_test.go TestOnlyTheViewerInControlCanType and TestNoSessionNoWatch

  @OBS-14 @test
  Scenario: Only the viewer in control types, and a viewer is let go when the session ends
    Given 2 viewers of a task's terminal, neither in control
    When the first types, then takes control, the second asks to take it too, the first types again, and later the session ends
    Then the keys typed while only watching never reach the session
    And the second's take is refused while the first holds control
    And the keys typed in control reach the session and show on every viewer's screen, both of them
    And when the session ends, the viewer waited on is disconnected within 5 seconds, to reconnect to the next turn's
    # README.md, Use it, A task; PLAN.md T-018; internal/terminal/terminal_test.go TestOnlyTheViewerInControlCanType

  @OBS-15 @test
  Scenario: A turn running on a worker streams its terminal to the server
    Given a task moved to a worker, whose turn resumes an engine session there
    When a viewer watches the task's terminal through the server's terminal hub
    Then the screen streamed from the worker holds the engine's command with its resumed session, within 5 seconds
    And asked whether the task's agent runs there, the worker answers yes
    # README.md, Add machines; README.md, How it works, Work across machines; PLAN.md T-022; internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  @OBS-16 @manual
  Scenario: The status line says what needs you and what runs
    Given Claude Code set up with Agentry, so it runs `agentry statusline`, and a server with 1 question waiting and 2 tasks working, neither an epic
    When the status line is read, and again with 3 questions, with none, and with the server stopped
    Then it is shown as "agentry · 1 needs you · 2 running", the needs-you part in yellow
    And with 3 it is shown as "3 need you", and with none the line has no needs-you part, only "agentry · 2 running"
    And an epic, or a task working with no current agent, is not counted as running
    And with the server stopped, or no answer within 2 seconds, it prints nothing and exits 0
    # README.md, Use it from Claude Code; the words are cmd/agentry/mcp.go runStatusLine; no test runs it

  @OBS-17 @test
  Scenario: The client on the server's machine reaches it through server.json and its CA
    Given a data folder where client.Save wrote server.json with an https url and the server's own CA, beside the page token
    When client.Local reads that folder and calls GET /api/config
    Then the call reaches the server at the url kept there, with the page token, answered with ok true: yes
    And without the CA from that file the same server is not trusted
    # not under test, checked by hand: agentry serve writes server.json at start, with the listen address too (cmd/agentry/serve.go line 334), and agentry statusline and agentry mcp, with no address or connection set, fall back to client.Local (cmd/agentry/mcp.go line 106)
    # README.md, Use it from Claude Code; internal/client/client_test.go TestLocalReachesAnHTTPSServerWithTheCAItSaved

  @OBS-18 @test
  Scenario: Agentry's news for Claude Code is only what is new since it started
    Given the news read once while 1 task was done already, and a worker online with a task on it
    When an epic finishes with a summary, its coordinator tells you something, and the worker goes offline
    Then the news holds the lines "done: epic-2 is done: Snake. The game runs.", "said: epic-2 (Snake): tech-lead says: Moved it to the desktop." and "offline: the machine <name> went offline; epic-2 waits until it is back"
    And the task done before is not in the news
    # not under test, checked by hand: Claude Code's notifications and agentry wait take their news from WaitForNew (internal/mcp/notify.go line 28, cmd/agentry/claudecode.go line 103)
    # README.md, Use it from Claude Code; internal/mcp/waiting_test.go TestTheNewsIsWhatIsNew

  @OBS-19 @manual
  Scenario: A desktop notification when something new waits, and the same news from agentry wait
    Given Claude Code running with Agentry's tools on Linux, on macOS and on Windows, and the page kept closed
    When a question starts waiting, a task finishes, a coordinator replies, and a machine with work on it goes offline
    Then each shows once as a notification: through notify-send with the app name Agentry on Linux, osascript on macOS, a PowerShell toast on Windows, and nothing on any other system
    And each has the title "Agentry: task-N needs you", "Agentry: task-N is done", "Agentry: task-N's coordinator", "Agentry: a machine went offline" or "Agentry: a new idea", and the body is the line and the task's page address
    And while the server is still starting, the watcher tries again every 15 seconds
    And `agentry wait` blocks until the same news, prints each line with the task's page address, and exits 0
    # README.md, Use it from Claude Code; internal/mcp/notify.go and cmd/agentry/claudecode.go runWait; no test covers either

  @OBS-20 @test
  Scenario: GET /api/waiting lists everything waiting for you, oldest first, then each feature's
    Given a server with a polls feature installed but not shown, an epic with a question from its coordinator, and a task with a push held for approval
    When GET /api/waiting?repo= is read with the repo, and before any of that was there
    Then before, the list is empty, "[]"
    And after, it holds exactly 3 entries in order: the approval "task-2: Waiting for you to approve a push (review_approvals)", the question "epic-1: tech-lead asks: Which layout?", then the feature's own line
    And each entry carries its key, its task, its kind, "needs you" or the feature's, and its line
    # README.md, Use it from Claude Code; PLAN.md T-053; internal/web/waiting_api_test.go TestWaitingIsTheCoreThenEachFeature

  @OBS-21 @test
  Scenario: A push goes once to every subscribed browser for each thing that newly waits
    Given 2 browsers subscribed, and an approval waiting already when the push sink starts
    When a new entry starts waiting, the sink checks twice, then the approval leaves and comes back
    Then the approval waiting at the start is not pushed
    And the new entry is pushed exactly once to each of the 2 browsers
    And 2 new entries at once are 2 pushes, and an entry that leaves and comes back is pushed again
    And a browser whose push fails does not stop the others from getting theirs
    # README.md, Install, Push; internal/webpush/sink_test.go TestWhatWaitsAtStartIsNotPushed, TestANewEntryIsPushedOnceToEachDevice, TestTwoNewEntriesAreTwoPushes, TestAnEntryThatLeavesAndComesBackIsNewAgain and TestADeviceThatFailsDoesNotStopTheOthers

  @OBS-22 @test
  Scenario: What a push carries
    Given a browser subscribed to pushes
    When an entry waiting on task-9 is pushed, and a message is made for an entry on task-1 whose line is 8,000 bytes
    Then the first message has the title "Agentry", the task "task-9", the url "/tasks/task-9", and a tag of 32 characters made from the entry's key
    And the second message's body is the line cut to at most 500 bytes ending in "…", exactly a prefix of the line
    And an entry with no task opens "/"
    And the same key gives the same tag each time: yes
    # not under test, checked by hand: the service worker shows a push under its tag (ui/public/sw.js line 38), so a repeat replaces the notification shown
    # README.md, Install, Push; internal/webpush/sink_test.go TestAnEntryWithALongKeyIsStillPushed, TestALongLineIsCutToFitAPush, TestTwoNewEntriesAreTwoPushes and TestATagIsShortAndTheSameForTheSameKey

  @OBS-23 @test
  Scenario: A browser the push service no longer knows is dropped
    Given 2 browsers subscribed
    When the push service answers 404 or 410 for the first browser's push
    Then that subscription is removed from the store and from push.json, and the second is kept
    And the sink goes on: the second browser gets every push after, and the dropped browser none
    # README.md, Install, Push; internal/webpush/send_test.go TestASubscriptionThePushServiceNoLongerHasIsRemoved; the sink: internal/webpush/sink_test.go TestAGoneDeviceIsDroppedAndTheOthersStillGetTheirs

  @OBS-24 @test
  Scenario: Push is offered only at an https public address, and keeps its data out of the log
    Given a server whose public address is unset, http, or https
    When GET /api/push is read and a browser subscribes at each
    Then push is available only at the https one, and only there is the public key given
    And the other two say why, "Start Agentry with -public-url https://" or "-public-url is http://...", and the subscription is refused with that reason
    And subscribing and unsubscribing write 0 events, and no event holds a subscription's address
    # not under test, checked by hand: serve sets it from -public-url (cmd/agentry/serve.go lines 98 and 328)
    # README.md, Install, Push; internal/web/push_api_test.go TestPushNeedsAnHTTPSPublicURL and TestPushDataStaysOutOfTheLog

  @OBS-25 @test
  Scenario: A turn's tokens are counted from the engine's transcript, this turn's part only
    Given a Claude Code session whose transcript holds earlier turns' entries and this turn's, each reply on several lines with the same message id
    When the turn ends and Agentry reads the transcript from the turn's start stamp
    Then only the entries at or after the start are counted, each message id once, in 4 parts: input, cache write, cache read and output
    And a turn's total is the 4 summed: 4 + 12,800 + 40,000 + 370 = 53,174
    And with no start stamp, as from an older worker, the whole transcript is counted
    And when every entry is older than the start, the tokens are 0 with the reason NoUsageThisTurn; a missing transcript gives TranscriptNotSaved; a Codex turn gives CodexTokensNotRead
    # not under test, checked by hand: the reasons read "no usage in this turn's part of the transcript", "transcript not saved" (internal/engines/claude.go lines 28 and 26) and "Codex's token use isn't read" (internal/engines/codex.go line 69)
    # README.md, Use it, Budgets and turn limits; PLAN.md T-051; internal/engines/tokens_test.go TestClaudeCountsOnlyThisTurnsTokens, TestClaudeCountsTheWholeTranscriptWithNoStart, TestClaudeSaysWhenNoneOfTheUsageIsThisTurns and TestCodexTokensStayUnread; internal/engines/engines_test.go for TranscriptNotSaved

  @OBS-26 @test
  Scenario: Each turn keeps its token split and its engine session from turn.ended
    Given a log where turn-1 ended with tokens 53,174, inputTokens 4, cacheWriteTokens 12,800, cacheReadTokens 40,000, outputTokens 370 and contextTokens 26,801, and turn-2 ended with only tokens 1,000
    When the task is read from the log
    Then turn-1's split is exactly 4, 12,800, 40,000 and 370, its context 26,801, and its engine session the one session.started named
    And turn-2 has no split, no session and a context of 0: nothing is made up
    And the task's split is turn-1's and its unsplit tokens are exactly 1,000
    # PLAN.md T-051; internal/tasks/split_test.go TestATurnKeepsItsEngineSessionAndSplit and TestATurnWithOnlyATotalHasNoSplit

  @OBS-27 @test
  Scenario: The task page shows each turn's split under its total
    Given a task whose turn-1 used 53,174 tokens split 4, 12,800, 40,000 and 370, and whose turn-2 used 1,000 with no split
    When its Details tab is shown
    Then turn-1's row shows "53.2k" with "in 4", "cache write 12.8k", "cache read 40k" and "out 370" under it, exactly 4 parts
    And turn-2's row shows "1k" and no parts
    And the task page shows "in 4 · cache write 12.8k · cache read 40k · out 370 · 1k not split"
    # not under test, checked by hand: that line is in the Tokens fact under the header (ui/src/pages/TaskPage.tsx line 376)
    # README.md, Use it, Budgets and turn limits; ui/src/pages/TaskPage.test.tsx "shows each turn's split stacked under its total, and a turn with none as before" and "shows the task's split and what isn't split"

  @OBS-28 @test
  Scenario: A turn whose tokens could not be counted says why
    Given a task with turn-1 ended with its tokens unknown for "transcript not saved" and turn-2 counted at 1,234
    When the task's detail, as the API sends it, is made
    Then turn-1 carries tokens 0 and tokensUnknown "transcript not saved", and turn-2 carries no reason
    And the task says "Tokens unknown on 1 turn: transcript not saved"; with 3 such turns of 2 reasons it says "Tokens unknown on 3 turns: transcript not saved (2), no tokens reported (1)"
    And a task every turn of which was counted says nothing of it, an empty string
    # README.md, Use it, Budgets and turn limits; internal/web/views_test.go TestATurnSaysWhyItsTokensAreUnknown and TestTheTaskSaysWhereItsTokensAreUnknown

  @OBS-33 @test
  Scenario: Each turn says how its session started: resumed, forked, from which story, or fresh and why
    # internal/web/split_view_test.go TestATurnSaysWhetherItResumedOnlyWhenKnown
    Given a task whose turns started fresh as the first turn, resumed its own session, resumed a sibling story's session, and forked a sibling story's session, and one turn from a log that said none of this
    When the task's detail, as the API sends it, is made
    Then each turn carries what its log said: freshBecause "first turn"; fromSession; fromSession with sessionFrom "task-1"; and those with forked true
    And the turn that resumed its own session names no story and no fork, and the turn from the older log carries none of them: yes
    Given a task's turns that resumed, forked, resumed from task-2 and forked from task-2, one that started fresh because its context was too big, and one whose log said neither
    When its Details tab is shown
    Then under each turn's tokens it reads exactly "resumed", "forked", "resumed from task-2" or "forked from task-2", with the session id shown on hover; "forked from task-2" links to task-2's page, and a plain "resumed" has no link
    And the turn that started fresh reads "fresh: " and its reason, and the turn whose log said neither reads nothing of how it started
    # Agentry task-206, sessions shared across an epic's stories (SESSION-02, SESSION-03); ui/src/pages/TaskPage.test.tsx "says which story a turn's session came from, and whether it forked it" and "shows under its tokens whether each turn resumed or started fresh, and nothing for a turn that didn't say"

  @OBS-29 @test
  Scenario: A worker that connects is listed online, with its details and history
    Given a worker joined with its code
    When it dials the server and says hello with its host and engines
    Then GET /api/workers lists it by name with "online": true: yes
    And GET /api/workers/{name} gives a host, a count of CPUs above 0, and a history whose first entry is "online"
    # not under test, checked by hand: the hello is kept as a worker.connected event in the workers stream with its name, host, os, arch, version, engines and missing tools (internal/workers/workers.go lines 186 to 188, written at internal/workerlink/server.go line 80), and the history is newest first (internal/web/workers_api.go line 108)
    # README.md, Add machines; PLAN.md T-022; internal/web/workers_test.go TestAWorkerJoinsAndTakesWork

  @OBS-30 @test
  Scenario: A worker that cannot trust the server logs each failed dial and the way back in, once
    Given a worker whose server now shows a certificate from another CA
    When it runs for 3.5 seconds
    Then it logs a line starting "link down" for each failed dial, 2 or more
    And it logs how to join again, with `agentry worker join -ca-fingerprint`, exactly once
    # not under test, checked by hand: it dials at 0, 1 and 3 seconds as the wait doubles, and the line goes on "; dialing again in" the wait (internal/workerlink/worker.go lines 82 and 88)
    # README.md, HTTPS and installing the app, Regenerate the CA; internal/workerlink/worker_test.go TestAWorkerWhoseCAChangedSaysOnceHowToJoinAgain

  @OBS-31 @manual
  Scenario: What a worker's log says
    Given a worker running as its service, with `agentry worker logs -f` open
    When it starts, connects, takes a turn, opens Claude in a task's Workspace, sends a shared file back, and is updated to the server's agentry
    Then the log shows, one line each: "<name>, working for <server>", "connected to <server>", "<task>: starting <turn>", "<task>: starting Claude in the Workspace", "sending <path> to the server, as a shared file" and "agentry was updated to the server's; restarting"
    And the log is kept in worker.log in the worker's folder, which `agentry worker logs` shows
    # README.md, Add machines; cmd/agentry/worker.go runAsWorker and internal/workerlink/worker.go; only the CA line of the log is tested, in OBS-30

  @OBS-32 @manual
  Scenario: A worker's link that goes silent is hung up and the worker recorded offline
    Given a worker connected, sending which tasks have a live agent on it every 2 seconds
    When its link carries nothing for 20 seconds, 10 status periods; and, apart, when a new link says no hello within 10 seconds; and when the server restarts
    Then the server closes the silent link and writes a worker.disconnected event, not when a newer link from the same worker replaced it
    And the link with no hello is closed with "say hello first", and no worker.connected is written for it
    And at the restart every worker the log had online is recorded offline, until it dials in again
    # README.md, Add machines; internal/workerlink/server.go, StatusEvery, silentFor, Connect and ForgetConnections; no test covers these; the 20-second offline mark on the Workers page is LIM-31; OfflineGrace, 30 seconds, is how long its turns still count as running (REL-41)
