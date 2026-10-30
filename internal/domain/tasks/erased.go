package tasks

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// An Erasure is all an erased task keeps: its id, which no task takes
// again, and when it was erased, so the list can say "task-12 was erased
// on 3 November" (docs/design/log-growth.md section 5).
type Erasure struct {
	ID string
	At time.Time
}

// Erased is the erasure of a task whose stream has a task.erased: the
// stream's last accepted event, as the rules refuse any after it. It is
// false for a task that isn't erased.
func Erased(stream []events.Event) (Erasure, bool) {
	for _, e := range stream {
		if e.Type == events.TaskErased {
			return Erasure{ID: e.Stream, At: e.At}, true
		}
	}
	return Erasure{}, false
}
