package web

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The JSON the page reads. Kept apart from the task state so the state can
// change shape without breaking the page, and the page sees only what it shows.

// taskSummary is a task in the list, as the tasks package summarizes it,
// so an archived task can be its summary alone.
type taskSummary = tasks.Summary

type taskDetail struct {
	taskSummary
	Description   string        `json:"description"`
	FirstTitle    string        `json:"firstTitle"` // when the coordinator renamed the task
	Options       []string      `json:"options"`    // answers the open question offers
	Page          string        `json:"page"`       // the page the open question came with
	Coordinator   string        `json:"coordinator"`
	TeamChosen    bool          `json:"teamChosen"` // false: its coordinator may still pass it to the right team
	EpicTitle     string        `json:"epicTitle"`  // a story's epic's
	StoryList     []taskSummary `json:"storyList"`  // an epic's stories, in the order they were planned
	Shared        []sharedFile  `json:"shared"`     // its shared files
	MovingTo      string        `json:"movingTo"`   // the repo it moves to when the turn in progress ends
	Summary       string        `json:"summary"`
	Worktree      *worktree     `json:"worktree"`
	Trail         []step        `json:"trail"`
	Turns         []turn        `json:"turnList"`
	TokensUnknown string        `json:"tokensUnknown"` // on how many turns the tokens couldn't be counted, and why; "" when every turn was
	Approvals     []action      `json:"approvals"`     // waiting for you
	Actions       []action      `json:"actions"`
	Events        []eventEntry  `json:"events"`
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
	ID            string     `json:"id"`
	Agent         string     `json:"agent"`
	Worker        string     `json:"worker"` // the machine it ran on
	StartedAt     time.Time  `json:"startedAt"`
	LastEventAt   time.Time  `json:"lastEventAt"` // when it ended, once it has
	Acknowledged  bool       `json:"acknowledged"`
	Ended         bool       `json:"ended"`
	Issue         string     `json:"issue"`
	Tokens        int        `json:"tokens"`
	TokensUnknown string     `json:"tokensUnknown,omitempty"` // why tokens is 0, when the engine couldn't say
	Plan          string     `json:"plan"`                    // a Markdown checklist, "" until the agent writes one
	PlanFrom      string     `json:"planFrom"`                // the turn whose unfinished plan it started on, "" for a fresh one
	Stopped       bool       `json:"stopped"`                 // you ended it, to tell the coordinator something
	Limited       string     `json:"limited,omitempty"`       // what the engine said when it stopped at a usage limit
	ResetsAt      *time.Time `json:"resetsAt,omitempty"`      // when that limit resets, when the engine said
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

func detailOf(t *tasks.Task, roster agents.Roster, log []events.Event) taskDetail {
	detail := taskDetail{taskSummary: tasks.SummaryOf(t, roster), Description: t.Description, FirstTitle: t.FirstTitle, Options: listOf(t.Asked.Options), Page: t.Asked.Page,
		Coordinator: t.Coordinator, TeamChosen: t.TeamChosen, Summary: t.Summary, MovingTo: t.MovingTo, TokensUnknown: tokensUnknownOf(t)}
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
			LastEventAt: tu.LastEventAt, Acknowledged: tu.Acknowledged, Ended: tu.Ended, Issue: tu.Issue, Tokens: tu.Tokens, TokensUnknown: tu.TokensUnknown, Plan: tu.Plan, PlanFrom: tu.PlanFrom, Stopped: tu.Stopped, Limited: tu.Limited})
		if resets := tu.ResetsAt; !resets.IsZero() {
			detail.Turns[len(detail.Turns)-1].ResetsAt = &resets
		}
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

// tokensUnknownOf says on how many turns the tokens couldn't be counted,
// so the tokens used fall short of what was spent, and why: "Tokens unknown
// on 3 turns: transcript not saved". "" when every turn was counted.
func tokensUnknownOf(t *tasks.Task) string {
	var reasons []string
	count := map[string]int{}
	for _, turn := range t.Turns {
		if reason := turn.TokensUnknown; reason != "" {
			if count[reason] == 0 {
				reasons = append(reasons, reason)
			}
			count[reason]++
		}
	}
	if len(reasons) == 1 {
		return fmt.Sprintf("Tokens unknown on %s: %s", turnCount(count[reasons[0]]), reasons[0])
	}
	total := 0
	for i, reason := range reasons {
		total += count[reason]
		reasons[i] = fmt.Sprintf("%s (%d)", reason, count[reason])
	}
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("Tokens unknown on %s: %s", turnCount(total), strings.Join(reasons, ", "))
}

func turnCount(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return fmt.Sprintf("%d turns", n)
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

// agentView is an agent with the profile that judges its calls, when there
// is a policy.
type agentView struct {
	agents.Agent
	Effective *effectiveView `json:"effective,omitempty"`
}

func agentsOf(roster agents.Roster, p *policy.Policy) []agentView {
	list := []agentView{}
	for _, a := range roster {
		if a.Workers == nil {
			a.Workers = []string{} // the page reads a list, never null
		}
		view := agentView{Agent: a}
		if p != nil {
			effective := effectiveOf(p, a.Profile)
			view.Effective = &effective
		}
		list = append(list, view)
	}
	return list
}

type ideaView struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Problem    string      `json:"problem"`
	Proposal   string      `json:"proposal"`
	WhyNow     string      `json:"whyNow"`
	Size       string      `json:"size"` // S, M or L
	Risks      string      `json:"risks"`
	BackedBy   []string    `json:"backedBy"`
	DoubtedBy  []string    `json:"doubtedBy"`
	Task       string      `json:"task"` // the session it came from
	Repo       string      `json:"repo"`
	Epic       string      `json:"epic"`       // the epic it follows up, "" for none
	About      string      `json:"about"`      // the epic whose close started the session it came from, "" for none
	AboutTitle string      `json:"aboutTitle"` // that epic's title
	Kind       tasks.Kind  `json:"kind"`       // what it suggests making of it: "" for a story, "epic"
	State      ideas.State `json:"state"`
	Note       string      `json:"note"`         // what you said when you accepted or dismissed it
	Became     string      `json:"became"`       // the task an accepted idea became
	Auto       bool        `json:"autoAccepted"` // Agentry accepted it, as every Visionary agreed, not you
	CreatedAt  time.Time   `json:"createdAt"`
}

// viewOf is the idea as the page shows it; about is the epic whose close
// started its session, nil for none.
func viewOf(idea *ideas.Idea, about *tasks.Task) ideaView {
	view := ideaView{ID: idea.ID, Title: idea.Title, Problem: idea.Problem, Proposal: idea.Proposal, WhyNow: idea.WhyNow,
		Size: idea.Size, Risks: idea.Risks, BackedBy: listOf(idea.BackedBy), DoubtedBy: listOf(idea.DoubtedBy), Task: idea.Task,
		Repo: idea.Repo, Epic: idea.Epic, Kind: idea.Kind, State: idea.State, Note: idea.Note, Became: idea.Became,
		Auto: idea.AutoAccepted, CreatedAt: idea.CreatedAt}
	if about != nil {
		view.About, view.AboutTitle = about.ID, about.Title
	}
	return view
}

// acceptance is what accepting an idea did (ideas.Acceptance).
type acceptance struct {
	Task     string `json:"task"`     // the task made from the idea
	Accepted bool   `json:"accepted"` // the idea is marked accepted
	Refused  string `json:"refused"`  // why it isn't, "" once accepted
}
