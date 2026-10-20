package schedules

import (
	"maps"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// State is whether a schedule makes tasks.
type State string

const (
	On      State = "on"
	Paused  State = "paused"
	Removed State = "removed"
)

// A Schedule is one schedule as its stream leaves it.
type Schedule struct {
	ID                 string
	Rule               Rule
	Title, Description string
	Repo, Team         string
	Start              bool // its tasks start at once, rather than wait in the Backlog
	Budget, MaxTurns   *int // nil: the server's defaults
	State              State
	Since              time.Time // the last created, changed or resumed: slots at or before it are not due
	CreatedAt          time.Time
	LastSlot           time.Time       // the last slot fired or skipped; zero for none
	LastTask           string          // the task its last run made, "" for none
	LastRun            Run             // what the last firing or skip said, for the page
	Runs               int             // runs made, Run now included
	Named              map[string]bool // every task a schedule.fired or an unrecorded skip named
}

// A Run is what one schedule.fired or schedule.skipped said.
type Run struct {
	Slot   time.Time // zero for a Run now or an unrecorded run
	Task   string    // the task it made, or for a skip the open or unrecorded one
	Reason string    // "" for a run that made its task; a skip's Open, Refused or Unrecorded
	Why    string    // a skip's explanation
	Manual bool      // you asked for it with Run now
	Missed int       // earlier slots not run
}

// FromEvents builds a schedule by applying its accepted events in order. It
// returns nil if the events don't include schedule.created.
func FromEvents(stream []events.Event) *Schedule {
	var schedule *Schedule
	for _, e := range stream {
		if e.Type == ScheduleCreated {
			schedule = &Schedule{ID: e.Stream}
		}
		if schedule != nil {
			schedule.apply(e)
		}
	}
	return schedule
}

// Next is the schedule after e, built on a copy: schedule itself is left as
// it was, so whoever still holds it keeps reading what it said. FromEvents
// applies in place instead, and stays the reference Next must agree with.
// apply replaces fields and copies Named before adding to it, so a shallow
// copy is enough.
func Next(schedule *Schedule, e events.Event) *Schedule {
	switch {
	case e.Type == ScheduleCreated:
		schedule = &Schedule{ID: e.Stream}
	case schedule == nil:
		return nil
	default:
		copied := *schedule
		schedule = &copied
	}
	schedule.apply(e)
	return schedule
}

// apply folds e in. Times come from e.At, never the clock, so a replay folds
// to the same schedule however late it runs. A type or field it doesn't
// know is ignored, and a missing field reads as its zero value.
func (s *Schedule) apply(e events.Event) {
	switch e.Type {
	case ScheduleCreated:
		s.read(e)
		s.State, s.Since, s.CreatedAt = On, e.At, e.At
	case ScheduleChanged:
		s.read(e)
		s.Since = e.At
	case SchedulePaused:
		s.State = Paused
	case ScheduleResumed:
		s.State, s.Since = On, e.At
	case ScheduleRemoved:
		s.State = Removed
	case ScheduleFired:
		run := runOf(e)
		s.moveTo(e.Text("slot"))
		s.LastTask, s.LastRun = run.Task, run
		s.Runs++
		s.name(run.Task)
	case ScheduleSkipped:
		run := runOf(e)
		s.LastRun = run
		if run.Reason == Unrecorded {
			s.name(run.Task)
		} else {
			s.moveTo(e.Text("slot"))
		}
	}
}

// read takes what a created or changed event says the schedule is: the
// whole of it, so a field left out goes back to its default.
func (s *Schedule) read(e events.Event) {
	s.Rule = Rule{Every: e.Text("every"), Days: e.List("days"), At: e.Text("at"), Zone: e.Text("zone")}
	s.Title, s.Description = e.Text("title"), e.Text("description")
	s.Repo, s.Team, s.Start = e.Text("repo"), e.Text("team"), e.Flag("start")
	s.Budget, s.MaxTurns = optional(e, "budget"), optional(e, "maxTurns")
}

// moveTo makes slot the last slot, unless it is "" or can't be read.
func (s *Schedule) moveTo(slot string) {
	if at, ok := slotOf(slot); ok {
		s.LastSlot = at
	}
}

// name adds task to a copy of Named, so the schedule Next was given keeps
// its own map.
func (s *Schedule) name(task string) {
	if task == "" {
		return
	}
	named := maps.Clone(s.Named)
	if named == nil {
		named = map[string]bool{}
	}
	named[task] = true
	s.Named = named
}

// runOf is what a fired or skipped event says of its run.
func runOf(e events.Event) Run {
	slot, _ := slotOf(e.Text("slot"))
	return Run{Slot: slot, Task: e.Text("task"), Reason: e.Text("reason"), Why: e.Text("why"),
		Manual: e.Flag("manual"), Missed: e.Number("missed")}
}

// slotOf reads a slot, false for "" or one that isn't RFC 3339.
func slotOf(slot string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339, slot)
	return at, err == nil
}

// optional is a number field, nil when e doesn't carry it.
func optional(e events.Event, field string) *int {
	if !e.Has(field) {
		return nil
	}
	n := e.Number(field)
	return &n
}
