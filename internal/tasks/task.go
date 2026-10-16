// Package tasks computes a task's state from its events. The state is never
// stored, so it can never disagree with what happened.
package tasks

import "time"

// Status is the one word the page shows for a task.
type Status string

const (
	Working  Status = "working"
	NeedsYou Status = "needs you"
	Done     Status = "done"
)

type Task struct {
	ID          string
	Title       string
	FirstTitle  string // the title it was created with, when it has been renamed
	Labels      []string
	Description string
	Repo        string
	Coordinator string // the agent that decides who works on the task next
	CreatedAt   time.Time

	Worktree *Worktree // nil until the worktree is ready
	Turns    []*Turn   // every agent turn, in order
	Actions  []*Action // every action an agent asked to take, in order
	Trail    []Step    // the handoffs, as the page shows them

	Question     string // an open question for the user, "" when there is none
	Summary      string // the coordinator's summary, once done
	IsDone       bool
	LastActiveAt time.Time

	// What is waiting to happen. The coordinator loop acts on these.
	Handoff         *Step  // the latest step to an agent, until that agent's turn starts
	PendingDecision string // a coordinator's reply, until its decision is applied

	// For the loop's safety stops.
	TurnsSinceAnswer int // turns since the task started or you last answered
	IssuesInARow     int // issues with no normal turn end between them
}

type Worktree struct {
	Repo   string // the main checkout it was made from
	Path   string
	Branch string
	Base   string // the commit the branch started from
	Port   int
}

// Turn is one agent working once, in its own session, from one prompt.
type Turn struct {
	ID           string // also the correlation of its events
	Agent        string
	Hash         string // of the prompt line it was started with
	StartedAt    time.Time
	LastEventAt  time.Time
	Acknowledged bool   // the engine echoed the prompt's hash (proof 02)
	Waiting      string // what the agent is waiting on, until it does something
	Ended        bool
	Report       string // the agent's final message
	Issue        string // why the turn was cut short, if it was
}

type Decision string

const (
	Undecided Decision = ""
	Approved  Decision = "approved"
	Denied    Decision = "denied"
)

// Action is one tool call an agent asked to make, and what was decided.
type Action struct {
	ID          string // the engine's tool_use_id, the correlation of its events
	Turn        string
	Agent       string
	Tool        string
	Command     string
	RequestedAt time.Time
	Asked       bool // the policy handed it to the user
	Decision    Decision
	Reason      string // the rule that asked, then the reason for the decision
	HasResult   bool
}

// Step is one line of the handoff trail: who passed what to whom.
type Step struct {
	At   time.Time
	From string
	To   string
	Text string
}

// Names used in the trail for the people and things that are not agents.
const (
	You     = "you"
	Agentry = "agentry"
)

func (t *Task) Status() Status {
	switch {
	case t.IsDone:
		return Done
	case t.Question != "" || len(t.WaitingForYou()) > 0:
		return NeedsYou
	case t.OpenTurn() != nil && t.OpenTurn().Waiting != "":
		return NeedsYou
	}
	return Working
}

// OpenTurn is the turn still running, or nil between turns.
func (t *Task) OpenTurn() *Turn {
	if len(t.Turns) == 0 || t.Turns[len(t.Turns)-1].Ended {
		return nil
	}
	return t.Turns[len(t.Turns)-1]
}

func (t *Task) LastTurn() *Turn {
	if len(t.Turns) == 0 {
		return nil
	}
	return t.Turns[len(t.Turns)-1]
}

func (t *Task) Turn(id string) *Turn {
	for _, turn := range t.Turns {
		if turn.ID == id {
			return turn
		}
	}
	return nil
}

// NextTurnID is the ID the next turn must have.
func (t *Task) NextTurnID() string {
	return turnID(len(t.Turns) + 1)
}

func (t *Task) Action(id string) *Action {
	for _, action := range t.Actions {
		if action.ID == id {
			return action
		}
	}
	return nil
}

// WaitingForYou are the actions the policy asked the user about, not yet decided.
// action.asked is the only way an action puts a task in Needs you (proof 05).
func (t *Task) WaitingForYou() []*Action {
	var waiting []*Action
	for _, action := range t.Actions {
		if action.Asked && action.Decision == Undecided {
			waiting = append(waiting, action)
		}
	}
	return waiting
}

// CurrentAgent is who has the task right now: the agent in the open turn, or "".
func (t *Task) CurrentAgent() string {
	if turn := t.OpenTurn(); turn != nil {
		return turn.Agent
	}
	return ""
}
