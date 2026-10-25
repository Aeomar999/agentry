Feature: Workers
  A machine joined to Agentry so that agents can run on it.

  @WORKER-01 @test
  Scenario: A machine joins once, with a code, and gets its own credential
    Given an install command made on the Workers page, with its join code
    When the machine runs the command and joins
    Then the machine gets a credential of its own, and the log keeps only its hash
    And the same code is refused for a second machine
    # README.md, Add machines; PLAN.md T-022; internal/workers/workers_test.go
