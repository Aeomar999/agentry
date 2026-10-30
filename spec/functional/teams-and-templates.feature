Feature: Teams and templates
  The teams on the Agents page, teams of the user's own, templates and each agent's page.
  # Sources: internal/domain/agents, internal/web/agents_api.go,
  # internal/web/templates_api.go, ui/src/pages/AgentsPage.tsx,
  # ui/src/pages/AgentPage.tsx, ui/src/pages/AgentEditPage.tsx,
  # ui/src/components/organisms/TeamPanel.tsx,
  # ui/src/components/organisms/AgentForm.tsx, ui/src/lib/teams.ts,
  # ui/src/lib/templates.ts and their tests.

  # ---- The Agents page and its teams -----------------------------------------

  @TEAM-01 @user @manual
  Scenario: The Agents page: the teams as cards, an org chart or a list, each team with its 3 actions
    # ui/src/pages/AgentsPage.tsx lines 126 to 157, the header and the views; lines 276 to 298, a team's heading; line 96, Remove team only with more than 1 team
    # the list's 7 columns: ui/src/pages/AgentsPage.tsx lines 45 to 69; a card: lines 304 to 340; the org chart: lines 343 to 401
    # README.md, Use it, "Teams"
    Given the Software and Writing teams, 9 agents in all
    When the Agents page is shown
    Then the header has exactly 2 buttons, "Add a team" and "New agent", and the line reads "2 teams · 9 agents"
    And the views are Teams, "Org chart" and List, the address carrying ?view=org or ?view=list for the last 2
    And each team's heading shows its name, "5 agents" or "4 agents", "· <description>" from its template, then "Edit team", "+ Add agent" and "Remove team"
    And each card shows the agent, the badge "coordinator" on the lead, a Review or Shipping badge where its work shows, its description, engine · model, profile and where it runs
    When the user picks List
    Then exactly 7 columns read Name, Team, Role, Description, Engine, Profile, "Runs on"
    When the user picks "Org chart"
    Then each team shows its coordinator, badged "lead", over its agents; a team with none reads "This team has no coordinator yet."
    Given only 1 team
    Then "Remove team" is hidden on its heading

  @TEAM-02 @user @test
  Scenario: Add a team from a template: Build your own, then your templates newest first, then the built-in ones
    # ui/src/components/organisms/AddTeam.test.tsx "yours come between Build your own and Built in, newest first, each saying when and where it was saved", "only yours can be deleted or exported", "every card's actions are named with it, and its coordinator chip uses the brand ink" and "is added by its id"
    # ui/src/pages/AgentsPage.test.tsx "while ${request} runs, Esc and a click outside leave it open; once it ends, both close it", run for an add, a delete and an import
    # a built-in template is added by its team name, yours by its id: ui/src/components/organisms/AddTeam.tsx lines 75 to 81
    Given 3 built-in templates and 3 templates of yours, Orders, Papers and Desk, saved on 3 dates
    When the user clicks "Add a team"
    Then the dialog "Add a team" shows exactly 3 headings in order: "Build your own", "Your templates", "Built in"
    And exactly 7 cards come in the order "A team of your own", Orders, Papers, Desk, Software, Writing, Visionaries: yours newest first
    And Papers reads "Saved 12 Oct from the Reading team."
    And only yours have "Delete template" and an Export link to /api/teams/templates/papers/export; a built-in one has neither
    And each card's buttons are named with it: "Add this team, Software", "Change it first, Papers", and "Add another" for a team you have
    When the user clicks "Add this team, Papers"
    Then a request adds the template papers by its id, and a toast "Added the Papers team" with "Pick it on New task; its coordinator takes the task." is shown
    And while the add runs, Esc and a click outside leave the dialog open; once it ends, both close it

  @TEAM-03 @user @test
  Scenario: A team whose profile the policy lacks is refused, then added with the profile in one click
    # internal/web/policy_api_test.go TestATeamNeedsItsProfilesInThePolicy
    # ui/src/components/organisms/AddTeam.test.tsx "refused for its profiles, offers them in its own card and adds it with them"
    # the offer's words, "The Visionaries team needs the read-mostly profile, which your policy file doesn't have": ui/src/components/molecules/MissingProfilesOffer.tsx lines 20 to 29; the test asserts the card's alert and button, not the words
    Given a policy file without the read-mostly profile
    When the user adds the Visionaries team
    Then it is refused with 409, naming exactly 1 missing profile, read-mostly, and the Policy page, and nothing is proposed
    And an offer is shown in that card only, with the button "Add the profile and the team"
    When the user clicks it
    Then the next request sent carries addMissingProfiles and the same policy hash the page saw
    And the policy is changed exactly once, "added profile read-mostly for team Visionaries", and the team is there with its 4 agents under visionary-lead
    When a template that does not exist is named, or the hash is stale
    Then the first is refused with 404 and the second with 409, each as JSON with an error line
    When an agent is added with a profile nobody wrote
    Then it is refused with 400 "no profile called dreaming", never naming the policy file's path

  @TEAM-04 @agent @test
  Scenario: The Writing team has one coordinator and three agents, the fact-checker checking the work
    # internal/domain/agents/agents_test.go TestTeamsComeFromTemplatesWithTheirOwnCoordinator
    Given the Writing team added from its template
    When the roster is read
    Then its coordinator is the editor, and it has exactly 3 other agents, the fact-checker last
    And a task that names no team still goes to the first team's coordinator, tech-lead
    And added again, the team "Writing 2" has the coordinator editor-2: yes
    # not under test, checked by hand: nothing the user has is renamed
    # internal/domain/agents/teams.go AddTeam, MadeFor

  # ---- Teams of your own -----------------------------------------------------

  @TEAM-05 @user @test
  Scenario: Build a team of your own: the New team panel, blank or from a copy of a template
    # ui/src/components/organisms/TeamPanel.test.tsx "Build a team in Add a team opens the panel with one coordinator" and "creates the team as typed, the coordinator on the default prompt"
    # internal/domain/agents/custom_test.go TestACustomTeamGetsDefaultPrompts
    # the panel "New team" at ?team=new: ui/src/pages/AgentsPage.tsx lines 114 to 117; "Team name", "Start from" with Blank and "Copy a template", "Create team": ui/src/components/organisms/TeamPanel.tsx lines 63, 174, 202 and 289 to 297
    Given the Add a team dialog
    When the user clicks "Build a team"
    Then the dialog closes and the panel "New team" opens with the focus in "Team name", empty
    And "Start from" has Blank pressed, no Template list, and the row coordinator open and picked as "Coordinator, coordinator"
    And the foot reads "Creates the team and its coordinator." and the prompt's hint "Empty: it uses Agentry's coordinator prompt for a team of these agents."
    When the user types " Docs " and clicks "Create team"
    Then exactly 1 request is sent: the team "Docs" with 1 agent, coordinator, coordinates true, engine claude, colour blue, an empty prompt
    And the server gives an empty prompt its default: the coordinator's starts "You are the coordinator", an agent's names it as "one of the agents of the <team> team" and its first line ends "Your job: <its description>."
    And a prompt the user wrote is kept as written

  @TEAM-06 @user @test
  Scenario: What a team of your own is refused for, every problem at once
    # internal/domain/agents/custom_test.go TestACustomTeamIsRefused
    # ui/src/components/organisms/TeamPanel.test.tsx "an empty team name is in the summary, which takes focus, and under the field" and "every problem at once: team, agents and each row's fields"
    # an agent's name: lowercase letters, digits and dashes, starting with a letter, at most 31 long: internal/domain/agents/agent.go lines 18 to 22; the page's limit: ui/src/lib/custom-team.ts line 54
    Given the New team panel
    When the user clicks "Create team" with no name
    Then nothing is sent, the summary reads "Couldn't create the team: 1 thing to fix" and takes the focus, and "Give the team a name." is shown under the field
    When a team is sent with a name a team has, in any case, or a built-in template's team
    Then it is refused: "a team called Writing already", "a team called wRiTiNg already" or "NETWORKING is the name of the Networking template's team"
    When a team is sent with no agents, no coordinator or 2
    Then it is refused: "the Kitchen team needs agents", "exactly one coordinator; it has none", "exactly one coordinator; it has two: chef, taster"
    When an agent's name is blank, in upper case, 32 characters, repeated or in the roster
    Then each is refused: "every agent of the Kitchen team needs a name", "lowercase letters, digits and dashes, starting with a letter", "two agents called cook", "an agent called editor already exists"
    When an agent's column is serve or its colour mauve
    Then each is refused, the column being builds, review or ship only

  @TEAM-07 @user @test
  Scenario: Edit a team: saved whole in one request, its tasks following, with the profiles it needs
    # internal/web/team_edit_api_test.go TestATeamIsSavedWholeInOneRequest and TestATeamEditNeedsItsProfilesInThePolicy
    # the panel "Edit the <team> team" at ?team=<name>: ui/src/lib/custom-team.ts line 478; "No changes to save.": line 492; "Save changes": ui/src/components/organisms/EditTeamPanel.tsx line 268
    Given the Data team with its lead, analyst and charter, the analyst having learned "Know."
    When the user saves the team as " Numbers ", the analyst renamed statistician on codex, charter removed and plotter added
    Then exactly 1 team.edited event is written and the reply names the team Numbers with its new version
    And statistician keeps "Know." and is in Numbers, plotter is in Numbers with its prompt, and charter is no longer there
    And the team's version is no longer the one sent
    When an agent is saved with a profile nobody ships
    Then it is refused with 400 naming row 1, field profile: "analyst's profile dreaming isn't in policy.toml"
    When an agent is saved with a shipped profile the policy lacks
    Then it is refused with 409 naming missingProfiles read-mostly
    When the save is sent again with addMissingProfiles
    Then it is saved, the agent has read-mostly, and the policy changed exactly once

  @TEAM-08 @user @test
  Scenario: Rename a team, and remove a team or an agent once no unfinished task needs it
    # internal/web/api_test.go TestATeamIsRenamedWithItsTasks, TestATeamIsRemovedWithItsAgents and TestAgentsAreEditedFromThePage
    # ui/src/components/organisms/TeamPanel.test.tsx "Remove team asks in the footer, focus on Keep; Keep puts the footer back and focus on Remove team" and "Remove removes the team, closes the panel going back over its entry, says so and shows the page without it"
    # the refusals: internal/web/agents_api.go lines 78, 387 and 399; the page's dialog "Remove the <team> team?": ui/src/pages/AgentsPage.tsx lines 431 to 437
    Given the Data team with task-1 open
    When the user renames it " Analytics "
    Then the reply is exactly team Data, to Analytics; its coordinator and 2 agents are in Analytics and no team is called Data
    And task-1 stays with data-lead, and a new task for analytics goes to data-lead
    Given only the Software team
    When the user removes it
    Then it is refused with 409 "the only one"
    Given the Writing team with task-1 open
    When the user removes the team, or its editor
    Then each is refused with 409 naming task-1; an agent no task needs is removed
    When the user removes the only coordinator of a team
    Then it is refused with 409
    When the user clicks "Remove team" in the Edit team panel
    Then "Remove the Docs team and its 3 agents?" is shown with "Their prompts and what they learned go too.", the focus on Keep, nothing sent yet
    When the user clicks Remove
    Then exactly 1 request removes the team, a toast "Removed the Docs team" is shown, and the panel closes

  @TEAM-09 @user @test
  Scenario: What a team does: its coordinator's description, read by other coordinators or taken from its template
    # ui/src/components/organisms/TeamPanel.test.tsx "it shows the coordinator's description and the line other coordinators read, following the team's name", "a team made from a built-in template is read by the template's description, even with a new coordinator" and "on a team with no template, the coordinator's Description says other coordinators read it"
    # internal/web/templates_api_test.go TestYourTemplatesDescriptionDescribesItsCoordinator; internal/domain/agents/user_templates_test.go TestAUserTemplatesDescriptionFillsAnEmptyCoordinators
    # the field "What this team does" and its hint: ui/src/components/organisms/TeamPanelParts.tsx lines 169 to 176
    # not under test, checked by hand: the hint "One line. Other coordinators read it to choose what to send this team.", and the Agents page heading's "· <description>" only for a team whose template has one: ui/src/pages/AgentsPage.tsx lines 169 and 285
    Given the Edit team panel of a team with no template
    When "What this team does" is shown
    Then it holds exactly the coordinator's description
    And the line other coordinators read is shown following the team's name as it is typed
    Given a team made from a built-in template
    Then only the template's description is read, and the coordinator's Description says so
    Given a team added from your template
    Then its coordinator is described by the template's description only when it has none of its own

  # ---- Templates -------------------------------------------------------------

  @TEAM-10 @user @test
  Scenario: Save a team as a template, never over a name a template has
    # internal/web/templates_api_test.go TestATemplateIsSavedUnderItsName and TestATemplateNameOrIDTakenIsRefused
    # ui/src/components/organisms/TeamPanel.test.tsx "the footer becomes a form, named, with the team's name and its coordinator's description, focus on Template name" and "saved, the form closes, says so and focus goes back to Save as a template" and "with unsaved changes it is off and says why beside it, as on a phone"
    # "Save your changes first." while the panel has unsaved changes: ui/src/components/organisms/SaveAsTemplate.tsx lines 134 to 145
    Given the Edit team panel of the Planning team, with no unsaved changes
    When the user clicks "Save as a template"
    Then a form "Save as a template" is shown in the footer's place, "Template name" preset to Planning and "What it's for" to the coordinator's description, the focus in the name
    And the hint reads "Keeps each agent's prompt and settings and which one coordinates; not knowledge or where they run."
    When the user saves it with no name
    Then the template is saved as exactly id planning, team Planning, from Planning, yours, with the time it was saved
    When the user saves it as "  Reading Room  "
    Then its id is reading-room and its team "Reading Room"; built-in templates carry no saved time and no from
    When the user saves a name of yours in another case, or padded, or a built-in template's, or an id of yours
    Then each is refused with 409: "You have a template called My Planning already. Pick another name." or "Software is a built-in template. Pick another name.", and nothing is written
    When the save goes through on the page
    Then a toast "Saved Docs kit to your templates" with "Add it again from Add a team." is shown and the form closes
    Given unsaved changes in the panel
    Then "Save as a template" is disabled, "Save your changes first." shown beside it

  @TEAM-11 @user @test
  Scenario: A team from your template comes with its prompts, a second copy is numbered, and Update my template saves it back
    # internal/domain/agents/user_templates_test.go TestATeamIsAddedFromAUserTemplateWithItsPrompts, TestAUserTemplatesDescriptionFillsAnEmptyCoordinators and TestAUserTemplateAddedAgainGetsNumbers
    # internal/web/templates_api_test.go TestATemplateIsUpdatedFromItsTeam; ui/src/components/organisms/TeamPanel.test.tsx "it updates the template with the team as last saved, and says so" and "with unsaved changes it is off and says why beside it, as on a phone"
    Given your template Papers, described "Reads papers.", with lead and reader
    When the user adds a team from it
    Then lead coordinates Papers with the prompt "Hand out the reading." and reader is in Papers with "Read the papers.", each linked to the template papers
    And lead's empty description is filled with "Reads papers."; one it has is kept, and reader's stays empty
    When the user adds it again
    Then the second team is "Papers 2" with lead-2 and reader-2, the same prompts
    Given the Dreamers team saved as your template Dreams, "Big ideas.", and its lead's prompt changed since
    When the user clicks "Update my template"
    Then exactly 1 request is sent, replacing true, and the template keeps its id dreams and name Dreams with the prompt as last saved
    And a description sent is trimmed and kept, none keeps "Big ideas."; you still have exactly 1 template
    And a toast "Updated Docs kit in your templates" with "It has the Docs team as last saved." is shown
    Given unsaved changes
    Then "Update my template" is disabled, "Save your changes first." shown beside it, in the footer and in the body

  @TEAM-12 @user @test
  Scenario: Export, import and delete one of your templates; a built-in one is neither exported nor removed
    # internal/web/templates_api_test.go TestAnExportedTemplateImportsAsItWas and TestATemplateIsRemoved
    # ui/src/components/organisms/AddTeam.test.tsx "Import a template opens a picker for .json files", "an import is sent as it is, said in a toast, and the list read again", "a refused import says why under the button, a name you have in the server's words" and "Delete deletes it, reads the list again and moves focus to the next card's Add this team"
    # a built-in template: internal/web/templates_api.go lines 188 and 264
    Given your template my-planning
    When the user clicks Export on its card
    Then the file my-planning.json is downloaded as an attachment, 200
    When the user deletes the template and imports the file
    Then the template is back exactly as it was, yours, saved anew
    When the user clicks "Import a template"
    Then a file picker opens for .json files only
    And an import that goes through shows a toast "Imported Papers to your templates" and the list is read again: yes
    When the import names a template you have
    Then the card "A team of your own" reads "Couldn't import the template" and "You have a template called Papers already. Pick another name.", and the button is not disabled
    When the user clicks "Delete template, Orders" then Delete
    Then the template is deleted, a toast "Deleted the Orders template" is shown, and the focus moves to "Add this team, Papers"
    When a built-in template is exported or removed
    Then the export is refused with 400 and the removal with 409

  # ---- An agent's page -------------------------------------------------------

  @TEAM-13 @user @manual
  Scenario: An agent's page and its edit page: the prompt, what it knows, its engine, model, profile and where it runs
    # ui/src/pages/AgentPage.tsx lines 42 and 43, the buttons; line 49, Prompt; line 52, Knowledge; lines 56 to 67, Settings; line 33, no such agent
    # ui/src/pages/AgentEditPage.tsx lines 25 and 26; ui/src/components/organisms/AgentForm.tsx lines 118 to 199, the Agent panel; lines 201 to 246, Engine; lines 248 to 277, Runs on; line 280, the button
    # the "Starts each turn" row of an agent with no resume setting of its own, not a reviewer: ui/src/lib/resume.ts line 32
    # the model names claude knows: ui/src/components/organisms/AgentForm.tsx line 28; the knowledge history shows at most 8 changes: ui/src/components/organisms/KnowledgeEditor.tsx line 63
    Given the engineer, on claude with no model, under the developer profile
    When its page is shown
    Then the header shows its name, role badge and description, with exactly 2 buttons, "Redact a text" and Edit
    And the panel "Prompt" reads "Its standing instructions, given at the start of every turn"
    And the panel "Knowledge" has Save, disabled until the text changes, and lists at most 8 past changes, newest first, each by an agent or "you"
    And "Settings" lists exactly 6 rows: Engine claude, Model "the engine's default", "Policy profile" developer, Role "Works on the task", "Starts each turn" "Going on with its session", "Runs on"
    When the user clicks Edit
    Then the form shows exactly 7 fields in the Agent panel: Name, Team, Description, Colour, "Its work counts as" with Building, Review and Shipping, "Starts each turn", and Coordinates
    And Engine offers exactly claude and codex, Model lists opus, sonnet and haiku for claude, and "Policy profile" starts "default (developer)"
    And Save is disabled while the name or the prompt is empty
    When the user opens /agents/new
    Then the page is titled "New agent" and exactly this line is shown: "The coordinator can hand work to it from its next turn."
    Given a name no agent has
    Then the page reads "No agent named <name>"

  @TEAM-14 @user @test
  Scenario: An agent is added, changed, renamed and removed from the page, and what it knows can be edited
    # internal/web/api_test.go TestAgentsAreEditedFromThePage, TestRenamingAnAgentKeepsItsTasksGoing, TestYouEditWhatAnAgentKnows and TestTheKnowledgeHistoryIsTheSameFromPatches
    # the toasts "Added <name>" and "Saved <name>" with "The coordinator sees it from its next turn.": ui/src/components/organisms/AgentForm.tsx line 83; "Saved what <name> knows": ui/src/components/organisms/KnowledgeEditor.tsx line 39
    # the fixture starts with the Software team's 5 agents; the test asserts 5 again once designer is removed, not the count in between
    Given the Software team's 5 agents
    When the user adds designer and changes the engineer to codex with the model gpt-5
    Then the engineer is on codex, gpt-5, with an empty list of workers, not null
    When an agent is saved with a worker that does not exist, or the only coordinator is removed
    Then each is refused with 409
    When the user removes designer
    Then it is removed: exactly 5 agents again
    When the user renames the coordinator to lead-dev
    Then it keeps its prompt, its knowledge and coordinates; its open task goes on with lead-dev, and the task's first step is kept naming the old name
    When the user renames it to a name in use
    Then it is refused with 409
    When the user saves the engineer's knowledge as "  Prefer table tests.  "
    Then it is kept trimmed, its history has a change by "you", and a later change to the engineer's settings leaves it
    And the history lists exactly the same versions whether the log kept each as a copy or as a patch, across a rename
