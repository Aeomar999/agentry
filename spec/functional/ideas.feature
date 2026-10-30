Feature: Ideas
  The Visionaries suggest features as ideas; the user accepts or dismisses each one.

  # ---- The Ideas page --------------------------------------------------------

  @IDEA-01 @user @manual
  Scenario: Ideas is in the sidebar under Work, with the count of open ideas
    # the Work section, Tasks then Plan, then each feature's item with its badge: ui/src/components/organisms/Sidebar.tsx lines 14 to 22 and 65 to 77
    # the Ideas item: ui/src/extensions/ideas/index.tsx line 10; its badge counts the open ideas: internal/ideas/routes_test.go TestIdeasBadgeCountsOpenIdeas
    # the page header: ui/src/extensions/ideas/IdeasPage.tsx lines 64 to 66
    Given a Visionaries team and 2 open ideas
    When the sidebar is shown
    Then Work lists exactly 3 items in order: Tasks, Plan, Ideas
    And the Ideas item carries the count 2, and Tasks the count of tasks that need you
    When the user opens Ideas
    Then the page is titled "Ideas" and exactly this line is shown under it: "Features the Visionaries suggest, from sessions on a theme or a question. Accept one to make it a task in your Backlog, or accept and start it."
    And the header shows exactly 1 button, "Ask the visionaries"

  @IDEA-02 @user @test
  Scenario: Open ideas are grouped by the session that suggested them, newest session first
    # internal/ideas/routes_test.go TestIdeasListNewestSessionFirst
    # the groups on the page: ui/src/extensions/ideas/IdeasPage.tsx lines 106 to 124 and 304 to 312, sessionsOf
    Given task-1, an older session in one repo, and task-2, a newer session in another
    And task-2 suggested an idea, then task-1 one, then task-2 another
    When the ideas are read
    Then they come in exactly the order task-2's second, task-2's first, task-1's
    And each is open, with an empty backedBy and no decidedAt
    When one repo's ideas are read with ?repo=
    Then exactly 1 idea comes back, task-1's
    When a repo with no ideas is read
    Then the list is empty

  @IDEA-03 @user @manual
  Scenario: An idea's card: the problem, the proposal, why now, the size, the risks and who backed or doubted it
    # ui/src/extensions/ideas/IdeaCard.tsx lines 25 and 26, the badges; lines 31 to 37, the parts; line 39, Not auto-accepted; lines 41 to 55, the buttons and links; line 97, "no one"
    # the sizes S, M and L read small, medium and large: ui/src/extensions/ideas/IdeaCard.tsx line 13
    # README.md, Ideas
    Given an open idea of size M, backed by product-strategist and technologist, doubted by skeptic, that follows up epic-7 and was suggested by task-12
    When its card is shown
    Then the title is shown with the badge "medium", and the badge "epic" only when the idea suggests an epic
    And exactly 4 parts are shown in order, "Problem", "Proposal", "Why now" and "Risks", a part with no text hidden
    And "Backed by" lists exactly 2 agents and "Doubted by" exactly 1; with nobody it reads "no one"
    And the line "Not auto-accepted: doubted by skeptic" is shown
    And the foot shows 2 buttons, Accept and Dismiss, then "follows up epic-7" and "from task-12", in which only epic-7 and task-12 are links, each to that task

  @IDEA-04 @user @manual
  Scenario: The History panel, and the page with every idea decided
    # ui/src/extensions/ideas/IdeasPage.tsx lines 86 to 104, the empty states; lines 137 to 145, History; lines 274 to 301, a row
    # README.md, Ideas: the history, "Auto-accepted" with its note on hover
    Given 3 ideas: 1 accepted as task-9, now In progress, 1 Agentry auto-accepted and 1 dismissed with a note
    When the Ideas page is shown
    Then "No open ideas: you have decided on every one. Ask the visionaries for more." is shown above the history
    And the panel "History" reads "3 decided" and lists each idea with its badge, accepted or dismissed
    And the accepted row reads "became task-9 (In progress)", "from task-N" and the date; the dismissed row shows its note under it
    And the auto-accepted row wears the badge "Auto-accepted", its note shown on hover
    Given no ideas at all and a Visionaries team
    Then "No ideas yet" is shown with the button "Ask the visionaries"

  @IDEA-05 @user @test
  Scenario: Ideas shows while a Visionaries team or an idea is there, and the page says what it is otherwise
    # internal/ideas/routes_test.go TestIdeasHiddenWithNoTeamAndNoIdeas, TestIdeasOnWithTheVisionariesTeam, TestIdeasKeptForTheirIdeasWhenTheTeamIsRemoved and TestIdeasBadgeCountsOpenIdeas
    # ui/src/extensions/ideas/view.test.ts "is off with no ideas and no team, and on with the team", "is on with ideas and no team" and "settles when no template turns Ideas on, on with ideas"
    # internal/ideas/extension_test.go TestIdeasIsOnByTheTeamsTemplate, for the why
    # not under test, checked by hand: the Ideas page's off view, which shows what Ideas is and only the button "Ask the visionaries", or with no template that turns Ideas on reads "This Agentry has no team template that turns Ideas on, so there is no team to ask." and offers no button: ui/src/extensions/ideas/IdeasPage.tsx lines 155 to 167; ui/src/extensions/ideas/AskVisionaries.tsx lines 10 to 24
    Given no Visionaries team and no ideas
    When the features are read
    Then ideas is not enabled and not visible, its why "add a team from the Visionaries template, or one of yours based on it, to get ideas"
    When the Visionaries team is added
    Then ideas is enabled and shown: "the Visionaries team suggests ideas"
    When the team is removed while 2 ideas are left
    Then ideas is visible but not enabled: "kept while 2 ideas remain; no team from the Visionaries template or one based on it now"
    Given 4 ideas, 1 accepted and 1 dismissed
    Then the badge is exactly 2

  @IDEA-06 @user @manual
  Scenario: Ask the visionaries opens New task with the team picked, adding the team first when you have none
    # ui/src/extensions/ideas/visionaries.ts lines 49 to 75, ask; ui/src/extensions/ideas/IdeasPage.tsx lines 58 and 72, the button's label
    # ui/src/lib/new-task.ts: ?new=task&newTeam=<team>; README.md, Ideas, "Ask the visionaries"
    Given the Ideas page and a team made from the Visionaries template
    When the user clicks "Ask the visionaries"
    Then the New task dialog opens with that team picked and the address carries exactly ?new=task&newTeam=Visionaries
    Given no such team yet
    Then the button shown reads exactly "Add the Visionaries team"
    When the user clicks it
    Then the team is added from its template once, the button reads "Adding the team…" meanwhile, and New task opens when the team shows
    When the policy lacks the profile the team needs
    Then the banner "The Visionaries team needs the read-mostly profile, which your policy file doesn't have" is shown with "Add the profile and the team"
    When the team was added but does not show up
    Then the banner "The Visionaries team isn't there" is shown, reading "The team was added but didn't show up; try again."

  # ---- Accepting and dismissing ----------------------------------------------

  @IDEA-07 @user @test
  Scenario: Accept makes a task or an epic in the idea's repo, in your Backlog, with your note
    # internal/ideas/ideas_test.go TestAcceptMakesABacklogTaskInTheIdeasRepo and TestAnIdeaIsAcceptedOrDismissedOnce; internal/ideas/routes_test.go TestYouAcceptAnIdeaOnce
    # README.md, Ideas
    Given an open idea "Search the board" in a repo, suggested as a task
    When the user accepts it as an epic with the note "Yes, soon"
    Then task-2 is made, kind epic, parked in backlog, in the idea's repo, for the coordinator tech-lead
    And its description names each of the idea's id, its title "Search the board", its proposal and the note "Yes, soon"
    And the idea is accepted, became that task, with the note "Yes, soon" kept
    And the reply is exactly task-2, column backlog, kind epic, accepted true
    When the user accepts it again
    Then it is refused with 409 "accepted already"
    When the user dismisses it instead
    Then it is refused with 409

  @IDEA-08 @user @manual
  Scenario: The Accept dialog: task or epic, the note, the token budget and where the task went
    # ui/src/extensions/ideas/IdeaDialogs.tsx line 63, the title; lines 95 to 109, Make it; line 110, Note; lines 113 to 127, Token budget; lines 130 to 139, the buttons
    # what was made: lines 64 to 75 and 176 to 191, placeOf; the toast: line 53
    # the budget preset to the epic's for an idea that follows one up, else the server's: lines 160 to 171, untouchedBudget
    Given an open idea "Dark mode" that follows up epic-7, whose budget is 2,000,000 tokens
    When the user clicks Accept on its card
    Then a dialog "Accept “Dark mode”" is shown with "Make it" offering exactly 2 choices, "A task" and "An epic", the suggested one pressed
    And the hint shown for a task is exactly "The session suggests one task: one change, on one branch."
    And "Note" is optional, "Kept with the idea, in its history."
    And "Token budget" is preset to exactly 2,000,000, the hint starting "From epic-7."
    And exactly 3 buttons are shown: "Accept as a task", "Accept and start" and Cancel
    When the user clicks "Accept as a task"
    Then the dialog reads "Made task-N from it. In your Backlog until you start it." with only a Close button, the task linked
    And a toast "Made task-N from the idea" is shown once
    Given the task was made but the idea not marked accepted
    Then the banner "The idea wasn't marked accepted" is shown with the server's reason

  @IDEA-09 @user @test
  Scenario: Accept and start makes the task In progress, with the budget you give
    # internal/ideas/routes_test.go TestYouAcceptAndStartAnIdea and TestATaskFromAnIdeaYouAcceptTakesTheServersLimits
    # internal/ideas/ideas_test.go TestAcceptAndStartMakesATaskOutOfTheBacklog and TestAgentryCantAcceptAndStart
    # 5,000,000 tokens and 12 turns are the test server's limits: internal/ideas/routes_test.go line 256
    Given a server whose defaults are 5,000,000 tokens and 12 turns, and open ideas to accept
    When the user accepts 1 with start and a budget of 700
    Then the task is made In progress, not parked, with a budget of exactly 700 and the server's 12 turns
    When the user accepts 1 with start and no budget
    Then its budget is exactly 5,000,000
    When the user accepts 1 with start and a budget of -1
    Then its budget is 0, which is no limit
    When the user accepts 1 without start and a budget of 0
    Then the task is parked in backlog with the budget 5,000,000
    When an accept sends start as "yes" or 1, or a budget of -2 or "lots"
    Then each is refused with 400 and no task is made
    And every accepted idea's becameColumn is the column its task is in: in-progress or backlog
    Given Agentry accepting an agreed idea for you
    Then it never starts the task: an accept with start is refused and nothing is made

  @IDEA-10 @user @test
  Scenario: An idea that looks like an open task makes none until you say anyway
    # internal/ideas/routes_test.go TestAnIdeaLikeAnOpenTaskIsRefusedUntilYouSayAnyway
    # ui/src/extensions/ideas/IdeaDialogs.test.tsx "shows the open task an idea looks like, and makes another only when you say anyway" and "Cancel leaves the idea open, sending nothing more"
    # the titles share more than half their words, past the filler words: internal/ideas/duplicate.go line 121, SameTitleAbove 0.5
    # the banner "It looks like an open task", its words and the buttons: ui/src/extensions/ideas/IdeaDialogs.tsx lines 76 to 92; the test asserts the link and the requests, not the words
    Given the idea "Make a ship the user chose actually push, and count it shipped only after the push" accepted as task-2
    And the open idea "Make a ship the user chose push, and count it shipped only when it pushed" in the same repo
    When the user accepts the second
    Then it is refused with 409 "looks like the open task task-2", the body naming task-2, its title and its column backlog
    And nothing is written to the log
    And the dialog shows a link to task-2, and 1 request has been sent
    When the user clicks "Make another anyway"
    Then exactly 1 more request is sent, with anyway true, and task-3 is made in the Backlog
    When the user clicks Cancel instead
    Then the dialog closes and nothing more is sent: the idea stays open

  @IDEA-11 @user @test
  Scenario: Dismiss puts the idea in the history with your reason
    # internal/ideas/routes_test.go TestYouDismissAnIdea and TestAnUnknownIdeaIsNotFound; internal/ideas/ideas_test.go TestDismissKeepsYourNote
    # the dialog "Dismiss “<title>”" with Reason, Dismiss and Keep, and the toast "Dismissed “<title>”" with "It is in the history.": ui/src/extensions/ideas/IdeaDialogs.tsx lines 205 to 229
    # the words "no such idea": internal/ideas/routes.go lines 143 and 152; the test asserts the 404 only
    Given an open idea "Dark mode"
    When the user dismisses it with the note " The OS does it "
    Then the idea is dismissed with exactly the note "The OS does it" and a decidedAt
    When the user accepts it afterwards
    Then it is refused with 409
    When an unknown idea is accepted or dismissed, or a task's id dismissed
    Then each is refused with 404

  @IDEA-12 @claude-code @test
  Scenario: Only an open task in the idea's repo counts as the task it looks like
    Given an idea and tasks with close titles in its repo and in another, some done
    When the open task it looks like is found
    Then a done task counts: no, a task in another repo counts: no, and a task in the Backlog counts: yes
    And of 2 candidates the closest title wins, and on a tie the 1 made first
    # internal/ideas/duplicate_test.go TestOnlyAnOpenTaskInTheReposCounts

  # ---- Auto-accept and duplicates --------------------------------------------

  @IDEA-13 @user @test
  Scenario: Auto-accept is off until you turn it on, and starting them counts only with it
    # internal/web/settings_api_test.go TestYouTurnAutoAcceptingIdeasOnAndOff and TestYouChooseTheAutoAcceptRule
    # internal/ideas/agreed_test.go TestAgentryAcceptsOnlyWhatTheSettingAndTheVisionariesAllow and TestAgentryStartsOnlyWhileTheSettingsSay
    # the defaults: internal/settings/settings.go line 46
    Given settings never changed
    When they are read
    Then visionariesOnEpicClose is yes, autoAcceptIdeas and autoStartIdeas are no, and autoAcceptRule is everyoneBacked
    When the user turns autoAcceptIdeas and autoStartIdeas on
    Then both are saved and the reply holds every setting
    When the user turns autoStartIdeas off
    Then autoAcceptIdeas is kept on
    When autoAcceptIdeas is sent as "yes", or autoAcceptRule as mostBacked
    Then each is refused with 400, the rule's refusal naming everyoneBacked and nobodyDoubted, and the settings are kept
    Given auto-accept off and an idea every viewpoint backed
    Then Agentry accepts nothing, and the rules refuse an accept in its name
    Given auto-accept on
    Then an idea 1 viewpoint doubted, or 1 did not back, stays open for you; your own accept of it is allowed
    And with autoStartIdeas or autoAcceptIdeas off, a start in Agentry's name is refused

  @IDEA-14 @user @test
  Scenario: Which ideas count as agreed: Everyone backed it, or Nobody doubted it
    # internal/ideas/agreed_test.go TestAnIdeaIsAgreedWhenEveryViewpointBacksIt and TestAnIdeaNobodyDoubtedIsAgreedWithTwoBackers; internal/ideas/routes_test.go TestAnOpenIdeaSaysWhyItWasntAutoAccepted
    # the 4 viewpoints of the Visionaries template, product-strategist, user-advocate, technologist and skeptic, under visionary-lead: internal/agents/teams.go lines 71 to 79
    # the page's line: ui/src/extensions/ideas/IdeaCard.tsx line 39
    Given the Visionaries team with its 4 viewpoints
    When an idea is judged under "Everyone backed it"
    Then it is agreed only when every 1 of the 4 backed it and none doubted it
    And 1 doubter gives "doubted by skeptic", 1 missing backer "not backed by skeptic", and nobody "not backed by product-strategist, user-advocate, technologist, skeptic"
    And a team of only its coordinator agrees on nothing: "the Visionaries have no viewpoints"
    When an idea is judged under "Nobody doubted it"
    Then 2 backers and no doubter is agreed; 1 backer is "only one backer"; none is "no backers"
    And a backer outside the team, or named twice, does not count
    Given auto-accept off
    When the ideas are read
    Then each open idea says why under the rule set, such as "doubted by skeptic", and an agreed one says nothing

  @IDEA-15 @user @test
  Scenario: Agentry never accepts a duplicate, and says which idea it repeats
    # internal/ideas/agreed_test.go TestAgentryAcceptsNoIdeaYouHadBefore and TestTheRulesRefuseAgentryADuplicate; internal/ideas/duplicate_test.go TestAnIdeaLooksLikeAnOpenTaskWithACloseTitle
    # the same title once case, punctuation and filler words are dropped, or more than 35% shared words in the problems and proposals, each at least 10 words: internal/ideas/duplicate.go lines 66 to 72, 144 and 148
    # README.md, Ideas: "Under either rule Agentry never accepts a duplicate"
    Given an open idea "Search the board" every viewpoint backed
    When another idea " search the Board" in the same repo was dismissed earlier
    Then the open idea is not auto-accepted: "duplicate of idea-1"
    When the earlier idea was accepted and became task-64
    Then the reason is exactly "duplicate of idea-1 (task-64)"
    When the same title was suggested later and is open or dismissed
    Then this idea is the original: nothing stops it; a later one counts only once it was accepted
    When the same title is in another repo, or the Visionaries named the idea it repeats
    Then another repo's never counts, and a named one gives "duplicate of <that id>"
    And the rules refuse Agentry's accept of a duplicate even when every viewpoint agreed

  @IDEA-16 @user @manual
  Scenario: The settings on the Ideas page: follow-ups on an epic close, auto-accept, the rule and starting them
    # ui/src/extensions/ideas/IdeasPage.tsx lines 226 to 262, the controls; lines 180 to 184, the 2 rules and their hints; lines 264 to 267, Saving and the banners
    # README.md, Ideas: the ticks at the top of the Ideas page; the page has them, not Settings
    Given the Ideas page with a Visionaries team and settings never changed
    When the controls are shown
    Then "Ask the visionaries for follow-ups when you close an epic" is ticked and "Auto-accept ideas the Visionaries agree on" is not
    And "Which ideas qualify" offers exactly 2 rules, "Everyone backed it" and "Nobody doubted it", the first picked, both greyed out
    And "and start them" is greyed out and not ticked
    When the user ticks "Auto-accept ideas the Visionaries agree on"
    Then "Saving…" is shown once, and the rules and "and start them" can be changed
    When a change is refused
    Then the banner "The setting wasn't saved" is shown with the server's words, and the control shows what the server kept

  # ---- Where ideas come from -------------------------------------------------

  @IDEA-17 @agent @test
  Scenario: The Visionaries' ideas land in Ideas from the lead's done decision, for the user to decide
    # internal/ideas/ideas_test.go TestASuggestionComesFromATask; the done decision: internal/coordinator/ideas_test.go TestAVisionariesDoneKeepsItsIdeas; once: TestADoneNamingOneTitleTwiceKeepsOneIdea in the same file
    Given the visionary-lead's done decision carries "ideas", each with a title, the problem, the proposal and a size of S, M or L
    When the loop applies the decision
    Then each idea is kept once, from that task and about its repo
    And an idea with an empty title, a size "XL", a kind "story", from a task that does not exist, or from an agent not working on the task is refused
    And only the user accepts or dismisses an idea, never an agent
    # IdeasFormat and suggest: internal/ideas/coordinator.go; the user decides: TestOnlyYouAcceptOrDismiss in internal/ideas/ideas_test.go;
    # the skeptic's verdict closes the session: internal/agents/defaults/visionaries/skeptic.md
