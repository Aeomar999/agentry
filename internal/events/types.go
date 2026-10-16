package events

// Type is the kind of an event. Only the types in the catalogue can be written.
type Type string

// The agents stream: who can be handed work, and their prompts.
const (
	AgentCreated Type = "agent.created"
	AgentChanged Type = "agent.changed"
	AgentRemoved Type = "agent.removed"
	AgentLearned Type = "agent.learned" // an agent's knowledge, all of it: changed by the agent as it works, or by you
)

// A task's stream, in roughly the order they happen.
const (
	TaskCreated   Type = "task.created"
	TaskRenamed   Type = "task.renamed"  // a title that says what the task is, from the coordinator once it has read it
	TaskDirected  Type = "task.directed" // you tell the coordinator what you want next, as when you move a card on the board
	TaskLabeled   Type = "task.labeled"  // the task's labels, all of them, replacing the ones before
	TaskBudgeted  Type = "task.budgeted" // a new token budget for the task, as when you raise it
	TaskRanked    Type = "task.ranked"   // where the task sits in your priority order: lower is first
	TaskHeld      Type = "task.held"     // parked in a place ("todo", "backlog") until you move it on ("")
	TaskReopened  Type = "task.reopened" // a finished task back to work, with what you want done
	WorktreeReady Type = "worktree.ready"

	TaskAssigned       Type = "task.assigned" // the coordinator hands the task to an agent
	TurnStarted        Type = "turn.started"  // an agent's session started, with its prompt
	PromptAcknowledged Type = "prompt.acknowledged"
	SessionStarted     Type = "session.started" // the engine's own session id, for its transcript

	ActionRequested Type = "action.requested"
	ActionApproved  Type = "action.approved"
	ActionAsked     Type = "action.asked" // the policy hands the decision to the user
	ActionDenied    Type = "action.denied"
	ActionResult    Type = "action.result"

	AgentWaiting Type = "agent.waiting"
	TurnEnded    Type = "turn.ended"   // the agent finished; its final message is its report
	IssueRaised  Type = "issue.raised" // something went wrong; ends the open turn

	TaskNeedsYou Type = "task.needs_you"
	TaskAnswered Type = "task.answered"
	TaskDone     Type = "task.done"
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
	AgentsStream = "agents" // every agent event
	PolicyStream = "policy" // every policy event
)
