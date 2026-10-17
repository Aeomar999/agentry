package web

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	ids, err := s.store.TaskIDs()
	if err != nil {
		failed(w, err)
		return
	}
	var all []*tasks.Task
	done := map[string]bool{}
	for i := len(ids) - 1; i >= 0; i-- { // newest first, which a tie keeps
		if task, err := s.task(ids[i]); err == nil && task != nil {
			all, done[task.ID] = append(all, task), task.IsDone
		}
	}
	summaries := []taskSummary{}
	for _, task := range all {
		summary := summaryOf(task)
		summary.BlockedBy = blockedBy(task, func(id string) bool { return done[id] })
		summaries = append(summaries, summary)
	}
	// In your priority order; a new task starts first.
	slices.SortStableFunc(summaries, func(a, b taskSummary) int { return cmp.Compare(a.Rank, b.Rank) })
	reply(w, summaries)
}

// changeRepo moves a task's work to another repo: when the turn in progress
// ends, or at once between turns. A finished task gets its worktree of the
// new repo here and now, its whole history counted as the task's work, so
// its Workspace shows what was done there.
func (s *Server) changeRepo(w http.ResponseWriter, r *http.Request) {
	var request struct{ Repo string }
	if !decode(w, r, &request) {
		return
	}
	repo := strings.TrimSpace(request.Repo)
	if reason := worktrees.IsRepo(repo); reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	moved := events.Proposal{Stream: task.ID, Actor: you, Type: events.RepoChanged, Data: map[string]any{"repo": repo}}
	if !task.IsDone || s.config.Fleet == nil {
		s.propose(w, moved)
		return
	}
	if !s.proposeOrRefuse(w, moved) {
		return
	}
	here := s.config.Fleet.Of(workers.Local)
	made, err := here.Prepare(r.Context(), machines.WorktreeRequest{Task: task.ID, Repo: repo})
	if err != nil {
		failed(w, err)
		return
	}
	s.propose(w, events.Proposal{Stream: task.ID, Actor: events.Actor{Kind: events.Worker, ID: here.Name()}, Type: events.WorktreeReady,
		Data: map[string]any{"repo": made.Repo, "worker": here.Name(), "path": made.Path, "branch": made.Branch, "base": "", "port": made.Port}})
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Title, Description, Repo string
		NewRepo                  bool     // make Repo, a new folder, the task's own new git repo
		Team                     string   // whose agents work on it; "" for the first team
		Kind                     string   // "story" or "epic"; "" lets the coordinator split it if it is too big
		Stories                  []string // for an epic: tasks there already, to file under it as its stories
		Epic                     string   // for a story: the epic to file it under
		Labels                   []string
		Budget                   int // tokens; 0 takes the default
	}
	if !decode(w, r, &request) {
		return
	}
	request.Title, request.Repo = strings.TrimSpace(request.Title), strings.TrimSpace(request.Repo)
	request.Description = strings.TrimSpace(request.Description)
	if request.Title == "" && request.Description == "" {
		http.Error(w, "describe the task", http.StatusBadRequest)
		return
	}
	if len(request.Stories) > 0 {
		request.Kind = "epic"
	}
	if request.Epic != "" {
		request.Kind = "story"
	}
	kind := tasks.Story
	switch request.Kind {
	case "", "story":
	case "epic":
		kind = tasks.Epic
	default:
		http.Error(w, "a task is a story or an epic, not "+request.Kind, http.StatusBadRequest)
		return
	}
	if request.Title == "" {
		request.Title = titleFrom(request.Description) // the coordinator renames it once it has read the task
	}
	switch {
	case request.NewRepo:
		if err := worktrees.NewRepo(request.Repo, request.Title); err != nil {
			http.Error(w, "making the new repo: "+err.Error(), http.StatusBadRequest)
			return
		}
	case request.Repo == "" && request.Epic != "":
		if epic, _ := s.task(request.Epic); epic != nil {
			request.Repo = epic.Repo // a story works where its epic does
		}
		request.Repo = cmp.Or(request.Repo, s.config.DefaultRepo)
	case request.Repo == "":
		request.Repo = s.config.DefaultRepo
	default:
		if reason := worktrees.IsRepo(request.Repo); reason != "" {
			http.Error(w, reason, http.StatusBadRequest)
			return
		}
	}
	budget := request.Budget
	if budget <= 0 {
		budget = s.config.Budget
	}
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	coordinator, ok := roster.Coordinator(request.Team)
	if !ok {
		http.Error(w, "there is no coordinator in the "+cmp.Or(request.Team, "first")+" team", http.StatusConflict)
		return
	}

	s.creating.Lock()
	defer s.creating.Unlock()
	ids, err := s.store.TaskIDs()
	if err != nil {
		failed(w, err)
		return
	}
	id := fmt.Sprintf("task-%d", len(ids)+1)
	parked := ""
	if request.Epic != "" {
		parked = "backlog" // with the epic's other stories, until you start it
	}
	created, rejection, err := s.store.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": request.Title, "description": request.Description,
		"repo": request.Repo, "coordinator": coordinator.Name, "labels": cleanLabels(request.Labels), "budget": budget,
		"teamChosen": request.Team != "", "kind": kind, "kindChosen": request.Kind != "", "parked": parked}})
	switch {
	case err != nil:
		failed(w, err)
		return
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
		return
	}
	// The task is made: filing it is a second step, and what fails there
	// is said, with the task kept.
	var refused []string
	for _, story := range request.Stories {
		if why := s.fileUnder(story, id); why != "" {
			refused = append(refused, story+": "+why)
		}
	}
	if request.Epic != "" {
		if why := s.fileUnder(id, request.Epic); why != "" {
			refused = append(refused, why)
		}
	}
	reply(w, map[string]any{"stream": created.Stream, "position": created.Position, "notFiled": refused})
}

