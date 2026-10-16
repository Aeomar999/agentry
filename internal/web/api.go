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
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	ids, err := s.store.TaskIDs()
	if err != nil {
		failed(w, err)
		return
	}
	summaries := []taskSummary{}
	for i := len(ids) - 1; i >= 0; i-- { // newest first, which a tie keeps
		if task, err := s.task(ids[i]); err == nil && task != nil {
			summaries = append(summaries, summaryOf(task))
		}
	}
	// In your priority order; a new task starts first.
	slices.SortStableFunc(summaries, func(a, b taskSummary) int { return cmp.Compare(a.Rank, b.Rank) })
	reply(w, summaries)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Title, Description, Repo string
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
	if request.Title == "" {
		request.Title = titleFrom(request.Description) // the coordinator renames it once it has read the task
	}
	if request.Repo == "" {
		request.Repo = s.config.DefaultRepo
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
	coordinator, ok := roster.Coordinator()
	if !ok {
		http.Error(w, "there is no coordinator agent", http.StatusConflict)
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
	s.propose(w, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": request.Title, "description": request.Description,
		"repo": request.Repo, "coordinator": coordinator.Name, "labels": cleanLabels(request.Labels), "budget": budget}})
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
	reply(w, detailOf(task, log))
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var request struct{ Answer string }
	if !decode(w, r, &request) {
		return
	}
	s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: events.TaskAnswered,
		Data: map[string]any{"answer": strings.TrimSpace(request.Answer)}})
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
		"engines": rules.KnownEngines, "workers": knownWorkers(), "colors": agents.Colors})
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

// knownWorkers are the machines an agent can be given; none until T-022.
func knownWorkers() []string {
	if rules.KnownWorkers == nil {
		return []string{}
	}
	return rules.KnownWorkers
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
