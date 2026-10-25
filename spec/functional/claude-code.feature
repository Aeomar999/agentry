Feature: Agentry from Claude Code
  Plain claude in any repo hands work to Agentry through its MCP tools and hears back from it.

  @CC-01 @test
  Scenario: A task made with create_task is on the board
    Given Claude Code connected to Agentry and open in a repo
    When the model calls the create_task tool with a title and a description
    Then exactly 1 task is created, in that repo
    And the new task's id and the link to its page are shown in the tool's answer
    # README.md, Use it from Claude Code; internal/mcp/server_test.go
