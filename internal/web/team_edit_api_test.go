package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
)

// teamVersion is the team's version as GET /api/agents gives it with each
// of its agents.
func teamVersion(t *testing.T, server *httptest.Server, team string) string {
	t.Helper()
	_, body := call(t, server, "GET", "/api/agents", "")
	var list []struct {
		Team, TeamVersion string
	}
	json.Unmarshal([]byte(body), &list)
	for _, a := range list {
		if (agents.Agent{Team: a.Team}).TeamName() == team {
			return a.TeamVersion
		}
	}
	t.Fatalf("no agent of %s in %s", team, body)
	return ""
}

// dataEdit is a PUT /api/teams/{team} body for the Data team as dataTeam
// adds it, at version, with the agents in rows.
func dataEdit(version, team, rows string) string {
	return `{"version":"` + version + `","team":"` + team + `","agents":[` + rows + `]}`
}

const (
	leadRow    = `{"was":"data-lead","name":"data-lead","engine":"claude","coordinates":true,"prompt":"Lead."}`
	analystRow = `{"was":"analyst","name":"analyst","engine":"claude","prompt":"Analyse."}`
	charterRow = `{"was":"charter","name":"charter","engine":"claude","prompt":"Chart."}`
)

// addData adds the Data team and returns its version.
func addData(t *testing.T, server *httptest.Server) string {
	t.Helper()
	if code, body := call(t, server, "POST", "/api/teams", dataTeam); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	return teamVersion(t, server, "Data")
}

