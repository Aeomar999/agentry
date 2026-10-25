Feature: The coordinator
  An ordinary agent that reads every report and ends each turn with a decision the loop carries out.

  @COORD-01 @test
  Scenario: An assign decision hands the task to one agent
    Given a task whose coordinator has a turn open
    When the coordinator ends its turn with an assign decision that names the engineer and its instructions
    Then the loop starts exactly 1 turn, the engineer's
    And the coordinator's instructions are shown in the engineer's brief
    # README.md, How a task moves; internal/coordinator/loop_test.go
