package current

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/schedules"
)

// withIdeasLog is everyStream's log with a second idea, as a build with
// ideas wrote it.
func withIdeasLog() []events.Event {
	var proposals []events.Proposal
	for _, e := range everyStream() {
		proposals = append(proposals, e.Proposal)
	}
	return logOf(append(proposals,
		proposal("idea-2", system, ideas.IdeaSuggested, map[string]any{"title": "Smaller", "problem": "Big", "proposal": "Trim", "size": "S", "repo": "/r", "task": "task-2"}),
		proposal("task-2", you, events.TaskLabeled, map[string]any{"labels": []any{"api"}}),
	)...)
}

// A build without ideas skips their streams and counts them: the tasks
// and rosters are what a build with ideas makes of the same log.
func TestIdeasStreamsAreSkippedAndCountedWithoutIdeas(t *testing.T) {
	log := withIdeasLog()
	want := []Unknown{{Prefix: ideas.Prefix, Streams: 2, Events: 3, Last: 24}}
	for name, s := range map[string]*State{"loaded": loadedWith(events.Core(), log), "applied": appliedWith(events.Core(), log)} {
		with := loaded(log)
		if got := s.Unknown(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: unknown %+v, want %+v", name, got, want)
		}
		if !reflect.DeepEqual(s.Tasks(), with.Tasks()) || !slices.Equal(s.TaskIDs(), with.TaskIDs()) {
			t.Errorf("%s: tasks %v, with ideas %v", name, idsOf(s.Tasks()), idsOf(with.Tasks()))
		}
		if !reflect.DeepEqual(s.Roster(), with.Roster()) || s.Settings() != with.Settings() {
			t.Errorf("%s: the rosters differ from a build with ideas", name)
		}
		if len(ideas.All(s)) != 0 {
			t.Errorf("%s: %d ideas, want none", name, len(ideas.All(s)))
		}
		if task, err := s.Task("idea-1"); task != nil || err != nil {
			t.Errorf("%s: idea-1 reads as a task: %+v, %v", name, task, err)
		}
		if got := s.StreamPosition("idea-2"); got != 24 {
			t.Errorf("%s: idea-2's last position %d, want 24", name, got)
		}
		if len(with.Unknown()) != 0 {
			t.Errorf("with ideas, unknown %+v, want none", with.Unknown())
		}
	}
}

// withSchedules is the catalogue of a build with ideas and schedules.
var withSchedules = func() *events.Catalogue {
	catalogue, err := events.NewCatalogue(extensions.Registrations(ideas.Extension(), schedules.Extension())...)
	if err != nil {
		panic(err)
	}
	return catalogue
}()

