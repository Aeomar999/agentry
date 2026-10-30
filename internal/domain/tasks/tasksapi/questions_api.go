package tasksapi

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// Question is one thing waiting for you, from any task: an action the
// policy held, a question the coordinator asked, the budget or the limit of
// turns, or an agent stuck on a prompt in its terminal. The Questions tab lists them.
type Question struct {
	ID        string    `json:"id"`   // unique on the page
	Kind      string    `json:"kind"` // "approval", "question", "budget", "turns" or "waiting"
	Task      string    `json:"task"`
	TaskTitle string    `json:"taskTitle"`
	TaskKind  string    `json:"taskKind,omitempty"` // "epic" for an epic, left out for a story or a task on its own
	Agent     string    `json:"agent"`              // who asks
	At        time.Time `json:"at"`
	Text      string    `json:"text"`    // the question, or the command to approve
	Context   string    `json:"context"` // the step before it, so you know what led here

	Action  string    `json:"action,omitempty"` // approval: the action's id
	Tool    string    `json:"tool,omitempty"`
	Reason  string    `json:"reason,omitempty"` // approval: the rule that asked
	Held    bool      `json:"held,omitempty"`   // approval: no answer came in time, so the agent was told no
	HeldAt  time.Time `json:"heldAt,omitzero"`
	Options []string  `json:"options,omitempty"`
	Page    string    `json:"page,omitempty"` // a page the coordinator wrote to ask it

	TokensUsed int `json:"tokensUsed,omitempty"` // budget
	Budget     int `json:"budget,omitempty"`

	TurnsSinceAnswer int `json:"turnsSinceAnswer,omitempty"` // turns
	MaxTurns         int `json:"maxTurns,omitempty"`
}

func (a API) listQuestions(w http.ResponseWriter, r *http.Request) {
	httpapi.Reply(w, a.Questions())
}

// Questions are what waits for you of every task not done, oldest first.
func (a API) Questions() []Question {
	questions := []Question{}
	for _, task := range a.State.Tasks() {
		if !task.IsDone {
			questions = append(questions, questionsOf(task)...)
		}
	}
	slices.SortStableFunc(questions, func(a, b Question) int { return cmp.Compare(a.At.UnixNano(), b.At.UnixNano()) }) // oldest first
	return questions
}

func questionsOf(t *tasks.Task) []Question {
	var questions []Question
	base := func(kind, key string) Question {
		return Question{ID: t.ID + "/" + kind + "/" + key, Kind: kind, Task: t.ID, TaskTitle: t.Title, TaskKind: string(t.Kind), Context: contextOf(t)}
	}
	for _, a := range t.WaitingForYou() {
		q := base("approval", a.ID)
		q.Agent, q.At, q.Text, q.Action, q.Tool, q.Reason = a.Agent, a.RequestedAt, a.Command, a.ID, a.Tool, a.Reason
		q.Held, q.HeldAt = a.Held, a.HeldAt
		questions = append(questions, q)
	}
	if t.Question != "" {
		q := base("question", "open")
		q.Agent, q.At, q.Text, q.Options, q.Page = t.Asked.By, t.Asked.At, t.Question, t.Asked.Options, t.Asked.Page
		switch t.StoppedAt() {
		case tasks.LimitBudget:
			q.Kind, q.TokensUsed, q.Budget = "budget", t.TokensUsed, t.Budget
		case tasks.LimitTurns:
			q.Kind, q.TurnsSinceAnswer, q.MaxTurns = "turns", t.TurnsSinceAnswer, t.MaxTurns
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