// setBudget gives the task a new token budget. A task stopped at its budget
// takes it as your answer and carries on.
func (s *Server) setBudget(w http.ResponseWriter, r *http.Request) {
	var request struct{ Budget int }
	if !decode(w, r, &request) {
		return
	}
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	if !s.proposeOrRefuse(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskBudgeted, Data: map[string]any{"budget": request.Budget}}) {
		return
	}
	if task.Question != "" && task.TokensUsed < request.Budget {
		s.propose(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskAnswered, Data: map[string]any{
			"answer": "I raised the budget to " + coordinator.Tokens(request.Budget) + " tokens. Carry on."}})
		return
	}
	reply(w, map[string]int{"budget": request.Budget})
}

// rank puts the task in your priority order: the page sends a rank between
// the two tasks it was dropped between.
func (s *Server) rank(w http.ResponseWriter, r *http.Request) {
	var request struct{ Rank float64 }
	if !decode(w, r, &request) {
		return
	}
	s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: events.TaskRanked, Data: map[string]any{"rank": request.Rank}})
}

// label sets the task's labels, all of them.
func (s *Server) label(w http.ResponseWriter, r *http.Request) {
	var request struct{ Labels []string }
	if !decode(w, r, &request) {
		return
	}
	s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: events.TaskLabeled,
		Data: map[string]any{"labels": cleanLabels(request.Labels)}})
}

// cleanLabels trims labels, lowercases them so "Bug" and "bug" are one, and
// drops empty and repeated ones. The rules check what is left.
func cleanLabels(labels []string) []string {
	clean := []string{}
	for _, label := range labels {
		label = strings.ToLower(strings.Join(strings.Fields(label), " "))
		if label != "" && !slices.Contains(clean, label) {
			clean = append(clean, label)
		}
	}
	return clean
}

// titleFrom stands in for a title nobody gave: the description's first
// line, cut at a word to at most 72 characters.
func titleFrom(description string) string {
	line, _, _ := strings.Cut(description, "\n")
	line = strings.Join(strings.Fields(line), " ")
	if len(line) <= 72 {
		return line
	}
	cut := strings.LastIndexByte(line[:72], ' ')
	if cut < 40 {
		cut = 72
	}
	return strings.TrimRight(line[:cut], ",.;:") + "..."
}

