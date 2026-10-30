package tasksapi

import (
	"fmt"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/runtime/coordinator"
)

// setBudget gives the task a new token budget, 0 (or -1) for no limit. A
// task stopped at its budget takes it as your answer and carries on.
func (a API) setBudget(w http.ResponseWriter, r *http.Request) {
	var request struct{ Budget int }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		a.noTask(w, r.PathValue("task"))
		return
	}
	budget := httpapi.LimitOf(request.Budget, 0)
	if !httpapi.ProposeOrRefuse(w, a.Log, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskBudgeted, Data: map[string]any{"budget": budget}}) {
		return
	}
	answered, refused := false, ""
	if task.StoppedAt() == tasks.LimitBudget && (budget == 0 || task.TokensUsed < budget) {
		answer := "I raised the budget to " + coordinator.Tokens(budget) + " tokens. Carry on."
		if budget == 0 {
			answer = "I made the budget unlimited. Carry on."
		}
		if answered, refused, err = a.carryOn(task, answer); err != nil {
			httpapi.Failed(w, err)
			return
		}
	}
	httpapi.Reply(w, limitReply("budget", budget, answered, refused))
}

// setMaxTurns gives the task a new limit of turns without your answer, 0 (or
// -1) for no limit. A task stopped at its limit takes it as your answer and
// carries on.
func (a API) setMaxTurns(w http.ResponseWriter, r *http.Request) {
	var request struct{ MaxTurns int }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		a.noTask(w, r.PathValue("task"))
		return
	}
	maxTurns := httpapi.LimitOf(request.MaxTurns, 0)
	if !httpapi.ProposeOrRefuse(w, a.Log, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskTurnsLimited, Data: map[string]any{"maxTurns": maxTurns}}) {
		return
	}
	answered, refused := false, ""
	if task.StoppedAt() == tasks.LimitTurns && (maxTurns == 0 || task.TurnsSinceAnswer < maxTurns) {
		answer := fmt.Sprintf("I raised the turn limit to %d. Carry on.", maxTurns)
		if maxTurns == 0 {
			answer = "I made the turns unlimited. Carry on."
		}
		if answered, refused, err = a.carryOn(task, answer); err != nil {
			httpapi.Failed(w, err)
			return
		}
	}
	httpapi.Reply(w, limitReply("maxTurns", maxTurns, answered, refused))
}

// carryOn answers the loop's stop for you, so the coordinator goes on. It
// says whether it did, or why the rules refused it.
func (a API) carryOn(task *tasks.Task, answer string) (answered bool, refused string, err error) {
	_, rejection, err := a.Log.Propose(events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskAnswered, Data: map[string]any{"answer": answer}})
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
