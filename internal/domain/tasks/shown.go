package tasks

import (
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// An epic shows as epic-N wherever Agentry tells you of it, so you can tell
// it from a story or a task on its own. Only what you read changes: its
// stream, its branch, its page and the API stay task-N, since a task
// becomes an epic after it is made.

// EpicPrefix starts an epic's id as you read it.
const EpicPrefix = "epic-"

// ShownID is the task's id as you read it: epic-N for an epic, its own id otherwise.
func ShownID(id string, kind Kind) string {
	if kind == Epic {
		return ShownEpic(id)
	}
	return id
}

// ShownEpic is an epic's id as you read it, such as a story's epic: "" stays "".
func ShownEpic(id string) string {
	if n, ok := strings.CutPrefix(id, events.TaskPrefix); ok {
		return EpicPrefix + n
	}
	return id
}

// StoredID is the id you gave, epic-N or task-N, as the API takes it: task-N.
func StoredID(id string) string {
	if n, ok := strings.CutPrefix(id, EpicPrefix); ok {
		return events.TaskPrefix + n
	}
	return id
}
