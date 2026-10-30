Feature: Settings
  The Settings page, and redacting a text from a task or from everywhere.
  # Sources: internal/extensions, internal/web/extensions_api.go,
  # internal/web/redact_api.go, internal/web/redact_log_api.go,
  # internal/core/store/redact.go, internal/core/rules/redact_rules.go,
  # internal/domain/settings/settingsfile,
  # ui/src/pages/SettingsPage.tsx, ui/src/components/organisms/RedactText.tsx,
  # ui/src/components/organisms/RedactLog.tsx and their tests.

  # ---- Features that are off -------------------------------------------------

  @SET-01 @user @test
  Scenario: The Settings page lists the features that are off, with the way to turn each on, and what the log holds that this build skipped
    # ui/src/pages/SettingsPage.test.tsx "on a fresh install, lists Ideas with its why and Ask the visionaries", "doesn't list a visible feature, and says nothing is off", "says nothing until the manifest is read", "shows one tally line per prefix and per type", "ignores a feature only the server knows" and "shows a feature with no button by its label and why alone"
    # the page header and the Privacy panel: ui/src/pages/SettingsPage.tsx lines 33 and 53 to 67; the heading "Features that are off" is not asserted
    # the why is the server's sentence, internal/extensions/ideas/extension.go line 66; the page test's fixture carries a shorter one, ui/src/pages/SettingsPage.test.tsx lines 37 and 52
    Given a fresh install, with no Visionaries team
    When the Settings page is shown
    Then Ideas is shown among the features that are off, with the server's why and the button "Ask the visionaries"
    And "Every feature is on." is not shown
    Given Ideas on
    Then Ideas is not listed, no button is shown, and "Every feature is on." is shown
    Given the features not read yet
    Then nothing is said either way
    Given events the server skipped, 31 from the prefix idea- and 4 of the type git.pr_opened
    Then the lines shown read "31 events from a feature this build doesn't have: idea-" and "4 events of a type this build doesn't know: git.pr_opened"
    Given a feature only the server lists, or one with no button
    Then the first is not listed at all, and the second shows its label and why alone

  # ---- Redacting a text ------------------------------------------------------

  @SET-02 @user @test
  Scenario: Redact a text counts first, then redacts, and says what is left
    # ui/src/components/organisms/RedactText.test.tsx "shows the count first, redacts only once you say yes, then what it did and the note on backups"
    # ui/src/pages/TaskPage.test.tsx "asks about the text marked on the page, in %s"
    # internal/web/redact_api.go lines 37 to 68
    Given the user marked "sk-live-0123456789" on task-1's page
    When the user clicks "Redact a text" in the header
    Then a dialog "Redact a text in task-1" is shown and "The text to take out" holds exactly the marked text
    When the user gives the reason "pasted a key" and asks for the count
    Then "3 rows of task-1 hold this text." is shown with "Nothing undoes this.", and 0 rows are redacted yet
    When the user clicks "Redact 3 rows"
    Then exactly 3 rows are redacted, with the reason "pasted a key"
    And "Redacted 3 rows of task-1 at <time>." is shown, and backups made before then still hold the text

  @SET-03 @user @test
  Scenario: What redacting refuses, and a count gone stale
    # ui/src/components/organisms/RedactText.test.tsx "shows a refusal in the server's words, and redacts nothing" and "a row written since the count is refused 409, counted again and asked again with the new count"
    # internal/web/redact_api_test.go TestARedactionWithAStaleCountIsRefused and TestARefusedRedactionWritesNothing
    Given the text to redact is a number in a row's JSON
    When the user asks for the count
    Then "It can't be redacted: <the server's reason>" is shown and 0 rows are redacted
    Given 1 row held the text when it was counted, and a message holding it arrived since
    When the user clicks "Redact 1 row"
    Then the redaction is refused with 409 "2 rows hold this text now, not 1: count them again", and nothing is written
    And the page counts again and asks again with "Redact 2 rows"

  @SET-04 @user @manual
  Scenario: Redact a text everywhere, from the Settings page
    # ui/src/pages/SettingsPage.tsx lines 53 to 69; ui/src/components/organisms/RedactLog.tsx lines 39 and 44
    # README.md, Archiving, erasing and redacting
    Given the Settings page
    When the user clicks "Redact a text everywhere" in the Privacy panel
    Then the address carries ?redact=everywhere and the dialog "Redact a text everywhere" is shown
    And its field's hint reads "Paste it. It is matched exactly, spaces and all, in every task, archived ones too, in the agents and what they learned, and in Agentry's other records, such as the policy, the workers and the ideas."
    When the user closes the dialog
    Then ?redact is no longer in the address and nothing was redacted
