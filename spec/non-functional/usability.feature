Feature: Usability
  The page: keys, panels and what the user sees.

  @USE-01 @manual
  Scenario: Ctrl+J opens and closes the dock
    Given the page open and signed in, on any of its pages
    When the user presses Ctrl+J
    Then the dock is shown at the bottom of the content area, never over the top bar
    And the dock is hidden when the user presses Ctrl+J again
    # README.md, Use it, The dock; PLAN.md T-045
