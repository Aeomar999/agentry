Feature: Operations
  Running Agentry: the policy file, the settings, the log and its upkeep.

  @OPS-01 @test
  Scenario: A policy file that does not load is refused and the policy before it stays
    Given Agentry running with a policy.toml in force
    When the file is saved with a mistake in it
    Then the new file is refused and the policy before it is kept in force
    And what is wrong with the file is shown on the Policy page
    # README.md, The policy; PLAN.md T-042; internal/policy/file_test.go, internal/web/api_test.go
