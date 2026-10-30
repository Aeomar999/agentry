package schedules

import (
	"maps"
	"reflect"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

var (
	you = events.Actor{Kind: events.Human, ID: "you"}
	job = events.Actor{Kind: events.System, ID: "schedules"}
)

// at is the time of a schedule's nth event.
func at(n int) time.Time { return time.Date(2026, 10, 19, 9, 0, n, 0, time.UTC) }

// event is the nth event on schedule-1.
func event(n int, actor events.Actor, kind events.Type, data map[string]any) events.Event {
	return events.Event{Position: int64(n), At: at(n),
		Proposal: events.Proposal{Stream: "schedule-1", Actor: actor, Type: kind, Data: data}}
}

// slot reads a slot as the fold does, so the two compare equal.
func slot(text string) time.Time {
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return parsed
}

func number(n int) *int { return &n }

// nightly is a created schedule's data, as a log reads it back.
func nightly() map[string]any {
	return map[string]any{"title": "Nightly tidy", "description": "Tidy the board", "repo": "/r", "team": "tech",
		"every": "week", "days": []any{"mon", "thu"}, "at": "02:00", "zone": "Europe/London",
		"start": true, "budget": float64(5), "maxTurns": float64(40)}
}

func TestACreatedScheduleKeepsEveryField(t *testing.T) {
	got := FromEvents([]events.Event{event(1, you, ScheduleCreated, nightly())})
	want := &Schedule{ID: "schedule-1", Rule: Rule{Every: "week", Days: []string{"mon", "thu"}, At: "02:00", Zone: "Europe/London"},
		Title: "Nightly tidy", Description: "Tidy the board", Repo: "/r", Team: "tech", Start: true,
		Budget: number(5), MaxTurns: number(40), State: On, Since: at(1), CreatedAt: at(1)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("created:\n%+v\nwant\n%+v", got, want)
	}
}

// A field a later version adds to schedule.created, such as a cron rule,
// changes nothing here.
func TestAFutureFieldIsIgnored(t *testing.T) {
	data := nightly()
	data["cron"] = "0 2 * * *"
	got := FromEvents([]events.Event{event(1, you, ScheduleCreated, data)})
	if want := FromEvents([]events.Event{event(1, you, ScheduleCreated, nightly())}); !reflect.DeepEqual(got, want) {
		t.Errorf("with cron:\n%+v\nwant\n%+v", got, want)
	}
}

// Each type sets what section 3 of docs/design/schedules.md says, and
// nothing else; the schedule it is applied to, Named included, stays as it
// was.
func TestEachTypeFoldsWhatItSays(t *testing.T) {
	fired := event(2, job, ScheduleFired, map[string]any{"task": "task-1", "slot": "2026-10-19T02:00:00+01:00"})
	base := []events.Event{event(1, you, ScheduleCreated, nightly()), fired}
	cases := []struct {
		name   string
		event  events.Event
		change func(s *Schedule)
	}{
		{"changed", event(3, you, ScheduleChanged, map[string]any{"title": "Weekly tidy", "repo": "/s", "team": "ops",
			"every": "day", "at": "03:30", "zone": "Asia/Kolkata"}), func(s *Schedule) {
			s.Rule = Rule{Every: "day", At: "03:30", Zone: "Asia/Kolkata"}
			s.Title, s.Description, s.Repo, s.Team = "Weekly tidy", "", "/s", "ops"
			s.Start, s.Budget, s.MaxTurns, s.Since = false, nil, nil, at(3)
		}},
		{"paused", event(3, you, SchedulePaused, map[string]any{"note": "away"}), func(s *Schedule) { s.State = Paused }},
		{"resumed", event(3, you, ScheduleResumed, nil), func(s *Schedule) { s.State, s.Since = On, at(3) }},
		{"removed", event(3, you, ScheduleRemoved, nil), func(s *Schedule) { s.State = Removed }},
		{"fired", event(3, job, ScheduleFired, map[string]any{"task": "task-4", "slot": "2026-10-22T02:00:00+01:00", "missed": float64(2)}),
			func(s *Schedule) {
				s.LastSlot, s.LastTask, s.Runs = slot("2026-10-22T02:00:00+01:00"), "task-4", 2
				s.LastRun = Run{Slot: s.LastSlot, Task: "task-4", Missed: 2}
				s.Named = map[string]bool{"task-1": true, "task-4": true}
			}},
		{"fired by Run now", event(3, you, ScheduleFired, map[string]any{"task": "task-4", "slot": "", "manual": true}),
			func(s *Schedule) {
				s.LastTask, s.Runs, s.LastRun = "task-4", 2, Run{Task: "task-4", Manual: true}
				s.Named = map[string]bool{"task-1": true, "task-4": true}
			}},
		{"fired with a slot that can't be read", event(3, job, ScheduleFired, map[string]any{"task": "task-4", "slot": "Thursday"}),
			func(s *Schedule) {
				s.LastTask, s.Runs, s.LastRun = "task-4", 2, Run{Task: "task-4"}
				s.Named = map[string]bool{"task-1": true, "task-4": true}
			}},
		{"fired with no task", event(3, job, ScheduleFired, map[string]any{"slot": "2026-10-22T02:00:00+01:00"}),
			func(s *Schedule) {
				s.LastSlot, s.LastTask, s.Runs = slot("2026-10-22T02:00:00+01:00"), "", 2
				s.LastRun = Run{Slot: s.LastSlot}
			}},
		{"skipped while a task is open", event(3, job, ScheduleSkipped, map[string]any{"slot": "2026-10-22T02:00:00+01:00", "reason": Open, "task": "task-2"}),
			func(s *Schedule) {
				s.LastSlot = slot("2026-10-22T02:00:00+01:00")
				s.LastRun = Run{Slot: s.LastSlot, Task: "task-2", Reason: Open}
			}},
		{"skipped as refused", event(3, job, ScheduleSkipped, map[string]any{"slot": "2026-10-22T02:00:00+01:00", "reason": Refused,
			"task": "task-4", "why": "no such team"}), func(s *Schedule) {
			s.LastSlot = slot("2026-10-22T02:00:00+01:00")
			s.LastRun = Run{Slot: s.LastSlot, Task: "task-4", Reason: Refused, Why: "no such team"}
		}},
		{"skipped as unrecorded", event(3, job, ScheduleSkipped, map[string]any{"slot": "", "reason": Unrecorded,
			"task": "task-3", "why": "made before a crash"}), func(s *Schedule) {
			s.LastRun = Run{Task: "task-3", Reason: Unrecorded, Why: "made before a crash"}
			s.Named = map[string]bool{"task-1": true, "task-3": true}
		}},
		{"skipped as unrecorded, with a slot", event(3, job, ScheduleSkipped, map[string]any{"slot": "2026-10-22T02:00:00+01:00",
			"reason": Unrecorded, "task": "task-3"}), func(s *Schedule) {
			s.LastRun = Run{Slot: slot("2026-10-22T02:00:00+01:00"), Task: "task-3", Reason: Unrecorded}
			s.Named = map[string]bool{"task-1": true, "task-3": true}
		}},
		{"an unknown type", event(3, you, "schedule.renamed", map[string]any{"title": "Other"}), func(*Schedule) {}},
	}
	for _, c := range cases {
		prev := FromEvents(base)
		named, kept := prev.Named, maps.Clone(prev.Named)
		want := FromEvents(base)
		c.change(want)
		if got := Next(prev, c.event); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%+v\nwant\n%+v", c.name, got, want)
		}
		if !reflect.DeepEqual(prev, FromEvents(base)) || !maps.Equal(named, kept) {
			t.Errorf("%s changed the schedule it was applied to: %+v", c.name, prev)
		}
	}
}

