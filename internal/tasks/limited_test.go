package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A usage limit marks the open turn with when it resets and keeps it open;
// it isn't an issue, so the count of issues stays as it was.
func TestAUsageLimitKeepsTheTurnOpen(t *testing.T) {
	log := limitedLog(map[string]any{"agent": "engineer", "error": "rate_limit",
		"details": "You've hit your session limit · resets 3am (UTC)", "resetsAt": "2026-10-20T03:00:00Z"})
	task := FromEvents(log)
	turn := task.OpenTurn()
	if turn == nil || turn.ID != "turn-3" || turn.Limited != "You've hit your session limit · resets 3am (UTC)" ||
		!turn.ResetsAt.Equal(time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("the turn should stay open, limited until 3am: %+v", turn)
	}
	if task.IssuesInARow != 1 {
		t.Errorf("issues in a row %d, want the 1 from before", task.IssuesInARow)
	}
}

// The rules refuse an odd turn.limited now, but a log can hold what they
// let through before: apply takes each without a panic, and changes no
// more than it can.
func TestOddUsageLimitsInAnOldLogReplay(t *testing.T) {
	cases := map[string]struct {
		correlation string
		data        map[string]any
		limited     bool
	}{
		"no reset time":  {"turn-3", map[string]any{"agent": "engineer", "error": "rate_limit", "details": "You've hit your limit", "resetsAt": ""}, true},
		"no data":        {"turn-3", nil, true},
		"an unread time": {"turn-3", map[string]any{"resetsAt": "3am"}, true},
		"another agent":  {"turn-3", map[string]any{"agent": "tech-lead", "resetsAt": "2026-10-20T03:00:00Z"}, true},
		"no such turn":   {"turn-9", map[string]any{"agent": "engineer"}, false},
		"no correlation": {"", map[string]any{"agent": "engineer"}, false},
		"an ended turn":  {"turn-2", map[string]any{"agent": "engineer"}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			log := limitedLog(c.data)
			log[len(log)-1].Correlation = c.correlation
			task := FromEvents(log)
			if turn := task.OpenTurn(); turn == nil || turn.ID != "turn-3" || (turn.Limited != "") != c.limited {
				t.Errorf("open turn %+v, want it limited: %v", turn, c.limited)
			}
			if task.Turns[0].Limited != "" || task.Turns[1].Limited != "" || task.IssuesInARow != 1 {
				t.Errorf("ended turns %+v %+v, issues in a row %d", task.Turns[0], task.Turns[1], task.IssuesInARow)
			}
		})
	}

	// Before any turn at all.
	task := FromEvents(limitedLog(nil)[:1:1])
	task.Apply(events.Event{Proposal: events.Proposal{Stream: "task-1", Type: events.TurnLimited, Correlation: "turn-1"}})
	if len(task.Turns) != 0 {
		t.Errorf("turns: %+v", task.Turns)
	}
}

