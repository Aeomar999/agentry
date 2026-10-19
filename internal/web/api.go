package web

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	all := s.state.Tasks()
	slices.Reverse(all) // newest first, which a tie keeps
	done := map[string]bool{}
	for _, task := range all {
		done[task.ID] = task.IsDone
	}
	isDone := func(id string) bool {
		if isDone, listed := done[id]; listed {
			return isDone
		}
		return s.isDone(id)
	}
	summaries := []taskSummary{}
	for _, task := range all {
		summary := tasks.SummaryOf(task, roster)
		summary.BlockedBy = blockedBy(task, isDone)
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
	repo, reason := s.repoFor(strings.TrimSpace(request.Repo))
	if reason != "" {
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

// repoFor is the folder a task's work goes in, for a repo named by its
// folder on this machine or by a git address, which is cloned here; or why
// it can't be worked in.
func (s *Server) repoFor(repo string) (string, string) {
	if worktrees.IsAddress(repo) {
		folder, err := worktrees.Clone(s.config.Clones, repo)
		if err != nil {
			return "", err.Error()
		}
		return folder, ""
	}
	return repo, worktrees.IsRepo(repo)
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
		Budget                   int // tokens; 0 takes the default, -1 is no limit
		MaxTurns                 int // turns without your answer; 0 takes the default, -1 is no limit
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
		repo, reason := s.repoFor(request.Repo)
		if reason != "" {
			http.Error(w, reason, http.StatusBadRequest)
			return
		}
		request.Repo = repo
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

	defaults := s.config.defaults()
	parked := ""
	if request.Epic != "" {
		parked = "backlog" // with the epic's other stories, until you start it
	}
	created, rejection, err := s.store.Create("task-", events.Proposal{Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": request.Title, "description": request.Description,
		"repo": request.Repo, "coordinator": coordinator.Name, "labels": cleanLabels(request.Labels),
		"budget": limitOf(request.Budget, defaults.Budget), "maxTurns": limitOf(request.MaxTurns, defaults.MaxTurns),
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
	id := created.Stream
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

// noLimit is what a request sends for no limit. At creation 0 takes the
// server's default, so a task made without one is unlimited only when asked.
const noLimit = -1

// defaults are the limits a new task gets when it names none.
func (c Config) defaults() tasks.Limits { return tasks.Limits{Budget: c.Budget, MaxTurns: c.MaxTurns} }

// limitOf is a budget or limit of turns as the task keeps it, 0 for no
// limit, from a request's: 0 takes byDefault (the server's for a new task, no
// limit for a change), and noLimit is none.
func limitOf(asked, byDefault int) int {
	switch asked {
	case 0:
		return byDefault
	case noLimit:
		return 0
	}
	return asked // below -1, the rules refuse it
}

// setBudget gives the task a new token budget, 0 (or -1) for no limit. A
// task stopped at its budget takes it as your answer and carries on.
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
	budget := limitOf(request.Budget, 0)
	if !s.proposeOrRefuse(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskBudgeted, Data: map[string]any{"budget": budget}}) {
		return
	}
	answered, refused := false, ""
	if task.StoppedAt() == tasks.LimitBudget && (budget == 0 || task.TokensUsed < budget) {
		answer := "I raised the budget to " + coordinator.Tokens(budget) + " tokens. Carry on."
		if budget == 0 {
			answer = "I made the budget unlimited. Carry on."
		}
		if answered, refused, err = s.carryOn(task, answer); err != nil {
			failed(w, err)
			return
		}
	}
	reply(w, limitReply("budget", budget, answered, refused))
}

// setMaxTurns gives the task a new limit of turns without your answer, 0 (or
// -1) for no limit. A task stopped at its limit takes it as your answer and
// carries on.
func (s *Server) setMaxTurns(w http.ResponseWriter, r *http.Request) {
	var request struct{ MaxTurns int }
	if !decode(w, r, &request) {
		return
	}
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	maxTurns := limitOf(request.MaxTurns, 0)
	if !s.proposeOrRefuse(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskTurnsLimited, Data: map[string]any{"maxTurns": maxTurns}}) {
		return
	}
	answered, refused := false, ""
	if task.StoppedAt() == tasks.LimitTurns && (maxTurns == 0 || task.TurnsSinceAnswer < maxTurns) {
		answer := fmt.Sprintf("I raised the turn limit to %d. Carry on.", maxTurns)
		if maxTurns == 0 {
			answer = "I made the turns unlimited. Carry on."
		}
		if answered, refused, err = s.carryOn(task, answer); err != nil {
			failed(w, err)
			return
		}
	}
	reply(w, limitReply("maxTurns", maxTurns, answered, refused))
}

// carryOn answers the loop's stop for you, so the coordinator goes on. It
// says whether it did, or why the rules refused it.
func (s *Server) carryOn(task *tasks.Task, answer string) (answered bool, refused string, err error) {
	_, rejection, err := s.store.Propose(events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskAnswered, Data: map[string]any{"answer": answer}})
	switch {
	case err != nil:
		return false, "", err
	case rejection != nil:
		return false, rejection.Reason, nil
	}
	return true, "", nil
}

// limitReply is what a saved limit answers: the limit, whether it answered
// the task's stop, and why not if the rules refused that answer. The limit is
// kept either way.
func limitReply(field string, limit int, answered bool, refused string) map[string]any {
	body := map[string]any{field: limit, "answered": answered}
	if refused != "" {
		body["refused"] = refused
	}
	return body
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

// showTask is a task's detail. An archived task's isn't kept: it answers
// 410 Gone, so you can tell it from a task that never was.
func (s *Server) showTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.task(r.PathValue("task"))
	if errors.Is(err, current.ErrArchived) {
		http.Error(w, r.PathValue("task")+" is archived", http.StatusGone)
		return
	}
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	log, err := s.store.Log(task.ID)
	if err != nil {
		failed(w, err)
		return
	}
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	detail := detailOf(task, roster, log)
	detail.BlockedBy = blockedBy(task, s.isDone)
	if epic, _ := s.task(task.Epic); epic != nil {
		detail.EpicTitle = epic.Title
	}
	detail.StoryList = []taskSummary{}
	for _, id := range task.Stories {
		if story, _ := s.task(id); story != nil {
			summary := tasks.SummaryOf(story, roster)
			summary.BlockedBy = blockedBy(story, s.isDone)
			detail.StoryList = append(detail.StoryList, summary)
		}
	}
	reply(w, detail)
}

// answer sends your answer to the coordinator's question. Two answers are
// approvals, which only you give, on the page (or through Claude Code, which
// asks you itself): closing an epic, and copying a file from outside a
// machine's worktrees. The desk passes on what you say, and approves nothing.
func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var request struct{ Answer string }
	if !decode(w, r, &request) {
		return
	}
	answer := strings.TrimSpace(request.Answer)
	task, err := s.task(r.PathValue("task"))
	closes := err == nil && task != nil && task.Closing != "" && strings.EqualFold(answer, coordinator.CloseEpic)
	copies := err == nil && task != nil && task.Asked.Fetch != nil && strings.EqualFold(answer, coordinator.CopyFile)
	switch {
	case !closes && !copies:
		s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: events.TaskAnswered, Data: map[string]any{"answer": answer}})
	case !s.carriesUIToken(r):
		http.Error(w, "that is approved on the page", http.StatusForbidden)
	case closes:
		// One event, the answer and the close together: an answer alone
		// would start the coordinator's next turn.
		s.propose(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": task.Closing}})
	default:
		s.copyApproved(w, r, task.ID, *task.Asked.Fetch)
	}
}