// Named is how the job tells a recorded run from one a crash cut short, so
// only a run that made a task adds to it.
func TestNamedGrowsOnlyByAFiringAndAnUnrecordedSkip(t *testing.T) {
	var schedule *Schedule
	for _, e := range everyEvent() {
		next := Next(schedule, e)
		var before map[string]bool
		if schedule != nil {
			before = schedule.Named
		}
		grows := e.Type == ScheduleFired || e.Type == ScheduleSkipped && e.Text("reason") == Unrecorded
		if added := next != nil && len(next.Named) > len(before); added != grows {
			t.Errorf("%s %v: Named %v after %v", e.Type, e.Data, next.Named, before)
		}
		schedule = next
	}
}

// Next must leave the schedule it is given as it was, and folding it over a
// log must give what FromEvents gives.
func TestNextAgreesWithFromEvents(t *testing.T) {
	log := everyEvent()
	var schedule *Schedule
	for i, e := range log {
		next := Next(schedule, e)
		if before := FromEvents(log[:i]); !reflect.DeepEqual(schedule, before) {
			t.Errorf("event %d, %s, changed the schedule it was applied to", i+1, e.Type)
		}
		if want := FromEvents(log[:i+1]); !reflect.DeepEqual(next, want) {
			t.Errorf("after event %d, %s, Next differs from FromEvents:\n%+v\n%+v", i+1, e.Type, next, want)
		}
		schedule = next
	}
	if schedule == nil || schedule.State != Removed || schedule.Runs != 2 || len(schedule.Named) != 3 {
		t.Errorf("the log didn't run through: %+v", schedule)
	}
}

func TestNextBeforeTheScheduleIsCreated(t *testing.T) {
	if schedule := Next(nil, everyEvent()[0]); schedule != nil {
		t.Errorf("a schedule out of an event before schedule.created: %+v", schedule)
	}
}

// everyEvent is a log that has every event a schedule applies, and one it
// doesn't know.
func everyEvent() []events.Event {
	return []events.Event{
		event(1, you, SchedulePaused, nil),
		event(2, you, ScheduleCreated, nightly()),
		event(3, job, ScheduleFired, map[string]any{"task": "task-1", "slot": "2026-10-19T02:00:00+01:00"}),
		event(4, job, ScheduleSkipped, map[string]any{"slot": "2026-10-22T02:00:00+01:00", "reason": Open, "task": "task-4"}),
		event(5, job, ScheduleSkipped, map[string]any{"slot": "", "reason": Unrecorded, "task": "task-2"}),
		event(6, you, SchedulePaused, map[string]any{"note": "away"}),
		event(7, you, ScheduleChanged, map[string]any{"title": "Daily tidy", "repo": "/r", "team": "tech", "every": "day", "at": "01:00", "zone": "Europe/London"}),
		event(8, you, ScheduleResumed, nil),
		event(9, you, "schedule.renamed", map[string]any{"title": "Other"}),
		event(10, you, ScheduleFired, map[string]any{"task": "task-5", "slot": "", "manual": true}),
		event(11, job, ScheduleSkipped, map[string]any{"slot": "2026-10-26T01:00:00Z", "reason": Refused, "task": "task-6", "why": "no such team"}),
		event(12, you, ScheduleRemoved, map[string]any{"note": "done with it"}),
	}
}