// limitedLog is a task whose engineer's turn, after an issue, hits a usage
// limit with data.
func limitedLog(data map[string]any) []events.Event {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	agentry := events.Actor{Kind: events.System, ID: "watchdog"}
	at := time.Now()
	event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
		return events.Event{At: at, Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
	}
	return []events.Event{
		event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		event(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"}),
		event(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "tech-lead", "report": "assign"}),
		event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
		event(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h"}),
		event(agentry, events.IssueRaised, "", map[string]any{"reason": "the session died"}),
		event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"}),
		event(worker, events.TurnStarted, "turn-3", map[string]any{"agent": "engineer", "hash": "h"}),
		event(worker, events.TurnLimited, "turn-3", data),
	}
}

// The trail notes the limit for the coordinator, and it is no handoff.
func TestAUsageLimitIsNotedInTheTrail(t *testing.T) {
	resets := time.Now().Add(time.Minute).Truncate(time.Second)
	task := FromEvents(limitedLog(map[string]any{"agent": "engineer", "resetsAt": resets.UTC().Format(time.RFC3339)}))
	last := task.Trail[len(task.Trail)-1]
	want := "engineer's turn-3 hit a " + LimitText(resets, time.Now())
	if last.From != Agentry || last.To != "tech-lead" || last.Text != want {
		t.Errorf("last step %+v, want %q", last, want)
	}
	if task.Stage() != "engineer" || task.Handoff != nil {
		t.Errorf("stage %q, handoff %+v: a note is no handoff", task.Stage(), task.Handoff)
	}
}

// A limit on a turn already limited updates when it resets, with no second
// note in the trail.
func TestALimitAgainIsNotedOnce(t *testing.T) {
	log := limitedLog(map[string]any{"agent": "engineer", "resetsAt": "2026-10-20T03:00:00Z"})
	again := log[len(log)-1]
	again.Data = map[string]any{"agent": "engineer", "resetsAt": "2026-10-20T08:00:00Z"}
	before := FromEvents(log)
	task := FromEvents(append(log, again))
	if turn := task.OpenTurn(); !turn.ResetsAt.Equal(time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("resets at %v, want the later time", turn.ResetsAt)
	}
	if len(task.Trail) != len(before.Trail) {
		t.Errorf("trail %d steps, want %d: %+v", len(task.Trail), len(before.Trail), task.Trail[len(task.Trail)-1])
	}
}

func TestLimitText(t *testing.T) {
	hit := time.Date(2026, 10, 19, 22, 30, 0, 0, time.Local)
	cases := map[string]struct {
		resetsAt time.Time
		want     string
	}{
		"later today": {time.Date(2026, 10, 19, 23, 45, 0, 0, time.Local), "usage limit, resets ~23:45"},
		"tomorrow":    {time.Date(2026, 10, 20, 3, 0, 0, 0, time.Local), "usage limit, resets ~Oct 20 03:00"},
		"in UTC":      {time.Date(2026, 10, 19, 23, 45, 0, 0, time.Local).UTC(), "usage limit, resets ~23:45"},
		"not said":    {time.Time{}, "usage limit, reset time unknown"},
	}
	for name, c := range cases {
		if got := LimitText(c.resetsAt, hit); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

// Resuming clears the limit and counts it; the turn stays open, the issues
// in a row stay as they were, and the trail notes it.
func TestAResumedTurnIsNoLongerLimited(t *testing.T) {
	log := limitedLog(map[string]any{"agent": "engineer", "error": "rate_limit", "resetsAt": "2026-10-20T03:00:00Z"})
	resumed := log[len(log)-1]
	resumed.Type, resumed.Data = events.TurnResumed, map[string]any{"agent": "engineer", "reason": "reset"}
	task := FromEvents(append(log, resumed))
	turn := task.OpenTurn()
	if turn == nil || turn.ID != "turn-3" || turn.Limited != "" || !turn.ResetsAt.IsZero() || turn.Resumes != 1 {
		t.Errorf("the turn should stay open, no longer limited, resumed once: %+v", turn)
	}
	if task.IssuesInARow != 1 {
		t.Errorf("issues in a row %d, want the 1 from before", task.IssuesInARow)
	}
	last := task.Trail[len(task.Trail)-1]
	if last.From != Agentry || last.To != "tech-lead" || last.Text != "engineer's turn-3 resumed after its usage limit" {
		t.Errorf("last step %+v", last)
	}
	if task.Stage() != "engineer" || task.Handoff != nil {
		t.Errorf("stage %q, handoff %+v: a note is no handoff", task.Stage(), task.Handoff)
	}
}

// The rules refuse an odd turn.resumed, but apply takes one from an old log
// without a panic, and changes nothing.
func TestOddResumesInAnOldLogReplay(t *testing.T) {
	limit := map[string]any{"agent": "engineer", "error": "rate_limit", "resetsAt": "2026-10-20T03:00:00Z"}
	cases := map[string]struct {
		correlation string
		data        map[string]any
		limited     bool // the turn the odd one comes after
	}{
		"no data":            {"turn-3", nil, true},
		"another agent":      {"turn-3", map[string]any{"agent": "tech-lead", "reason": "reset"}, true},
		"a turn not limited": {"turn-3", map[string]any{"agent": "engineer", "reason": "engine"}, false},
		"no such turn":       {"turn-9", map[string]any{"agent": "engineer", "reason": "engine"}, true},
		"no correlation":     {"", map[string]any{"agent": "engineer", "reason": "engine"}, true},
		"an ended turn":      {"turn-2", map[string]any{"agent": "engineer", "reason": "engine"}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			log := limitedLog(limit)
			if !c.limited {
				log = log[:len(log)-1]
			}
			odd := log[len(log)-1]
			odd.Type, odd.Correlation, odd.Data = events.TurnResumed, c.correlation, c.data
			before := FromEvents(log)
			task := FromEvents(append(log, odd))
			if turn := task.OpenTurn(); turn == nil || turn.ID != "turn-3" || turn.Limited != before.OpenTurn().Limited || turn.Resumes != 0 {
				t.Errorf("open turn %+v, want it as it was", turn)
			}
			if len(task.Trail) != len(before.Trail) || task.IssuesInARow != 1 {
				t.Errorf("trail %d steps, want %d; issues in a row %d", len(task.Trail), len(before.Trail), task.IssuesInARow)
			}
		})
	}

	// Before any turn at all.
	task := FromEvents(limitedLog(nil)[:1:1])
	task.Apply(events.Event{Proposal: events.Proposal{Stream: "task-1", Type: events.TurnResumed, Correlation: "turn-1"}})
	if len(task.Turns) != 0 {
		t.Errorf("turns: %+v", task.Turns)
	}
}
