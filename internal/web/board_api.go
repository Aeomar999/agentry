package web

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
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
// moved anywhere but Done is reopened first. A task waiting for your answer
// takes the words as the answer.
func (s *Server) direct(w http.ResponseWriter, r *http.Request) {
	var request struct{ To, Note string }
	if !decode(w, r, &request) {
		return
	}
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	note := strings.TrimSpace(request.Note)
	shippers := teammates(roster, task, agents.Ships)
	park := func(place string) events.Proposal {
		return events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": place, "note": note}}
	}

	switch request.To {
	case "backlog", "todo":
		if task.IsDone && !s.proposeOrRefuse(w, s.reopen(task, "Reopen this task and park it.", note)) {
			return
		}
		s.propose(w, park(request.To))
		return
	case "done":
		if task.IsDone {
			http.Error(w, "the task is done already", http.StatusConflict)
			return
		}
		done := events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskDone,
			Data: map[string]any{"summary": "The user moved it to Done, as it was.", "note": note, "moved": true}}
		if task.Kind != tasks.Epic {
			s.propose(w, done)
			return
		}
		if s.proposeOrRefuse(w, done) && s.parkStories(w, task, note) {
			reply(w, map[string]string{"task": task.ID})
		}
		return
	case "in-progress", "review":
	case "shipping":
		if len(shippers) == 0 {
			http.Error(w, teamOf(roster, task)+" has no agent that ships work", http.StatusConflict)
			return
		}
	default:
		http.Error(w, "no column called "+request.To, http.StatusBadRequest)
		return
	}

	// Done work goes back for more, as does work a checker or a shipper had,
	// parked or not.
	from := workColumn(task, roster)
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
		if text == "" {
			text = "It needs more work."
		}
		s.propose(w, moved(s.reopen(task, "Reopen this task. "+text, note)))
	case task.Parked != "":
		if !s.proposeOrRefuse(w, moved(park(""))) {
			return
		}
		if text == "" { // started from To do or the Backlog: nothing more to say
			reply(w, map[string]string{"task": task.ID})
			return
		}
		s.propose(w, s.direction(task, text, note))
	case text == "": // already there
		reply(w, map[string]string{"task": task.ID})
	default:
		s.propose(w, moved(s.direction(task, text, note)))
	}
}

// parkStories parks the stories of an epic you closed that aren't done, in
// the Backlog: their turns in progress stop, and their coordinators only
// wrap up. A story parked already stays where it is.
func (s *Server) parkStories(w http.ResponseWriter, epic *tasks.Task, note string) bool {
	for _, id := range epic.Stories {
		story, err := s.task(id)
		if err != nil {
			failed(w, err)
			return false
		}
		if story == nil || story.IsDone || story.Parked != "" {
			continue
		}
		held := events.Proposal{Stream: id, Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": "backlog", "note": note}}
		if !s.proposeOrRefuse(w, held) {
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
	return events.Proposal{Stream: task.ID, Actor: you, Type: kind, Data: map[string]any{field: text}}
}

func (s *Server) reopen(task *tasks.Task, text, note string) events.Proposal {
	if note != "" {
		text += " " + note
	}
	return events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": text}}
}

// proposeOrRefuse writes a proposal that comes before another, and answers
// for the request only if it fails.
func (s *Server) proposeOrRefuse(w http.ResponseWriter, p events.Proposal) bool {
	_, rejection, err := s.store.Propose(p)
	switch {
	case err != nil:
		failed(w, err)
		return false
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
		return false
	}
	return true
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

func teamOf(roster agents.Roster, task *tasks.Task) string {
	lead, _ := roster.Get(task.Coordinator)
	return lead.TeamName()
}
