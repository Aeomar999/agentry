package ideas

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// routes are the ideas' HTTP API, for the page and Claude Code: list them,
// accept one, dismiss one. Every write goes through c.Log.
func routes(c features.Core) []features.Route {
	return []features.Route{
		{Pattern: "GET /api/ideas", Auth: features.UI, Handler: func(w http.ResponseWriter, r *http.Request) { listIdeas(c, w, r) }},
		{Pattern: "POST /api/ideas/{idea}/accept", Auth: features.UI, Handler: func(w http.ResponseWriter, r *http.Request) { acceptIdea(c, w, r) }},
		{Pattern: "POST /api/ideas/{idea}/dismiss", Auth: features.UI, Handler: func(w http.ResponseWriter, r *http.Request) { dismissIdea(c, w, r) }},
	}
}

// listIdeas answers the ideas, of every repo or of ?repo=, newest session
// first: by when the task they came from was made, then newest idea first.
// An idea from a session after an epic closed names that epic, an open one
// why Agentry wouldn't accept it for you, under your rule (LeftForYou), and an accepted
// one the column its task is in now.
func listIdeas(c features.Core, w http.ResponseWriter, r *http.Request) {
	list := List(c.Read, r.URL.Query().Get("repo"))
	newestSessionFirst(c.Read, list)
	about := map[string]*tasks.Task{}
	for _, idea := range list {
		if _, seen := about[idea.Task]; !seen {
			about[idea.Task] = nil
			if task, err := c.Read.Task(idea.Task); err == nil && task != nil {
				if epic, err := c.Read.Task(task.About); err == nil && epic != nil {
					about[idea.Task] = epic
				}
			}
		}
	}
	rule, roster, bases := c.Read.Settings().AutoAcceptRule, c.Read.Roster(), agents.BasesIn(c.Read)
	views := []ideaView{}
	for _, idea := range list {
		why := ""
		if idea.State == Open {
			why = LeftForYou(c.Read, idea, list, roster, bases, rule)
		}
		var became *tasks.Task
		if idea.Became != "" {
			became, _ = c.Read.Task(idea.Became) // one that can't be read shows with no column
		}
		views = append(views, viewOf(idea, about[idea.Task], became, roster, why))
	}
	httpapi.Reply(w, views)
}

