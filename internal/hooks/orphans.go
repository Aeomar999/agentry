package hooks

import (
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// DenyOrphanedAsks runs when the server starts. An action waiting for your
// decision is held by its hook call, and no hook call survives a restart: the
// hook failed closed and the agent was already told no. So each such action
// is denied, and the task stops showing an approval nothing is waiting on.
func DenyOrphanedAsks(s *store.Store) error {
	ids, err := s.TaskIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		stream, err := s.Stream(id)
		if err != nil {
			return err
		}
		task := tasks.FromEvents(stream)
		if task == nil || task.IsDone {
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
