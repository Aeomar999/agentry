Feature: Codex token count
  Agentry counts the tokens of a Codex turn as it counts a Claude Code turn's:
  from the session's file, this turn's part only, split into its parts, with
  the input of the turn's last call as its context. Today every Codex turn
  ends with its tokens unknown, for the reason "Codex's token use isn't
  read", so a task worked by Codex shows 0 tokens for those turns, its
  budget never stops it and its context is never known. Nothing else
  changes: the turn keeps the fields a Claude Code turn fills, and the task
  sums them the same way.

  # ---- Counting a turn ---------------------------------------------------------

  @PROP-23 @test @to-observability
  Scenario: A Codex turn's tokens are counted from its session file, this turn's part only
    # the Claude Code count this follows: @OBS-25 (internal/platform/engines/claude.go claudeTokens); the Codex turn today: @OBS-25 and @COMPAT-14 (internal/platform/engines/codex.go CodexTokensNotRead)
    # assumed, not yet recorded: Codex writes its running totals into the session file as token count entries: input, cached input and output, each since the session began
    # test: internal/platform/engines/tokens_test.go (to write), with a recorded Codex session file
    Given a Codex session file whose totals before the turn's start stamp are 10,000 input, of them 6,000 cached, and 500 output
    And whose last totals at the turn's end are 30,000 input, of them 20,000 cached, and 1,500 output
    When the turn ends and Agentry reads the file from the turn's start stamp
    Then the turn's part is the last totals less the totals before the start: 20,000 input, of them 14,000 cached, and 1,000 output
    And its split is input 6,000, cache write 0, cache read 14,000 and output 1,000, in the same 4 parts as a Claude Code turn
    And its total is the 4 summed: 6,000 + 0 + 14,000 + 1,000 = 21,000
    And the turn carries no reason for unknown tokens: yes

  @PROP-24 @test @to-observability
  Scenario: A Codex turn's context is the input of its last call
    # @OBS-26: contextTokens is the input of the turn's last call, cached input included (internal/platform/engines/signals.go Signal.Context; claudeTokens adds input, cache write and cache read)
    # test: internal/platform/engines/tokens_test.go (to write)
    Given a Codex turn whose last call, as the session file gives it, had 26,801 input tokens, cached ones included
    When the turn ends
    Then the turn's context is exactly 26,801
    And the task page shows it as it shows a Claude Code turn's context: yes

  @PROP-25 @test @to-observability
  Scenario: A resumed Codex session counts only the new turn, and a file with no start stamp is counted whole
    # @OBS-25 counts a Claude Code transcript from the start stamp, and the whole of it from an older worker with none
    # test: internal/platform/engines/tokens_test.go (to write)
    Given a Codex session resumed for turn-3, whose file holds the token counts of turn-1 and turn-2 before turn-3's start
    When turn-3 ends
    Then only the counts at or after turn-3's start are counted, so the 3 turns together sum to the file's last totals exactly once
    Given a turn with no start stamp, as from an older worker
    Then the whole file is counted: yes

  @PROP-26 @test @to-observability
  Scenario: A Codex turn whose tokens cannot be counted says why, with the reasons a Claude Code turn gives
    # the reasons TranscriptNotSaved, NoUsageInTranscript and NoUsageThisTurn (internal/platform/engines/claude.go); @OBS-28 shows a turn's reason
    # test: internal/platform/engines/tokens_test.go (to write)
    Given a Codex turn whose session file is not there, one whose file has no token counts, and one whose counts are all older than its start
    When each turn ends
    Then their tokens are 0 with the same reasons a Claude Code turn gives, these 3 among them: "transcript not saved", "no usage in transcript" and "no usage in this turn's part of the transcript"
    And no turn ends with the reason "Codex's token use isn't read" any more: yes

  # ---- What the count feeds ------------------------------------------------------

  @PROP-27 @test @to-observability
  Scenario: A task worked by both engines sums every turn's tokens
    # @OBS-01: a task's tokens are the sum of its turns'
    # test: internal/tasks/split_test.go (to write)
    Given a task whose turn-1 ran on Claude Code and used 53,174 tokens, and whose turn-2 ran on Codex and used 21,000
    When the task is read from the log
    Then the task's tokens used are exactly 74,174
    And the task's split adds both turns' parts, with nothing left unsplit: yes

  @PROP-28 @test @to-limits
  Scenario: A Codex task stops at its token budget
    # @LIM-01: a task stops at its token budget; today a Codex task never reaches it, as its turns count 0
    # test: internal/coordinator/next_test.go (to write)
    Given a task with a token budget of 100k tokens, whose agents run on Codex
    When its turns have used 110k tokens
    Then the coordinator's next move asks the user, not a turn, the question saying 110k tokens of its 100k budget: yes

  @PROP-29 @test @to-reliability
  Scenario: A line of the session file Agentry cannot read is skipped, and the rest is counted
    # a Claude Code transcript is read the same way, line by line (internal/platform/engines/claude.go claudeTokens)
    # test: internal/platform/engines/tokens_test.go (to write)
    Given a Codex session file with 1 line cut short by a crash between 2 token counts
    When the turn ends
    Then the cut line is skipped and the turn is counted from the counts that can be read: yes

# Open questions
# Cache writes: Codex reports none, so the split records cache write 0 (recommended), or the split could leave that part out for a Codex turn.
# Reasoning tokens: counted inside output, as Codex's output total already holds them (recommended), or shown as a part of their own.
# Where the session file comes from: the transcript path in the Stop hook's payload (recommended, as for Claude Code), or the file Codex keeps for the session under its own folder, found by the session's id.
# This turn's part: the last totals less the totals before the start (recommended), or the sum of each call's own usage after the start.
# Whether Codex's session file holds running totals or each call's usage only (recommended: record a session file in the adapter's payloads before building, and pick the method from it).
