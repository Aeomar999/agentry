package schedules

import (
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// Name is Schedules' name as an extension.
const Name = "schedules"

// Extension is Schedules as an extension: so far only its streams, whose types
// extensions.Registration adds to the catalogue.
func Extension() extensions.Extension {
	return extensions.Extension{Name: Name, Version: 1, Streams: Streams()}
}

// Streams are the schedules' streams, for the catalogue to check and the
// state to build and keep: each holds the seven schedule types, and its
// state is its *Schedule, nil until it is created.
func Streams() *extensions.Streams {
	return &extensions.Streams{Prefix: Prefix, Types: types, FromEvents: fromEvents, Next: next}
}

// fromEvents is FromEvents with no schedule as a plain nil, which the state
// keeps nothing for, rather than a nil *Schedule.
func fromEvents(accepted []events.Event) any {
	if schedule := FromEvents(accepted); schedule != nil {
		return schedule
	}
	return nil
}

// next is Next on what the state held, the same way.
func next(prev any, e events.Event) any {
	held, _ := prev.(*Schedule)
	if schedule := Next(held, e); schedule != nil {
		return schedule
	}
	return nil
}
