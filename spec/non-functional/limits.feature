Feature: Limits
  Budgets, turn limits, sizes and caps.

  @LIM-01 @test
  Scenario: A task stops at its token budget
    Given a task with a token budget of 100k tokens
    When its turns have used 110k tokens
    Then no new turn starts, and the user is asked to raise the budget or wrap up
    And after the user's answer the coordinator gets exactly 1 turn
    # README.md, Use it, Budgets and turn limits; PLAN.md T-051; internal/coordinator/next_test.go
