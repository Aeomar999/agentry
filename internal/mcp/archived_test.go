package mcp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
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
