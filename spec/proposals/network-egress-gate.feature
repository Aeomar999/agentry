Feature: Network egress gate
  Agents' network traffic goes through a proxy Agentry runs, which judges
  each connection by the host it is for: a host on the profile's allowlist
  passes, and a host not on the list is asked about in Needs you, where the
  user allows it once, always, or not at all. Where the system allows it,
  traffic is also blocked at the system level, so the proxy is the only
  way out for an agent's programs. This is what makes the autonomous
  profile trustworthy: hooks see commands, not effects, and python
  deploy.py looks harmless. Today no traffic passes through Agentry, no host
  is allowlisted or asked about, and nothing is blocked at the system
  level, so the autonomous profile's asks for acting outside the machine
  rest on its shell and tool rules alone, as its own why says.

  # ---- The proxy and the allowlist ---------------------------------------------

  @PROP-78 @agent @test @to-policy
  Scenario: An agent's session is started with Agentry's proxy as its way out
    # the old build plan, "agent traffic through an Agentry proxy"; a turn's environment is set by Agentry (internal/turns/turns.go environment, internal/platform/sessions/session.go Env)
    # test: internal/turns/turns_test.go (to write)
    Given an agent whose profile has a network allowlist
    When its turn starts
    Then its session's environment names Agentry's proxy for http, https and every other proxy setting the common tools read: yes
    And the proxy listens only on the machine the agent runs on, and takes only connections from that turn's session: yes
    Given an agent whose profile has no network allowlist
    Then its environment is the same as today: yes

  @PROP-79 @agent @test @to-policy
  Scenario: A connection to an allowlisted host passes without asking
    # the old build plan, "allowlisted hosts pass"; the allowlist is part of the profile in policy.toml, as its rules are (internal/policy)
    # test: internal/policy/network_test.go and internal/egress/proxy_test.go (to write)
    Given the developer profile with an allowlisted host, written as a pattern that covers its subdomains
    When the engineer's go mod download opens 3 connections to that host and 1 to a subdomain of it
    Then all 4 pass with no wait, and 0 asks are made
    And the proxy judges each by the host the connection names, and never reads what it carries: yes

  @PROP-80 @agent @test @to-policy
  Scenario: A connection to a host not on the list is asked about, and waits for the user
    # the old build plan, "unknown hosts go to Needs you"; an ask waits up to the gate's wait, 30 minutes: @APPROVE-01, @APPROVE-02
    # test: internal/egress/proxy_test.go and internal/web/api_test.go (to write)
    Given the autonomous profile, and a script whose shell command the profile allows
    When the script opens a connection to a host not on the list
    Then the connection is held, and the task needs you, its line exactly "Waiting for you to allow a connection to" with the host and port
    And Needs you shows it with Allow once, Allow always and Deny: yes
    And while it is held, a second connection to the same host waits on the same ask, not a new one: yes

  @PROP-81 @user @test @to-approvals-and-questions
  Scenario: The user's answer: once lets this connection through, always adds the host to the list, and Deny refuses it
    # the locked write path for policy.toml, backed up, checked to load and logged with its diff (internal/policy/live.go Live.Change); @PROP-01 writes a rule the same way
    # test: internal/web/api_test.go and internal/policy/network_test.go (to write)
    Given a connection to a host not on the list, held for the user
    When the user clicks Allow once
    Then that connection passes, and the next connection to the host is asked again: yes
    When the user clicks Allow always on another
    Then the host is added to the agent's profile's allowlist in policy.toml, and exactly 1 policy event is logged, by you, with the diff
    And later connections to it pass with no ask, for every agent on that profile: yes
    When the user clicks Deny on a third
    Then the connection is refused, and the program that opened it gets a refused connection: yes

  @PROP-82 @agent @test @to-policy
  Scenario: A connection nobody answers in time is refused, and the agent carries on
    # @APPROVE-02: an approval nobody answered in time is held, and the agent told no
    # test: internal/egress/proxy_test.go (to write)
    Given a connection held for the user
    When the gate's wait of 30 minutes runs out with no answer
    Then the connection is refused, and the agent's turn goes on: yes
    And the ask stays in Questions, and Allow always answered late adds the host to the list for the next connection: yes

  @PROP-83 @agent @test @to-policy
  Scenario: Agentry's own server and the engine's service are always reachable
    # the hook posts every call to the server (internal/platform/hookcall); the engine reaches its own service to run at all (internal/platform/engines)
    # test: internal/egress/proxy_test.go (to write)
    Given an agent on Claude Code, and a profile whose allowlist is empty
    When its session starts and its hooks post to Agentry
    Then the hooks reach Agentry's server, and the engine reaches its own service, with 0 asks
    And no other host is reachable without an ask: yes

  # ---- Blocking at the system level --------------------------------------------

  @PROP-84 @test @to-security
  Scenario: Where the system allows it, a program that ignores the proxy cannot reach out
    # the old build plan, "OS-level block so the proxy is the only way out"
    # test: internal/egress/block_test.go (to write), on a system where the block is built
    Given an agent whose profile has a network allowlist, on a machine whose system supports the block
    When a program in its session opens a connection to an allowlisted host directly, ignoring the proxy settings
    Then the connection fails, and only connections through the proxy or to Agentry's server leave the session: yes
    And the block covers only the agent's session, and every other program on the machine reaches the network as before: yes

  @PROP-85 @user @test @to-policy
  Scenario: Where the system does not allow the block, Agentry says so plainly
    # the Policy page shows each profile (@POLICY-01); the autonomous profile's why names the gap today (internal/policy/default.toml profiles.autonomous)
    # test: ui/src/pages/PolicyPage.test.tsx and internal/egress/block_test.go (to write)
    Given a machine whose system Agentry cannot block traffic on, or a worker without the permission to block it
    When the Policy page shows a profile with a network allowlist
    Then it reads "Traffic is not blocked at the system level on this machine: a program that ignores the proxy can reach any host." beside the profile: yes
    And the proxy still judges every connection that goes through it: yes
    And the agents on that machine still run, with the same proxy: yes

  # ---- What the autonomous profile gains --------------------------------------

  @PROP-86 @test @to-security
  Scenario: Under autonomous, a script that reaches out is asked about, though its command is allowed
    # the old build plan says the egress gate is needed for the autonomous profile to be trustworthy; builds on @SEC-31 and @SEC-32 (internal/policy/default_test.go TestTheAutonomousProfile)
    # test: internal/policy/default_test.go and internal/egress/proxy_test.go (to write)
    Given the shipped policy with use_for_everyone turned off, an agent with the autonomous profile, and the block in force on its machine
    When it runs python deploy.py, which the profile allows, and the script opens a connection to a host not on the list
    Then the command runs and the connection is asked: yes
    And curl to the same host is asked by the rule "acts outside this machine: you approve it", as today: yes
    And the Policy page shows no note that traffic is not blocked on that machine: yes

  @PROP-87 @test @to-compatibility
  Scenario: Profiles with no allowlist work as today
    # the shipped profiles other than autonomous keep reaching the network as they do (internal/policy/default.toml)
    # test: internal/policy/default_test.go (to write)
    Given the shipped policy, and a policy.toml written before the egress gate was built
    When every agent but those on autonomous works
    Then 0 connections are asked about, and their sessions' environments are the same as today: yes
    Given the shipped autonomous profile
    Then it ships with an empty allowlist, so every host but Agentry's server and the engine's service is asked: yes

  # ---- Logged, and failing closed ----------------------------------------------

  @PROP-88 @test @to-observability
  Scenario: Each host an agent reached, asked about or was refused is on the Actions tab, once per turn
    # actions and their decisions are events in the task's stream (internal/events/catalogue.go action.requested, action.asked, action.approved, action.denied)
    # test: internal/egress/proxy_test.go (to write)
    Given a turn that opened 50 connections to 1 allowlisted host, 1 to a host the user allowed once and 1 the user denied
    When the task's Actions tab is shown
    Then it lists exactly 3 network actions, each with its host and port and its decision: allowed by the list, approved by you, denied by you
    And no event holds what a connection carried, a header or a query: yes

  @PROP-89 @test @to-reliability
  Scenario: When the proxy cannot judge, connections are refused, not passed
    # the gate fails closed, the server being down included (spec/glossary.md, gate)
    # test: internal/egress/proxy_test.go (to write)
    Given an agent on a worker whose link to the server is down, or a proxy that cannot read the policy in force
    When a program opens a connection to any host but Agentry's server
    Then the connection is refused: yes
    And when the proxy itself has stopped, a session with the block in force reaches 0 hosts: yes

  @PROP-90 @benchmark @to-performance
  Scenario: A connection to an allowlisted host adds little time
    # the gate's bounds on judging an action: @PERF-11 and @PERF-12
    # to measure: a Go benchmark in internal/egress (to write), on the machine Agentry runs on
    Given an allowlist of 100 host patterns
    When 1,000 connections to an allowlisted host go through the proxy
    Then the proxy adds under 5 ms to each connection's start at the 95th percentile

# Open questions
# The system-level block: Linux by the agent's session in its own network namespace or by a firewall rule on the session's user (recommended), macOS by a sandbox profile for the session, Windows by a firewall rule on the session's programs; which of these is built first is open.
# Workers: the proxy runs on the machine where the agent runs, judging by the server's policy (recommended), or every agent's traffic goes through the server's machine.
# Allow once: this one connection (recommended), or the host for the rest of the turn.
# Where the allowlist lives: on each profile in policy.toml (recommended, so Allow always is a policy change with its diff), or one list for every agent.
# The engine's service: the engine adapter names its service's hosts, always reachable (recommended), or they are written into each shipped allowlist where the user can see them.
# Which profiles get the gate: autonomous ships with it and an empty list (recommended), or every profile with a list of the hosts its tools need.
# What is logged: each distinct host once per turn with its decision (recommended), or every connection.
# Encrypted traffic: judged by the host a connection names, never decrypted (recommended); reading what it carries would need a certificate in every agent's trust store.
# The benchmark's number: 5 ms at the 95th percentile (recommended), to be set from the first measurement.
