package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// direct is what moving a card on the board does. Your move stands at once,
// in the same request; the columns are the work's stages, not agents, so
// the coordinator still decides who works next, within your move.
//
//   - Backlog, To do: park it there at once; a turn in progress stops, and
//     the coordinator only wraps up
//   - In progress: start it (let it go); from Review or Shipping, send it
//     back for more work, for an agent that builds it
//   - Review: have it checked next, by an agent that checks work if its
//     team has one
//   - Shipping: ship it now, by an agent of its team that ships work; a
//     team with none refuses the move. Checked or not: your move stands, so
//     work not yet reviewed or tested ships as you chose, with no earlier
//     review needed; the agent that ships it still runs the tests and build
//     after bringing it up to date, and a failure stops the push. Once it
//     has pushed, the coordinator can finish the task without another round
//     of checks; only a change after that needs checking again. A ship turn
//     that crashed, was stopped or ended without a push hasn't shipped it:
//     the next one ships it, as you chose, if nothing changed or checked the
//     work in between
//   - Done: it is done at once, as it is; a turn in progress stops, and the
//     coordinator only wraps up (an epic closes, and its open stories are
//     parked in the Backlog)
//
// In progress, Review and Shipping stand until the coordinator next hands
// the task on: the first event the move writes carries the column. A finished task
// moved anywhere but Done is reopened first, and an archived one is
// unarchived before that. A task waiting for your answer takes the words as
// the answer.
func (s *Server) direct(w http.ResponseWriter, r *http.Request) {
	var request struct{ To, Note string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	id := r.PathValue("task")
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	task, err := s.task(id)
	switch {
	case errors.Is(err, current.ErrArchived):
		refusal, err := s.archivedMove(id, request.To, roster)
		if err != nil {
			httpapi.Failed(w, err)
			return
		}
		if refusal != "" {
			http.Error(w, refusal, http.StatusConflict)
			return
		}
		if task = s.unarchive(w, id); task == nil {
			return
		}
	case err != nil || task == nil:
		s.noTask(w, id)
		return
	}
	note := strings.TrimSpace(request.Note)
	shippers := teammates(roster, task, agents.Ships)
	park := func(place string) events.Proposal {
		return events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskHeld, Data: map[string]any{"place": place, "note": note}}
	}

	switch request.To {
	case "backlog", "todo":
		if task.IsDone && !(s.dropErasedStories(w, task) && httpapi.ProposeOrRefuse(w, s.store, s.reopen(task, "Reopen this task and park it.", note))) {
			return
		}
		httpapi.Propose(w, s.store, park(request.To))
		return
	case "done":
		if task.IsDone {
			http.Error(w, "the task is done already", http.StatusConflict)
			return
		}
		done := events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskDone,
			Data: map[string]any{"summary": "The user moved it to Done, as it was.", "note": note, "moved": true}}
		if task.Kind != tasks.Epic {
			httpapi.Propose(w, s.store, done)
			return
		}
		if httpapi.ProposeOrRefuse(w, s.store, done) && s.parkStories(w, task, note) {
			httpapi.Reply(w, map[string]string{"task": task.ID})
		}
		return
	case "in-progress", "review":
	case "shipping":
		if len(shippers) == 0 {
			http.Error(w, noShipperIn(roster, task), http.StatusConflict)
			return
		}
	default:
		http.Error(w, "no column called "+request.To, http.StatusBadRequest)
		return
	}

	// Done work goes back for more, as does work a checker or a shipper had,
	// parked or not.
	from := task.WorkColumn(roster)
	sentBack := request.To == "in-progress" && (task.IsDone || from == "review" || from == "shipping")
	text := ""
	switch {
	case request.To == "shipping":
		text = "Ship this task now: hand it to " + strings.Join(shippers, " or ") + ", to land the work as it is. " +
			"The user chose to ship it now, maybe before it was reviewed or tested, so it needs no earlier review; " +
			"the agent that ships it still runs the tests and build after bringing it up to date, and a failure stops the push."
	case request.To == "review":
		text = "Have this task reviewed next."
	case sentBack:
		text = "This needs more work: hand it back to an agent that builds it."
	case note != "":
		text = "Carry on."
	}
	moved := func(p events.Proposal) events.Proposal {
		p.Data["column"], p.Data["sentBack"] = request.To, sentBack
		return p
	}
	switch {
	case task.IsDone:
		if !s.dropErasedStories(w, task) {
			return
		}
		if text == "" {
			text = "It needs more work."
		}
		httpapi.Propose(w, s.store, moved(s.reopen(task, "Reopen this task. "+text, note)))
	case task.Parked != "":
		if !httpapi.ProposeOrRefuse(w, s.store, moved(park(""))) {
			return
		}
		if text == "" { // started from To do or the Backlog: nothing more to say
			httpapi.Reply(w, map[string]string{"task": task.ID})
			return
		}
		httpapi.Propose(w, s.store, s.direction(task, text, note))
	case text == "": // already there
		httpapi.Reply(w, map[string]string{"task": task.ID})
	default:
		httpapi.Propose(w, s.store, moved(s.direction(task, text, note)))
	}
}

