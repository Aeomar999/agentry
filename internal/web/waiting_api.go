package web

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/waitlist"
)

// listWaiting answers everything waiting for you, about ?repo= ("" when not
// known), as Waiting lists it.
func (s *Server) listWaiting(w http.ResponseWriter, r *http.Request) {
	httpapi.Reply(w, s.Waiting(r.URL.Query().Get("repo")))
}

// Waiting is everything waiting for you, about repo ("" when not known):
// the core's entries oldest first, as Questions lists them, then what each
// extension says waits. Every extension is asked, hidden or not.
func (s *Server) Waiting(repo string) []extensions.Waiting {
	waiting := []extensions.Waiting{}
	for _, q := range s.questions() {
		waiting = append(waiting, waitlist.Entry(waitlist.Question{Kind: q.Kind, Task: q.Task, TaskTitle: q.TaskTitle,
			TaskKind: q.TaskKind, Agent: q.Agent, Text: q.Text, Action: q.Action, Options: q.Options}))
	}
	for _, f := range s.config.Extensions {
		if f.Waiting != nil {
			waiting = append(waiting, f.Waiting(s.state, repo)...)
		}
	}
	return waiting
}
