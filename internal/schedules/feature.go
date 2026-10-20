package schedules

import (
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
)

// Name is Schedules' name as a feature.
const Name = "schedules"

// Feature is Schedules as a feature: so far only its streams, whose types
// features.Registration adds to the catalogue.
func Feature() features.Feature {
	return features.Feature{Name: Name, Version: 1, Streams: Streams()}
}

// Streams are the schedules' streams, for the catalogue to check and the
// state to build and keep: each holds the seven schedule types, and its
// state is its *Schedule, nil until it is created.
func Streams() *features.Streams {
	return &features.Streams{Prefix: Prefix, Types: types, FromEvents: fromEvents, Next: next}
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
