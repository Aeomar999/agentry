package web

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The JSON the page reads. Kept apart from the task state so the state can
// change shape without breaking the page, and the page sees only what it shows.

type taskSummary struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Repo         string       `json:"repo"`
	Status       tasks.Status `json:"status"`
	CurrentAgent string       `json:"currentAgent"`
	Stage        string       `json:"stage"` // the agent the coordinator last handed it to: its column on the board
	Turns        int          `json:"turns"`
	NeedsYou     string       `json:"needsYou"`    // what you are asked, "" when nothing
	HasQuestion  bool         `json:"hasQuestion"` // the coordinator asked you something: what you send next is the answer
	Labels       []string     `json:"labels"`
	Budget       int          `json:"budget"`     // tokens; 0 is no limit
	TokensUsed   int          `json:"tokensUsed"` // by every turn that has ended
	OverBudget   bool         `json:"overBudget"` // stopped, or about to stop, for its budget
	Rank         float64      `json:"rank"`       // your priority order, lower first
	Parked       string       `json:"parked"`     // "todo" or "backlog" while you hold it there
	CreatedAt    time.Time    `json:"createdAt"`
	LastActiveAt time.Time    `json:"lastActiveAt"`
}

type taskDetail struct {
	taskSummary
	Description string       `json:"description"`
	FirstTitle  string       `json:"firstTitle"` // when the coordinator renamed the task
	Options     []string     `json:"options"`    // answers the open question offers
	Page        string       `json:"page"`       // the page the open question came with
	Coordinator string       `json:"coordinator"`
	Summary     string       `json:"summary"`
	Worktree    *worktree    `json:"worktree"`
	Trail       []step       `json:"trail"`
	Turns       []turn       `json:"turnList"`
	Approvals   []action     `json:"approvals"` // waiting for you
	Actions     []action     `json:"actions"`
	Events      []eventEntry `json:"events"`
}

type worktree struct {
	Worker string `json:"worker"` // the machine it is on: "local", or a worker's name
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
	Port   int    `json:"port"`
}

type step struct {
	At   time.Time `json:"at"`
	From string    `json:"from"`
	To   string    `json:"to"`
	Text string    `json:"text"`
}

type turn struct {
	ID           string    `json:"id"`
	Agent        string    `json:"agent"`
	Worker       string    `json:"worker"` // the machine it ran on
	StartedAt    time.Time `json:"startedAt"`
	Acknowledged bool      `json:"acknowledged"`
	Ended        bool      `json:"ended"`
	Issue        string    `json:"issue"`
	Tokens       int       `json:"tokens"`
}

type action struct {
	ID          string    `json:"id"`
	Turn        string    `json:"turn"`
	Agent       string    `json:"agent"`
	Tool        string    `json:"tool"`
	Command     string    `json:"command"`
	RequestedAt time.Time `json:"requestedAt"`
	Asked       bool      `json:"asked"`
	Decision    string    `json:"decision"`
	Reason      string    `json:"reason"`
}

type eventEntry struct {
	Position    int64          `json:"position"`
	At          time.Time      `json:"at"`
	Type        events.Type    `json:"type"`
	Actor       string         `json:"actor"`
	Correlation string         `json:"correlation,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

func summaryOf(t *tasks.Task) taskSummary {
	needsYou := t.Question
	if waiting := t.WaitingForYou(); needsYou == "" && len(waiting) > 0 {
		needsYou = waiting[0].Agent + " asks to run: " + waiting[0].Command
	}
	if open := t.OpenTurn(); needsYou == "" && open != nil {
		needsYou = open.Waiting
	}
	return taskSummary{ID: t.ID, Title: t.Title, Repo: t.Repo, Status: t.Status(), CurrentAgent: t.CurrentAgent(), Stage: stageOf(t), HasQuestion: t.Question != "", Labels: listOf(t.Labels), Budget: t.Budget, TokensUsed: t.TokensUsed,
		OverBudget: t.Budget > 0 && t.TokensUsed >= t.Budget, Rank: t.Rank, Parked: t.Parked,
		Turns: len(t.Turns), NeedsYou: needsYou, CreatedAt: t.CreatedAt, LastActiveAt: t.LastActiveAt}
}

// listOf makes nil an empty list, so the page reads [], never null.
func listOf(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

// stageOf is the agent the coordinator last handed the task to, or "" before
// the first handoff.
func stageOf(t *tasks.Task) string {
	for i := len(t.Trail) - 1; i >= 0; i-- {
		if step := t.Trail[i]; step.From == t.Coordinator && step.To != tasks.You {
			return step.To
		}
	}
	return ""
}

func detailOf(t *tasks.Task, log []events.Event) taskDetail {
	detail := taskDetail{taskSummary: summaryOf(t), Description: t.Description, FirstTitle: t.FirstTitle, Options: listOf(t.Asked.Options), Page: t.Asked.Page,
		Coordinator: t.Coordinator, Summary: t.Summary}
	if w := t.Worktree; w != nil {
		detail.Worktree = &worktree{Worker: w.Worker, Path: w.Path, Branch: w.Branch, Base: w.Base, Port: w.Port}
	}
	for _, s := range t.Trail {
		detail.Trail = append(detail.Trail, step{At: s.At, From: s.From, To: s.To, Text: s.Text})
	}
	for _, tu := range t.Turns {
		detail.Turns = append(detail.Turns, turn{ID: tu.ID, Agent: tu.Agent, Worker: tu.Worker, StartedAt: tu.StartedAt,
			Acknowledged: tu.Acknowledged, Ended: tu.Ended, Issue: tu.Issue, Tokens: tu.Tokens})
	}
	for _, a := range t.Actions {
		view := action{ID: a.ID, Turn: a.Turn, Agent: a.Agent, Tool: a.Tool, Command: a.Command,
			RequestedAt: a.RequestedAt, Asked: a.Asked, Decision: string(a.Decision), Reason: a.Reason}
		detail.Actions = append(detail.Actions, view)
		if a.Asked && a.Decision == tasks.Undecided {
			detail.Approvals = append(detail.Approvals, view)
		}
	}
	for _, e := range log {
		detail.Events = append(detail.Events, eventEntry{Position: e.Position, At: e.At, Type: e.Type,
			Actor: e.Actor.String(), Correlation: e.Correlation, Data: e.Data})
	}
	return detail
}

func agentsOf(roster agents.Roster) []agents.Agent {
	list := []agents.Agent{}
	for _, a := range roster {
		if a.Workers == nil {
			a.Workers = []string{} // the page reads a list, never null
		}
		list = append(list, a)
	}
	return list
}
