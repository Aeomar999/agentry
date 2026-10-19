package ideas

import (
	"reflect"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Next must leave the idea it is given as it was, and folding it over a
// log must give what FromEvents gives.
func TestNextLeavesTheIdeaAsItWas(t *testing.T) {
	log := everyEvent()
	var idea *Idea
	for i, e := range log {
		next := Next(idea, e)
		if before := FromEvents(log[:i]); !reflect.DeepEqual(idea, before) {
			t.Errorf("event %d, %s, changed the idea it was applied to", i+1, e.Type)
		}
		if want := FromEvents(log[:i+1]); !reflect.DeepEqual(next, want) {
			t.Errorf("after event %d, %s, Next differs from FromEvents:\n%+v\n%+v", i+1, e.Type, next, want)
		}
		idea = next
	}
	if idea == nil || idea.State != Accepted || idea.Became != "task-2" || idea.AutoAccepted || len(idea.BackedBy) != 2 {
		t.Errorf("the log didn't run through: %+v", idea)
	}
}

// An accepted or dismissed idea carries the time of the event that decided
// it; an open one has none.
func TestADecidedIdeaKeepsWhenItWasDecided(t *testing.T) {
	log := everyEvent()
	for _, step := range []struct {
		after int
		want  time.Time
	}{{1, time.Time{}}, {2, log[2].At}, {4, log[4].At}} {
		if idea := FromEvents(log[:step.after+1]); !idea.DecidedAt.Equal(step.want) {
			t.Errorf("after %s: decided at %v, want %v", log[step.after].Type, idea.DecidedAt, step.want)
		}
	}
}

func TestNextBeforeTheIdeaIsSuggested(t *testing.T) {
	if idea := Next(nil, everyEvent()[0]); idea != nil {
		t.Errorf("an idea out of an event before idea.suggested: %+v", idea)
	}
}

// everyEvent is a log that has every event an idea applies.
func everyEvent() []events.Event {
	you := events.Actor{Kind: events.Human, ID: "you"}
	agentry := events.Actor{Kind: events.System, ID: "agentry"}
	add := func(log []events.Event, actor events.Actor, kind events.Type, data map[string]any) []events.Event {
		at := time.Date(2026, 10, 19, 9, 0, len(log), 0, time.UTC)
		return append(log, events.Event{Position: int64(len(log) + 1), At: at,
			Proposal: events.Proposal{Stream: "idea-1", Actor: actor, Type: kind, Data: data}})
	}

	var log []events.Event
	log = add(log, you, IdeaDismissed, map[string]any{"note": "before it was suggested"})
	log = add(log, agentry, IdeaSuggested, map[string]any{"title": "Search the board", "problem": "Scrolling",
		"proposal": "A search box", "whyNow": "200 tasks", "size": "M", "risks": "Slow", "backedBy": []any{"dreamer", "builder"},
		"doubtedBy": []any{"skeptic"}, "repo": "/r", "task": "task-1", "epic": "task-9", "kind": "story"})
	log = add(log, you, IdeaDismissed, map[string]any{"note": "not now"})
	log = add(log, agentry, IdeaAccepted, map[string]any{"task": "task-3", "note": "auto-accepted: all Visionaries agreed"})
	log = add(log, you, IdeaAccepted, map[string]any{"task": "task-2", "note": "now"})
	return log
}
