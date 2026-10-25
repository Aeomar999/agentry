Feature: Observability
  What Agentry shows about itself: tokens, turns, events, the status line.

  @OBS-01 @test
  Scenario: A task's tokens are the sum of its turns'
    Given a task whose 2 turns have ended, each with the tokens its engine counted from its transcript
    When the task is read from the log
    Then the task's tokens used are exactly the sum of the 2 turns' tokens
    And the sum is shown on the task's page, in the list and on its card
    # PLAN.md T-051; README.md, Use it, Budgets and turn limits; internal/tasks/split_test.go
