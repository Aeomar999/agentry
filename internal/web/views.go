package web

import (
	"fmt"
	"maps"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The JSON the page reads. Kept apart from the task state so the state can
// change shape without breaking the page, and the page sees only what it shows.

type taskSummary struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	Repo             string       `json:"repo"`
	Status           tasks.Status `json:"status"`
	CurrentAgent     string       `json:"currentAgent"`
	Stage            string       `json:"stage"`  // the agent the coordinator last handed it to
	Column           string       `json:"column"` // its column on the board: backlog, todo, in-progress, review or done
	Turns            int          `json:"turns"`
	NeedsYou         string       `json:"needsYou"`    // what you are asked, "" when nothing
	HasQuestion      bool         `json:"hasQuestion"` // the coordinator asked you something: what you send next is the answer
	Labels           []string     `json:"labels"`
	Budget           int          `json:"budget"`           // tokens; 0 is no limit
	TokensUsed       int          `json:"tokensUsed"`       // by every turn that has ended
	OverBudget       bool         `json:"overBudget"`       // stopped, or about to stop, for its budget
	MaxTurns         int          `json:"maxTurns"`         // turns it may have without your answer; 0 is no limit
	TurnsSinceAnswer int          `json:"turnsSinceAnswer"` // turns since it started or you last answered
	AtTurnLimit      bool         `json:"atTurnLimit"`      // stopped, or about to stop, for its limit of turns
	StoppedAt        string       `json:"stoppedAt"`        // the limit whose stop is the open question, "budget" or "turns": raising it answers; "" for none
	Rank             float64      `json:"rank"`             // your priority order, lower first
	Parked           string       `json:"parked"`           // "todo" or "backlog" while you hold it there
	Parent           string       `json:"parent"`           // the task, of another team, that asked for this one
	WaitingOn        string       `json:"waitingOn"`        // another team's task this one waits for
	Kind             tasks.Kind   `json:"kind"`             // "" for a story, "epic"
	Epic             string       `json:"epic"`             // a story's epic
	After            []string     `json:"after"`            // the stories it builds on
	BlockedBy        []string     `json:"blockedBy"`        // of those, the ones not done, so it hasn't started
	Stories          int          `json:"stories"`          // an epic's
	StoriesDone      int          `json:"storiesDone"`
	Closing          string       `json:"closing"` // an epic with every story done: the summary you can close it with, "" otherwise
	Told             *told        `json:"told"`    // the coordinator's latest word to you besides questions and summaries
	CreatedAt        time.Time    `json:"createdAt"`
	LastActiveAt     time.Time    `json:"lastActiveAt"`
}

type told struct {
	At   time.Time `json:"at"`
	From string    `json:"from"`
	Text string    `json:"text"`
}

type taskDetail struct {
	taskSummary
	Description string        `json:"description"`
	FirstTitle  string        `json:"firstTitle"` // when the coordinator renamed the task
	Options     []string      `json:"options"`    // answers the open question offers
	Page        string        `json:"page"`       // the page the open question came with
	Coordinator string        `json:"coordinator"`
	TeamChosen  bool          `json:"teamChosen"` // false: its coordinator may still pass it to the right team
	EpicTitle   string        `json:"epicTitle"`  // a story's epic's
	StoryList   []taskSummary `json:"storyList"`  // an epic's stories, in the order they were planned
	Shared      []sharedFile  `json:"shared"`     // its shared files
	MovingTo    string        `json:"movingTo"`   // the repo it moves to when the turn in progress ends
	Summary     string        `json:"summary"`
	Worktree    *worktree     `json:"worktree"`
	Trail       []step        `json:"trail"`
	Turns       []turn        `json:"turnList"`
	Approvals   []action      `json:"approvals"` // waiting for you
	Actions     []action      `json:"actions"`
	Events      []eventEntry  `json:"events"`
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
	Turn string    `json:"turn"` // the turn it led to, "" when none has started
}

type turn struct {
	ID           string    `json:"id"`
	Agent        string    `json:"agent"`
	Worker       string    `json:"worker"` // the machine it ran on
	StartedAt    time.Time `json:"startedAt"`
	LastEventAt  time.Time `json:"lastEventAt"` // when it ended, once it has
	Acknowledged bool      `json:"acknowledged"`
	Ended        bool      `json:"ended"`
	Issue        string    `json:"issue"`
	Tokens       int       `json:"tokens"`
	Plan         string    `json:"plan"`     // a Markdown checklist, "" until the agent writes one
	PlanFrom     string    `json:"planFrom"` // the turn whose unfinished plan it started on, "" for a fresh one
	Stopped      bool      `json:"stopped"`  // you ended it, to tell the coordinator something
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
	Finished    bool      `json:"finished"` // the tool ran and returned
}

