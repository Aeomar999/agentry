Feature: Performance
  Measured numbers: how long things take.

  @PERF-01 @benchmark
  Scenario: Writes to the log are not held up by a reader
    Given a log with 1 reader that is not reading
    When 1,200 events are written
    Then the writes take 0.07 seconds, as measured on 2026-10-16
    And the reader then reads all 1,200 back, in order
    # PLAN.md T-005; go test ./internal/store -run Reader
