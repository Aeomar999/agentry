package mcp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// show_task reads an archived task back from a real server, as it was
// before it was archived, and says it is archived and when.
func TestShowTaskReadsAnArchivedTask(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskCreated,
		Data: map[string]any{"title": "Fix the login", "description": "The form forgets the email.", "repo": "/repo", "coordinator": "tech-lead"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	storetest.ArchiveIn(t, s, state, "task-1", blobs)
	agentry := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: "secret", Blobs: blobs}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(agentry.Close)
	c := startOn(t, client.Client{Where: client.Where{URL: agentry.URL, Page: "https://agentry.example"}, Token: "secret"}, false, nil)

	text := c.tool("show_task", map[string]any{"task": "task-1"})
	archived := "\nArchived on " + time.Now().UTC().Format(time.DateOnly) + ": it can be read, not changed.\n"
	if !strings.HasPrefix(text, "task-1: Fix the login\nStatus: done") || !strings.Contains(text, "\nThe form forgets the email.\n") || !strings.Contains(text, archived) {
		t.Errorf("show_task of the archived task:\n%s", text)
	}
}

// show_task of an erased task says it was erased and when, and list_tasks
// leaves it out without failing.
func TestShowTaskSaysATaskWasErased(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	for _, title := range []string{"Fix the login", "Keep me"} {
		if _, rejection, err := s.Create(events.TaskPrefix, events.Proposal{Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": title, "repo": "/repo", "coordinator": "tech-lead"}}); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	if _, rejection, err := s.Erase("task-1", you, "", blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	agentry := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: "secret", Blobs: blobs}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(agentry.Close)
	c := startOn(t, client.Client{Where: client.Where{URL: agentry.URL, Page: "https://agentry.example"}, Token: "secret"}, false, nil)

	want := "error: task-1 was erased on " + time.Now().UTC().Format("2 January 2006")
	if text := c.tool("show_task", map[string]any{"task": "task-1"}); text != want {
		t.Errorf("show_task of the erased task: %q, want %q", text, want)
	}
	if text := c.tool("list_tasks", map[string]any{"all": true}); strings.HasPrefix(text, "error") || strings.Contains(text, "task-1") || !strings.Contains(text, "task-2") {
		t.Errorf("list_tasks with task-1 erased:\n%s", text)
	}
}
