package web

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// question is one thing waiting for you, from any task: an action the
// policy held, a question the coordinator asked, the budget, or an agent
// stuck on a prompt in its terminal. The Questions tab lists them.
type question struct {
	ID        string    `json:"id"`   // unique on the page
	Kind      string    `json:"kind"` // "approval", "question", "budget" or "waiting"
	Task      string    `json:"task"`
	TaskTitle string    `json:"taskTitle"`
	Agent     string    `json:"agent"` // who asks
	At        time.Time `json:"at"`
	Text      string    `json:"text"`    // the question, or the command to approve
	Context   string    `json:"context"` // the step before it, so you know what led here

	Action  string   `json:"action,omitempty"` // approval: the action's id
	Tool    string   `json:"tool,omitempty"`
	Reason  string   `json:"reason,omitempty"` // approval: the rule that asked
	Options []string `json:"options,omitempty"`
	Page    string   `json:"page,omitempty"` // a page the coordinator wrote to ask it

	TokensUsed int `json:"tokensUsed,omitempty"` // budget
	Budget     int `json:"budget,omitempty"`
}

func (s *Server) listQuestions(w http.ResponseWriter, r *http.Request) {
	ids, err := s.store.TaskIDs()
	if err != nil {
		failed(w, err)
		return
	}
	questions := []question{}
	for _, id := range ids {
		if task, err := s.task(id); err == nil && task != nil && !task.IsDone {
			questions = append(questions, questionsOf(task)...)
		}
	}
	slices.SortStableFunc(questions, func(a, b question) int { return cmp.Compare(a.At.UnixNano(), b.At.UnixNano()) }) // oldest first
	reply(w, questions)
}

func questionsOf(t *tasks.Task) []question {
	var questions []question
	base := func(kind, key string) question {
		return question{ID: t.ID + "/" + kind + "/" + key, Kind: kind, Task: t.ID, TaskTitle: t.Title, Context: contextOf(t)}
	}
	for _, a := range t.WaitingForYou() {
		q := base("approval", a.ID)
		q.Agent, q.At, q.Text, q.Action, q.Tool, q.Reason = a.Agent, a.RequestedAt, a.Command, a.ID, a.Tool, a.Reason
		questions = append(questions, q)
	}
	if t.Question != "" {
		q := base("question", "open")
		q.Agent, q.At, q.Text, q.Options, q.Page = t.Asked.By, t.Asked.At, t.Question, t.Asked.Options, t.Asked.Page
		if t.Budget > 0 && t.TokensUsed >= t.Budget {
			q.Kind, q.TokensUsed, q.Budget = "budget", t.TokensUsed, t.Budget
		}
		questions = append(questions, q)
	}
	if turn := t.OpenTurn(); turn != nil && turn.Waiting != "" {
		q := base("waiting", turn.ID)
		q.Agent, q.At, q.Text = turn.Agent, turn.LastEventAt, turn.Waiting
		questions = append(questions, q)
	}
	return questions
}

// contextOf is the last step before the question: usually what the agent
// reported or was told.
func contextOf(t *tasks.Task) string {
	for i := len(t.Trail) - 1; i >= 0; i-- {
		if t.Trail[i].To != tasks.You {
			return t.Trail[i].From + " → " + t.Trail[i].To + ": " + t.Trail[i].Text
		}
	}
	return ""
}