// coordinatorOf is the task's coordinator as GET /api/tasks/{id} gives it.
func coordinatorOf(t *testing.T, server *httptest.Server, id string) string {
	t.Helper()
	_, body := call(t, server, "GET", "/api/tasks/"+id, "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	return detail.Coordinator
}

// A team, named in any case, is saved whole in one event: renamed, with an
// agent renamed and changed, one removed and one added. The reply gives
// its new version, the one GET /api/agents gives.
func TestATeamIsSavedWholeInOneRequest(t *testing.T) {
	server, s := newTestServer(t)
	version := addData(t, server)
	before, _ := s.Stream(events.AgentsStream)
	if _, rejection, err := s.Propose(agents.Agent{Name: "analyst"}.Learned(httpapi.You, "Know.")); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	code, body := call(t, server, "PUT", "/api/teams/data", dataEdit(version, " Numbers ", leadRow+
		`,{"was":"analyst","name":"statistician","engine":"codex","prompt":"Count.","color":"teal"}`+
		`,{"name":"plotter","engine":"claude","description":"Plot the numbers.","column":"review"}`))
	if code != 200 {
		t.Fatalf("saving the team: %d %s", code, body)
	}
	if want := `{"team":"Numbers","version":"` + teamVersion(t, server, "Numbers") + `"}` + "\n"; body != want {
		t.Errorf("the reply is %s, want %s", body, want)
	}
	after, _ := s.Stream(events.AgentsStream)
	if len(after) != len(before)+2 || after[len(after)-1].Type != events.TeamEdited {
		t.Errorf("%d agent events written, want the learning and one team.edited", len(after)-len(before))
	}
	roster := agents.FromEvents(after)
	statistician, _ := roster.Get("statistician")
	plotter, _ := roster.Get("plotter")
	if _, ok := roster.Get("charter"); ok || statistician.Knowledge != "Know." || statistician.Team != "Numbers" ||
		plotter.Team != "Numbers" || !strings.Contains(plotter.Prompt, "Plot the numbers") {
		t.Errorf("charter is still there, or statistician %+v, plotter %+v", statistician, plotter)
	}
	if teamVersion(t, server, "Numbers") == version {
		t.Error("the team's version didn't change")
	}
}

// Every problem comes at once, each agent's by its row and field, with
// nothing written: 400 for what the edit gets wrong, 404 for no such team,
// 409 for a version that isn't the team's now and for an agent removed that
// a task still needs.
func TestATeamEditsProblemsComeAtOnce(t *testing.T) {
	server, s := newTestServer(t)
	version := addData(t, server)
	at := s.LastPosition()
	unwritten := func(what string) {
		t.Helper()
		if s.LastPosition() != at {
			t.Errorf("%s wrote %d events", what, s.LastPosition()-at)
		}
	}

	code, body := call(t, server, "PUT", "/api/teams/Data", dataEdit(version, "Software",
		`{"was":"data-lead","name":"data-lead","engine":"claude","prompt":"Lead."}`+
			`,{"was":"analyst","name":"Engineer","engine":"gpt","prompt":"Analyse."},{"name":"engineer","engine":"claude"}`))
	var refusal struct {
		Error    string
		Problems []struct {
			Row   *int
			Field string
			Error string
		}
	}
	json.Unmarshal([]byte(body), &refusal)
	var got []string
	for _, p := range refusal.Problems {
		row := "team"
		if p.Row != nil {
			row = string(rune('0' + *p.Row))
		}
		got = append(got, row+":"+p.Field)
	}
	if want := []string{"team:team", "team:coordinates", "1:name", "1:engine", "2:name"}; code != http.StatusBadRequest || !slices.Equal(got, want) ||
		!strings.HasPrefix(refusal.Error, "the Software team can't be saved: there is a team called Software already; ") {
		t.Errorf("an edit with five problems: %d %v (%s), want 400 %v", code, got, body, want)
	}
	unwritten("an edit with five problems")

	if code, body := call(t, server, "PUT", "/api/teams/Numbers", dataEdit(version, "", leadRow)); code != http.StatusNotFound || body != `{"error":"there is no team called Numbers"}`+"\n" {
		t.Errorf("an unknown team: %d %s", code, body)
	}

	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "data-lead"}, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "charter", "instructions": "Chart."}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	at = s.LastPosition()
	code, body = call(t, server, "PUT", "/api/teams/Data", dataEdit(version, "", leadRow+","+analystRow))
	if code != http.StatusConflict || !strings.Contains(body, `"field":"removed","agent":"charter","error":"charter is still needed by task-1`) {
		t.Errorf("removing charter while task-1 needs it: %d %s", code, body)
	}
	unwritten("removing charter while task-1 needs it")

	if code, body := call(t, server, "PUT", "/api/agents/analyst", `{"engine":"claude","prompt":"Analyse more.","team":"Data"}`); code != 200 {
		t.Fatalf("editing analyst on its own: %d %s", code, body)
	}
	at = s.LastPosition()
	code, body = call(t, server, "PUT", "/api/teams/Data", dataEdit(version, "", leadRow+","+analystRow+","+charterRow))
	var stale map[string]string
	json.Unmarshal([]byte(body), &stale)
	if code != http.StatusConflict || stale["error"] != "changed elsewhere" || stale["version"] != teamVersion(t, server, "Data") ||
		!strings.Contains(stale["message"], "the Data team changed since you opened it") {
		t.Errorf("an edit of an older version: %d %s", code, body)
	}
	unwritten("an edit of an older version")
}