// withSchedulesLog is withIdeasLog with two schedules and a task one of
// them made, as a build with schedules wrote it.
func withSchedulesLog() []events.Event {
	var proposals []events.Proposal
	for _, e := range withIdeasLog() {
		proposals = append(proposals, e.Proposal)
	}
	return logOf(append(proposals,
		proposal("schedule-1", you, schedules.ScheduleCreated, map[string]any{"title": "Nightly", "repo": "/r", "team": "tech", "every": "day", "at": "02:00", "zone": "Europe/London"}),
		proposal("task-3", you, events.TaskCreated, map[string]any{"title": "Nightly", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("schedule-1", system, schedules.ScheduleFired, map[string]any{"task": "task-3", "slot": "2026-10-19T02:00:00+01:00"}),
		proposal("schedule-2", you, schedules.ScheduleCreated, map[string]any{"title": "Weekly", "repo": "/r", "team": "tech", "every": "week", "days": []any{"mon"}, "at": "09:00", "zone": "Asia/Kolkata"}),
		proposal("schedule-2", you, schedules.SchedulePaused, nil),
		proposal("task-3", you, events.TaskLabeled, map[string]any{"labels": []any{"chore"}}),
		proposal("schedule-1", system, schedules.ScheduleSkipped, map[string]any{"slot": "2026-10-20T02:00:00+01:00", "reason": schedules.Open, "task": "task-3"}),
	)...)
}

// A build without schedules skips their streams and counts them, beside
// the ideas': the tasks and rosters are what a build with schedules makes
// of the same log.
func TestSchedulesStreamsAreSkippedAndCountedWithoutSchedules(t *testing.T) {
	log := withSchedulesLog()
	want := []Unknown{{Prefix: ideas.Prefix, Streams: 2, Events: 3, Last: 24},
		{Prefix: schedules.Prefix, Streams: 2, Events: 5, Last: 32}}
	with := loadInto(New(withSchedules, ideas.Streams(), schedules.Streams()), log)
	if len(with.Unknown()) != 0 || len(schedules.All(with)) != 2 {
		t.Fatalf("with schedules: unknown %+v and %d schedules, want none and 2", with.Unknown(), len(schedules.All(with)))
	}
	for name, s := range map[string]*State{"loaded": loadedWith(events.Core(), log), "applied": appliedWith(events.Core(), log)} {
		if got := s.Unknown(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: unknown %+v, want %+v", name, got, want)
		}
		if !reflect.DeepEqual(s.Tasks(), with.Tasks()) || !slices.Equal(s.TaskIDs(), with.TaskIDs()) {
			t.Errorf("%s: tasks %v, with schedules %v", name, idsOf(s.Tasks()), idsOf(with.Tasks()))
		}
		if !reflect.DeepEqual(s.Roster(), with.Roster()) || s.Settings() != with.Settings() {
			t.Errorf("%s: the rosters differ from a build with schedules", name)
		}
		if len(schedules.All(s)) != 0 {
			t.Errorf("%s: %d schedules, want none", name, len(schedules.All(s)))
		}
		if task, err := s.Task("schedule-1"); task != nil || err != nil {
			t.Errorf("%s: schedule-1 reads as a task: %+v, %v", name, task, err)
		}
		if got := s.StreamPosition("schedule-1"); got != 32 {
			t.Errorf("%s: schedule-1's last position %d, want 32", name, got)
		}
	}
}

// Equal holds on a build without ideas, whether the state loaded the log
// or applied it event by event: foreign streams are compared by their count.
func TestEqualCountsForeignStreams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	written, err := store.Open(path, withIdeas, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range withIdeasLog() {
		if e.Type != events.ProposalRejected {
			propose(t, written, e.Proposal)
		}
	}
	written.Close()

	loaded := New(events.Core())
	s, err := store.Open(path, events.Core(), loaded, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := Equal(s, loaded); err != nil {
		t.Errorf("loaded: %v", err)
	}
	if len(loaded.Unknown()) != 1 || loaded.Unknown()[0].Events != 3 {
		t.Errorf("loaded: unknown %+v, want idea-'s 3 events", loaded.Unknown())
	}
	applied := New(events.Core())
	applied.Loaded(0)
	fromTheLog(t, s, applied)
	if err := Equal(s, applied); err != nil {
		t.Errorf("applied: %v", err)
	}

	miscounted := applied.skipped.prefixes[ideas.Prefix]
	miscounted.Events--
	applied.skipped.prefixes[ideas.Prefix] = miscounted
	if err := Equal(s, applied); err == nil || !strings.HasPrefix(err.Error(), "unknown:") {
		t.Errorf("Equal on a miscounted foreign stream: %v, want an unknown: difference", err)
	}
}

// NextTask counts every accepted task.created, as Store.Create does: one in
// a foreign stream too, though it makes no task, and never a rejected one.
func TestNextTaskCountsEveryTaskCreatedAsStoreCreateDoes(t *testing.T) {
	var proposals []events.Proposal
	for _, e := range everyStream() {
		proposals = append(proposals, e.Proposal)
	}
	log := logOf(append(proposals, proposal("pr-1", you, events.TaskCreated, map[string]any{"title": "Foreign", "repo": "/r", "coordinator": "tech-lead"}))...)
	for name, s := range map[string]*State{"loaded": loadedWith(events.Core(), log), "applied": appliedWith(events.Core(), log)} {
		if got := s.NextTask(); got != "task-4" {
			t.Errorf("%s: next task %s, want task-4 after task-1, task-2 and pr-1's", name, got)
		}
		if ids := s.TaskIDs(); !slices.Equal(ids, []string{"task-1", "task-2"}) {
			t.Errorf("%s: tasks %v, want pr-1's left out", name, ids)
		}
	}
}

// A type the catalogue doesn't know, inside a task's stream, is counted and
// changes nothing but the task's LastActiveAt.
func TestAnUnknownTypeInATaskChangesOnlyLastActiveAt(t *testing.T) {
	prOpened := events.Type("git.pr_opened")
	log := logOf(
		proposal("task-1", you, events.TaskCreated, map[string]any{"title": "Ship it", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-1", lead, events.TaskAssigned, map[string]any{"agent": "engineer", "instructions": "Go."}),
		proposal("task-1", system, prOpened, map[string]any{"url": "https://example.com/pr/1"}),
		proposal("task-1", you, events.TaskLabeled, map[string]any{"labels": []any{"web"}}),
		proposal("task-1", system, prOpened, map[string]any{"url": "https://example.com/pr/2"}),
	)
	without := slices.DeleteFunc(slices.Clone(log), func(e events.Event) bool { return e.Type == prOpened })
	want := []Unknown{{Type: prOpened, Events: 2, Last: 5}}
	for name, s := range map[string]*State{"loaded": loaded(log), "applied": applied(log)} {
		task, err := s.Task("task-1")
		if err != nil || task == nil {
			t.Fatalf("%s: %+v, %v", name, task, err)
		}
		plain, err := loaded(without).Task("task-1")
		if err != nil {
			t.Fatal(err)
		}
		if !task.LastActiveAt.Equal(log[4].At) {
			t.Errorf("%s: LastActiveAt %v, want the unknown event's %v", name, task.LastActiveAt, log[4].At)
		}
		got, other := *task, *plain
		got.LastActiveAt, other.LastActiveAt = time.Time{}, time.Time{}
		if !reflect.DeepEqual(got, other) {
			t.Errorf("%s: the task differs from one without the unknown type:\n%+v\n%+v", name, got, other)
		}
		if unknown := s.Unknown(); !reflect.DeepEqual(unknown, want) {
			t.Errorf("%s: unknown %+v, want %+v", name, unknown, want)
		}
	}
}