func (s *Server) showTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	log, err := s.store.Log(task.ID)
	if err != nil {
		failed(w, err)
		return
	}
	detail := detailOf(task, log)
	isDone := func(id string) bool { other, _ := s.task(id); return other != nil && other.IsDone }
	detail.BlockedBy = blockedBy(task, isDone)
	if epic, _ := s.task(task.Epic); epic != nil {
		detail.EpicTitle = epic.Title
	}
	detail.StoryList = []taskSummary{}
	for _, id := range task.Stories {
		if story, _ := s.task(id); story != nil {
			summary := summaryOf(story)
			summary.BlockedBy = blockedBy(story, isDone)
			detail.StoryList = append(detail.StoryList, summary)
		}
	}
	reply(w, detail)
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var request struct{ Answer string }
	if !decode(w, r, &request) {
		return
	}
	answer := strings.TrimSpace(request.Answer)
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil || task.Closing == "" || !strings.EqualFold(answer, coordinator.CloseEpic) {
		s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: events.TaskAnswered, Data: map[string]any{"answer": answer}})
		return
	}
	// Closing an epic is yours to approve, on the page: the desk passes on
	// what you say, and doesn't approve for you.
	if !s.carriesUIToken(r) {
		http.Error(w, "closing an epic is approved on the page", http.StatusForbidden)
		return
	}
	// One event, the answer and the close together: an answer alone would
	// start the coordinator's next turn.
	s.propose(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": task.Closing}})
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	s.decide(w, r, events.ActionApproved)
}
func (s *Server) deny(w http.ResponseWriter, r *http.Request) { s.decide(w, r, events.ActionDenied) }

func (s *Server) decide(w http.ResponseWriter, r *http.Request, kind events.Type) {
	var request struct{ Reason string }
	if r.ContentLength != 0 && !decode(w, r, &request) {
		return
	}
	if request.Reason == "" {
		request.Reason = "no reason given"
	}
	s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: kind,
		Correlation: r.PathValue("action"), Data: map[string]any{"reason": request.Reason}})
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	reply(w, agentsOf(roster))
}

// showConfig is what the page needs to know about the server.
func (s *Server) showConfig(w http.ResponseWriter, r *http.Request) {
	reply(w, map[string]any{"defaultRepo": s.config.DefaultRepo, "budget": s.config.Budget, "hasTerminal": s.terminal != nil, "hasDesk": s.config.Desk != nil,
		"engines": rules.KnownEngines, "workers": s.workerChoices(), "colors": agents.Colors, "canAddWorkers": s.config.Workers != nil})
}

// setTheme records the page's theme, "light" or "dark". Sessions started
// after it use it, so the engines draw colours that suit the terminal.
func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	var request struct{ Theme string }
	if !decode(w, r, &request) {
		return
	}
	if request.Theme != "light" && request.Theme != "dark" {
		http.Error(w, `the theme is "light" or "dark"`, http.StatusBadRequest)
		return
	}
	if s.config.SetTheme != nil {
		s.config.SetTheme(request.Theme)
	}
	reply(w, map[string]string{"theme": request.Theme})
}

// workerChoices are the machines an agent can be given: this one, then
// every worker that has joined.
func (s *Server) workerChoices() []string {
	roster, _ := s.workers()
	return append([]string{workers.Local}, roster.Names()...)
}

// showPolicy is the policy in force, and why the file on disk isn't, if it isn't.
func (s *Server) showPolicy(w http.ResponseWriter, r *http.Request) {
	if s.config.Policy == nil {
		http.Error(w, "no policy", http.StatusNotFound)
		return
	}
	current := s.config.Policy.Current()
	reply(w, map[string]any{"path": s.config.Policy.Path(), "hash": current.Hash(), "problem": s.config.Policy.Problem(),
		"defaultProfile": current.DefaultProfile(), "useForEveryone": current.UseForEveryone(),
		"profiles": current.Summary(), "text": current.Text()})
}

// waitForChanges answers when the log has moved past ?after=, or after 25 s,
// with the newest position. The page long-polls it and refreshes on change:
// it reads the log at its own pace, like every other reader (proof 05).
func (s *Server) waitForChanges(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	reader := s.store.NewReader(after)
	defer reader.Close()
	reader.Next(ctx) // returns early with events, or when ctx ends
	position, err := s.store.LastPosition()
	if err != nil {
		failed(w, err)
		return
	}
	reply(w, map[string]int64{"position": position})
}

// propose writes a proposal and replies with the event, or with the
// validator's reason as a 409 when it was refused.
func (s *Server) propose(w http.ResponseWriter, p events.Proposal) {
	event, rejection, err := s.store.Propose(p)
	switch {
	case err != nil:
		failed(w, err)
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
	default:
		reply(w, event)
	}
}

func (s *Server) task(id string) (*tasks.Task, error) {
	stream, err := s.store.Stream(id)
	if err != nil {
		return nil, err
	}
	return tasks.FromEvents(stream), nil
}

func (s *Server) roster() (agents.Roster, error) {
	stream, err := s.store.Stream(events.AgentsStream)
	return agents.FromEvents(stream), err
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(into); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func failed(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