// A profile the policy lacks gets the same offer as for a new team, and
// with addMissingProfiles the profile is added and the team saved.
func TestATeamEditNeedsItsProfilesInThePolicy(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	version := addData(t, server.Server)
	edit := func(profile, more string) string {
		return `{"version":"` + version + `","agents":[` + leadRow + `,{"was":"analyst","name":"analyst","engine":"claude","prompt":"Analyse.","profile":"` +
			profile + `"},` + charterRow + `]` + more + `}`
	}
	if code, body := call(t, server.Server, "PUT", "/api/teams/Data", edit("dreaming", "")); code != http.StatusBadRequest ||
		!strings.Contains(body, `{"row":1,"field":"profile","error":"analyst's profile dreaming isn't in policy.toml`) {
		t.Errorf("a profile nobody ships: %d %s", code, body)
	}
	code, body := call(t, server.Server, "PUT", "/api/teams/Data", edit("read-mostly", ""))
	if code != http.StatusConflict || !strings.Contains(body, `"missingProfiles":["read-mostly"]`) {
		t.Errorf("a shipped profile the policy lacks: %d %s", code, body)
	}
	if code, body := call(t, server.Server, "PUT", "/api/teams/Data", edit("read-mostly", `,"addMissingProfiles":true`)); code != 200 {
		t.Fatalf("saving with the profile added: %d %s", code, body)
	}
	stream, _ := server.log.Stream(events.AgentsStream)
	if analyst, _ := agents.FromEvents(stream).Get("analyst"); analyst.Profile != "read-mostly" || len(server.policyEvents(t)) != 1 {
		t.Errorf("analyst = %+v, with %d policy changes, want read-mostly with one", analyst, len(server.policyEvents(t)))
	}
}

