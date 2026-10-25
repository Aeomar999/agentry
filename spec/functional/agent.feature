Feature: The agents
  The engineer, reviewer, tester, deployer and the other teams' agents, each in its own worktree and session, every action through the gate.

  @AGENT-01 @test
  Scenario: A push asks the user first
    Given an agent under the developer profile, in its task's worktree
    When the agent runs git push origin HEAD:dev
    Then the user is asked before the push runs
    And a push to main, or a force push, is denied
    # PLAN.md T-013, T-033; README.md, The policy; internal/policy/default_test.go
