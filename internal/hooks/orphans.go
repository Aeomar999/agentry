package hooks

import (
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// DenyOrphanedAsks runs when the server starts. An action waiting for your
// decision is held by its hook call, and no hook call survives a restart: the
// hook failed closed and the agent was already told no. So each such action
// is denied, and the task stops showing an approval nothing is waiting on.
// A task the state can't hand out, as a broken one, is skipped: it can't
// take an event either, and it must not keep the server from starting.
func DenyOrphanedAsks(s *store.Store, state *current.State) error {
	for _, id := range state.TaskIDs() {
		task, err := state.Task(id)
		if err != nil || task == nil || task.IsDone {
			continue
		}
		for _, action := range task.WaitingForYou() {
			_, _, err := s.Propose(events.Proposal{Stream: id, Actor: ruleset, Type: events.ActionDenied, Correlation: action.ID,
				Data: map[string]any{"reason": "Agentry restarted while this waited for you; the agent was told no and can ask again"}})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
