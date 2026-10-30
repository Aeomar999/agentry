package tasksapi

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// The views, limits and steps the routes' tests in tasksapi_test read.

type (
	TaskDetail = taskDetail
	EventEntry = eventEntry
	EventsPage = eventsPage
	Erased     = erased
)

const (
	EventPage     = eventPage
	EventPageMax  = eventPageMax
	EventTypesMax = eventTypesMax
)

var EntryOf = entryOf

func (a API) DropErasedStories(w http.ResponseWriter, epic *tasks.Task) bool {
	return a.dropErasedStories(w, epic)
}

func (a API) LeaveEpic(id string) (string, error) { return a.leaveEpic(id) }
