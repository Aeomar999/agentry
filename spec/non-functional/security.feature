Feature: Security
  Signing in, credentials, the gate and what no agent may do.

  @SEC-01 @test
  Scenario: Wrong sign-in codes are counted and limited
    Given the sign-in page and a one-time code
    When 5 wrong codes come from one address within a minute
    Then the next sign-in from that address is refused with 429 and a Retry-After of 60 seconds
    And a sign-in from another address is allowed
    # README.md, Signing in the installed app, or another device; PLAN.md T-063; internal/web/signin_test.go
