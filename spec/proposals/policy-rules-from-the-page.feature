Feature: Policy rules from the page
  The user adds, changes, moves and removes the rules of a profile from its
  panel on the Policy page, and every rule keeps its why. Today the page
  manages the default profiles, use_for_everyone and the history, and shows
  each profile's rules as a table, but a rule is changed only by editing
  policy.toml by hand. A change from the page goes through the same locked
  write path as the page's other changes: it is made against the version the
  page saw, backed up, checked to load, put in force and logged with its diff.

  # ---- Changing a rule -------------------------------------------------------

  @PROP-01 @user @test @to-policy
  Scenario: A rule added from a profile's panel is written with its why and logged
    # builds on @POLICY-02 and the policy's one locked write path (internal/domain/policy/live.go Live.Change)
    # test: internal/web/policy_api_test.go (to write)
    Given the developer profile's panel, its rules as loaded, and the hash the page saw
    When the user adds the rule "never curl to a pipe" as rule 3, decision deny, runs_any "curl * | sh", why "A script from the internet never runs unread."
    Then the file holds the rule as rule 3 of developer, with its name, decision, pattern and why: yes
    And the new version is in force within the same request, and Try a call with Bash "curl x | sh" under developer is deny by "never curl to a pipe"
    And exactly 1 policy event is logged, by you, "added rule never curl to a pipe to developer", with the diff and the file's text
    And a backup beside the file holds exactly the file as it was

  @PROP-02 @user @test @to-policy
  Scenario: A rule changed from the page changes only its own lines
    # the edits Agentry makes touch nothing outside the part they change (internal/domain/policy/edit.go)
    # test: internal/domain/policy/edit_test.go (to write)
    Given a policy file with comments between its rules and 6 profiles
    When the user changes the decision of developer's rule "never sudo" from deny to ask and its why
    Then every line of the file outside that rule is the same, comments included
    And the rule keeps its place, its name and its patterns: yes
    And the event reads "changed rule never sudo in developer", with the diff showing only that rule's lines

  @PROP-03 @user @test @to-policy
  Scenario: A rule moved up or down changes which rule decides
    # "Rules are read top to bottom and the first that matches decides" (@POLICY-07)
    # test: internal/web/policy_api_test.go (to write)
    Given a profile whose rule 2 allows runs_any "go test *" and whose rule 5 asks for runs_any "go test ./build/*"
    When the user moves rule 5 above rule 2
    Then the rules keep every field, and only their order changes: yes
    And Try a call with Bash "go test ./build/..." is decided by the moved rule, now rule 2
    And the event reads "moved rule <name> in <profile> from 5 to 2", with its diff

  @PROP-04 @user @test @to-policy
  Scenario: A rule removed from the page can be restored from the history
    # Revert restores a version as a new version (@POLICY-06)
    # test: internal/web/policy_api_test.go (to write)
    Given a profile with 8 rules
    When the user removes its rule "never force push"
    Then the profile has exactly 7 rules and the file no longer names that rule
    And the event reads "removed rule never force push from <profile>", with its diff
    When the user reverts to the version before it in the history
    Then the rule is back in its place, with its why: yes

  @PROP-05 @user @test @to-policy
  Scenario: A rule without its why, or one that would not load, is refused and nothing is written
    # the old build plan, "keeping each rule's why"; the checks a file must pass to load: internal/domain/policy/file.go
    # a change that would not load is a policy.Refusal, which applyPolicy answers with 409 (internal/web/policy_api.go); a missing why and a repeated name are checks the page makes before the edit, so they get 400
    # test: internal/web/policy_api_test.go (to write)
    Given the developer profile's panel
    When a rule is sent with an empty why, or a why of only spaces
    Then it is refused with 400 "a rule needs its why: say what it is for"
    When a rule is sent with a name another rule of that profile has
    Then it is refused with 400, naming the field and why, and the file is not changed
    When a rule is sent with the decision "maybe" or a pattern that does not compile
    Then each is refused with 409, naming the field and why, as a policy that would not load is today, and the file is not changed
    And no refused change is logged

  @PROP-06 @user @test @to-policy
  Scenario: Changing a rule of a default profile makes it yours
    # the rows "outdated" and "yours" of @POLICY-03 (internal/domain/policy/edit.go ErrProfileEdited)
    # test: internal/web/policy_api_test.go (to write)
    Given read-only as Agentry ships it, marked current
    When the user changes the why of one of its rules from the page
    Then read-only is marked "yours", and an update to a newer default is refused with 409 "won't overwrite it"
    And "Use the default" still replaces it with the shipped profile, as it does for a profile edited by hand: yes

  # ---- What the page may not change ------------------------------------------

  @PROP-07 @test @to-security
  Scenario: Only the page token changes a rule
    # the other policy changes take the page token only (internal/web/policy_api_test.go TestOnlyTheUITokenChangesThePolicy, @SEC-48, @SEC-50)
    # test: internal/web/policy_api_test.go (to write)
    Given the server has a page token, a hook token, a desk token and a helper token
    When a rule is added, changed, moved or removed with the hook, desk or helper token, or with no token
    Then the answer is HTTP 401 every time, and the file is not changed
    And an agent can never change the rules it runs under: yes

  @PROP-08 @test @to-security
  Scenario: A rule added from the page cannot open Agentry's own files
    # Agentry's own files are off limits for every profile, and no policy file can change that (@POLICY-38)
    # test: internal/domain/policy/default_test.go (to write)
    Given the user adds to developer, as rule 1, an allow rule for every path under the data folder
    When an agent under developer reads the sign-in token or edits policy.toml
    Then each is denied by the built-in rule for Agentry's own files, not allowed by the new rule

  @PROP-09 @test @to-reliability
  Scenario: A rule change made against an older version, or while the file does not load, is refused
    # Live.Change refuses a stale version with ErrStale and a file that does not load (internal/domain/policy/live.go)
    # test: internal/domain/policy/live_test.go (to write)
    Given the page loaded the version with hash A, and the file was then saved by hand as version B
    When a rule change is sent with hash A
    Then it is refused with 409 "changed since the page loaded it: try again", and version B is kept
    Given the file on disk has a mistake, so the version before it is in force
    When a rule change is sent
    Then it is refused with 409 "policy.toml doesn't load, so Agentry won't change it until you fix it", and the file is kept as it is

  @PROP-10 @test @to-reliability
  Scenario: A rule change to a policy file that is a link is refused, and the policy in force is kept
    # WriteFile refuses a symbolic link before it backs anything up (internal/domain/policy/write.go)
    # test: internal/web/policy_api_test.go (to write)
    Given a policy.toml that is a symbolic link to a file elsewhere
    When a rule is added from the page
    Then the change is refused with "policy.toml is a link; edit it yourself", and the version in force is the same as before
    And no event is logged and no backup is made: yes

  # ---- The panel ---------------------------------------------------------------

  @PROP-11 @manual @to-usability
  Scenario: Each profile's rules table offers Add, Edit, Move and Remove, and the form asks for the why
    # the table of a profile's rules, #, Decision, Rule, Why (@POLICY-07); a form with unsaved changes asks before it closes, as the team panels do (@USE-19)
    Given the Policy page with the developer profile's panel open
    When the user clicks Add rule
    Then a form is shown with Name, Decision, each field a rule may match on, and Why, and Save is not allowed while Why is empty
    When the user clicks Edit on rule 4, changes its why and presses Esc
    Then the page asks once whether to discard the change, and Keep editing keeps the form open: yes
    When the user saves
    Then the table shows the changed why in rule 4's row, and the History panel lists the new version first, yours
    And Remove asks once, naming the rule, before it removes it: yes

# Open questions
# Whether the page also changes a profile's own why, otherwise and unreadable, or only its rules, as the old build plan said (recommended: only the rules).
# Whether a why is required in policy.toml too, so a hand-written rule without one is refused, or only on the page (recommended: only on the page, so no file that loads today stops loading).
# Whether a repeated rule name in a profile is refused only on the page, or in the file too (recommended: only on the page, as a verdict names its rule).
# Whether the page's own checks, a missing why and a repeated name, answer 400 (recommended, as they are not a policy that would not load) or 409 as every refused policy edit does today.
# Whether a rule is moved by drag or by Up and Down buttons (recommended: Up and Down, which the keys reach too).
# Whether the banner "Edit the file to change the policy" stays, or names only what the page cannot change (recommended: it names the rest, the profiles' own settings and shell_tools).
