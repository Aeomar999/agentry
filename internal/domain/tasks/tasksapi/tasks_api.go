package tasksapi

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
	"gitlab.com/dev.chempa/agentry/internal/runtime/coordinator"
)

// listTasks lists every task's summary, an archived task's too: it is done,
// so nothing blocks it and it waits for no slot.
func (a API) listTasks(w http.ResponseWriter, r *http.Request) {
	if _, err := a.Roster(); err != nil {
		httpapi.Failed(w, err)
		return
	}
	summaries := a.State.Summaries()
	slices.SortStableFunc(summaries, tasks.SummariesInPriorityOrder)
	done := map[string]bool{}
	for _, summary := range summaries {
		done[summary.ID] = summary.Status == tasks.Done
	}
	blocksNone := func(id string) bool {
		if isDone, listed := done[id]; listed {
			return isDone
		}
		return a.blocksNone(id)
	}
	slots := a.slots()
	for i, summary := range summaries {
		// A task archived since Summaries was read is nil here: it is listed without BlockedBy or Queued.
		if task, _ := a.State.Task(summary.ID); task != nil {
			summaries[i].BlockedBy = blockedBy(task, blocksNone)
			summaries[i].Queued = slots.Queued(task.ID)
		}
	}
	httpapi.Reply(w, summaries)
}

// slots are how many tasks run now and which wait for a slot, as the loop
// holds them back.
func (a API) slots() coordinator.Slots {
	return coordinator.SlotsOf(a.State.Tasks(), a.State.Roster(), a.State.Settings().MaxRunningTasks)
}

func (a API) createTask(w http.ResponseWriter, r *http.Request) {
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
	if !httpapi.Decode(w, r, &request) {
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
		if epic, _ := a.State.Task(request.Epic); epic != nil {
			request.Repo = epic.Repo // a story works where its epic does
		}
		request.Repo = cmp.Or(request.Repo, a.DefaultRepo)
	case request.Repo == "":
		request.Repo = a.DefaultRepo
	default:
		repo, reason := a.RepoFor(request.Repo)
		if reason != "" {
			http.Error(w, reason, http.StatusBadRequest)
			return
		}
		request.Repo = repo
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	// A team you name is matched without case; one there isn't keeps its
	// name, and has no coordinator.
	coordinator, ok := roster.Coordinator(cmp.Or(roster.TeamNamed(request.Team), request.Team))
	if !ok {
		http.Error(w, "there is no coordinator in the "+cmp.Or(request.Team, "first")+" team", http.StatusConflict)
		return
	}

	defaults := a.Defaults
	parked := ""
	if request.Epic != "" {
		parked = "backlog" // with the epic's other stories, until you start it
	}
	created, rejection, err := a.Store.Create("task-", events.Proposal{Actor: httpapi.You, Type: events.TaskCreated, Data: map[string]any{
		"title": request.Title, "description": request.Description,
		"repo": request.Repo, "coordinator": coordinator.Name, "labels": CleanLabels(request.Labels),
		"budget": httpapi.LimitOf(request.Budget, defaults.Budget), "maxTurns": httpapi.LimitOf(request.MaxTurns, defaults.MaxTurns),
		"teamChosen": request.Team != "", "kind": kind, "kindChosen": request.Kind != "", "parked": parked}})
	switch {
	case err != nil:
		httpapi.Failed(w, err)
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
		if why := a.FileUnder(story, id); why != "" {
			refused = append(refused, story+": "+why)
		}
	}
	if request.Epic != "" {
		if why := a.FileUnder(id, request.Epic); why != "" {
			refused = append(refused, why)
		}
	}
	httpapi.Reply(w, map[string]any{"stream": created.Stream, "position": created.Position, "notFiled": refused})
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

// showTask is a task's detail; an archived task's as it was before it
// was archived, and when that was.
func (a API) showTask(w http.ResponseWriter, r *http.Request) {
	task, archived := a.shownTask(w, r)
	if task == nil {
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	slots := a.slots()
	detail := detailOf(task, roster)
	detail.Shared = a.SharedFiles(task)
	detail.BlockedBy = blockedBy(task, a.blocksNone)
	detail.Queued = slots.Queued(task.ID)
	if epic, found := a.State.Summary(task.Epic); found {
		detail.EpicTitle = epic.Title
	}
	detail.StoryList = []taskSummary{}
	for _, id := range task.Stories {
		summary, found := a.State.Summary(id)
		if !found {
			continue
		}
		// An archived story is done, so nothing blocks it and it waits for no slot.
		if story, _ := a.State.Task(id); story != nil {
			summary.BlockedBy = blockedBy(story, a.blocksNone)
			summary.Queued = slots.Queued(story.ID)
		}
		detail.StoryList = append(detail.StoryList, summary)
	}
	if archived != nil {
		detail.Archived, detail.ArchivedAt = true, &archived.marker.At
	}
	httpapi.Reply(w, detail)
}

// blocksNone says whether a story after task id may start for it: id is
// done, or archived, as only a done task is, or erased, which can never
// be done and leaves nothing to wait for. A broken or unknown one blocks.
func (a API) blocksNone(id string) bool {
	task, err := a.State.Task(id)
	if errors.Is(err, current.ErrArchived) || errors.Is(err, current.ErrErased) {
		return true
	}
	return task != nil && task.IsDone
}
