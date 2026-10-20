package schedules

import (
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A schedule's stream, named by its ID (schedule-N): what you made it say,
// and each run it made or skipped. "slot" is an RFC 3339 time with its
// offset, the instant a run was due, "" for one you asked for.
const (
	ScheduleCreated events.Type = "schedule.created" // you made it, on
	ScheduleChanged events.Type = "schedule.changed" // you edited it: the whole schedule as it now is
	SchedulePaused  events.Type = "schedule.paused"  // it makes nothing until resumed
	ScheduleResumed events.Type = "schedule.resumed" // on again, from now
	ScheduleRemoved events.Type = "schedule.removed" // gone from the list
	ScheduleFired   events.Type = "schedule.fired"   // a run made the task in "task"
	ScheduleSkipped events.Type = "schedule.skipped" // a due run made nothing, saying why in "reason"
)

// Why a run was skipped, in a schedule.skipped's "reason".
const (
	Open       = "open"       // a task from this schedule is still open
	Refused    = "refused"    // the task was refused; "why" says so
	Unrecorded = "unrecorded" // a run a crash cut short, found and recorded later
)

// Prefix starts every schedule's stream name.
const Prefix = "schedule-"

// whole are a created or changed schedule's required fields: either carries
// the whole schedule.
var whole = []string{"title", "repo", "team", "every", "at", "zone"}

// types are the schedules' events, with their required fields and writers.
// Only you make and change a schedule; the clock, a System actor, fires and
// skips, and you fire too with Run now.
var types = map[events.Type]events.Spec{
	ScheduleCreated: {Fields: whole, Writers: []events.ActorKind{events.Human}},
	ScheduleChanged: {Fields: whole, Writers: []events.ActorKind{events.Human}},
	SchedulePaused:  {Writers: []events.ActorKind{events.Human}},
	ScheduleResumed: {Writers: []events.ActorKind{events.Human}},
	ScheduleRemoved: {Writers: []events.ActorKind{events.Human}},
	ScheduleFired:   {Fields: []string{"task", "slot"}, Writers: []events.ActorKind{events.System, events.Human}},
	ScheduleSkipped: {Fields: []string{"slot", "reason"}, Writers: []events.ActorKind{events.System}},
}
