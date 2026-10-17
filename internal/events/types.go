package events

// Type is the kind of an event. Only the types in the catalogue can be written.
type Type string

// The agents stream: who can be handed work, and their prompts.
const (
	AgentCreated Type = "agent.created"
	AgentChanged Type = "agent.changed"
	AgentRemoved Type = "agent.removed"
	AgentLearned Type = "agent.learned" // an agent's knowledge, all of it: changed by the agent as it works, or by you
	AgentRenamed Type = "agent.renamed" // an agent goes by a new name, keeping everything else
)

// A task's stream, in roughly the order they happen.
const (
	ConnectionInvited Type = "connection.invited" // you made an install command for Claude Code on a machine
	ConnectionMade    Type = "connection.made"    // a machine used it: its Claude Code has Agentry's tools
	ConnectionRevoked Type = "connection.revoked" // you took them away again

	TaskCreated        Type = "task.created"
	TaskRenamed        Type = "task.renamed"       // a title that says what the task is, from the coordinator once it has read it
	TaskDirected       Type = "task.directed"      // you tell the coordinator what you want next, as when you move a card on the board
	TaskLabeled        Type = "task.labeled"       // the task's labels, all of them, replacing the ones before
	TaskBudgeted       Type = "task.budgeted"      // a new token budget for the task, as when you raise it
	TaskRanked         Type = "task.ranked"        // where the task sits in your priority order: lower is first
	TaskHeld           Type = "task.held"          // parked in a place ("todo", "backlog") until you move it on ("")
	TaskReopened       Type = "task.reopened"      // a finished task back to work, with what you want done
	RepoChanged        Type = "task.repo_changed"  // the task's work goes in another repo from now on; its worktree is made again there
	AgentRenamedInTask Type = "task.agent_renamed" // an agent the task names was renamed: its coordinator, or who it is handed to

	TaskTransferred Type = "task.transferred"      // the task is another team's work: their coordinator takes it
	TaskRequested   Type = "task.requested"        // the task asked another team for something, a task of theirs it waits for
	RequestAnswered Type = "task.request_answered" // the other team's task it waited for is done: its summary comes back
	TaskSplit       Type = "task.split"            // the coordinator split the task into stories: it is an epic now, or one with more stories
	StoryDone       Type = "task.story_done"       // one of the epic's stories is done: its summary comes back
	EpicWaits       Type = "task.epic_waits"       // the epic's coordinator has nothing to do until more of its stories are done
	EpicJoined      Type = "task.epic_joined"      // you made the task a story of an epic
	EpicLeft        Type = "task.epic_left"        // you took the task out of its epic
	StoryAdded      Type = "task.story_added"      // you added a task of yours to the epic, as a story
	StoryRemoved    Type = "task.story_removed"    // you took a story out of the epic

	// A task's shared files (package sharedfiles).
	FileAdded      Type = "file.added"      // a file shared with the task, or a new version of one: yours, or from a turn's folder
	FileRemoved    Type = "file.removed"    // a file no longer shared with it
	FilesCollected Type = "files.collected" // a turn's folder was read back: what it changed is in file.added and file.removed
	FilesDelivered Type = "files.delivered" // a machine's folder holds the task's files as they are now
	FileFetching   Type = "file.fetching"   // the coordinator asked for a file from a machine; it is coming
	FilesFetched   Type = "files.fetched"   // it came: a file, or a folder's files
	WorktreeReady  Type = "worktree.ready"
	WorktreeMoved  Type = "worktree.moved" // the task's work went to another machine, for the agent working next

	TaskAssigned       Type = "task.assigned" // the coordinator hands the task to an agent
	TurnStarted        Type = "turn.started"  // an agent's session started, with its prompt
	PromptAcknowledged Type = "prompt.acknowledged"
	SessionStarted     Type = "session.started" // the engine's own session id, for its transcript

	ActionRequested Type = "action.requested"
	ActionApproved  Type = "action.approved"
	ActionAsked     Type = "action.asked" // the policy hands the decision to the user
	ActionDenied    Type = "action.denied"
	ActionResult    Type = "action.result"

	PlanUpdated  Type = "plan.updated" // the turn's plan, all of it, as the agent last wrote it
	AgentWaiting Type = "agent.waiting"
	TurnEnded    Type = "turn.ended"   // the agent finished; its final message is its report
	IssueRaised  Type = "issue.raised" // something went wrong; ends the open turn
	TurnStopped  Type = "turn.stopped" // you ended the open turn, to tell the coordinator something

	TaskNeedsYou Type = "task.needs_you"
	TaskAnswered Type = "task.answered"
	TaskDone     Type = "task.done"
)

// The workers stream: the machines, besides Agentry's own, that run agents.
const (
	WorkerInvited      Type = "worker.invited"      // a join code was made; only its hash is kept
	WorkerJoined       Type = "worker.joined"       // a machine used a code, and got its credential (hash kept)
	WorkerConnected    Type = "worker.connected"    // it is online, with what it has (engines, tools)
	WorkerDisconnected Type = "worker.disconnected" // it went offline
	WorkerRemoved      Type = "worker.removed"      // its credential no longer works
)

// The policy stream: every policy.toml Agentry loaded or refused.
const (
	PolicyLoaded  Type = "policy.loaded"
	PolicyRefused Type = "policy.refused" // the file had an error; the policy before it stays
)

// ProposalRejected is written by the store, never proposed.
const ProposalRejected Type = "proposal.rejected"

// Streams that aren't tasks. Every other stream is one task's, named by its ID.
const (
	AgentsStream      = "agents"      // every agent event
	PolicyStream      = "policy"      // every policy event
	WorkersStream     = "workers"     // every worker event
	ConnectionsStream = "connections" // every Claude Code connection's event
)