// The team's unfinished tasks follow an edit: those of an agent renamed go
// on with its new name, and those of a coordinator that no longer
// coordinates move to the one that does. A finished task keeps its names.
func TestATeamsTasksFollowAnEdit(t *testing.T) {
	server, s := newTestServer(t)
	version := addData(t, server)
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the churn","team":"Data"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Write the notes"}`)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-2", Actor: httpapi.You, Type: events.TaskDone,
		Data: map[string]any{"summary": "Charted.", "moved": true}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	code, body := call(t, server, "PUT", "/api/teams/Data", dataEdit(version, "",
		`{"was":"data-lead","name":"lead","engine":"claude","coordinates":true,"prompt":"Lead."}`+","+analystRow+","+charterRow))
	if code != 200 {
		t.Fatalf("renaming data-lead: %d %s", code, body)
	}
	if open, done, other := coordinatorOf(t, server, "task-1"), coordinatorOf(t, server, "task-2"), coordinatorOf(t, server, "task-3"); open != "lead" || done != "data-lead" || other != "tech-lead" {
		t.Errorf("task-1 is %s's, task-2 %s's, task-3 %s's, want lead, data-lead and tech-lead", open, done, other)
	}

	code, body = call(t, server, "PUT", "/api/teams/Data", dataEdit(teamVersion(t, server, "Data"), "",
		`{"was":"lead","name":"lead","engine":"claude","prompt":"Lead."}`+
			`,{"was":"analyst","name":"analyst","engine":"claude","coordinates":true,"prompt":"Analyse."}`+","+charterRow))
	if code != 200 {
		t.Fatalf("making analyst the coordinator: %d %s", code, body)
	}
	if open, done, other := coordinatorOf(t, server, "task-1"), coordinatorOf(t, server, "task-2"), coordinatorOf(t, server, "task-3"); open != "analyst" || done != "data-lead" || other != "tech-lead" {
		t.Errorf("task-1 is %s's, task-2 %s's, task-3 %s's, want analyst, data-lead and tech-lead", open, done, other)
	}
}

// An edit whose follow-ups were cut short leaves tasks naming an agent by
// the name it had, or with an agent that no longer coordinates: saving the
// team again, as it is, finishes them, once no agent they name is at work.
func TestSavingATeamAgainFinishesItsTasksFollowUps(t *testing.T) {
	server, s := newTestServer(t)
	addData(t, server)
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the churn","team":"Data"}`)
	roster := agents.FromEvents(mustStream(t, s))
	cutShort := agents.EditTeam(roster, "Data", "Data", []agents.EditedAgent{
		{Was: "data-lead", Name: "lead", Engine: "claude", Prompt: "Lead."},
		{Was: "analyst", Name: "analyst", Engine: "claude", Prompt: "Analyse.", Coordinates: true},
		{Was: "charter", Name: "charter", Engine: "claude", Prompt: "Chart."},
	})
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	for _, p := range []events.Proposal{
		cutShort.Proposal(httpapi.You),
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "data-lead", "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	again := dataEdit(teamVersion(t, server, "Data"), "", `{"name":"lead","engine":"claude","prompt":"Lead."}`+
		`,{"name":"analyst","engine":"claude","coordinates":true,"prompt":"Analyse."}`+","+charterRow)

	if code, body := call(t, server, "PUT", "/api/teams/Data", again); code != http.StatusConflict || !strings.Contains(body, "while task-1 has data-lead at work") {
		t.Errorf("saving again during data-lead's turn: %d %s, want 409", code, body)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1",
		Data: map[string]any{"agent": "data-lead", "report": "r"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	at := s.LastPosition()
	if code, body := call(t, server, "PUT", "/api/teams/Data", again); code != 200 {
		t.Fatalf("saving again: %d %s", code, body)
	}
	if one, two := coordinatorOf(t, server, "task-1"), coordinatorOf(t, server, "task-2"); one != "analyst" || two != "analyst" {
		t.Errorf("task-1 is %s's and task-2 %s's, want analyst's", one, two)
	}
	if teamEditsAfter(mustStream(t, s), at) != 0 {
		t.Error("saving the team as it is wrote another team.edited")
	}
}

// A follow-up refused partway says the team is saved and how many tasks
// still need moving; saving again moves them.
func TestAFollowUpRefusedPartwaySaysToSaveAgain(t *testing.T) {
	refuse := true
	server, _ := newTestServerChecking(t, func(p events.Proposal) *events.Rejection {
		if p.Type == events.CoordinatorMoved && p.Stream == "task-2" && refuse {
			return &events.Rejection{Reason: "not now"}
		}
		return nil
	})
	version := addData(t, server)
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the churn","team":"Data"}`)
	code, body := call(t, server, "PUT", "/api/teams/Data", dataEdit(version, "",
		`{"name":"data-lead","engine":"claude","prompt":"Lead."},{"name":"analyst","engine":"claude","coordinates":true,"prompt":"Analyse."}`+","+charterRow))
	if want := `{"error":"saved the Data team, but 1 task still needs moving: save again to finish (task-2: not now)"}` + "\n"; code != http.StatusConflict || body != want {
		t.Errorf("a follow-up refused: %d %s, want 409 %s", code, body, want)
	}
	refuse = false
	if code, body := call(t, server, "PUT", "/api/teams/Data", dataEdit(teamVersion(t, server, "Data"), "",
		`{"name":"data-lead","engine":"claude","prompt":"Lead."},{"name":"analyst","engine":"claude","coordinates":true,"prompt":"Analyse."}`+","+charterRow)); code != 200 {
		t.Fatalf("saving again: %d %s", code, body)
	}
	if one, two := coordinatorOf(t, server, "task-1"), coordinatorOf(t, server, "task-2"); one != "analyst" || two != "analyst" {
		t.Errorf("task-1 is %s's and task-2 %s's, want analyst's", one, two)
	}
}

// newTestServerChecking is newTestServer whose log also runs check after
// the rules, so a test can refuse one proposal.
func newTestServerChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store) {
	t.Helper()
	catalogue := events.Core()
	state := storetest.NewState(catalogue)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), catalogue, state, func(p events.Proposal) *events.Rejection {
		if rejection := rules.Check(p, state, catalogue, storetest.Installed()...); rejection != nil {
			return rejection
		}
		return check(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s
}

func mustStream(t *testing.T, s *store.Store) []events.Event {
	t.Helper()
	stream, err := s.Stream(events.AgentsStream)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

// teamEditsAfter counts the team.edited in stream after position at.
func teamEditsAfter(stream []events.Event, at int64) int {
	n := 0
	for _, e := range stream {
		if e.Type == events.TeamEdited && e.Position > at {
			n++
		}
	}
	return n
}