// copyApproved copies the file you allowed, and answers with it.
func (s *Server) copyApproved(w http.ResponseWriter, r *http.Request, id string, f tasks.Fetch) {
	if s.config.Fleet == nil {
		http.Error(w, "this server has no machines to copy from", http.StatusConflict)
		return
	}
	m, online := s.config.Fleet.Get(f.Machine)
	if !online {
		http.Error(w, f.Machine+" is offline; answer again once it is back", http.StatusConflict)
		return
	}
	files, err := m.SharedFetch(r.Context(), f.Path, f.As, true)
	if err != nil {
		http.Error(w, "copying "+f.Machine+":"+f.Path+": "+err.Error(), http.StatusConflict)
		return
	}
	s.propose(w, events.Proposal{Stream: id, Actor: you, Type: events.FilesFetched, Data: coordinator.Fetched(f, files, true)})
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
	var inForce *policy.Policy
	if s.config.Policy != nil {
		inForce = s.config.Policy.Current()
	}
	reply(w, agentsOf(roster, inForce))
}

// showConfig is what the page needs to know about the server. policyProblem
// says policy.toml has a mistake, and missingProfiles names the profiles
// agents have that it lacks, with those agents, so every page can point to
// the Policy page, which says what to do.
func (s *Server) showConfig(w http.ResponseWriter, r *http.Request) {
	policyProblem := s.config.Policy != nil && s.config.Policy.Problem() != ""
	reply(w, map[string]any{"defaultRepo": s.config.DefaultRepo, "budget": s.config.Budget, "maxTurns": s.config.MaxTurns, "hasTerminal": s.terminal != nil, "hasDesk": s.config.Desk != nil,
		"engines": rules.KnownEngines, "workers": s.workerChoices(), "colors": agents.Colors, "canAddWorkers": s.config.Workers != nil,
		"policyProblem": policyProblem, "missingProfiles": s.missingProfilesInUse()})
}

// missingProfilesInUse is policy.MissingProfiles for the roster: none while
// the policy or the roster can't be read.
func (s *Server) missingProfilesInUse() map[string][]string {
	roster, err := s.roster()
	if s.config.Policy == nil || err != nil {
		return map[string][]string{}
	}
	return s.config.Policy.Current().MissingProfiles(agentProfiles(roster))
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
	reply(w, map[string]int64{"position": s.store.LastPosition()})
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

func (s *Server) task(id string) (*tasks.Task, error) { return s.state.Task(id) }

// isDone says whether a task is done. An archived task is: only a done task
// is archived. A broken or unknown one isn't.
func (s *Server) isDone(id string) bool {
	task, err := s.task(id)
	if errors.Is(err, current.ErrArchived) {
		return true
	}
	return task != nil && task.IsDone
}

func (s *Server) roster() (agents.Roster, error) {
	return s.state.Roster(), s.state.Broken(events.AgentsStream)
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