// reopeningColumns are the columns that reopen a done task moved to them:
// every one but Done.
var reopeningColumns = []string{"backlog", "todo", "in-progress", "review", "shipping"}

// archivedMove is why an archived task can't be moved to a column, or ""
// when it can: only a column that reopens it, and Shipping only while its
// team has an agent that ships work. The team is read from the task's
// history, so a move refused here unarchives nothing.
func (s *Server) archivedMove(id, to string, roster agents.Roster) (string, error) {
	if !slices.Contains(reopeningColumns, to) {
		return rules.ArchivedRefusal(id), nil
	}
	if to != "shipping" {
		return "", nil
	}
	archived, err := s.readArchived(id)
	if err != nil {
		return "", err
	}
	if len(teammates(roster, archived.task, agents.Ships)) == 0 {
		return noShipperIn(roster, archived.task), nil
	}
	return "", nil
}

// parkStories parks the stories of an epic you closed that aren't done, in
// the Backlog: their turns in progress stop, and their coordinators only
// wrap up. A story parked already stays where it is.
func (s *Server) parkStories(w http.ResponseWriter, epic *tasks.Task, note string) bool {
	for _, id := range epic.Stories {
		story, err := s.task(id)
		if errors.Is(err, current.ErrArchived) {
			continue // done, and kept as done
		}
		if errors.Is(err, current.ErrErased) {
			continue // gone, though the epic still lists it: nothing is left to park
		}
		if err != nil {
			httpapi.Failed(w, err)
			return false
		}
		if story == nil || story.IsDone || story.Parked != "" {
			continue
		}
		held := events.Proposal{Stream: id, Actor: httpapi.You, Type: events.TaskHeld, Data: map[string]any{"place": "backlog", "note": note}}
		if !httpapi.ProposeOrRefuse(w, s.store, held) {
			return false
		}
	}
	return true
}

// direction gives the coordinator your words: as the answer, if it asked
// you something, or as a direction it reads at its next turn.
func (s *Server) direction(task *tasks.Task, text, note string) events.Proposal {
	if note != "" {
		text += " " + note
	}
	kind, field := events.TaskDirected, "text"
	if task.Question != "" {
		kind, field = events.TaskAnswered, "answer"
	}
	return events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: kind, Data: map[string]any{field: text}}
}

func (s *Server) reopen(task *tasks.Task, text, note string) events.Proposal {
	if note != "" {
		text += " " + note
	}
	return events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskReopened, Data: map[string]any{"text": text}}
}

// dropErasedStories takes the erased stories a done epic still lists out of
// it before it is reopened, with the task.story_removed the erase route's
// leaveEpic writes: an erased story listed would keep the epic from
// finishing. It still lists one erased while it was archived, as leaveEpic
// leaves an archived epic as it is, or after a clean-up failed part way. A
// story already gone from the epic, as another reopen took it out first,
// counts as taken out. It is false once it has answered that it failed.
func (s *Server) dropErasedStories(w http.ResponseWriter, epic *tasks.Task) bool {
	for _, story := range epic.Stories {
		if _, erased := s.state.Erased(story); !erased {
			continue
		}
		removal := events.Proposal{Stream: epic.ID, Actor: httpapi.You, Type: events.StoryRemoved, Data: map[string]any{"story": story}}
		_, rejection, err := s.store.Propose(removal)
		switch {
		case err != nil:
			httpapi.Failed(w, err)
			return false
		case rejection != nil && s.lists(epic.ID, story):
			httpapi.Failed(w, fmt.Errorf("erased %s can't be taken out of %s: %s", story, epic.ID, rejection.Reason))
			return false
		}
	}
	return true
}

// lists is whether epic, as the state holds it now, lists story.
func (s *Server) lists(epic, story string) bool {
	task, err := s.task(epic)
	return err == nil && task != nil && slices.Contains(task.Stories, story)
}

// teammates are the agents of the task's team whose work counts as column.
func teammates(roster agents.Roster, task *tasks.Task, column string) []string {
	var names []string
	for _, agent := range roster.Assignees(teamOf(roster, task)) {
		if agent.Column == column {
			names = append(names, agent.Name)
		}
	}
	return names
}

// noShipperIn is why a task can't move to Shipping.
func noShipperIn(roster agents.Roster, task *tasks.Task) string {
	return teamOf(roster, task) + " has no agent that ships work"
}

func teamOf(roster agents.Roster, task *tasks.Task) string {
	lead, _ := roster.Get(task.Coordinator)
	return lead.TeamName()
}
