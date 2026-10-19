package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

var agentry = events.Actor{Kind: events.System, ID: "agentry"}

// suggestIdea keeps an idea from the session task about repo, as a
// Visionaries done does.
func suggestIdea(t *testing.T, s *store.Store, task, repo, title string) string {
	t.Helper()
	created, rejection, err := s.Create(events.IdeaPrefix, events.Proposal{Actor: agentry, Type: events.IdeaSuggested, Data: map[string]any{
		"title": title, "problem": "p", "proposal": "f", "size": "M", "repo": repo, "task": task, "kind": ""}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return created.Stream
}

func ideaIDs(t *testing.T, body string) []string {
	t.Helper()
	var list []ideaView
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err, body)
	}
	ids := []string{}
	for _, idea := range list {
		ids = append(ids, idea.ID)
	}
	return ids
}

func TestIdeasListNewestSessionFirst(t *testing.T) {
	server, s := newTestServer(t)
	repo, other := gitRepo(t), gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Older session","repo":"`+repo+`"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Newer session","repo":"`+other+`"}`)
	newer := suggestIdea(t, s, "task-2", other, "From the newer")
	older := suggestIdea(t, s, "task-1", repo, "From the older, kept last")
	again := suggestIdea(t, s, "task-2", other, "From the newer, again")

	code, body := call(t, server, "GET", "/api/ideas", "")
	if want := []string{again, newer, older}; code != 200 || !slices.Equal(ideaIDs(t, body), want) {
		t.Errorf("every idea: %d, want %v: %s", code, want, body)
	}
	if !strings.Contains(body, `"state":"open"`) || !strings.Contains(body, `"backedBy":[]`) {
		t.Errorf("an idea's view: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/ideas?repo="+repo, ""); !slices.Equal(ideaIDs(t, body), []string{older}) {
		t.Errorf("one repo's ideas: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/ideas?repo=/no/such/repo", ""); body != "[]\n" {
		t.Errorf("no ideas is an empty list: %q", body)
	}
}

func TestYouAcceptAnIdeaOnce(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	id := suggestIdea(t, s, "task-1", repo, "Search the board")

	code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{"kind":"epic","note":"Soon"}`)
	var got acceptance
	json.Unmarshal([]byte(body), &got)
	if code != 200 || got != (acceptance{Task: "task-2", Accepted: true}) {
		t.Fatalf("accept: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/tasks/task-2", ""); !strings.Contains(body, `"kind":"epic"`) || !strings.Contains(body, `"parked":"backlog"`) {
		t.Errorf("the task made from it: %s", body)
	}
	if code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{}`); code != http.StatusConflict || !strings.Contains(body, "accepted already") {
		t.Errorf("a second accept: %d %s", code, body)
	}
	if code, _ := call(t, server, "POST", "/api/ideas/"+id+"/dismiss", `{"note":"No"}`); code != http.StatusConflict {
		t.Errorf("dismissing an accepted idea: %d", code)
	}
	if _, body := call(t, server, "GET", "/api/ideas", ""); !strings.Contains(body, `"state":"accepted"`) || !strings.Contains(body, `"became":"task-2"`) ||
		!strings.Contains(body, `"autoAccepted":false`) {
		t.Errorf("the accepted idea: %s", body)
	}
}

func TestAnIdeaAgentryAcceptedSaysSo(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	id := suggestIdea(t, s, "task-1", repo, "Search the board")
	for _, p := range []events.Proposal{
		{Stream: "task-2", Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": "Search the board", "repo": repo,
			"coordinator": "tech-lead", "parked": "backlog", "idea": id}},
		{Stream: id, Actor: agentry, Type: events.IdeaAccepted, Data: map[string]any{"task": "task-2", "note": "auto-accepted: all Visionaries agreed"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	if _, body := call(t, server, "GET", "/api/ideas", ""); !strings.Contains(body, `"state":"accepted"`) || !strings.Contains(body, `"autoAccepted":true`) {
		t.Errorf("the idea Agentry accepted: %s", body)
	}
}

func TestYouDismissAnIdea(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	id := suggestIdea(t, s, "task-1", repo, "Dark mode")

	if code, body := call(t, server, "POST", "/api/ideas/"+id+"/dismiss", `{"note":" The OS does it "}`); code != 200 {
		t.Fatalf("dismiss: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/ideas", ""); !strings.Contains(body, `"state":"dismissed"`) || !strings.Contains(body, `"note":"The OS does it"`) {
		t.Errorf("the dismissed idea: %s", body)
	}
	if code, _ := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{}`); code != http.StatusConflict {
		t.Errorf("accepting a dismissed idea: %d", code)
	}
}

func TestAnUnknownIdeaIsNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	for _, path := range []string{"/api/ideas/idea-9/accept", "/api/ideas/idea-9/dismiss", "/api/ideas/task-1/dismiss"} {
		if code, _ := call(t, server, "POST", path, `{}`); code != http.StatusNotFound {
			t.Errorf("%s: %d", path, code)
		}
	}
}

// An idea from the session Agentry started after an epic closed names the
// epic; others name none.
func TestAnIdeaNamesTheEpicItsSessionWasAbout(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ideas list", "repo": repo,
			"coordinator": "tech-lead", "kind": "epic"}},
		{Stream: "task-2", Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": "Follow-ups after Ideas list",
			"repo": repo, "coordinator": "tech-lead", "about": "task-1", "close": 9}},
		{Stream: "task-3", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Think", "repo": repo, "coordinator": "tech-lead"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	after, plain := suggestIdea(t, s, "task-2", repo, "Search ideas"), suggestIdea(t, s, "task-3", repo, "Dark mode")

	_, body := call(t, server, "GET", "/api/ideas", "")
	var list []ideaView
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err, body)
	}
	for _, idea := range list {
		want := ideaView{}
		if idea.ID == after {
			want = ideaView{About: "task-1", AboutTitle: "Ideas list"}
		}
		if idea.ID != after && idea.ID != plain || idea.About != want.About || idea.AboutTitle != want.AboutTitle {
			t.Errorf("%s: about %q %q, want %q %q", idea.ID, idea.About, idea.AboutTitle, want.About, want.AboutTitle)
		}
	}
}
