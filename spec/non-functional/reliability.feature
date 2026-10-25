Feature: Reliability
  What Agentry does when a turn, a process or a machine fails.

  @REL-01 @test
  Scenario: A dead process ends its turn and the coordinator gets the issue
    Given a turn whose agent process has exited
    When the watchdog's 1 second tick finds the process gone, after the 3 second start grace
    Then the watchdog raises exactly 1 issue, saying the process exited, and stops the session
    And no turn is left open, so the coordinator gets the issue
    # PLAN.md T-014; internal/watchdog/watchdog_test.go
