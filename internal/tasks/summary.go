package tasks

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
)

// Summary is a task as the list on the page shows it. It is all an
// archived task will keep, so it is made from the task and the roster
// alone; BlockedBy, which needs the other tasks, is the caller's to fill.
type Summary struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Repo             string    `json:"repo"`
	Status           Status    `json:"status"`
	CurrentAgent     string    `json:"currentAgent"`
	Stage            string    `json:"stage"`     // the agent the coordinator last handed it to
	Column           string    `json:"column"`    // its column on the board: backlog, todo, in-progress, review, shipping or done
	CanShip          bool      `json:"canShip"`   // its team has an agent that ships work, so it may be moved to Shipping
	Pushed           bool      `json:"pushed"`    // an agent pushed its work since it was last reopened: a done one says Shipped
	DoneByYou        bool      `json:"doneByYou"` // you moved it to Done, rather than its coordinator finishing it
	Turns            int       `json:"turns"`
	NeedsYou         string    `json:"needsYou"`    // what you are asked, "" when nothing
	HasQuestion      bool      `json:"hasQuestion"` // the coordinator asked you something: what you send next is the answer
	Labels           []string  `json:"labels"`
	Budget           int       `json:"budget"`           // tokens; 0 is no limit
	TokensUsed       int       `json:"tokensUsed"`       // by every turn that has ended
	OverBudget       bool      `json:"overBudget"`       // stopped, or about to stop, for its budget
	MaxTurns         int       `json:"maxTurns"`         // turns it may have without your answer; 0 is no limit
	TurnsSinceAnswer int       `json:"turnsSinceAnswer"` // turns since it started or you last answered
	AtTurnLimit      bool      `json:"atTurnLimit"`      // stopped, or about to stop, for its limit of turns
	StoppedAt        string    `json:"stoppedAt"`        // the limit whose stop is the open question, "budget" or "turns": raising it answers; "" for none
	Rank             float64   `json:"rank"`             // your priority order, lower first
	Parked           string    `json:"parked"`           // "todo" or "backlog" while you hold it there
	Parent           string    `json:"parent"`           // the task, of another team, that asked for this one
	WaitingOn        string    `json:"waitingOn"`        // another team's task this one waits for
	Kind             Kind      `json:"kind"`             // "" for a story, "epic"
	Epic             string    `json:"epic"`             // a story's epic
	Idea             string    `json:"idea"`             // the idea it was accepted as, "" for none
	After            []string  `json:"after"`            // the stories it builds on
	BlockedBy        []string  `json:"blockedBy"`        // of those, the ones not done, so it hasn't started
	Stories          int       `json:"stories"`          // an epic's
	StoriesDone      int       `json:"storiesDone"`
	Closing          string    `json:"closing"` // an epic with every story done: the summary you can close it with, "" otherwise
	Told             *Said     `json:"told"`    // the coordinator's latest word to you besides questions and summaries
	CreatedAt        time.Time `json:"createdAt"`
	LastActiveAt     time.Time `json:"lastActiveAt"`
}

// Said is what a coordinator last told you, as a summary shows it.
type Said struct {
	At   time.Time `json:"at"`
	From string    `json:"from"`
	Text string    `json:"text"`
}

// SummaryOf is the task's summary. The roster says which column its stage
// is in.
func SummaryOf(t *Task, roster agents.Roster) Summary {
	needsYou := t.Question
	if waiting := t.WaitingForYou(); needsYou == "" && len(waiting) > 0 {
		needsYou = WaitingLine(waiting[0].Agent, waiting[0].Command)
	}
	if open := t.OpenTurn(); needsYou == "" && open != nil {
		needsYou = open.Waiting
	}
	closing := t.Closing
	if t.IsDone {
		closing = ""
	}
	var said *Said
	if t.Told.Text != "" {
		said = &Said{At: t.Told.At, From: t.Told.From, Text: t.Told.Text}
	}
	return Summary{Told: said, ID: t.ID, Title: t.Title, Repo: t.Repo, Status: t.Status(), CurrentAgent: t.CurrentAgent(), Stage: t.Stage(), Column: t.boardColumn(roster), CanShip: TeamHas(roster, t.Coordinator, agents.Ships), Pushed: t.pushedSinceReopen(), DoneByYou: t.DoneByYou, HasQuestion: t.Question != "", Labels: listOf(t.Labels), Budget: t.Budget, TokensUsed: t.TokensUsed,
		OverBudget: t.AtBudget(), MaxTurns: t.MaxTurns, TurnsSinceAnswer: t.TurnsSinceAnswer, AtTurnLimit: t.AtTurnLimit(), StoppedAt: t.StoppedAt(), Rank: t.Rank, Parked: t.Parked,
		Parent: t.Parent, WaitingOn: t.WaitingOn,
		Kind: t.Kind, Epic: t.Epic, Idea: t.MadeFrom, After: listOf(t.After), BlockedBy: []string{}, Stories: len(t.Stories), StoriesDone: len(t.StoriesDone), Closing: closing,
		Turns: len(t.Turns), NeedsYou: needsYou, CreatedAt: t.CreatedAt, LastActiveAt: t.LastActiveAt}
}

// pushedSinceReopen says whether any of the task's turns since it was last
// reopened pushed its work: an earlier push didn't take the rework to dev.
func (t *Task) pushedSinceReopen() bool {
	for _, turn := range t.Turns[t.TurnsBeforeReopen:] {
		if t.Pushed(turn.ID) {
			return true
		}
	}
	return false
}

// listOf makes nil an empty list, so the page reads [], never null.
func listOf(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

// boardColumn is the task's column on the board: Done, where you parked it,
// or its work's column.
func (t *Task) boardColumn(roster agents.Roster) string {
	switch {
	case t.IsDone:
		return "done"
	case t.Parked != "":
		return t.Parked
	}
	return t.WorkColumn(roster)
}

// WorkColumn is the column of the task's work, done or parked or not: the
// column you moved it to while that stands; otherwise its stage's: Review
// while an agent that checks work has it, Shipping while one that ships
// work has it.
func (t *Task) WorkColumn(roster agents.Roster) string {
	if t.Column != "" {
		return t.Column
	}
	agent, _ := roster.Get(t.Stage())
	switch agent.Column {
	case agents.Reviews:
		return "review"
	case agents.Ships:
		return "shipping"
	}
	return "in-progress"
}

// Stage is the agent the coordinator last handed the task to, or "" before
// the first handoff.
func (t *Task) Stage() string {
	for i := len(t.Trail) - 1; i >= 0; i-- {
		if step := t.Trail[i]; step.From == t.Coordinator && step.To != You {
			return step.To
		}
	}
	return ""
}
