package web

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// direct is what moving a card on the board does. The columns are the
// work's stages, not agents: the coordinator still decides who works next,
// so the board never disagrees with the log.
//
//   - Backlog, To do: park it there; the coordinator leaves it alone
//   - In progress: start it (let it go); from Review, send it back for more
//     work
//   - Review: have it checked next
//   - Done: wrap it up
//
// A finished task moved anywhere but Done is reopened first. A task
// waiting for your answer takes the words as the answer.
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
	park := func(place string) events.Proposal {
		return events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": place}}
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
		s.tell(w, task, "Wrap this task up: finish it as it is, checked or not.", note, true)
		return
	case "in-progress", "review":
	default:
		http.Error(w, "no column called "+request.To, http.StatusBadRequest)
		return
	}

	inReview := false
	if agent, ok := roster.Get(stageOf(task)); ok {
		inReview = agent.Column == "review"
	}
	text := ""
	switch {
	case request.To == "review":
		text = "Have this task reviewed next."
	case inReview:
		text = "This needs more work: hand it back to an agent that builds it."
	case note != "":
		text = "Carry on."
	}
	if task.IsDone {
		if text == "" {
			text = "It needs more work."
		}
		s.propose(w, s.reopen(task, "Reopen this task. "+text, note))
		return
	}
	if task.Parked != "" && !s.proposeOrRefuse(w, park("")) {
		return
	}
	if text == "" { // started from To do or the Backlog: nothing more to say
		reply(w, map[string]string{"task": task.ID})
		return
	}
	s.tell(w, task, text, note, false)
}

// tell gives the coordinator your words: as the answer, if it asked you
// something, or as a direction it reads at its next turn.
// tell sends the coordinator what you said; asIs lets it finish the task
// without the checks it needs otherwise.
func (s *Server) tell(w http.ResponseWriter, task *tasks.Task, text, note string, asIs bool) {
	if note != "" {
		text += " " + note
	}
	kind, field := events.TaskDirected, "text"
	if task.Question != "" {
		kind, field = events.TaskAnswered, "answer"
	}
	data := map[string]any{field: text}
	if asIs {
		data["asIs"] = true
	}
	s.propose(w, events.Proposal{Stream: task.ID, Actor: you, Type: kind, Data: data})
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
