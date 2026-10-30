package schedules_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/schedules"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

var (
	you = events.Actor{Kind: events.Human, ID: "you"}
	job = events.Actor{Kind: events.System, ID: "schedules"}
)

// Three schedules, between them every event a schedule takes, written
// through the store as their writers may, beside the tasks they made: the
// state that followed the log event by event, and snapshots of it, equal a
// replay (storetest checks both when the test ends), and so does the state
// a restart loads.
func TestALogWithEveryScheduleEventEqualsAReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, state := storetest.OpenWith(t, path, storetest.WithExtensions())
	for _, p := range agents.SeedProposals(nil) {
		mustPropose(t, s, p)
	}
	nightly := create(t, s, map[string]any{"title": "Nightly tidy", "repo": "/r", "team": "tech",
		"every": "day", "at": "02:00", "zone": "Europe/London", "budget": 5})
	weekly := create(t, s, map[string]any{"title": "Weekly report", "description": "What moved", "repo": "/r", "team": "tech",
		"every": "week", "days": []string{"mon", "thu"}, "at": "09:00", "zone": "Asia/Kolkata", "start": true, "maxTurns": 40})
	gone := create(t, s, map[string]any{"title": "Old job", "repo": "/r", "team": "tech",
		"every": "weekdays", "at": "23:30", "zone": "America/Nuuk"})

	task(t, s, "task-1", "Nightly tidy")
	mustPropose(t, s, events.Proposal{Stream: nightly, Actor: job, Type: schedules.ScheduleFired,
		Data: map[string]any{"task": "task-1", "slot": "2026-10-19T02:00:00+01:00"}})
	mustPropose(t, s, events.Proposal{Stream: nightly, Actor: job, Type: schedules.ScheduleSkipped,
		Data: map[string]any{"slot": "2026-10-20T02:00:00+01:00", "reason": schedules.Open, "task": "task-1"}})
	task(t, s, "task-2", "Nightly tidy")
	mustPropose(t, s, events.Proposal{Stream: nightly, Actor: job, Type: schedules.ScheduleSkipped,
		Data: map[string]any{"slot": "", "reason": schedules.Unrecorded, "task": "task-2", "why": "made before a crash"}})
	mustPropose(t, s, events.Proposal{Stream: nightly, Actor: job, Type: schedules.ScheduleSkipped,
		Data: map[string]any{"slot": "2026-10-22T02:00:00+01:00", "reason": schedules.Refused, "task": "task-3", "why": "no such team"}})

	mustPropose(t, s, events.Proposal{Stream: weekly, Actor: you, Type: schedules.SchedulePaused, Data: map[string]any{"note": "away"}})
	mustPropose(t, s, events.Proposal{Stream: weekly, Actor: you, Type: schedules.ScheduleChanged,
		Data: map[string]any{"title": "Weekly report", "repo": "/r", "team": "ops", "every": "week", "days": []string{"fri"}, "at": "10:00", "zone": "Asia/Kolkata"}})
	mustPropose(t, s, events.Proposal{Stream: weekly, Actor: you, Type: schedules.ScheduleResumed})
	task(t, s, "task-3", "Weekly report")
	mustPropose(t, s, events.Proposal{Stream: weekly, Actor: you, Type: schedules.ScheduleFired,
		Data: map[string]any{"task": "task-3", "slot": "", "manual": true}})

	mustPropose(t, s, events.Proposal{Stream: gone, Actor: you, Type: schedules.ScheduleRemoved, Data: map[string]any{"note": "done with it"}})

	before := schedules.All(state)
	if len(before) != 3 || before[0].Runs != 1 || len(before[0].Named) != 2 || before[1].State != schedules.On ||
		!before[1].LastRun.Manual || before[2].State != schedules.Removed {
		t.Fatalf("the log didn't fold as written: %+v", before)
	}
	_, restarted := storetest.OpenWith(t, path, storetest.WithExtensions())
	if after := schedules.All(restarted); !reflect.DeepEqual(after, before) {
		t.Errorf("after a restart the schedules are\n%+v\nbefore\n%+v", after, before)
	}
}

// create makes a schedule with data, as the page will, and returns its ID.
func create(t *testing.T, s *store.Store, data map[string]any) string {
	t.Helper()
	created, rejection, err := s.Create(schedules.Prefix, events.Proposal{Actor: you, Type: schedules.ScheduleCreated, Data: data})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return created.Stream
}

// task makes the task id, which a run of a schedule would make.
func task(t *testing.T, s *store.Store, id, title string) {
	t.Helper()
	mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": title, "repo": "/r", "coordinator": "tech-lead"}})
}

func mustPropose(t *testing.T, s *store.Store, p events.Proposal) {
	t.Helper()
	if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
		t.Fatalf("%s on %s: %v %v", p.Type, p.Stream, rejection, err)
	}
}