// acceptIdea makes the idea a task in your Backlog (Accept), or started with
// "start", with "budget" as a new task's: absent or 0 takes the one
// tasks.NewLimits gives it (the server's, or the epic's it follows up), -1
// is no limit, and below that is a 400. Its turns are always NewLimits's.
// It answers 404 for no such idea; 409 with the reason when it was refused
// and nothing was written (the idea isn't open, the kind or the repo is
// wrong); 409 with JSON {error, duplicate: {id, title, column}} when the
// idea looks like an open task, which "anyway" makes it despite; otherwise
// 200 with what happened: the task made, the column it is in, whether the
// idea is marked accepted, and if not, why. A task made and the idea not
// marked is still a 200, so the page can show the task.
func acceptIdea(c features.Core, w http.ResponseWriter, r *http.Request) {
	var request struct {
		Kind, Note string
		Start      bool
		Budget     int
		Anyway     bool
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	if request.Budget < httpapi.NoLimit {
		http.Error(w, "a budget is a number of tokens, 0 for the default or -1 for no limit", http.StatusBadRequest)
		return
	}
	id := r.PathValue("idea")
	if !ideaExists(c, w, id) {
		return
	}
	got, err := Accept(logOf(c), c.Read, id, Choice{Kind: strings.TrimSpace(request.Kind), Note: strings.TrimSpace(request.Note),
		Defaults: c.Defaults, Budget: chosenBudget(request.Budget), Start: request.Start, Anyway: request.Anyway})
	switch {
	case err != nil:
		httpapi.Failed(w, err)
	case got.Duplicate != "":
		refuseDuplicate(c, w, got)
	case got.Task == "":
		http.Error(w, got.Refused, http.StatusConflict)
	default:
		httpapi.Reply(w, acceptance{Task: got.Task, Column: got.Column, Accepted: got.Accepted, Refused: got.Refused})
	}
}

// refuseDuplicate answers 409 naming the open task the idea looks like, so
// the page can show it before you choose to make another.
func refuseDuplicate(c features.Core, w http.ResponseWriter, got Acceptance) {
	like := duplicateView{ID: got.Duplicate}
	if task, err := c.Read.Task(got.Duplicate); err == nil && task != nil {
		like.Title, like.Column = task.Title, task.BoardColumn(c.Read.Roster())
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(duplicateRefusal{Error: got.Refused, Duplicate: like})
}

// chosenBudget is the budget a request chose for an idea's task, nil when
// it chose none (0), so the task takes the one its limits give.
func chosenBudget(asked int) *int {
	if asked == 0 {
		return nil
	}
	budget := httpapi.LimitOf(asked, 0)
	return &budget
}

// dismissIdea dismisses an open idea with your note: 404 for no such idea,
// 409 when the rules refuse it, else the event.
func dismissIdea(c features.Core, w http.ResponseWriter, r *http.Request) {
	var request struct{ Note string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	id := r.PathValue("idea")
	if !ideaExists(c, w, id) {
		return
	}
	httpapi.Propose(w, c.Log, events.Proposal{Stream: id, Actor: httpapi.You, Type: IdeaDismissed, Data: map[string]any{"note": strings.TrimSpace(request.Note)}})
}

// ideaExists answers 404 when there is no such idea.
func ideaExists(c features.Core, w http.ResponseWriter, id string) bool {
	if !IsStream(id) {
		http.Error(w, "no such idea", http.StatusNotFound)
		return false
	}
	idea, err := Get(c.Read, id)
	switch {
	case err != nil:
		httpapi.Failed(w, err)
		return false
	case idea == nil:
		http.Error(w, "no such idea", http.StatusNotFound)
		return false
	}
	return true
}

// logOf is what Accept writes through and reads of the log, as c gives them.
func logOf(c features.Core) Log {
	return struct {
		features.Proposer
		features.EventLog
	}{c.Log, c.Events}
}

type ideaView struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Problem      string     `json:"problem"`
	Proposal     string     `json:"proposal"`
	WhyNow       string     `json:"whyNow"`
	Size         string     `json:"size"` // S, M or L
	Risks        string     `json:"risks"`
	BackedBy     []string   `json:"backedBy"`
	DoubtedBy    []string   `json:"doubtedBy"`
	Task         string     `json:"task"` // the session it came from
	Repo         string     `json:"repo"`
	Epic         string     `json:"epic"`       // the epic it follows up, "" for none
	About        string     `json:"about"`      // the epic whose close started the session it came from, "" for none
	AboutTitle   string     `json:"aboutTitle"` // that epic's title
	Kind         tasks.Kind `json:"kind"`       // what it suggests making of it: "" for a story, "epic"
	State        State      `json:"state"`
	Note         string     `json:"note"`               // what you said when you accepted or dismissed it
	DecidedAt    time.Time  `json:"decidedAt,omitzero"` // when it was accepted or dismissed; left out while it is open
	Became       string     `json:"became"`             // the task an accepted idea became
	BecameColumn string     `json:"becameColumn"`       // that task's column on the board now, as a task summary's column; "" when it can't be read
	Auto         bool       `json:"autoAccepted"`       // Agentry accepted it, as the Visionaries agreed, not you
	Repeats      string     `json:"repeats"`            // an idea the Visionaries said it raises again, "" for none
	CreatedAt    time.Time  `json:"createdAt"`
	// NotAutoAccepted says why Agentry wouldn't accept an open idea for you
	// under your rule (LeftForYou), even with that turned off; "" when
	// it would, and for an idea that isn't open.
	NotAutoAccepted string `json:"notAutoAccepted"`
}

// viewOf is the idea as the page shows it; about is the epic whose close
// started its session, nil for none, became the task it became, nil for
// none, and notAutoAccepted why Agentry wouldn't accept it for you.
func viewOf(idea *Idea, about, became *tasks.Task, roster agents.Roster, notAutoAccepted string) ideaView {
	view := ideaView{ID: idea.ID, Title: idea.Title, Problem: idea.Problem, Proposal: idea.Proposal, WhyNow: idea.WhyNow,
		Size: idea.Size, Risks: idea.Risks, BackedBy: listOf(idea.BackedBy), DoubtedBy: listOf(idea.DoubtedBy), Task: idea.Task,
		Repo: idea.Repo, Epic: idea.Epic, Kind: idea.Kind, State: idea.State, Note: idea.Note, Became: idea.Became,
		DecidedAt: idea.DecidedAt, Auto: idea.AutoAccepted, Repeats: idea.Repeats, CreatedAt: idea.CreatedAt,
		NotAutoAccepted: notAutoAccepted}
	if about != nil {
		view.About, view.AboutTitle = about.ID, about.Title
	}
	if became != nil {
		view.BecameColumn = became.BoardColumn(roster)
	}
	return view
}

// acceptance is what accepting an idea did (Acceptance).
type acceptance struct {
	Task     string `json:"task"`     // the task made from the idea
	Column   string `json:"column"`   // its column on the board now, as a task summary's column; "" when it can't be read
	Accepted bool   `json:"accepted"` // the idea is marked accepted
	Refused  string `json:"refused"`  // why it isn't, "" once accepted
}

// duplicateRefusal is the 409 for an idea that looks like an open task.
type duplicateRefusal struct {
	Error     string        `json:"error"`
	Duplicate duplicateView `json:"duplicate"`
}

// duplicateView is the open task an idea looks like.
type duplicateView struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Column string `json:"column"` // its column on the board, as a task summary's column; "" when it can't be read
}
