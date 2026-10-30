package web

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// The task's detail and its events as the page reads them, the parts the
// tests here decode; they mirror internal/domain/tasks/tasksapi/views.go
// and events_api.go.

type taskSummary = tasks.Summary

type taskDetail struct {
	taskSummary
	StoryList []taskSummary `json:"storyList"`
	Worktree  *struct {
		Path string `json:"path"`
	} `json:"worktree"`
	Trail []struct {
		To   string `json:"to"`
		Text string `json:"text"`
	} `json:"trail"`
	Actions []struct {
		Decision string `json:"decision"`
	} `json:"actions"`
}

const eventPage = 500 // events in a page unless ?limit= says otherwise

type eventsPage struct {
	Events []eventEntry `json:"events"`
	More   bool         `json:"more"`
}

type eventEntry struct {
	Position    int64          `json:"position"`
	At          time.Time      `json:"at"`
	Type        events.Type    `json:"type"`
	Actor       string         `json:"actor"`
	Correlation string         `json:"correlation,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}