type eventEntry struct {
	Position    int64          `json:"position"`
	At          time.Time      `json:"at"`
	Type        events.Type    `json:"type"`
	Actor       string         `json:"actor"`
	Correlation string         `json:"correlation,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

func summaryOf(t *tasks.Task, roster agents.Roster) taskSummary {
	needsYou := t.Question
	if waiting := t.WaitingForYou(); needsYou == "" && len(waiting) > 0 {
		needsYou = waiting[0].Agent + " asks to run: " + waiting[0].Command
	}
	if open := t.OpenTurn(); needsYou == "" && open != nil {
		needsYou = open.Waiting
	}
	closing := t.Closing
	if t.IsDone {
		closing = ""
	}
	var said *told
	if t.Told.Text != "" {
		said = &told{At: t.Told.At, From: t.Told.From, Text: t.Told.Text}
	}
	return taskSummary{Told: said, ID: t.ID, Title: t.Title, Repo: t.Repo, Status: t.Status(), CurrentAgent: t.CurrentAgent(), Stage: stageOf(t), Column: columnOf(t, roster), HasQuestion: t.Question != "", Labels: listOf(t.Labels), Budget: t.Budget, TokensUsed: t.TokensUsed,
		OverBudget: t.AtBudget(), MaxTurns: t.MaxTurns, TurnsSinceAnswer: t.TurnsSinceAnswer, AtTurnLimit: t.AtTurnLimit(), StoppedAt: coordinator.StoppedAt(t), Rank: t.Rank, Parked: t.Parked,
		Parent: t.Parent, WaitingOn: t.WaitingOn,
		Kind: t.Kind, Epic: t.Epic, After: listOf(t.After), BlockedBy: []string{}, Stories: len(t.Stories), StoriesDone: len(t.StoriesDone), Closing: closing,
		Turns: len(t.Turns), NeedsYou: needsYou, CreatedAt: t.CreatedAt, LastActiveAt: t.LastActiveAt}
}

// blockedBy are the stories a story builds on that aren't done, while it
// waits to start for them.
func blockedBy(t *tasks.Task, done func(id string) bool) []string {
	blocked := []string{}
	if t.IsDone || t.Worktree != nil {
		return blocked
	}
	for _, id := range t.After {
		if !done(id) {
			blocked = append(blocked, id)
		}
	}
	return blocked
}

// listOf makes nil an empty list, so the page reads [], never null.
func listOf(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

// columnOf is the task's column on the board: Done, where you parked it, or
// its work's column.
func columnOf(t *tasks.Task, roster agents.Roster) string {
	switch {
	case t.IsDone:
		return "done"
	case t.Parked != "":
		return t.Parked
	}
	return workColumn(t, roster)
}

// workColumn is the column of the task's work, done or parked or not: the
// column you moved it to while that stands; otherwise its stage's, Review
// while an agent that checks work has it.
func workColumn(t *tasks.Task, roster agents.Roster) string {
	if t.Column != "" {
		return t.Column
	}
	if agent, ok := roster.Get(stageOf(t)); ok && agent.Column == agents.Reviews {
		return "review"
	}
	return "in-progress"
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

func detailOf(t *tasks.Task, roster agents.Roster, log []events.Event) taskDetail {
	detail := taskDetail{taskSummary: summaryOf(t, roster), Description: t.Description, FirstTitle: t.FirstTitle, Options: listOf(t.Asked.Options), Page: t.Asked.Page,
		Coordinator: t.Coordinator, TeamChosen: t.TeamChosen, Summary: t.Summary, MovingTo: t.MovingTo}
	detail.Shared = []sharedFile{}
	for _, f := range t.SharedFiles() {
		detail.Shared = append(detail.Shared, sharedFile{Name: f.Name, Size: f.Size, By: f.By, At: f.At.Format(time.RFC3339), From: f.From})
	}
	if w := t.Worktree; w != nil {
		detail.Worktree = &worktree{Worker: w.Worker, Path: w.Path, Branch: w.Branch, Base: w.Base, Port: w.Port}
	}
	for _, s := range t.Trail {
		detail.Trail = append(detail.Trail, step{At: s.At, From: s.From, To: s.To, Text: s.Text, Turn: turnOf(t, s)})
	}
	for _, tu := range t.Turns {
		detail.Turns = append(detail.Turns, turn{ID: tu.ID, Agent: tu.Agent, Worker: tu.Worker, StartedAt: tu.StartedAt,
			LastEventAt: tu.LastEventAt, Acknowledged: tu.Acknowledged, Ended: tu.Ended, Issue: tu.Issue, Tokens: tu.Tokens, Plan: tu.Plan, PlanFrom: tu.PlanFrom, Stopped: tu.Stopped})
	}
	for _, a := range t.Actions {
		view := action{ID: a.ID, Turn: a.Turn, Agent: a.Agent, Tool: a.Tool, Command: a.Command,
			RequestedAt: a.RequestedAt, Asked: a.Asked, Decision: string(a.Decision), Reason: a.Reason, Finished: a.HasResult}
		detail.Actions = append(detail.Actions, view)
		if a.Asked && a.Decision == tasks.Undecided {
			detail.Approvals = append(detail.Approvals, view)
		}
	}
	for _, e := range log {
		data := e.Data
		if page, ok := data["html"].(string); ok { // a page kept in the log, from when agents published artifacts
			data = maps.Clone(data)
			data["html"] = fmt.Sprintf("(a page of %d bytes)", len(page))
		}
		detail.Events = append(detail.Events, eventEntry{Position: e.Position, At: e.At, Type: e.Type,
			Actor: e.Actor.String(), Correlation: e.Correlation, Data: data})
	}
	return detail
}

// turnOf is the turn a trail step leads to: the next turn of the agent it
// was said to, so every turn is reached from the step that started it. What
// is said to you starts no turn: it belongs to the turn that said it.
func turnOf(t *tasks.Task, s tasks.Step) string {
	found := ""
	for _, turn := range t.Turns {
		if s.To == tasks.You && turn.Agent == s.From && !turn.StartedAt.After(s.At) {
			found = turn.ID
		}
		if s.To != tasks.You && turn.Agent == s.To && !turn.StartedAt.Before(s.At) {
			return turn.ID
		}
	}
	return found
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
