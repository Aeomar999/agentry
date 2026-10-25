Feature: Claude Code and Agentry
  Agentry runs as an MCP server inside Claude Code (`agentry mcp`), so plain
  `claude` in any repo can hand work to Agentry, read the board, relay what
  waits for the user, and let the user answer and approve. Claude Code asks
  the user itself (elicitation) for every decision that is theirs alone, so
  the model can never approve, close or accept for them.

  Sources: cmd/agentry/mcp.go, cmd/agentry/claudecode.go, internal/mcp,
  internal/ideas/tools.go, internal/connections, internal/web/integrations_api.go
  and their tests. CC-01 is the foundation's stub; the scenarios here start at CC-02.

  # ---- Installing the integration -------------------------------------------

  @CC-02 @manual
  # 1. On the machine Agentry runs on, run `agentry mcp install` with Claude Code installed.
  # 2. Run `claude mcp list` and check the server "agentry" is there at user scope.
  # 3. Open ~/.claude/settings.json and count the hooks that run `agentry claude-hook`.
  # 4. Check the statusLine entry runs `agentry statusline`; set one of your own first to see it kept.
  # 5. Run `agentry mcp install` a second time and check no hook is doubled.
  Scenario: Installing Agentry into Claude Code on the machine Agentry runs on
    Given Claude Code is installed on the machine Agentry runs on
    And the user's Claude Code settings have no status line
    When the user runs the install command for the integration
    Then Claude Code has 1 MCP server called agentry, at user scope, for every repo
    And the settings have 3 hook events for Agentry: SessionStart, UserPromptSubmit and Stop
    # cmd/agentry/mcp.go addHooks: the context hooks time out after 5 seconds
    And the SessionStart and UserPromptSubmit hooks each time out after 5 seconds
    # cmd/agentry/claudecode.go waiterLifetime: the Stop hook may wait 12 hours
    And the Stop hook is asynchronous and may run for 12 hours
    And the status line is set to Agentry's: yes
    When the user runs the install command again
    Then each hook event still holds 1 Agentry hook, and the earlier one is replaced

  @CC-03 @manual
  # 1. Put a statusLine of your own in ~/.claude/settings.json.
  # 2. Run `agentry mcp install`.
  # 3. Read the settings again and check your status line is the one still there.
  Scenario: A status line the user already has is kept
    Given the user's Claude Code settings have a status line of their own
    When the user runs the install command for the integration
    Then the user's status line is replaced: no
    And the output says 1 way to call Agentry's status line from theirs

  @CC-04 @test
  # internal/web/api_test.go
  Scenario: The Integrations page makes an install command for another machine
    Given the user is signed in on the Integrations page
    When they ask for an install command
    # internal/connections/connections.go InviteLifetime
    Then the command carries a one-time code that works for 15 minutes
    And there is 1 command for a terminal (sh) and 1 for PowerShell (Windows) per address the server has
    And the install script fetched with the code carries that code 1 time and runs `agentry mcp connect`
    And the script fetched with a wrong code is refused with status 401

  @CC-05 @test
  # internal/web/api_test.go
  Scenario: A machine connects its Claude Code with the code, once
    Given an install command whose code has not been used
    When the machine sends the code with its host name, user and system
    Then it gets 1 credential of its own, and the log keeps only the credential's hash
    And the same code sent a second time is refused with status 409
    And the script fetched with the used code is refused with status 401
    And a request to the API with the credential is answered with status 200
    And the Integrations page lists 1 connection, naming the machine, without the credential itself

  @CC-06 @test
  # internal/web/api_test.go
  Scenario: Revoke stops a connected machine at once
    Given a machine connected with its own credential
    When the user revokes that connection on the Integrations page
    Then a request to the API with that credential is answered with status 401

  @CC-07 @test
  # internal/web/signin_test.go
  Scenario: A connected machine's credential cannot issue sign-in codes
    Given a machine connected with its own credential
    When it asks the server for a sign-in code with that credential
    Then the request is refused with status 401
    And the same request with the page's own token is answered with status 200

  @CC-08 @test
  # cmd/agentry/mcp_test.go
  Scenario: Connecting over https checks the server's CA before the code is sent
    Given a server served over https with Agentry's own CA
    When the machine runs `agentry mcp connect` with the CA's fingerprint, the server and a code
    Then the code is sent 1 time, and the connection kept holds the server's https address and its CA
    When the machine runs it with another CA's fingerprint
    Then the code is sent 0 times, no connection is saved, and the error says the server isn't trusted
    When the machine runs it over https with no fingerprint
    Then the code is sent 0 times, and the error names the fingerprint to give

  @CC-09 @test
  # cmd/agentry/mcp_test.go
  Scenario: A connection whose server's CA changed says to connect again
    Given a machine connected to a server whose CA was regenerated since
    When Claude Code's Agentry calls the server
    Then the call fails, and the error says the CA was regenerated or this is another server, 1 time
    And it names the connect command to run again, with a new fingerprint and a code from the Integrations page, 1 time
    Given a connected server that is down instead
    When Claude Code's Agentry calls it
    Then the error says it can't reach Agentry, and says the CA changed: no

  # ---- Reading the board ----------------------------------------------------

  @CC-10 @test
  # internal/mcp/server_test.go
  Scenario: status is the board for this repo
    Given 2 tasks waiting for the user, 1 running, 1 epic with 2 of 2 stories done and 1 task in another repo
    When Claude calls status
    Then the reply has 4 sections: Needs you, Running, Epics and Not started
    And the Needs you section lists 2 lines, each naming the tool that deals with it
    And a running task's line says who is working, its board column and its usage, such as 0 tokens (no limit) and 7 turns (no limit)
    And the epic's line says 2 of 2 stories done
    And the other repo's task is on the board: no
    When Claude calls status with all
    Then tasks of every repo are listed: yes

  @CC-11 @test
  # internal/mcp/server_test.go
  Scenario: status shows an epic left open with every story done
    Given an epic whose 3 stories are all done, left open by the user
    When Claude calls status
    Then the Epics section says all 3 stories done, open until the user closes it (close_epic)
    And the epic is in the Needs you section: no

  @CC-12 @test
  # internal/mcp/server_test.go
  Scenario: list_tasks lists every task of this repo with its status and usage
    Given a task with a budget of 5M tokens and 24 turns, 1.2M tokens and 4 turns used
    And a working task in the Shipping column with 7 turns and no limits
    When Claude calls list_tasks
    Then the first task's line reads needs you, 1.2M of 5M tokens and 4 of 24 turns
    And the second task's line reads working, Shipping, 0 tokens (no limit) and 7 turns (no limit)
    And an epic's line counts its stories done, such as 2 of 2, and a story's line names its epic as epic-N
    # internal/mcp/tools.go listTasks: with a status, only tasks with that status are listed

  @CC-13 @test
  # internal/mcp/server_test.go
  Scenario: show_task is one task in full
    Given a done task whose work was pushed, moved to Done by the user
    When Claude calls show_task for it
    Then the reply reads its title, status shipped, its board column Done, its coordinator, its repo and 1 link to its page
    And it says Moved to Done by you: yes
    And it says what the task used of its budget and turns, such as 0 tokens (no limit)
    # internal/mcp/tools.go showTask: at most the last 8 steps of the trail are listed
    Given a task on 2 of whose turns the tokens could not be counted
    When Claude calls show_task for it
    Then the reply says Tokens unknown on 2 turns, with the server's reason

  @CC-14 @test
  # internal/mcp/archived_test.go
  Scenario: show_task reads an archived task back
    Given a task that was done and archived
    When Claude calls show_task for it
    Then the reply has its title, status done and its description as before it was archived: yes
    And it says Archived on its date, and that it can be changed: no

  @CC-15 @test
  # internal/mcp/archived_test.go
  Scenario: An erased task cannot be shown, and is left out of the list
    Given 2 tasks, 1 of them erased
    When Claude calls show_task for the erased one
    Then the tool fails, saying the task was erased, with 1 date
    When Claude calls list_tasks with all
    Then the erased task is listed: no, and the other task is listed: yes

  # ---- Handing over work ----------------------------------------------------

  @CC-16 @test
  # internal/mcp/server_test.go
  Scenario: create_task hands a task to Agentry in this repo
    Given Claude Code runs in a repo Agentry knows
    When Claude calls create_task with a title and a description and nothing else
    Then 1 task is made in this repo, with budget 0 and maxTurns 0 sent, which the server reads as its defaults
    # internal/web/api.go createTask: 0 takes the server's default, -1 is no limit (internal/mcp/tools.go createTask)
    And the reply names the new task as task-N, gives 1 link to its page and says its coordinator reads it next
    And the reply ends with Waiting for the user (2) when 2 things wait

  @CC-17 @test
  # internal/mcp/server_test.go
  Scenario: create_task makes a story of an epic, or an epic with its stories
    Given an open epic shown as epic-3
    When Claude calls create_task with a title and epic epic-3
    Then the server is sent epic task-3 and no repo, so the story works where its epic does
    And the reply names the new task-N and says it waits in the user's Backlog until they start it: yes
    When Claude calls create_task with kind epic and stories task-1 and epic-9
    Then the server is sent stories task-1 and task-9, each as task-N
    And the reply names the new epic as epic-N and says its coordinator splits it into stories next: yes

  @CC-18 @test
  # internal/web/api_test.go
  Scenario: A story that does not exist is not filed, and the epic is still made
    Given 2 tasks and no task-9
    When an epic is created with stories task-1 and task-9
    Then the epic is made with 1 story, task-1
    And the reply says Not filed: task-9: no task task-9

  @CC-19 @test
  # internal/web/api_test.go
  Scenario: create_task in a repo named by its git address works in a clone
    Given a repo at a git address the server can reach
    When Claude calls create_task with that address as repo
    # internal/worktrees/repo.go CloneTimeout; internal/mcp/tools.go createTask adds 1 minute for the request
    Then the server clones the repo before the task is made, allowing 10 minutes for the clone
    And the task works in the clone: yes

  @CC-20 @test
  # internal/mcp/server_test.go
  Scenario: tell_coordinator sends the user's words to a task's coordinator
    Given a task whose coordinator is between turns
    When Claude calls tell_coordinator with the task and the user's words
    Then 1 message is posted to the task, and the reply says it was sent to task-N's coordinator, which reads it now
    # internal/web/chat_api.go say: with a turn open, the message is recorded and that turn is stopped, so the coordinator reads it at once

  @CC-21 @test
  # internal/mcp/server_test.go
  Scenario: answer_question relays the user's own answer
    Given a task whose coordinator asked the user a question
    When Claude calls answer_question with the user's answer
    Then 1 answer is posted to the task as given, and the reply says the task is answered and its coordinator carries on
    And Claude Code is asked anything: no

  @CC-22 @test
  # internal/mcp/server_test.go
  Scenario: An answer that would close an epic is the user's to approve
    Given an epic left open with every story done, and Claude Code able to ask the user
    When Claude calls answer_question with the answer close the epic, in any case or spacing
    Then Claude Code asks the user 1 time whether to close the epic
    And when the user chooses keep open, the answer Close the epic is sent 0 times and the reply says the user kept it open
    And when the user chooses close, the answer Close the epic is sent 1 time and the reply says the user closed it

  @CC-23 @manual
  # 1. Have a coordinator ask to copy a file from outside a machine's worktrees (a fetch decision outside the worker's folder).
  # 2. In Claude Code, ask Claude to answer the task with "Copy it".
  # 3. Check Claude Code itself asks you whether to copy; choose "don't copy" and check nothing was answered.
  # 4. Repeat and choose "copy"; check the task's shared files hold the file.
  Scenario: Copying a file from outside a machine's worktrees is asked of the user
    Given a task waiting for the user to allow a copy, and Claude Code able to ask the user
    When Claude calls answer_question with the answer Copy it
    Then Claude Code asks the user 1 time whether to copy the file
    And when the user declines, the answer is sent 0 times and the reply says to send their reason with answer_question
    And when the user allows it, the answer Copy it is sent 1 time and the file is in the task's shared files
    Given Claude Code cannot ask the user
    When Claude calls answer_question with the answer Copy it
    Then the reply gives 1 link to the task's page and the answer is sent 0 times

  @CC-24 @test
  # internal/mcp/server_test.go
  Scenario: review_approvals asks the user themselves about each waiting action
    Given 1 action waiting for approval, and Claude Code able to ask the user
    When Claude calls review_approvals
    Then Claude Code asks the user 1 time, naming the agent, the task, the tool and the command, and why it asks
    And the model sees the question before the tool replies: no
    And when the user chooses deny with a reason, 1 deny is posted with that reason and the reply says denied by the user
    And when the user chooses approve, 1 approve is posted and the reply says approved by the user

  @CC-25 @test
  # internal/mcp/server_test.go
  Scenario: Without elicitation nothing is approved, and the user gets the link
    Given 1 action waiting for approval, and a Claude Code that cannot ask the user
    When Claude calls review_approvals
    Then the reply says 1 action waits for the user's approval, which is theirs to give, with 1 link to the page
    And approve or deny is posted 0 times
    When Claude calls review_approvals with a task that has 0 actions waiting
    Then the reply says no action is waiting for approval

  @CC-26 @test
  # internal/mcp/server_test.go
  Scenario: A held approval says when it was held and what approving does now
    Given an action that got no answer in time, so the agent was told no and it was held for the user
    When Claude calls review_approvals and Claude Code asks the user
    Then the question says the command, that it was held, the time it was held in the user's zone, and that the agent was told no
    And it says the agent may run this exact command 1 time in its next turn if the user approves
    # internal/hooks/handler.go replyMargin: the server stops waiting 5 seconds before the hook's deadline
    # internal/turns/turns.go environment: the deadline is GateTimeoutSeconds - 10 = 1790 seconds (internal/engines/engine.go GateTimeoutSeconds = 1800)

  @CC-27 @test
  # internal/mcp/server_test.go
  Scenario: close_epic is asked of the user themselves
    Given an epic whose coordinator asks to close it, and Claude Code able to ask the user
    When Claude calls close_epic for it
    Then Claude Code asks the user 1 time: Close epic-N, its title, with the coordinator's text
    And when the user chooses close, the answer Close the epic is posted 1 time and the reply says the user closed epic-N
    And when the user chooses keep open, it is posted 0 times and the reply says to ask the user what else it needs

  @CC-28 @test
  # internal/mcp/server_test.go
  Scenario: close_epic closes an epic the user left open
    Given an epic with all 3 stories done that is not waiting for the user
    When Claude calls close_epic for it and the user chooses close
    Then the user is shown the epic's summary before deciding: yes
    And the answer Close the epic is posted 1 time

  @CC-29 @test
  # internal/mcp/server_test.go
  Scenario: close_epic refuses a task that is not ready
    Given a task that is no epic, or an epic with stories still going
    When Claude calls close_epic for it
    Then the tool fails, saying it isn't ready to close because not every story of it is done, and 0 answers are posted
    Given an epic ready to close, and a Claude Code that cannot ask the user
    When Claude calls close_epic for it
    Then the reply says closing is the user's to approve, with 1 link to the epic's page, and the answer is posted 0 times

  @CC-30 @test
  # internal/mcp/server_test.go
  Scenario: start takes tasks out of the Backlog
    Given 2 tasks waiting in the user's Backlog
    When Claude calls start with both ids
    Then each task is directed to In progress, 2 calls in all
    And the reply names the 2 as started, epics as epic-N, and says they are on the Tasks board now

  @CC-31 @test
  # internal/mcp/server_test.go
  Scenario: set_priority orders tasks among themselves
    Given 2 tasks with ranks of their own
    When Claude calls set_priority with their ids in the order wanted
    Then the 2 tasks get the 2 ranks they held between them, first first, and no other task moves
    And the reply says In this order now, naming the 2 as task-N or epic-N

  @CC-32 @test
  # internal/mcp/server_test.go
  Scenario: file_under_epic files a task under an epic, moves it, or takes it out
    Given a task and an open epic shown as epic-3
    When Claude calls file_under_epic with the task and epic-3
    Then the server is sent epic task-3, and the reply says the task is a story of epic-3
    When Claude calls file_under_epic with the task and an empty epic
    Then the server is sent an empty epic, 1 call, and the reply says the task is out of its epic

  @CC-33 @test
  # internal/web/api_test.go
  Scenario: file_under_epic is refused where the rules refuse it
    Given a story and an epic
    When the epic is filed under the story
    Then it is refused with status 409, saying the story isn't an epic
    And the epic's stories change: no
    # internal/web/epics_api.go: an epic id no task has is refused with no task <id>; a done epic with <id> is done; reopen it to add stories
    # internal/rules/task_rules.go notAnEpic, inNoEpic, notItself: an epic can't be a story of another, a story is taken out of its epic first, a task can't be a story of itself

  @CC-34 @test
  # internal/mcp/server_test.go
  Scenario: Every tool takes an epic as epic-N and names epics epic-N
    Given an epic the user knows as epic-3
    When Claude calls show_task, tell_coordinator, answer_question, file_under_epic, create_task, set_priority, start, close_epic, attach_file or get_file with epic-3
    Then each asks the API for task-3, and sends epic- to the API 0 times
    And each reply names the epic epic-3, a story task-N with its epic as epic-N, and links stay task-N

  # ---- Shared files ----------------------------------------------------------

  @CC-35 @test
  # internal/mcp/server_test.go
  Scenario: attach_file shares a file from this machine with a task
    Given a file on the machine Claude Code runs on
    When Claude calls attach_file with the task, the file's path and a name to share it by
    Then the file is uploaded to the task's shared files under that name, 1 upload
    And the reply says it was shared with task-N as that name, and names 1 folder its agents find it in: .agentry/files
    When Claude calls attach_file with a folder's path
    Then the tool fails, saying it isn't a file and to share a folder's files 1 by 1, and 0 uploads are sent

  @CC-36 @test
  # internal/web/api_test.go
  Scenario: A shared file's name stays inside the task's folder, and its size is limited
    Given a task
    When a file is uploaded under a name that leaves the folder, such as one starting with ..
    Then it is refused with status 400
    # internal/sharedfiles/blobs.go MaxSize: a shared file is at most 2 GB

  @CC-37 @test
  # internal/mcp/server_test.go
  Scenario: get_file downloads one of a task's shared files
    Given a task with 1 shared file
    When Claude calls get_file with the task, the file's name and a folder to save in
    Then 1 file is saved in that folder under its own name, and the reply says Saved, from task-N, the path and its size in bytes
    # internal/mcp/files.go getFile: without a place to save, the file goes in the current folder under its own name

  # ---- Ideas ----------------------------------------------------------------

  @CC-38 @test
  # internal/ideas/tools_test.go
  Scenario: list_ideas shows the Visionaries' open ideas for this repo
    Given 1 open idea in this repo and 3 ideas accepted, dismissed or in another repo
    When Claude calls list_ideas
    Then 1 idea is listed, with its state, title, size, kind, the task it was suggested in, its problem and its feature
    # internal/ideas/tools.go listIdeas: the problem and feature lines are each cut at 300 characters (tasks.OneLine)
    And who backed and who doubted it are named: yes
    When Claude calls list_ideas with state all
    Then all 4 ideas are listed: accepted ones say what they became, as task-N or epic-N, and dismissed ones say dismissed with the user's note
    When Claude calls list_ideas with all
    Then every repo's ideas are listed: yes
    When Claude calls list_ideas with a state that is not open, accepted, dismissed or all
    Then the tool fails, naming the 4 states there are

  @CC-39 @test
  # internal/ideas/tools_test.go
  Scenario: accept_idea is the user's decision, asked through Claude Code
    Given an open idea, and Claude Code able to ask the user
    When Claude calls accept_idea for it
    Then Claude Code asks the user 1 time, showing the problem and the feature, with the kind, a note and start to choose
    And when the user accepts with a kind and a note, 1 accept is posted with that kind and note, and the reply names the task-N it became with 1 link
    And when the user chooses leave open, declines or closes the question, 0 posts are sent and the reply says nothing was written
    When the user accepts with start
    Then 1 accept is posted with start, and the reply says the task started

  @CC-40 @test
  # internal/ideas/tools_test.go
  Scenario: Without elicitation accept_idea and dismiss_idea act on the arguments as the user's decision
    Given an open idea that suggests an epic, and a Claude Code that cannot ask the user
    When Claude calls accept_idea with only its id and a note
    Then 1 accept is posted with kind epic, the idea's own, and the note
    When Claude calls dismiss_idea with its id and a note
    Then 1 dismiss is posted with the note, and the reply says the user dismissed the idea
    When Claude calls accept_idea with a kind that is neither task nor epic
    Then the tool fails with the server's reason, an idea becomes a task or an epic, and 0 tasks are made

  @CC-41 @test
  # internal/ideas/tools_test.go
  Scenario: An idea that looks like an open task makes none until the user chooses to anyway
    Given an open idea whose title is close to an open task's, and Claude Code able to ask the user
    # internal/ideas/duplicate.go SameTitleAbove: titles sharing more than 0.5 of their words look alike
    When Claude calls accept_idea for it and the user accepts
    Then the server refuses the accept with status 409, naming the open task
    And Claude Code asks the user a 2nd time, showing that task with 1 link, whether to make another anyway
    And when the user chooses leave open, the accept with anyway is posted 0 times and the reply says nothing was written
    And when the user chooses make another anyway, the accept is posted again with anyway, 2 posts in all, and the reply names the new task
    Given a Claude Code that cannot ask the user
    When Claude calls accept_idea for the idea
    Then the reply names the 1 open task it looks like, says nothing was written, and says to call accept_idea again with anyway
    When Claude calls accept_idea with anyway
    Then the accept is posted with anyway and the task is made: yes

  @CC-42 @test
  # internal/ideas/duplicate_test.go
  Scenario: Only an open task in the idea's repo counts as the task it looks like
    Given an idea and tasks with close titles in its repo and in another, some done
    When the open task it looks like is found
    Then a done task counts: no, a task in another repo counts: no, and a task in the Backlog counts: yes
    And of 2 candidates the closest title wins, and on a tie the 1 made first

  @CC-43 @test
  # internal/ideas/tools_test.go
  Scenario: dismiss_idea is the user's decision, asked through Claude Code
    Given an open idea, and Claude Code able to ask the user
    When Claude calls dismiss_idea for it
    Then Claude Code asks the user 1 time, with a note to give so the Visionaries don't suggest it again
    And when the user dismisses with a note, 1 dismiss is posted with it and the reply says the user dismissed the idea
    And when the user chooses leave open, 0 posts are sent
    When Claude calls dismiss_idea for an idea accepted already
    Then the tool fails, saying the idea was accepted already, and the idea is changed: no
    When Claude calls accept_idea for an idea dismissed already
    Then the tool fails, saying the idea was dismissed already, and a task is made: no
    When Claude calls accept_idea for an id no idea has
    Then the tool fails, saying no idea with that id

  @CC-44 @test
  # internal/ideas/mcp_test.go
  Scenario: The ideas tools are listed only while the Ideas feature shows
    Given a server whose Ideas feature is neither enabled nor visible
    When a Claude Code session starts
    Then tools/list has 0 of list_ideas, accept_idea and dismiss_idea, and the instructions have 0 lines about ideas
    And a call to one of them fails with no tool and its name
    When the Visionaries team is added, so the feature turns on
    Then the session sends notifications/tools/list_changed 1 time
    And tools/list has all 3 ideas tools: yes

  # ---- Hearing from Agentry -------------------------------------------------

  @CC-45 @test
  # internal/mcp/features_test.go
  Scenario: Every reply ends with what waits for the user
    Given 1 thing waiting for the user in this repo
    When Claude calls any tool but status
    Then the reply ends with Waiting for the user (1): and that 1 line
    # internal/mcp/tools.go waitingNote: at most 5 waiting lines are shown
    When Claude calls status
    Then the note is added: no, since the board says it already

  @CC-46 @test
  # internal/waitlist/waitlist_test.go
  Scenario: Each waiting line names its task as the user knows it, and the tool that deals with it
    Given an epic asking to close, a question on an epic and a question on a task waiting
    When the lines waiting for the user are read
    Then the epic's line reads epic-N: the epic, its title, is ready to close (close_epic), with 1 tool named
    And a question's line reads epic-N or task-N, who asks, and the question, as 2 lines name their task
    # internal/waitlist/waitlist.go Line: an approval ends with (review_approvals), cut at 140 characters; a copy says answer_question with Copy it asks the user, cut at 200
    # internal/waitlist/waitlist.go Line: an agent stuck on a prompt is cut at 120 characters; a question at 160, with Agentry's own stops named as Agentry
    And the approving answers Close the epic and Copy it are the coordinator's own 2 words: yes

  @CC-47 @test
  # internal/ideas/waiting_test.go
  Scenario: Open ideas wait for the user as one line
    Given 4 open ideas across 2 repos
    When the lines waiting for the user are read outside a repo
    Then there is 1 line: 4 open ideas from the Visionaries, the 3 newest titles, and 1 more, ending (list_ideas all)
    And when all open ideas are in this repo, the line ends (list_ideas), with all added: no
    And an idea the user decides on changes the line's key: no, so it is news 0 times
    And an idea Agentry accepted for the user by their rule is counted: no

  @CC-48 @manual
  # 1. Install the integration and leave 2 things waiting for you on the board.
  # 2. Start `claude` in a repo: the first reply mentions what waits, unasked.
  # 3. Send any message: Claude is told again, and repeats only what it hasn't told you.
  # 4. Stop Agentry and send a message: Claude Code carries on without Agentry.
  Scenario: SessionStart and UserPromptSubmit tell Claude what waits for the user
    Given 2 things waiting for the user and the hooks installed
    When a Claude Code session starts, or the user sends a message
    # cmd/agentry/claudecode.go sayWhatWaits: the hook gives the server 3 seconds
    Then the hook adds Agentry: 2 waiting for the user: and the 2 lines to what Claude reads, within 3 seconds
    And it tells Claude to say so briefly, offer to deal with them, and to repeat what it already told the user: no
    Given 0 things waiting, or no server answering
    When the hook runs
    Then it adds 0 lines, and Claude Code goes on without Agentry: yes

  @CC-49 @test
  # internal/mcp/waiting_test.go
  Scenario: The news a waiter wakes Claude with is only what is new
    Given 1 task done before the waiter started, and a worker online with a task
    When a task finishes, its coordinator tells the user something, and the worker goes offline with that task on it
    Then the news has 3 lines: the task is done with its summary, what the coordinator says, and the machine went offline with the task waiting until it is back
    # internal/mcp/waiting.go News: a summary is cut at 300 characters, a coordinator's word at 400
    And the task done before the waiter started is in the news: no
    And an epic is named epic-N: yes

  @CC-50 @manual
  # 1. With the hooks installed, send Claude a message and let it reply.
  # 2. Check one `agentry claude-hook rewake` process runs for the session (a pid file in the user cache's agentry/waiters folder).
  # 3. Let a task finish, or make something wait for you: Claude wakes and tells you what, with 1 link per item.
  # 4. Send another message and reply; check a second waiter is started: no, while the first lives.
  # 5. Run `agentry wait` in a terminal and make something wait: it prints the same news and returns.
  Scenario: After each reply a waiter wakes Claude when something new waits
    Given the Stop hook installed and a session idle
    When something new waits for the user, a task finishes, a coordinator replies, or a machine goes offline
    Then the waiter exits 2 with the news, which wakes Claude to tell the user in a line or two
    And 1 waiter runs per session: a second one started while the first lives exits 0 at once
    # cmd/agentry/claudecode.go waiterLifetime and waitToWake: 12 hours; the parent is checked every 10 seconds
    And a waiter ends after 12 hours, or within 10 seconds of Claude Code going away
    # internal/mcp/waiting.go WaitForNew: a wait call is answered within 40 seconds
    And it follows the log, each wait on the server answered within 40 seconds
    And `agentry wait` prints the same news 1 time and returns

  @CC-51 @manual
  # 1. Run `agentry mcp` through Claude Code on Linux with notify-send installed (osascript on macOS, PowerShell on Windows).
  # 2. Make something wait for you: a notification titled "Agentry: task-N needs you" shows with 1 link.
  # 3. Finish a task: the title reads "Agentry: task-N is done".
  # 4. Start Claude Code with the server stopped, then start the server: the notifications begin within 15 seconds.
  # 5. Start `agentry mcp --quiet`: no notification shows.
  Scenario: The computer shows a notification when something new waits
    Given Claude Code running with Agentry and a notifier on the computer
    When something starts waiting for the user, a task finishes, a coordinator replies, a machine goes offline or a new idea comes
    Then 1 notification shows, titled for its kind, with the task's line and 1 link to its page
    # internal/mcp/notify.go notifyEvery
    And while the server is starting, the watcher tries again every 15 seconds
    And with --quiet, 0 notifications show, and the status line still shows what waits

  @CC-52 @manual
  # 1. Install the integration; have 1 thing waiting and 3 tasks with an agent working.
  # 2. Look under Claude Code's prompt: "agentry · 1 needs you · 3 running", the first part in yellow.
  # 3. Make a second thing wait: it reads "2 need you".
  # 4. Stop Agentry: the line is empty within 2 seconds.
  Scenario: The status line shows what needs the user and what runs
    Given 1 thing waiting for the user and 3 tasks with an agent working
    When Claude Code draws its status line
    Then it reads agentry, 1 needs you, 3 running
    And running counts working tasks with an agent on them, and epics count: no
    And with 2 or more waiting it reads N need you
    # cmd/agentry/mcp.go runStatusLine: the server gets 2 seconds
    And when no server answers within 2 seconds, it prints 0 characters

  @CC-53 @manual
  # 1. In Claude Code, type /mcp__agentry__board.
  # 2. Check Claude calls status and then says what needs you, what runs and what is planned, in that order.
  Scenario: The board prompt shows the user their board
    Given Agentry installed in Claude Code
    When the user types the board prompt
    Then prompts/list has 1 prompt, board, and running it makes Claude call status 1 time
    And Claude says 3 things in order: what needs the user, with an offer to deal with it, what runs, and what is planned

  # ---- What the tools refuse ------------------------------------------------

  @CC-54 @test
  # internal/mcp/features_test.go
  Scenario: A hidden feature's tool cannot be called
    Given a feature the server shows: no
    When Claude calls one of its tools
    Then the tool fails with no tool and its name, and 0 calls reach the server for it

  @CC-55 @test
  # internal/mcp/server_test.go
  Scenario: The model cannot approve, close or copy for the user
    Given an approval and an epic asking to close, and a Claude Code that cannot ask the user
    When Claude calls review_approvals, close_epic, or answer_question with Close the epic
    Then approve, deny and Close the epic are posted 0 times
    And each reply says the decision is the user's and gives 1 link to the page

  @CC-56 @manual
  # 1. Call show_task with an id no task has, such as task-9999: the tool fails with "no such task task-9999".
  # 2. Call set_priority with an id no task has: it fails with "no task task-9999" and changes 0 ranks.
  # 3. Call close_epic with a task that is no epic: it fails saying it isn't ready to close.
  # 4. Call accept_idea with idea-9999: it fails with "no idea idea-9999".
  Scenario: A wrong id is refused with its name
    Given an id no task, epic or idea has
    When Claude calls show_task, set_priority, close_epic or accept_idea with it
    Then each tool fails naming the id, and 0 changes are made
    # internal/web/erase_api.go noTask: an unknown task is answered 404, an erased one 410 with its date
