Feature: Supervisor agents
  An agent of a team may be allowed to assign the task, as its coordinator
  does: a supervisor. It is a plain agent with one more permission, not a
  new kind of agent. Its report may end with an assign decision, and
  Agentry checks that assign as it checks the coordinator's, so it hands
  the task to an agent of its own team or is refused. The coordinator, the
  first agent allowed to assign, stays the one deciding everything else:
  every report still goes back to it, and only it finishes, asks, moves or
  passes the task on. Today only a team's coordinator assigns, and an
  assign by any other agent is refused by the rules.

  # ---- Giving an agent the permission -----------------------------------------

  @PROP-53 @user @test @to-teams-and-templates
  Scenario: An agent is allowed to assign from its form or the team panel, and is off until the user turns it on
    # the old build plan, "an ordinary agent with permission to propose ticket.assigned"; the agent form's fields: @TEAM-13 (ui/src/domains/agents/AgentForm.tsx); the team panel: @TEAM-05 (ui/src/domains/agents/TeamPanel.tsx)
    # test: ui/src/domains/agents/AgentForm.test.tsx, ui/src/domains/agents/TeamPanel.test.tsx and internal/web/api_test.go (to write)
    Given the Software team, with no agent allowed to assign
    When the user edits the reviewer and turns on "May assign the task"
    Then exactly 1 request is sent, saving the reviewer with assigns true, and its other fields the same: yes
    And the reviewer's page lists the row "Assigns the task" yes under Settings
    And the coordinator's row in the form and in the team panel has no such field: yes
    Given a new agent, or a new team built from Blank
    Then every agent but the coordinator has assigns off: yes

  @PROP-54 @user @test @to-teams-and-templates
  Scenario: A template keeps which agents may assign
    # templates save each agent's settings: @TEAM-10 and @TEAM-12 (internal/domain/agents/user_templates.go)
    # test: internal/domain/agents/user_templates_test.go (to write)
    Given a team whose reviewer may assign
    When the user saves the team as a template, exports it, imports it and adds a team from it
    Then the new team's reviewer may assign, and every other agent of it may not: yes
    Given a built-in template
    Then none of its agents may assign, so the teams Agentry ships are the same as today: yes

  @PROP-55 @user @test @to-teams-and-templates
  Scenario: The permission is refused on a coordinator
    # a coordinator assigns already: the rules let it (internal/core/rules/task_rules.go byTheCoordinator)
    # test: internal/domain/agents/custom_test.go and internal/web/api_test.go (to write)
    Given the Kitchen team
    When an agent is saved with coordinates true and assigns true
    Then it is refused with "chef coordinates the Kitchen team, so it assigns already", and nothing is saved

  # ---- What a supervisor's assign does -----------------------------------------

  @PROP-56 @agent @test @to-coordinator-and-handoffs
  Scenario: A supervisor's brief names the agents it may hand the task to, and how
    # the coordinator's brief lists the agents (internal/runtime/coordinator/brief.go coordinatorBrief); a worker's brief is handoffBrief or the worker brief (internal/runtime/coordinator/turns.go briefFor)
    # test: internal/runtime/coordinator/handoff_brief_test.go (to write)
    Given the reviewer of the Software team may assign
    When the coordinator hands it the task
    Then its brief lists the Software team's agents but the coordinator and the reviewer itself, each with its description: yes
    And it says the report may end with exactly 1 assign decision in the coordinator's form, and that the report goes back to the coordinator otherwise: yes
    Given an agent that may not assign
    Then its brief is the same as today: yes

  @PROP-57 @agent @test @to-coordinator-and-handoffs
  Scenario: A supervisor's report ending with an assign hands the task to that agent
    # builds on @HANDOFF-01 and @HANDOFF-06 (internal/runtime/coordinator/apply.go, where the coordinator's decision becomes task.assigned)
    # test: internal/runtime/coordinator/loop_test.go (to write)
    Given the reviewer may assign, and its turn ends with the report "Two tests fail on Windows paths." and {"action": "assign", "agent": "engineer", "instructions": "Fix the path joins in internal/x."}
    When the loop reads the task
    Then exactly 1 task.assigned is written, by the reviewer, naming the engineer with those instructions
    And the next turn is the engineer's, and its brief has those instructions: yes
    And the trail shows the step from the reviewer to the engineer, with the reviewer's report: yes

  @PROP-58 @coordinator @test @to-coordinator-and-handoffs
  Scenario: The coordinator stays the one deciding: every report after a supervisor's assign goes back to it
    # @HANDOFF-11: every agent's report returns to the coordinator; docs/decisions.md, 2026-10-16, "supervisor is just an agent"
    # the rule: a supervisor's assign is taken only on a turn the coordinator handed it; on a turn another supervisor handed it, its assign is not taken and the report goes back to the coordinator
    # test: internal/runtime/coordinator/loop_test.go (to write)
    Given the reviewer assigned the task to the engineer
    When the engineer's turn ends with its report
    Then the next turn is the coordinator's, not the reviewer's: yes
    And the coordinator's brief shows both steps, reviewer to engineer and engineer to the coordinator: yes
    Given the engineer may assign too, and the reviewer handed it the turn
    When the engineer's report ends with an assign to the tester
    Then the assign is not taken, 0 turns start for the tester, the report goes back to the coordinator, and exactly 1 issue goes back to the coordinator, saying the assign was not taken: yes
    Given the reviewer's report ends with a done, an ask, a transfer, a request, a move, a split, a wait or a fetch
    Then none of them is taken, and the report goes back to the coordinator as written: yes

  @PROP-59 @agent @test @to-coordinator-and-handoffs
  Scenario: A supervisor's report with no decision goes back to the coordinator, as every report does
    # @HANDOFF-11; a supervisor's report is not a coordinator's reply, so no decision is not an issue (@HANDOFF-13 is for the coordinator)
    # test: internal/runtime/coordinator/loop_test.go (to write)
    Given the reviewer may assign
    When its turn ends with "Verdict: approved" and no decision
    Then the next turn is the coordinator's, with the report in its brief, and 0 issues are written

  @PROP-60 @agent @test @to-coordinator-and-handoffs
  Scenario: A supervisor's assign gets the checks the coordinator's gets, and a refused one is an issue for the coordinator
    # the checks on task.assigned: internal/core/rules/task_rules.go (noOpenTurn, noQuestionOpen, notToTheCoordinator, notUndoingYourMove) and internal/core/rules/agent_rules.go assigneeExists; a refused assign is an issue: @HANDOFF-07
    # test: internal/core/rules/rules_test.go and internal/runtime/coordinator/next_test.go (to write)
    Given the reviewer of the Software team may assign
    When its report ends with an assign to an agent of the Writing team, to no agent of that name, to the coordinator, or to itself
    # "reviewer can't assign the task to itself" is a new check: notToTheCoordinator covers only the coordinator
    Then each is refused: "editor is in the Writing team, not Software", "no agent called designer", "reviewer can't hand the task to the coordinator; its report goes back there anyway" and "reviewer can't assign the task to itself"
    When it assigns while a question to the user is open, or after the user moved the task to Done, To do or the Backlog
    Then each is refused with the reason the coordinator's assign gets: yes
    And for each refusal 0 turns start and exactly 1 issue goes back to the coordinator, saying why

  # ---- Without the permission ---------------------------------------------------

  @PROP-61 @test @to-security
  Scenario: An agent that may not assign cannot, whatever its report says
    # @HANDOFF-41: an assignment by any agent other than the coordinator is refused (internal/core/rules/task_rules.go byTheCoordinator); the permission is read from the roster under the store's lock
    # test: internal/core/rules/rules_test.go (to write)
    Given the engineer, which may not assign
    When its report ends with an assign to the deployer
    Then nothing is assigned, the report goes back to the coordinator, and 0 task.assigned events are written
    When a task.assigned by the engineer is proposed to the store
    Then it is refused: "engineer is not this task's coordinator (coordinator) and may not assign"
    Given the reviewer's permission turned off before its turn ends
    Then its assign is refused the same way: yes

  @PROP-62 @test @to-security
  Scenario: The permission gives no other right
    # docs/design/extensions.md section 3: a supervisor is an ordinary agent; every tool call is judged with the agent's own profile (internal/domain/policy)
    # test: internal/domain/policy/default_test.go and internal/core/rules/rules_test.go (to write)
    Given the reviewer, under read-only, may assign
    When it edits a file or pushes
    Then each is judged by read-only and denied, exactly as for a reviewer that may not assign: yes
    And it may not answer an approval, close an epic or accept an idea, which only the user does: yes

  @PROP-63 @test @to-limits
  Scenario: Turns a supervisor starts count toward the task's turn limit and budget
    # @HANDOFF-26: the turn limit asks the user; @HANDOFF-29: the budget stop
    # test: internal/runtime/coordinator/next_test.go (to write)
    Given a task with the turn limit 24, and a coordinator and a reviewer that may assign, the reviewer handing the task to the engineer on each of its turns
    When its turns reach 24 since the user last answered, the coordinator's, the reviewer's and the engineer's counted alike
    Then no turn starts, and the user is asked, as for any task at its turn limit: yes
    Given a task with a budget of 100,000 tokens and turns a supervisor started using 110,000
    Then no turn starts, and the user is asked with "110k tokens of its 100k budget"

  @PROP-64 @test @to-compatibility
  Scenario: Agents, templates and logs from before read as agents that may not assign
    # agents.Agent and the templates gain an optional field (internal/domain/agents/agent.go); an old log replays the same (internal/core/current/real_log_test.go)
    # test: internal/domain/agents/agents_test.go and internal/domain/agents/user_templates_test.go (to write)
    Given a roster, an exported template and a log saved before supervisors were built
    When Agentry reads them
    Then every agent may not assign, and every task reads the same as before: yes

# Open questions
# How a supervisor assigns: the same JSON assign decision at the end of its report as a coordinator's (recommended), or an MCP tool in its session, or a file in its worktree.
# Where the next report goes: back to the coordinator (recommended, so the coordinator stays the one deciding), or back to the supervisor that assigned the task.
# What a supervisor may decide: assign only (recommended, as the old build plan named ticket.assigned alone), or done too, once every checker approved.
# Whose agents: its own team's only (recommended, as for the coordinator), or another team's named in its brief, as a transfer is.
# The field's name: "May assign the task" in the form and assigns in the roster (recommended), or a role such as "Supervises" beside "Its work counts as".
# Chained assigns: a supervisor's assign is taken only on a turn the coordinator handed it (recommended), or taken on any turn, supervisors handing the task to each other held back only by the turn limit.
# Agents of another kind: an agent that may assign may also be a checker or a shipper (recommended, since the permission is separate), or a supervisor's work counts as none of the three.
