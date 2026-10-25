Feature: Compatibility
  The systems, engines and browsers Agentry runs on and with.

  @COMPAT-01 @manual
  Scenario: The worker builds for six kinds of machine
    Given a checkout of the repo on a machine with Go
    When the user runs make workers
    Then 6 binaries are built in bin/, for darwin, linux and windows, each on arm64 and amd64
    # Makefile, WORKER_PLATFORMS; PLAN.md T-062; README.md, Add machines
