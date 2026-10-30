Feature: Output gate
  The gate judges a tool's result before the agent sees it, as it judges an
  action before it runs. A profile's rules for results match a tool and the
  text its result holds, and the first that matches decides: hold the
  result for the user, redact the text it matches, or block it. A result
  no rule matches reaches the agent as it is. This reviews what reaches the
  model: a page fetched, a file read, a command's output. Today the gate
  judges each action before it runs, and its result goes to the agent
  unseen: the engine's PostToolUse hook only reports that the action
  finished, and never blocks.

  # ---- Which results are judged -----------------------------------------------

  @PROP-65 @agent @test @to-policy
  Scenario: A result no rule for results matches reaches the agent unchanged
    # the gate today: @POLICY-07 and the reporting hooks, which end || true (internal/platform/engines/hook_config.go)
    # test: internal/hooks/output_test.go (to write)
    Given the developer profile with 1 rule for results, which holds a result from WebFetch that has "ignore previous instructions"
    When the engineer runs go test, reads a file and fetches a page without those words
    Then each result reaches the agent byte for byte as the tool gave it: yes
    And 0 results are held, redacted or blocked

  @PROP-66 @agent @test @to-policy
  Scenario: Rules for results are read top to bottom, and judge every tool's result
    # rules are read top to bottom and the first that matches decides (@POLICY-07, internal/policy); a rule names its tools as the action rules do
    # test: internal/policy/output_test.go (to write)
    Given a profile whose rule 1 redacts results that have "BEGIN PRIVATE KEY", and whose rule 2 blocks results from WebFetch that have "BEGIN"
    When a Bash command's output, a file read with Read, a page from WebFetch and an MCP tool's reply each hold "BEGIN PRIVATE KEY"
    Then each of the 4 is redacted by rule 1, and none is blocked
    And a rule for actions never judges a result, and a rule for results never judges an action: yes

  # ---- Hold, redact, block -----------------------------------------------------

  @PROP-67 @agent @test @to-policy
  Scenario: A held result waits for the user, as an ask does
    # @APPROVE-01: Needs you, approve or deny; the gate's wait, GateTimeoutSeconds, 30 minutes (internal/platform/engines)
    # test: internal/hooks/output_test.go and internal/web/api_test.go (to write)
    Given a rule "pages that talk to the agent" that holds results from WebFetch that have "ignore previous instructions"
    When the engineer fetches a page that holds those words
    Then the agent waits, and the task needs you, its line exactly "Waiting for you to let through a result from WebFetch: pages that talk to the agent"
    And Needs you shows the result to the user, with Let through and Withhold: yes
    When the user clicks Let through
    Then the result reaches the agent unchanged: yes
    When the user clicks Withhold on another held result
    Then the agent gets only "Agentry withheld this result: pages that talk to the agent", and none of the result

  @PROP-68 @agent @test @to-policy
  Scenario: A held result nobody answers in time is withheld, and the agent carries on
    # @APPROVE-02 for an action; a result cannot be let through after the agent moved on
    # test: internal/hooks/output_test.go (to write)
    Given a result held for the user
    When the gate's wait of 30 minutes runs out with no answer
    Then the agent gets "Agentry withheld this result: no answer came in time", and its turn goes on: yes
    And the hold is closed, so the task no longer needs you for it, and Let through is no longer shown: yes

  @PROP-69 @agent @test @to-policy
  Scenario: A redacted result reaches the agent with each matching text replaced by [redacted]
    # the marker the user's redaction writes: @SET-02 (internal/core/store/redact.go)
    # test: internal/hooks/output_test.go (to write)
    Given a rule "keys never reach a model" that redacts results that have a text of the form "sk-live-*"
    When cat config.yaml prints 40 lines, 2 of them holding a key of that form
    Then the agent gets the 40 lines with exactly those 2 keys replaced by [redacted], and every other character the same: yes
    And the agent is not told which rule redacted them: yes

  @PROP-70 @agent @test @to-policy
  Scenario: A blocked result never reaches the agent, and the coordinator is told
    # a denied action is shown to the coordinator under Actions Agentry refused: @HANDOFF-20
    # test: internal/hooks/output_test.go and internal/coordinator/brief_test.go (to write)
    Given a rule "no pages from paste sites" that blocks results from WebFetch of a page whose address has "paste"
    When the engineer fetches such a page
    Then the agent gets only "Agentry blocked this result: no pages from paste sites", and none of the page
    And its turn goes on, and the coordinator's next brief lists the block under Actions Agentry refused: yes

  # ---- What is logged ----------------------------------------------------------

  @PROP-71 @test @to-observability
  Scenario: Each decision on a result is logged with its rule, never the result's text
    # actions and their decisions are events by the policy (internal/core/events/catalogue.go action.approved, action.denied); action.result holds no text today (internal/hooks/handler.go)
    # test: internal/hooks/output_test.go (to write)
    Given one result held and let through, one redacted twice and one blocked
    When the task's Actions tab is shown
    Then each of the 3 actions shows its result's decision beside its own: "let through by you", "redacted 2 texts: keys never reach a model", "blocked: no pages from paste sites"
    And each decision is 1 event in the task's stream, by the policy or by you, naming the rule
    And no event, snapshot or line of the server's output holds the result's text or a text that was redacted: yes

  @PROP-72 @test @to-security
  Scenario: A held result is kept in the server's memory only, and only while it is held
    # the user's redaction keeps its secrets in memory only, never on disk (internal/hooks/redactions.go Redactions)
    # test: internal/hooks/output_test.go (to write)
    Given a result held for the user
    Then it is in no event, no snapshot and no file in the data folder: yes
    When the user answers, the wait runs out or the server restarts
    Then it is gone from the server's memory, and a restart withholds it from the agent: yes

  # ---- When the gate cannot judge ---------------------------------------------

  @PROP-73 @test @to-reliability
  Scenario: A result the gate has not judged by the hook's deadline does not reach the agent
    # the gate fails closed: a gate's hook ends || exit 2, which blocks a call at PreToolUse (docs/design/proofs.md, proof 01; internal/platform/engines/hook_config.go); nothing yet shows exit 2 at PostToolUse withholds a result, so Agentry ends the session instead, as in @PROP-75; a hook that crashes or cannot reach the server is an open question below
    # test: internal/hooks/output_test.go and internal/platform/engines/engines_test.go (to write)
    Given an agent whose profile has 1 rule for results
    When the hook's deadline passes before a decision
    Then the result reaches 0 model calls: Agentry ends the agent's session before the hook returns, as in @PROP-75, and exactly 1 issue goes back to the coordinator
    And the results hook also ends || exit 2, and that alone is not counted on to withhold a result until recorded payloads show it does: yes

  # ---- The engines -------------------------------------------------------------

  @PROP-74 @test @to-compatibility
  Scenario: On Claude Code, PostToolUse is where a result is judged
    # PostToolUse fires once a tool call succeeded, with its result (internal/platform/engines/claude.go claudeReports, internal/domain/tasks/task.go Pushed)
    # test: internal/platform/engines/engines_test.go (to write), with recorded payloads
    Given an agent on Claude Code whose profile has rules for results
    When its session is prepared
    Then PostToolUse is written to wait for the gate's decision on the result, and every other reporting hook ends || true as today: yes
    And a result Claude Code can replace reaches the agent as the decision says, held, redacted or blocked: yes
    Given an agent whose profile has no rule for results
    Then PostToolUse stays a report ending || true, and nothing waits: yes

  @PROP-75 @test @to-compatibility
  Scenario: Where an engine cannot replace or withhold a result, a hold, a redaction or a block ends the session first
    # docs/design/proofs.md, proof 07: Codex has the same hook events and fields; its PreToolUse can only deny and needs a reason; its PostToolUse fires for a command that failed too, with no exit code (internal/domain/tasks/task.go Pushed)
    # test: internal/platform/engines/engines_test.go and internal/hooks/output_test.go (to write)
    Given an agent on an engine whose PostToolUse cannot replace or withhold what the agent sees
    When a rule holds, redacts or blocks a result
    Then Agentry ends the agent's session before the hook returns, so the result reaches 0 model calls: yes
    And exactly 1 issue goes back to the coordinator, saying which rule stopped the result and that the engine cannot withhold it: yes
    And a result no rule matches reaches the agent as it is: yes

  @PROP-76 @test @to-compatibility
  Scenario: Profiles with no rules for results work as today
    # the shipped profiles (internal/policy/default.toml) gain no rule for results
    # test: internal/policy/default_test.go (to write)
    Given the shipped policy, and a policy.toml written before rules for results were built
    When every agent works
    Then 0 results are held, redacted or blocked, and the hooks are written as today: yes

  @PROP-77 @benchmark @to-performance
  Scenario: Judging a result adds little to a tool call
    # the gate already has bounds on how long it judges an action: @PERF-11 and @PERF-12 (internal/policy)
    # to measure: a Go benchmark in internal/policy (to write), on the machine Agentry runs on
    Given a profile with 20 rules for results, and a result of 1 MB that matches none
    When the gate judges it
    Then the decision takes under 20 ms at the 95th percentile

# Open questions
# Where the rules live: as rules of a profile with results_have and the decisions ask, redact and deny (recommended), or in a section of their own in policy.toml.
# Words: "hold" written as the policy's ask and "block" as its deny (recommended), or hold and block as decisions of their own.
# What the user sees of a held result: the whole result in Needs you, kept in memory (recommended), or its first 4 KB only.
# A late answer: a held result whose wait ran out is closed (recommended), or kept open so the agent's next turn gets it if the user lets it through.
# Claude Code: whether its PostToolUse can replace a built-in tool's result the agent sees, or only add to it and tell the model; recorded payloads should settle this before it is built, as proof 07 did for the gate.
# Codex: whether its PostToolUse can withhold or replace a result at all; until a proof shows it can, recommended: Codex ends the session as in @PROP-75.
# A hook that crashes, or cannot reach the server: whether the engine is stopped on the spot by the command after || (recommended) or the result goes through and an issue follows once the server is reached.
# A result too large to judge: judged whole up to 10 MB and held past that (recommended), or judged on its first 10 MB only.
# The benchmark's number: 20 ms at the 95th percentile for 1 MB against 20 rules (recommended), to be set from the first measurement.
