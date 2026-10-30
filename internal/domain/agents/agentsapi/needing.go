package agentsapi

import (
	"slices"
)

// Needing lists the unfinished tasks that one of the agents coordinates,
// works on now, or is about to.
func Needing(state State, names ...string) ([]string, error) {
	var busy []string
	for _, task := range state.Tasks() {
		if slices.ContainsFunc(names, task.Needs) {
			busy = append(busy, task.ID)
		}
	}
	return busy, nil
}

// atWork lists the unfinished tasks one of the agents works on now, or is
// about to as other than the task's coordinator.
func (a API) atWork(names ...string) []string {
	var busy []string
	for _, task := range a.State.Tasks() {
		open, next := task.OpenTurn(), task.Handoff
		switch {
		case task.IsDone:
		case open != nil && slices.Contains(names, open.Agent),
			next != nil && next.To != task.Coordinator && slices.Contains(names, next.To):
			busy = append(busy, task.ID)
		}
	}
	return busy
}
