Feature: The user
  The person at the page: what they add, see and answer.

  @USER-01 @test
  Scenario: A new task goes to its team's coordinator
    Given Agentry runs with its Software team
    When the user adds a task from the page, with a description in their own words
    Then the task is shown with the team's coordinator as its coordinator
    And the path its work took shows exactly 1 handoff, from the user to the coordinator
    # README.md, First steps; internal/web/api_test.go
